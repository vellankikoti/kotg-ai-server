package ollama

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

// fakeOllama emits a stream of JSON-per-line chat responses matching
// the Ollama /api/chat schema.
func fakeOllama(t *testing.T, deltas []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/chat" {
			http.NotFound(w, r)
			return
		}
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("ResponseWriter not flushable")
		}
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, d := range deltas {
			fmt.Fprintf(w, `{"model":"test","message":{"role":"assistant","content":%q},"done":false}`+"\n", d)
			flusher.Flush()
		}
		fmt.Fprintln(w, `{"model":"test","message":{"role":"assistant","content":""},"done":true,"prompt_eval_count":3,"eval_count":5}`)
	}))
}

func TestOllamaStreamContract(t *testing.T) {
	srv := fakeOllama(t, []string{"hello ", "world"})
	defer srv.Close()

	p, err := New(provider.Config{
		Type: "ollama", Endpoint: srv.URL, Model: "qwen2.5-coder:7b",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()

	provider.RunStreamContract(t, p)
}

func TestOllamaInvalidModelFails(t *testing.T) {
	if _, err := New(provider.Config{Type: "ollama", Endpoint: "http://x", Model: ""}); err == nil {
		t.Fatalf("expected error for empty model")
	}
}

func TestOllamaUnavailableMaps(t *testing.T) {
	p, err := New(provider.Config{Type: "ollama", Endpoint: "http://127.0.0.1:1", Model: "m"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	ch, err := p.ChatStream(ctx, []provider.Message{{Role: "user", Content: "x"}})
	if err != nil {
		return // initial-dial error is acceptable too
	}
	var sawError bool
	for ev := range ch {
		if ev.Kind == provider.KindError {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("expected KindError event when endpoint is dead")
	}
}
