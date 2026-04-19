package server

import (
	"context"

	kotgv1 "github.com/vellankikoti/kotg-schema/gen/go/kotg/v1"
)

// AIVersion is the kotg-ai-server build semver reported in Capabilities.
const AIVersion = "0.1.0"

// SchemaVersion is the kotg-schema version this binary was built against.
const SchemaVersion = "1.0.1"

// AIControlHandler implements kotg.v1.AIControl. v1 reports a single
// configured provider/model and disables Undo + Plans (those land in v2).
type AIControlHandler struct {
	kotgv1.UnimplementedAIControlServer
	providerType string
	model        string
}

func NewAIControl(providerType, model string) *AIControlHandler {
	return &AIControlHandler{providerType: providerType, model: model}
}

func (h *AIControlHandler) Capabilities(_ context.Context, _ *kotgv1.Empty) (*kotgv1.AICapabilities, error) {
	return &kotgv1.AICapabilities{
		SchemaVersion: SchemaVersion,
		AiVersion:     AIVersion,
		Providers:     []string{h.providerType},
		Models:        []string{h.model},
		SupportsUndo:  false,
		SupportsPlans: false,
	}, nil
}
