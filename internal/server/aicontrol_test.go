package server

import (
	"context"
	"testing"

	kotgv1 "github.com/vellankikoti/kotg-schema/gen/go/kotg/v1"
)

func TestCapabilitiesReportsConfiguredProvider(t *testing.T) {
	h := NewAIControl("ollama", "qwen2.5-coder:7b")
	resp, err := h.Capabilities(context.Background(), &kotgv1.Empty{})
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if resp.SchemaVersion != "1.0.1" {
		t.Errorf("SchemaVersion = %q, want 1.0.1", resp.SchemaVersion)
	}
	if len(resp.Providers) != 1 || resp.Providers[0] != "ollama" {
		t.Errorf("Providers = %v, want [ollama]", resp.Providers)
	}
	if len(resp.Models) != 1 || resp.Models[0] != "qwen2.5-coder:7b" {
		t.Errorf("Models = %v, want [qwen2.5-coder:7b]", resp.Models)
	}
	if resp.SupportsUndo || resp.SupportsPlans {
		t.Errorf("v1 must report SupportsUndo=false and SupportsPlans=false")
	}
}
