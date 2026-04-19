package anthropic

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

// fakeAnthropic emits a minimal SSE stream matching the Messages API.
func fakeAnthropic(t *testing.T, deltas []string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		fmt.Fprint(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"id\":\"m1\",\"type\":\"message\",\"role\":\"assistant\",\"content\":[],\"model\":\"claude-3-7-sonnet\",\"stop_reason\":null,\"stop_sequence\":null,\"usage\":{\"input_tokens\":3,\"output_tokens\":0}}}\n\n")
		fmt.Fprint(w, "event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"text\",\"text\":\"\"}}\n\n")
		flusher.Flush()
		for _, d := range deltas {
			fmt.Fprintf(w, "event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":%q}}\n\n", d)
			flusher.Flush()
		}
		fmt.Fprint(w, "event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n")
		fmt.Fprint(w, "event: message_delta\ndata: {\"type\":\"message_delta\",\"delta\":{\"stop_reason\":\"end_turn\",\"stop_sequence\":null},\"usage\":{\"output_tokens\":5}}\n\n")
		fmt.Fprint(w, "event: message_stop\ndata: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
}

func TestAnthropicStreamContract(t *testing.T) {
	srv := fakeAnthropic(t, []string{"hi ", "there"})
	defer srv.Close()
	p, err := New(provider.Config{
		Type: "anthropic", Endpoint: srv.URL, Model: "claude-3-7-sonnet", APIKey: "sk-test",
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	defer p.Close()
	provider.RunStreamContract(t, p)
}

func TestAnthropicRequiresAPIKey(t *testing.T) {
	if _, err := New(provider.Config{Type: "anthropic", Endpoint: "http://x", Model: "m"}); err == nil {
		t.Fatalf("expected error when api key empty")
	}
}
