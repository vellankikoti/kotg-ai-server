package server

import (
	"context"
	"strings"

	"github.com/vellankikoti/kotg-ai-server/internal/prompt"
	"github.com/vellankikoti/kotg-ai-server/internal/provider"
	"github.com/vellankikoti/kotg-ai-server/internal/session"

	kotgv1 "github.com/vellankikoti/kotg-schema/gen/go/kotg/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	tspb "google.golang.org/protobuf/types/known/timestamppb"
)

// ChatHandler implements kotg.v1.Chat. It composes session state, the
// system prompt builder, the token budget, and the provider stream into
// a single bidi turn loop.
type ChatHandler struct {
	kotgv1.UnimplementedChatServer
	sessions       *session.Manager
	p              provider.Provider
	maxBudgetToken int
}

func NewChat(sessions *session.Manager, p provider.Provider, maxBudgetTokens int) *ChatHandler {
	if maxBudgetTokens <= 0 {
		maxBudgetTokens = 16000
	}
	return &ChatHandler{sessions: sessions, p: p, maxBudgetToken: maxBudgetTokens}
}

func (h *ChatHandler) CreateSession(_ context.Context, req *kotgv1.CreateSessionRequest) (*kotgv1.Session, error) {
	if req.FocusClusterId == "" {
		return nil, status.Error(codes.InvalidArgument, "focus_cluster_id required")
	}
	s, err := h.sessions.Create(req.FocusClusterId, req.Title)
	if err != nil {
		return nil, status.Errorf(codes.ResourceExhausted, "create session: %v", err)
	}
	return sessionToProto(s), nil
}

func (h *ChatHandler) Send(stream kotgv1.Chat_SendServer) error {
	md, _ := metadata.FromIncomingContext(stream.Context())
	cids := md.Get("kotg-cluster-id")
	if len(cids) == 0 || cids[0] == "" {
		return status.Error(codes.InvalidArgument, "kotg-cluster-id metadata required")
	}
	clusterID := cids[0]

	msg, err := stream.Recv()
	if err != nil {
		return status.Errorf(codes.InvalidArgument, "first frame: %v", err)
	}
	if msg.SessionId == "" {
		return status.Error(codes.InvalidArgument, "session_id required; call CreateSession first")
	}

	sess, ok := h.sessions.Get(msg.SessionId)
	if !ok {
		return status.Error(codes.NotFound, "session not found")
	}
	if sess.FocusClusterID != clusterID {
		return status.Error(codes.PermissionDenied, "cluster_id does not match session focus")
	}

	text := msg.Text
	if msg.ContextHint != "" {
		text = msg.Text + "\n\n[context: " + msg.ContextHint + "]"
	}

	return h.handleTurnStream(stream, sess.ID, clusterID, text)
}

// HandleTurn runs a complete turn without a gRPC stream. Used by tests
// that exercise session-state side-effects (assistant append, error handling)
// directly. Production traffic always flows through Send.
func (h *ChatHandler) HandleTurn(ctx context.Context, sessionID, clusterID, userText string) {
	if err := h.sessions.Append(sessionID, provider.Message{Role: "user", Content: userText}); err != nil {
		return
	}
	sess, _ := h.sessions.Get(sessionID)
	msgs := []provider.Message{{Role: "system", Content: prompt.BuildSystemPrompt(clusterID)}}
	msgs = append(msgs, sess.Messages...)
	msgs = TrimToBudget(msgs, h.maxBudgetToken)

	turnCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	h.sessions.SetTurnCancel(sessionID, cancel)
	defer h.sessions.SetTurnCancel(sessionID, nil)

	ch, err := h.p.ChatStream(turnCtx, msgs)
	if err != nil {
		return
	}
	var buf strings.Builder
	var sawDelta bool
	for ev := range ch {
		if ev.Kind == provider.KindTextDelta {
			sawDelta = true
			buf.WriteString(ev.Text)
		}
	}
	if sawDelta {
		_ = h.sessions.Append(sessionID, provider.Message{Role: "assistant", Content: buf.String()})
	}
}

func (h *ChatHandler) handleTurnStream(stream kotgv1.Chat_SendServer, sessionID, clusterID, userText string) error {
	if err := h.sessions.Append(sessionID, provider.Message{Role: "user", Content: userText}); err != nil {
		return status.Errorf(codes.Internal, "append: %v", err)
	}
	sess, _ := h.sessions.Get(sessionID)
	msgs := []provider.Message{{Role: "system", Content: prompt.BuildSystemPrompt(clusterID)}}
	msgs = append(msgs, sess.Messages...)
	msgs = TrimToBudget(msgs, h.maxBudgetToken)

	turnCtx, cancel := context.WithCancel(stream.Context())
	defer cancel()
	h.sessions.SetTurnCancel(sessionID, cancel)
	defer h.sessions.SetTurnCancel(sessionID, nil)

	ch, err := h.p.ChatStream(turnCtx, msgs)
	if err != nil {
		return status.Error(provider.ToGRPCCode(err), err.Error())
	}

	var buf strings.Builder
	var sawDelta bool

	for ev := range ch {
		switch ev.Kind {
		case provider.KindTextDelta:
			sawDelta = true
			buf.WriteString(ev.Text)
			if err := stream.Send(&kotgv1.AssistantEvent{
				Event: &kotgv1.AssistantEvent_TextDelta{TextDelta: &kotgv1.TextDelta{Text: ev.Text}},
			}); err != nil {
				return err
			}
		case provider.KindError:
			return status.Error(provider.ToGRPCCode(ev.Error), ev.Error.Error())
		case provider.KindDone:
			// emit Done after loop
		}
	}

	if sawDelta {
		_ = h.sessions.Append(sessionID, provider.Message{Role: "assistant", Content: buf.String()})
	}
	promptTokens := int32(totalTokens(msgs))
	completionTokens := int32(approxTokens(buf.String()))

	return stream.Send(&kotgv1.AssistantEvent{
		Event: &kotgv1.AssistantEvent_Done{Done: &kotgv1.Done{
			PromptTokens:     promptTokens,
			CompletionTokens: completionTokens,
		}},
	})
}

func (h *ChatHandler) CancelTurn(_ context.Context, req *kotgv1.CancelTurnRequest) (*kotgv1.Empty, error) {
	if err := h.sessions.CancelTurn(req.SessionId); err != nil {
		return nil, status.Error(codes.NotFound, err.Error())
	}
	return &kotgv1.Empty{}, nil
}

func (h *ChatHandler) ListSessions(req *kotgv1.ListSessionsRequest, stream kotgv1.Chat_ListSessionsServer) error {
	for _, s := range h.sessions.List(int(req.Limit), req.SinceUnix) {
		if err := stream.Send(sessionToProto(s)); err != nil {
			return err
		}
	}
	return nil
}

func sessionToProto(s *session.Session) *kotgv1.Session {
	return &kotgv1.Session{
		SessionId:      s.ID,
		Title:          s.Title,
		FocusClusterId: s.FocusClusterID,
		CreatedAt:      tspb.New(s.CreatedAt),
		UpdatedAt:      tspb.New(s.UpdatedAt),
		TurnCount:      int32(len(s.Messages) / 2),
	}
}
