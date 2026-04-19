package transport

import (
	"bytes"
	"encoding/binary"
	"testing"
)

func writeBlob(parts ...[]byte) []byte {
	var buf bytes.Buffer
	for _, p := range parts {
		var hdr [4]byte
		binary.BigEndian.PutUint32(hdr[:], uint32(len(p)))
		buf.Write(hdr[:])
		buf.Write(p)
	}
	return buf.Bytes()
}

func TestReadCertBlobRoundtrip(t *testing.T) {
	ca := []byte("ca pem")
	cert := []byte("server cert pem")
	key := []byte("server key pem")
	raw := writeBlob(ca, cert, key)

	got, err := ReadCertBlob(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("ReadCertBlob: %v", err)
	}
	if string(got.CAPEM) != string(ca) || string(got.ServerCertPEM) != string(cert) || string(got.ServerKeyPEM) != string(key) {
		t.Errorf("roundtrip mismatch: %+v", got)
	}
}

func TestReadCertBlobRejectsHugePayload(t *testing.T) {
	var buf bytes.Buffer
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], 1<<24) // 16MB
	buf.Write(hdr[:])
	if _, err := ReadCertBlob(&buf); err == nil {
		t.Errorf("expected error on oversized payload")
	}
}
