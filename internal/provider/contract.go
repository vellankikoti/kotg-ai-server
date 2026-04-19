package provider

import (
	"context"
	"testing"
	"time"
)

// RunStreamContract asserts that p.ChatStream satisfies the Provider
// interface contract for one happy-path call. Each concrete provider
// test calls this after wiring up an httptest server that returns a
// well-formed streaming response.
func RunStreamContract(t *testing.T, p Provider) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ch, err := p.ChatStream(ctx, []Message{{Role: "user", Content: "hello"}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}

	var (
		gotDelta     int
		gotTerminal  int
		afterTerm    int
		terminalKind EventKind
	)
	for ev := range ch {
		if gotTerminal > 0 {
			afterTerm++
			continue
		}
		switch ev.Kind {
		case KindTextDelta:
			gotDelta++
		case KindDone, KindError:
			gotTerminal++
			terminalKind = ev.Kind
		}
	}

	if gotDelta == 0 && terminalKind != KindError {
		t.Errorf("contract: expected >=1 TextDelta or terminal Error, got 0 deltas and Done")
	}
	if gotTerminal != 1 {
		t.Errorf("contract: expected exactly 1 terminal event, got %d", gotTerminal)
	}
	if afterTerm > 0 {
		t.Errorf("contract: %d events after terminal", afterTerm)
	}
}

// RunCancellationContract asserts that ctx cancellation halts emission
// promptly and the channel closes.
func RunCancellationContract(t *testing.T, p Provider) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := p.ChatStream(ctx, []Message{{Role: "user", Content: "stream please"}})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	select {
	case _, ok := <-ch:
		if !ok {
			t.Fatalf("channel closed before first event")
		}
	case <-time.After(2 * time.Second):
		t.Fatalf("no event within 2s")
	}
	cancel()
	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return // closed — good
			}
		case <-deadline:
			t.Fatalf("channel did not close within 2s of cancel")
		}
	}
}
