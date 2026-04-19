package server

import (
	"strings"
	"testing"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

func TestTrimToBudgetUnderLimit(t *testing.T) {
	msgs := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
	}
	got := TrimToBudget(msgs, 16000)
	if len(got) != 2 {
		t.Errorf("expected unchanged, got %d msgs", len(got))
	}
}

func TestTrimToBudgetDropsOldest(t *testing.T) {
	msgs := []provider.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: strings.Repeat("a", 4000)},
		{Role: "assistant", Content: strings.Repeat("b", 4000)},
		{Role: "user", Content: strings.Repeat("c", 4000)},
		{Role: "assistant", Content: strings.Repeat("d", 4000)},
		{Role: "user", Content: "latest"},
	}
	got := TrimToBudget(msgs, 2500)
	if got[0].Role != "system" {
		t.Errorf("system must remain first, got role=%q", got[0].Role)
	}
	if got[len(got)-1].Content != "latest" {
		t.Errorf("latest user message must remain, got %q", got[len(got)-1].Content)
	}
	if len(got) >= len(msgs) {
		t.Errorf("expected trim, got %d msgs (input was %d)", len(got), len(msgs))
	}
}

func TestTrimToBudgetSystemNeverDropped(t *testing.T) {
	msgs := []provider.Message{
		{Role: "system", Content: strings.Repeat("S", 100000)},
		{Role: "user", Content: "u"},
	}
	got := TrimToBudget(msgs, 100)
	if len(got) < 2 || got[0].Role != "system" {
		t.Errorf("system must remain even when over budget; got: %+v", got)
	}
}
