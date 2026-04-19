package openai

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

func fakeOpenAI(t *testing.T, deltas []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		for _, d := range deltas {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"content\":%q}}]}\n\n", d)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
}

func TestOpenAIStreamContract(t *testing.T) {
	srv := fakeOpenAI(t, []string{"hi ", "there"})
	defer srv.Close()
	p, err := New(provider.Config{
		Type: "openai", Endpoint: srv.URL, Model: "gpt-4o-mini", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	provider.RunStreamContract(t, p)
}

func TestOpenAIRequiresAPIKey(t *testing.T) {
	if _, err := New(provider.Config{Type: "openai", Endpoint: "http://x", Model: "m"}); err == nil {
		t.Fatalf("expected error when api key empty")
	}
}
