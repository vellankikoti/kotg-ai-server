// Package anthropic implements provider.Provider against the Anthropic
// Messages streaming API via github.com/anthropics/anthropic-sdk-go.
package anthropic

import (
	"context"
	"errors"
	"fmt"

	sdk "github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

type Provider struct {
	client *sdk.Client
	model  string
}

func New(cfg provider.Config) (*Provider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("anthropic: model is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("anthropic: api key is required (check --api-key-env)")
	}
	opts := []option.RequestOption{option.WithAPIKey(cfg.APIKey)}
	if cfg.Endpoint != "" {
		opts = append(opts, option.WithBaseURL(cfg.Endpoint))
	}
	c := sdk.NewClient(opts...)
	return &Provider{client: &c, model: cfg.Model}, nil
}

func (p *Provider) Close() error { return nil }

func (p *Provider) ChatStream(ctx context.Context, msgs []provider.Message) (<-chan provider.Event, error) {
	out := make(chan provider.Event, provider.ChannelBuffer)

	var systemBlocks []sdk.TextBlockParam
	var convo []sdk.MessageParam
	for _, m := range msgs {
		switch m.Role {
		case "system":
			systemBlocks = append(systemBlocks, sdk.TextBlockParam{Text: m.Content})
		case "user":
			convo = append(convo, sdk.NewUserMessage(sdk.NewTextBlock(m.Content)))
		case "assistant":
			convo = append(convo, sdk.NewAssistantMessage(sdk.NewTextBlock(m.Content)))
		}
	}

	stream := p.client.Messages.NewStreaming(ctx, sdk.MessageNewParams{
		Model:     sdk.Model(p.model),
		MaxTokens: 4096,
		System:    systemBlocks,
		Messages:  convo,
	})

	go func() {
		defer close(out)
		defer stream.Close()
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
		for stream.Next() {
			if ctx.Err() != nil {
				return
			}
			ev := stream.Current()
			switch d := ev.AsAny().(type) {
			case sdk.ContentBlockDeltaEvent:
				if td, ok := d.Delta.AsAny().(sdk.TextDelta); ok && td.Text != "" {
					select {
					case out <- provider.Event{Kind: provider.KindTextDelta, Text: td.Text}:
					case <-ctx.Done():
						return
					}
				}
			case sdk.MessageStopEvent:
				emitTerm(provider.Event{Kind: provider.KindDone})
				return
			}
		}
		if err := stream.Err(); err != nil && !errors.Is(err, context.Canceled) {
			emitTerm(provider.Event{Kind: provider.KindError, Error: fmt.Errorf("%w: %v", provider.ErrUnavailable, err)})
			return
		}
		emitTerm(provider.Event{Kind: provider.KindDone})
	}()
	return out, nil
}
