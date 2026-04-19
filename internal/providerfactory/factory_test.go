package providerfactory_test

import (
	"testing"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
	"github.com/vellankikoti/kotg-ai-server/internal/providerfactory"
)

func TestNewUnknownType(t *testing.T) {
	if _, err := providerfactory.New(provider.Config{Type: "bogus", Model: "m"}); err == nil {
		t.Errorf("expected error for unknown provider type")
	}
}

func TestNewEmptyModel(t *testing.T) {
	if _, err := providerfactory.New(provider.Config{Type: "ollama", Model: ""}); err == nil {
		t.Errorf("expected error for empty model")
	}
}

func TestNewOllamaSucceeds(t *testing.T) {
	p, err := providerfactory.New(provider.Config{Type: "ollama", Endpoint: "http://127.0.0.1:11434", Model: "qwen2.5:7b"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	p.Close()
}
