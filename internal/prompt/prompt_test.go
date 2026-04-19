package prompt

import (
	"strings"
	"testing"
)

func TestBuildSystemPromptIncludesClusterID(t *testing.T) {
	got := BuildSystemPrompt("prod-east-1")
	if !strings.Contains(got, "prod-east-1") {
		t.Errorf("prompt missing cluster ID; got: %s", got)
	}
}

func TestBuildSystemPromptHasRules(t *testing.T) {
	got := BuildSystemPrompt("c1")
	for _, want := range []string{"Kubernetes", "kubectl", "--dry-run", "production"} {
		if !strings.Contains(got, want) {
			t.Errorf("prompt missing %q; got:\n%s", want, got)
		}
	}
}

func TestBuildSystemPromptStable(t *testing.T) {
	a := BuildSystemPrompt("c1")
	b := BuildSystemPrompt("c1")
	if a != b {
		t.Errorf("BuildSystemPrompt is non-deterministic")
	}
}
