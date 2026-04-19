package main

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	kotgv1 "github.com/vellankikoti/kotg-schema/gen/go/kotg/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

func writeBlob(w io.Writer, parts ...[]byte) error {
	for _, p := range parts {
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(p)))
		if _, err := w.Write(hdr[:]); err != nil {
			return err
		}
		if _, err := w.Write(p); err != nil {
			return err
		}
	}
	return nil
}

func mintTestCerts(t *testing.T) (caPEM, srvCertPEM, srvKeyPEM, cliCertPEM, cliKeyPEM []byte) {
	t.Helper()
	caKey, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	caTpl := &x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: "test-ca"},
		NotBefore:             time.Now().Add(-time.Minute),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature,
		BasicConstraintsValid: true, IsCA: true,
	}
	caDER, _ := x509.CreateCertificate(rand.Reader, caTpl, caTpl, &caKey.PublicKey, caKey)
	caPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: caDER})

	sign := func(cn string, eku []x509.ExtKeyUsage, ips []net.IP, dns []string) ([]byte, []byte) {
		k, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		tpl := &x509.Certificate{
			SerialNumber: big.NewInt(time.Now().UnixNano()),
			Subject:      pkix.Name{CommonName: cn},
			NotBefore:    caTpl.NotBefore, NotAfter: caTpl.NotAfter,
			KeyUsage:    x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
			ExtKeyUsage: eku, IPAddresses: ips, DNSNames: dns,
		}
		der, _ := x509.CreateCertificate(rand.Reader, tpl, caTpl, &k.PublicKey, caKey)
		keyDER, _ := x509.MarshalECPrivateKey(k)
		return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
			pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	}
	srvCertPEM, srvKeyPEM = sign("kotg-ai-server",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		[]net.IP{net.ParseIP("127.0.0.1")}, []string{"localhost"})
	cliCertPEM, cliKeyPEM = sign("test-client",
		[]x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}, nil, nil)
	return
}

func buildBinary(t *testing.T) string {
	t.Helper()
	out := filepath.Join(t.TempDir(), "kotg-ai-server")
	cmd := exec.Command("go", "build", "-o", out, ".")
	cmd.Dir = "."
	if outBytes, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, outBytes)
	}
	return out
}

func TestE2EHandshakeAndCapabilities(t *testing.T) {
	bin := buildBinary(t)
	ca, sCert, sKey, cCert, cKey := mintTestCerts(t)

	cmd := exec.Command(bin,
		"--provider=ollama",
		"--endpoint=http://127.0.0.1:1",
		"--model=qwen2.5:7b",
	)
	stdin, _ := cmd.StdinPipe()
	stdout, _ := cmd.StdoutPipe()
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	}()

	if err := writeBlob(stdin, ca, sCert, sKey); err != nil {
		t.Fatalf("write blob: %v", err)
	}
	stdin.Close()

	sc := bufio.NewScanner(stdout)
	if !sc.Scan() {
		t.Fatalf("no READY line: %v", sc.Err())
	}
	line := sc.Text()
	if !strings.HasPrefix(line, "READY ") {
		t.Fatalf("expected READY, got %q", line)
	}
	port, _ := strconv.Atoi(strings.TrimPrefix(line, "READY "))
	if port <= 0 {
		t.Fatalf("bad port: %q", line)
	}

	clientPair, _ := tls.X509KeyPair(cCert, cKey)
	pool := x509.NewCertPool()
	pool.AppendCertsFromPEM(ca)
	creds := credentials.NewTLS(&tls.Config{
		Certificates: []tls.Certificate{clientPair},
		RootCAs:      pool,
		ServerName:   "localhost",
		MinVersion:   tls.VersionTLS13,
	})
	conn, err := grpc.NewClient(fmt.Sprintf("127.0.0.1:%d", port), grpc.WithTransportCredentials(creds))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cli := kotgv1.NewAIControlClient(conn)
	resp, err := cli.Capabilities(ctx, &kotgv1.Empty{})
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if resp.SchemaVersion != "1.0.1" {
		t.Errorf("SchemaVersion = %q, want 1.0.1", resp.SchemaVersion)
	}
	if len(resp.Providers) != 1 || resp.Providers[0] != "ollama" {
		t.Errorf("Providers = %v, want [ollama]", resp.Providers)
	}
}
