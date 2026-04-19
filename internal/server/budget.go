// Package server hosts the gRPC handlers (AIControl + Chat) and helpers
// that compose providers, sessions, and prompts into the wire surface.
package server

import "github.com/vellankikoti/kotg-ai-server/internal/provider"

// approxTokens estimates token count using the chars/4 rule of thumb.
// Sufficient for budget enforcement; precise tokenization defers to v2.
func approxTokens(s string) int {
	return (len(s) + 3) / 4
}

func totalTokens(msgs []provider.Message) int {
	n := 0
	for _, m := range msgs {
		n += approxTokens(m.Content)
	}
	return n
}

// TrimToBudget returns msgs trimmed to fit within max approximate tokens.
//
// Invariants:
//   - The first message (assumed system) is never dropped.
//   - The last message (assumed latest user turn) is never dropped.
//   - Drops oldest user/assistant pairs in order until budget is met OR
//     only system + last remain.
func TrimToBudget(msgs []provider.Message, max int) []provider.Message {
	if len(msgs) <= 2 {
		return msgs
	}
	head := msgs[0]
	tail := msgs[len(msgs)-1]
	middle := append([]provider.Message{}, msgs[1:len(msgs)-1]...)

	out := append([]provider.Message{head}, middle...)
	out = append(out, tail)

	for totalTokens(out) > max && len(middle) > 0 {
		middle = middle[1:]
		out = append([]provider.Message{head}, middle...)
		out = append(out, tail)
	}
	return out
}
