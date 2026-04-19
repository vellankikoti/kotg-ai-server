// Package provider defines the LLM provider abstraction used by the
// chat handler. Each concrete provider lives in its own subpackage and
// must satisfy the Provider interface contract documented below.
package provider

import "context"

// Config carries the runtime configuration for a single provider instance.
type Config struct {
	Type     string // "ollama" | "openai" | "anthropic"
	Endpoint string // base URL
	Model    string // provider-specific model id
	APIKey   string // resolved from --api-key-env at startup; never logged
}

// Message is the canonical conversation entry handed from sidecar core
// to the provider. Each adapter maps this to its native SDK shape.
type Message struct {
	Role    string // "system" | "user" | "assistant"
	Content string
}

// EventKind distinguishes streamed unit kinds.
type EventKind int

const (
	KindTextDelta EventKind = iota
	KindDone
	KindError
)

// Event is the streamed unit. Mapped 1:1 by the chat handler to
// kotg-schema AssistantEvent variants. Kept internal so providers
// don't import kotg-schema directly.
type Event struct {
	Kind  EventKind
	Text  string // for KindTextDelta
	Error error  // for KindError; classified per errors.go
}

// Provider streams completions for a chat conversation.
//
// Contract:
//   - ChatStream returns a buffered receive channel (cap ChannelBuffer);
//     provider closes it exactly once on success, error, or ctx cancel.
//   - Provider MUST stop emitting events immediately when ctx is
//     cancelled. No goroutine leaks.
//   - Stream emits one or more KindTextDelta events, then exactly one
//     terminal event (KindDone OR KindError), then closes.
//   - No events may be emitted after the terminal event.
//   - Providers MUST NOT log API keys, full prompts, or completions.
type Provider interface {
	ChatStream(ctx context.Context, msgs []Message) (<-chan Event, error)
	// Close releases provider resources (HTTP clients, in-flight streams).
	// Idempotent. Safe to call multiple times.
	Close() error
}

// ChannelBuffer is the standard buffer size for provider event channels.
const ChannelBuffer = 16
