// Package ollama implements provider.Provider against an Ollama-compatible
// HTTP endpoint via the official Ollama Go SDK.
package ollama

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	api "github.com/ollama/ollama/api"
	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

type Provider struct {
	client *api.Client
	model  string
}

func New(cfg provider.Config) (*Provider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("ollama: model is required")
	}
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("ollama: endpoint is required")
	}
	u, err := url.Parse(cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("ollama: parse endpoint: %w", err)
	}
	return &Provider{
		client: api.NewClient(u, http.DefaultClient),
		model:  cfg.Model,
	}, nil
}

func (p *Provider) Close() error { return nil }

func (p *Provider) ChatStream(ctx context.Context, msgs []provider.Message) (<-chan provider.Event, error) {
	out := make(chan provider.Event, provider.ChannelBuffer)

	apiMsgs := make([]api.Message, 0, len(msgs))
	for _, m := range msgs {
		apiMsgs = append(apiMsgs, api.Message{Role: m.Role, Content: m.Content})
	}

	go func() {
		defer close(out)
		var emittedTerminal bool
		emitTerm := func(ev provider.Event) {
			if emittedTerminal {
				return
			}
			emittedTerminal = true
			select {
			case out <- ev:
			case <-ctx.Done():
			}
		}

		streamTrue := true
		err := p.client.Chat(ctx, &api.ChatRequest{
			Model:    p.model,
			Messages: apiMsgs,
			Stream:   &streamTrue,
		}, func(resp api.ChatResponse) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if resp.Message.Content != "" {
				select {
				case out <- provider.Event{Kind: provider.KindTextDelta, Text: resp.Message.Content}:
				case <-ctx.Done():
					return ctx.Err()
				}
			}
			if resp.Done {
				emitTerm(provider.Event{Kind: provider.KindDone})
			}
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			emitTerm(provider.Event{
				Kind:  provider.KindError,
				Error: fmt.Errorf("%w: %v", provider.ErrUnavailable, err),
			})
			return
		}
		// Defensive: synthesize Done if Ollama closed without it.
		emitTerm(provider.Event{Kind: provider.KindDone})
	}()

	return out, nil
}
