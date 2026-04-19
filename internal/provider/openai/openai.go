// Package openai implements provider.Provider against the OpenAI chat
// completions streaming API via github.com/sashabaranov/go-openai.
package openai

import (
	"context"
	"errors"
	"fmt"
	"io"

	sdk "github.com/sashabaranov/go-openai"
	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

type Provider struct {
	client *sdk.Client
	model  string
}

func New(cfg provider.Config) (*Provider, error) {
	if cfg.Model == "" {
		return nil, fmt.Errorf("openai: model is required")
	}
	if cfg.APIKey == "" {
		return nil, fmt.Errorf("openai: api key is required (check --api-key-env)")
	}
	sdkCfg := sdk.DefaultConfig(cfg.APIKey)
	if cfg.Endpoint != "" {
		sdkCfg.BaseURL = cfg.Endpoint
	}
	return &Provider{client: sdk.NewClientWithConfig(sdkCfg), model: cfg.Model}, nil
}

func (p *Provider) Close() error { return nil }

func (p *Provider) ChatStream(ctx context.Context, msgs []provider.Message) (<-chan provider.Event, error) {
	out := make(chan provider.Event, provider.ChannelBuffer)

	sdkMsgs := make([]sdk.ChatCompletionMessage, 0, len(msgs))
	for _, m := range msgs {
		sdkMsgs = append(sdkMsgs, sdk.ChatCompletionMessage{Role: m.Role, Content: m.Content})
	}

	stream, err := p.client.CreateChatCompletionStream(ctx, sdk.ChatCompletionRequest{
		Model:    p.model,
		Messages: sdkMsgs,
		Stream:   true,
	})
	if err != nil {
		close(out)
		return nil, classifyOpenAIError(err)
	}

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
		for {
			if ctx.Err() != nil {
				return
			}
			resp, err := stream.Recv()
			if errors.Is(err, io.EOF) {
				emitTerm(provider.Event{Kind: provider.KindDone})
				return
			}
			if err != nil {
				emitTerm(provider.Event{Kind: provider.KindError, Error: classifyOpenAIError(err)})
				return
			}
			for _, ch := range resp.Choices {
				if ch.Delta.Content != "" {
					select {
					case out <- provider.Event{Kind: provider.KindTextDelta, Text: ch.Delta.Content}:
					case <-ctx.Done():
						return
					}
				}
			}
		}
	}()
	return out, nil
}

func classifyOpenAIError(err error) error {
	var apiErr *sdk.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.HTTPStatusCode {
		case 429:
			return fmt.Errorf("%w: %v", provider.ErrRateLimited, err)
		case 400, 422:
			return fmt.Errorf("%w: %v", provider.ErrInvalidArgument, err)
		case 500, 502, 503, 504:
			return fmt.Errorf("%w: %v", provider.ErrUnavailable, err)
		}
	}
	return fmt.Errorf("%w: %v", provider.ErrUnavailable, err)
}
