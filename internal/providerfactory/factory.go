// Package providerfactory dispatches a provider.Config to the matching
// concrete provider constructor (ollama / openai / anthropic).
//
// Why a sibling package and not provider.New: the concrete provider
// subpackages already import provider (for Config, Message, Event, the
// Provider interface, etc.). Putting the factory inside package
// provider would form an import cycle. Hosting it one layer up keeps
// provider as a leaf package depended on by its concrete adapters,
// while the factory composes them.
package providerfactory

import (
	"fmt"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
	"github.com/vellankikoti/kotg-ai-server/internal/provider/anthropic"
	"github.com/vellankikoti/kotg-ai-server/internal/provider/ollama"
	"github.com/vellankikoti/kotg-ai-server/internal/provider/openai"
)

// New validates config and returns a configured provider. Fails fast on
// invalid type, empty model, or missing API key (provider-specific).
// Called once at startup; never at request time.
func New(cfg provider.Config) (provider.Provider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("provider: model is required")
	}
	switch cfg.Type {
	case "ollama":
		return ollama.New(cfg)
	case "openai":
		return openai.New(cfg)
	case "anthropic":
		return anthropic.New(cfg)
	default:
		return nil, fmt.Errorf("provider: unsupported type %q", cfg.Type)
	}
}
