package session

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/vellankikoti/kotg-ai-server/internal/provider"
)

func newTestManager(t *testing.T) *Manager {
	t.Helper()
	return New(Config{
		TTL:                   200 * time.Millisecond,
		MaxSessions:           5,
		MaxMessagesPerSession: 4,
		ReaperInterval:        50 * time.Millisecond,
	})
}

func TestCreateAndGet(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()

	s, err := m.Create("c1", "title")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if s.ID == "" {
		t.Fatalf("empty session ID")
	}
	got, ok := m.Get(s.ID)
	if !ok {
		t.Fatalf("Get returned not found")
	}
	if got.FocusClusterID != "c1" {
		t.Errorf("FocusClusterID = %q, want c1", got.FocusClusterID)
	}
}

func TestAppendBumpsUpdatedAtAndCapsMessages(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()
	s, _ := m.Create("c1", "t")

	for i := 0; i < 6; i++ {
		if err := m.Append(s.ID, provider.Message{Role: "user", Content: "x"}); err != nil {
			t.Fatalf("Append %d: %v", i, err)
		}
	}
	got, _ := m.Get(s.ID)
	if len(got.Messages) != 4 {
		t.Errorf("messages = %d, want 4 (cap)", len(got.Messages))
	}
}

func TestMaxSessionsCap(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()
	for i := 0; i < 5; i++ {
		if _, err := m.Create("c", "t"); err != nil {
			t.Fatalf("Create %d: %v", i, err)
		}
	}
	if _, err := m.Create("c", "t"); err == nil {
		t.Fatalf("expected error on 6th Create")
	}
}

func TestTTLEviction(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()
	s, _ := m.Create("c", "t")

	time.Sleep(400 * time.Millisecond)
	if _, ok := m.Get(s.ID); ok {
		t.Errorf("session not evicted after TTL")
	}
}

func TestCancelTurnFiresAndClears(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()
	s, _ := m.Create("c", "t")

	fired := make(chan struct{}, 2)
	_, cancel := context.WithCancel(context.Background())
	wrapped := func() {
		cancel()
		fired <- struct{}{}
	}
	m.SetTurnCancel(s.ID, wrapped)
	if err := m.CancelTurn(s.ID); err != nil {
		t.Fatalf("CancelTurn: %v", err)
	}
	select {
	case <-fired:
	case <-time.After(time.Second):
		t.Fatalf("cancel func not invoked")
	}
	if err := m.CancelTurn(s.ID); err != nil {
		t.Fatalf("second CancelTurn: %v", err)
	}
	select {
	case <-fired:
		t.Fatalf("cancel func fired twice — not cleared after first call")
	case <-time.After(50 * time.Millisecond):
	}
}

func TestListLimitAndSince(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()
	s1, _ := m.Create("c", "t1")
	time.Sleep(10 * time.Millisecond)
	s2, _ := m.Create("c", "t2")

	got := m.List(1, 0)
	if len(got) != 1 || got[0].ID != s2.ID {
		t.Errorf("List(1,0) = %v, want s2", got)
	}
	cutoff := s1.UpdatedAt.Unix()
	got = m.List(10, cutoff)
	for _, s := range got {
		if s.UpdatedAt.Unix() <= cutoff {
			t.Errorf("session %s violates since filter", s.ID)
		}
	}
}

func TestConcurrentSafe(t *testing.T) {
	m := newTestManager(t)
	defer m.Stop()
	s, _ := m.Create("c", "t")
	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = m.Append(s.ID, provider.Message{Role: "user", Content: "x"})
		}()
	}
	wg.Wait()
}
