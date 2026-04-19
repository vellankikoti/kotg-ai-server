package server

import (
	"context"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
	"github.com/vellankikoti/kotg-ai-server/internal/session"

	kotgv1 "github.com/vellankikoti/kotg-schema/gen/go/kotg/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type fakeProvider struct{ events []provider.Event }

func (f *fakeProvider) ChatStream(ctx context.Context, _ []provider.Message) (<-chan provider.Event, error) {
	out := make(chan provider.Event, len(f.events)+1)
	go func() {
		defer close(out)
		for _, e := range f.events {
			select {
			case out <- e:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out, nil
}
func (f *fakeProvider) Close() error { return nil }

func newTestChatClient(t *testing.T, p provider.Provider) (kotgv1.ChatClient, func()) {
	t.Helper()
	mgr := session.New(session.Config{TTL: time.Minute, MaxSessions: 100, MaxMessagesPerSession: 50, ReaperInterval: time.Second})
	h := NewChat(mgr, p, 16000)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	kotgv1.RegisterChatServer(srv, h)
	go srv.Serve(lis)

	conn, err := grpc.NewClient(lis.Addr().String(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	return kotgv1.NewChatClient(conn), func() {
		conn.Close()
		srv.Stop()
		mgr.Stop()
	}
}

func TestChatCreateSession(t *testing.T) {
	cli, cleanup := newTestChatClient(t, &fakeProvider{})
	defer cleanup()

	s, err := cli.CreateSession(context.Background(), &kotgv1.CreateSessionRequest{FocusClusterId: "c1", Title: "t"})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if s.SessionId == "" || s.FocusClusterId != "c1" {
		t.Errorf("bad session: %+v", s)
	}
}

func TestChatSendStreamsTextDeltas(t *testing.T) {
	p := &fakeProvider{events: []provider.Event{
		{Kind: provider.KindTextDelta, Text: "hello "},
		{Kind: provider.KindTextDelta, Text: "world"},
		{Kind: provider.KindDone},
	}}
	cli, cleanup := newTestChatClient(t, p)
	defer cleanup()

	sess, _ := cli.CreateSession(context.Background(), &kotgv1.CreateSessionRequest{FocusClusterId: "c1"})

	ctx := metadata.AppendToOutgoingContext(context.Background(), "kotg-cluster-id", "c1")
	stream, err := cli.Send(ctx)
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if err := stream.Send(&kotgv1.UserMessage{SessionId: sess.SessionId, TurnId: "t1", Text: "hi"}); err != nil {
		t.Fatalf("send msg: %v", err)
	}
	stream.CloseSend()

	var deltas []string
	var sawDone bool
	for {
		ev, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("Recv: %v", err)
		}
		switch e := ev.Event.(type) {
		case *kotgv1.AssistantEvent_TextDelta:
			deltas = append(deltas, e.TextDelta.Text)
		case *kotgv1.AssistantEvent_Done:
			sawDone = true
		}
	}
	if len(deltas) != 2 || deltas[0] != "hello " || deltas[1] != "world" {
		t.Errorf("deltas = %v", deltas)
	}
	if !sawDone {
		t.Errorf("no Done event")
	}
}

func TestChatSendRejectsMissingClusterID(t *testing.T) {
	cli, cleanup := newTestChatClient(t, &fakeProvider{})
	defer cleanup()
	sess, _ := cli.CreateSession(context.Background(), &kotgv1.CreateSessionRequest{FocusClusterId: "c1"})

	stream, _ := cli.Send(context.Background())
	stream.Send(&kotgv1.UserMessage{SessionId: sess.SessionId, TurnId: "t1", Text: "hi"})
	stream.CloseSend()
	_, err := stream.Recv()
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("err code = %v, want InvalidArgument", status.Code(err))
	}
}

func TestChatSendRejectsClusterMismatch(t *testing.T) {
	cli, cleanup := newTestChatClient(t, &fakeProvider{})
	defer cleanup()
	sess, _ := cli.CreateSession(context.Background(), &kotgv1.CreateSessionRequest{FocusClusterId: "c1"})

	ctx := metadata.AppendToOutgoingContext(context.Background(), "kotg-cluster-id", "c2-different")
	stream, _ := cli.Send(ctx)
	stream.Send(&kotgv1.UserMessage{SessionId: sess.SessionId, TurnId: "t1", Text: "hi"})
	stream.CloseSend()
	_, err := stream.Recv()
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("err code = %v, want PermissionDenied", status.Code(err))
	}
}

func TestChatSendRejectsUnknownSession(t *testing.T) {
	cli, cleanup := newTestChatClient(t, &fakeProvider{})
	defer cleanup()

	ctx := metadata.AppendToOutgoingContext(context.Background(), "kotg-cluster-id", "c1")
	stream, _ := cli.Send(ctx)
	stream.Send(&kotgv1.UserMessage{SessionId: "nonexistent", TurnId: "t1", Text: "hi"})
	stream.CloseSend()
	_, err := stream.Recv()
	if status.Code(err) != codes.NotFound {
		t.Errorf("err code = %v, want NotFound", status.Code(err))
	}
}

func TestChatErrorBeforeTokensSkipsAssistantAppend(t *testing.T) {
	p := &fakeProvider{events: []provider.Event{
		{Kind: provider.KindError, Error: provider.ErrUnavailable},
	}}
	mgr := session.New(session.Config{TTL: time.Minute, MaxSessions: 100, MaxMessagesPerSession: 50})
	defer mgr.Stop()
	h := NewChat(mgr, p, 16000)

	sess, _ := h.CreateSession(context.Background(), &kotgv1.CreateSessionRequest{FocusClusterId: "c1"})
	h.HandleTurn(context.Background(), sess.SessionId, "c1", "user msg")
	s, _ := mgr.Get(sess.SessionId)
	for _, m := range s.Messages {
		if m.Role == "assistant" {
			t.Errorf("assistant message appended despite error-before-tokens")
		}
	}
}
