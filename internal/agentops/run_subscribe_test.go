package agentops

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestRunEventSubscriptionPushesLifecycleAndRaw(t *testing.T) {
	exec := func(_ context.Context, spec RunSpec, _ string, onEvent func(string)) (SubagentResult, error) {
		onEvent("alpha")
		return SubagentResult{Client: spec.Client, Status: "completed", Output: "done"}, nil
	}
	m := startTestManager(t, exec)

	ch, cancelSub := m.SubscribeEvents("", 16)
	defer cancelSub()

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "subscribe", Client: "omp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Wait(ctx, run.ID); err != nil {
		t.Fatalf("wait: %v", err)
	}

	var got []RunEvent
	deadline := time.After(5 * time.Second)
	for len(got) < 4 {
		select {
		case ev := <-ch:
			if ev.RunID == run.ID {
				got = append(got, ev)
			}
		case <-deadline:
			t.Fatalf("timed out waiting for subscribed events: %+v", got)
		}
	}

	wantKinds := []string{"run.queued", "run.started", "harness.raw", "run.completed"}
	for i, want := range wantKinds {
		if got[i].Kind != want {
			t.Fatalf("event %d kind = %q, want %q", i, got[i].Kind, want)
		}
		if got[i].Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, got[i].Seq, i+1)
		}
	}
}

func TestRunEventSlowSubscriberRecoversFromJournal(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	exec := func(_ context.Context, spec RunSpec, _ string, onEvent func(string)) (SubagentResult, error) {
		for i := 0; i < 20; i++ {
			onEvent("event")
		}
		return SubagentResult{Client: spec.Client, Status: "completed", Output: "done"}, nil
	}
	m := startTestManager(t, exec, WithStore(path), WithEventCapacity(64))

	ch, cancelSub := m.SubscribeEvents("", 1)
	defer cancelSub()

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "slow", Client: "omp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	final, err := m.Wait(ctx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if final.Status != RunCompleted {
		t.Fatalf("status = %s, want %s", final.Status, RunCompleted)
	}

	// A one-entry live channel necessarily drops most of the burst, but the
	// durable journal still contains the complete ordered run.
	liveCount := 0
	for {
		select {
		case <-ch:
			liveCount++
		default:
			goto drained
		}
	}

drained:
	if liveCount > 1 {
		t.Fatalf("live channel delivered %d events with capacity 1", liveCount)
	}

	replayed := m.EventsAfter(run.ID, 0, 100)
	if len(replayed) != 23 { // queued + started + 20 raw + completed
		t.Fatalf("replayed events = %d, want 23", len(replayed))
	}
	if replayed[0].Kind != "run.queued" || replayed[len(replayed)-1].Kind != "run.completed" {
		t.Fatalf("replayed bounds = %q ... %q", replayed[0].Kind, replayed[len(replayed)-1].Kind)
	}
	for i, ev := range replayed {
		if ev.Seq != int64(i+1) {
			t.Fatalf("event %d seq = %d, want %d", i, ev.Seq, i+1)
		}
	}
}

func TestRunEventSubscriptionCanBeCancelled(t *testing.T) {
	m := startTestManager(t, func(_ context.Context, spec RunSpec, _ string, _ func(string)) (SubagentResult, error) {
		return SubagentResult{Client: spec.Client, Status: "completed"}, nil
	})

	ch, cancelSub := m.SubscribeEvents("", 4)
	cancelSub()
	cancelSub() // idempotent

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatal("subscription channel remained open after cancel")
		}
	case <-time.After(time.Second):
		t.Fatal("subscription channel was not closed after cancel")
	}
}

func TestRunEventSubscriptionsConcurrentSubscribeCancel(t *testing.T) {
	exec := func(_ context.Context, spec RunSpec, _ string, onEvent func(string)) (SubagentResult, error) {
		for i := 0; i < 100; i++ {
			onEvent("burst")
		}
		return SubagentResult{Client: spec.Client, Status: "completed"}, nil
	}
	m := startTestManager(t, exec, WithEventCapacity(256))

	var wg sync.WaitGroup
	for i := 0; i < 32; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, cancel := m.SubscribeEvents("", 8)
			defer cancel()
			select {
			case <-ch:
			case <-time.After(50 * time.Millisecond):
			}
		}()
	}

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "concurrent subscriptions", Client: "omp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := m.Wait(ctx, run.ID); err != nil {
		t.Fatalf("wait: %v", err)
	}
	wg.Wait()

	events := m.EventsAfter(run.ID, 0, 256)
	if len(events) != 103 { // queued + started + 100 raw + completed
		t.Fatalf("journal events = %d, want 103", len(events))
	}
}
