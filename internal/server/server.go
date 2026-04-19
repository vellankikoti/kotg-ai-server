package server

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
	"github.com/vellankikoti/kotg-ai-server/internal/session"
	"github.com/vellankikoti/kotg-ai-server/internal/transport"

	kotgv1 "github.com/vellankikoti/kotg-schema/gen/go/kotg/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// New constructs the gRPC server with mTLS using the supplied cert
// bundle and registers all kotg-ai-server services (Health, AIControl,
// Chat). The returned *grpc.Server is not yet serving — caller invokes
// Serve(lis) on the bound listener.
func New(bundle *transport.CertBundle, sessions *session.Manager, p provider.Provider, providerType, model string, maxTokens int) (*grpc.Server, error) {
	serverCert, err := tls.X509KeyPair(bundle.ServerCertPEM, bundle.ServerKeyPEM)
	if err != nil {
		return nil, fmt.Errorf("server keypair: %w", err)
	}
	caPool := x509.NewCertPool()
	if !caPool.AppendCertsFromPEM(bundle.CAPEM) {
		return nil, fmt.Errorf("ca pool: failed to append CA")
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    caPool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}

	srv := grpc.NewServer(grpc.Creds(credentials.NewTLS(tlsCfg)))

	h := health.NewServer()
	h.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)
	healthpb.RegisterHealthServer(srv, h)

	kotgv1.RegisterAIControlServer(srv, NewAIControl(providerType, model))
	kotgv1.RegisterChatServer(srv, NewChat(sessions, p, maxTokens))

	return srv, nil
}
