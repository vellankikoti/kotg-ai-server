// Command kotg-ai-server is the AI sidecar binary the kubilitics-backend
// supervisor exec's. See docs/superpowers/specs/2026-04-19-kotg-ai-server-v1-design.md
// in the kubilitics repo for the full design.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
	"github.com/vellankikoti/kotg-ai-server/internal/providerfactory"
	"github.com/vellankikoti/kotg-ai-server/internal/server"
	"github.com/vellankikoti/kotg-ai-server/internal/session"
	"github.com/vellankikoti/kotg-ai-server/internal/transport"
)

func main() {
	var (
		providerType    = flag.String("provider", "ollama", "ollama | openai | anthropic")
		endpoint        = flag.String("endpoint", "http://127.0.0.1:11434", "provider base URL")
		model           = flag.String("model", "", "provider-specific model id (required)")
		apiKeyEnv       = flag.String("api-key-env", "", "env var name holding the API key (optional for Ollama)")
		sessionTTL      = flag.Duration("session-ttl", 15*time.Minute, "idle session TTL")
		maxSessions     = flag.Int("max-sessions", 1000, "hard cap on active sessions")
		maxMessages     = flag.Int("max-messages-per-session", 100, "hard cap on retained messages per session")
		maxBudgetTokens = flag.Int("max-budget-tokens", 16000, "approximate token budget per provider call")
	)
	flag.Parse()

	if err := validateFlags(*providerType, *endpoint, *model); err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(2)
	}
	apiKey := ""
	if *apiKeyEnv != "" {
		apiKey = os.Getenv(*apiKeyEnv)
		if apiKey == "" {
			fmt.Fprintf(os.Stderr, "config error: env var %s is empty\n", *apiKeyEnv)
			os.Exit(2)
		}
	}

	bundle, err := transport.ReadCertBlob(os.Stdin)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cert blob: %v\n", err)
		os.Exit(2)
	}

	p, err := providerfactory.New(provider.Config{
		Type:     *providerType,
		Endpoint: *endpoint,
		Model:    *model,
		APIKey:   apiKey,
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "provider: %v\n", err)
		os.Exit(2)
	}
	defer p.Close()

	mgr := session.New(session.Config{
		TTL:                   *sessionTTL,
		MaxSessions:           *maxSessions,
		MaxMessagesPerSession: *maxMessages,
	})
	defer mgr.Stop()

	lis, port, err := transport.BindLocalhost()
	if err != nil {
		fmt.Fprintf(os.Stderr, "bind: %v\n", err)
		os.Exit(2)
	}

	grpcSrv, err := server.New(bundle, mgr, p, *providerType, *model, *maxBudgetTokens)
	if err != nil {
		fmt.Fprintf(os.Stderr, "server: %v\n", err)
		os.Exit(2)
	}

	// Bind succeeded → print READY (ONLY after bind).
	if err := transport.WriteReady(os.Stdout, port); err != nil {
		fmt.Fprintf(os.Stderr, "write ready: %v\n", err)
		os.Exit(2)
	}
	log.Printf("kotg-ai-server: provider=%s model=%s endpoint=%s port=%d", *providerType, *model, *endpoint, port)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	serveErr := make(chan error, 1)
	go func() { serveErr <- grpcSrv.Serve(lis) }()

	select {
	case <-ctx.Done():
		log.Printf("kotg-ai-server: shutdown signal received")
		grpcSrv.GracefulStop()
	case err := <-serveErr:
		if err != nil {
			log.Printf("kotg-ai-server: serve: %v", err)
		}
	}
}

func validateFlags(providerType, endpoint, model string) error {
	switch providerType {
	case "ollama", "openai", "anthropic":
	default:
		return fmt.Errorf("unsupported --provider %q (want ollama|openai|anthropic)", providerType)
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return fmt.Errorf("invalid --endpoint %q", endpoint)
	}
	if model == "" {
		return fmt.Errorf("--model is required")
	}
	return nil
}
