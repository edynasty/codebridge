package agentops

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

// runGate is one blocking point: a run waits until the test releases the task
// or its own context is done.
type runGate struct {
	once sync.Once
	ch   chan struct{}
}

// blockingExecutor is a fake ExecutorFunc. Every run blocks until its task is
// released or the run's context is done (explicitly, or by the manager
// closing).
type blockingExecutor struct {
	mu    sync.Mutex
	gates map[string]*runGate
}

func newBlockingExecutor() *blockingExecutor {
	return &blockingExecutor{gates: map[string]*runGate{}}
}

func (b *blockingExecutor) gate(task string) *runGate {
	b.mu.Lock()
	defer b.mu.Unlock()
	g, ok := b.gates[task]
	if !ok {
		g = &runGate{ch: make(chan struct{})}
		b.gates[task] = g
	}
	return g
}

// release lets the run for task proceed to completion.
func (b *blockingExecutor) release(task string) {
	b.gate(task).once.Do(func() { close(b.gate(task).ch) })
}

func (b *blockingExecutor) exec(ctx context.Context, spec RunSpec, root string, onEvent func(string)) (SubagentResult, error) {
	select {
	case <-b.gate(spec.Task).ch:
		return SubagentResult{Client: spec.Client, Status: "completed", Output: spec.Task + " done"}, nil
	case <-ctx.Done():
		return SubagentResult{Client: spec.Client, Status: "cancelled"}, ctx.Err()
	}
}

// startTestManager builds a manager around exec and closes it when the test
// ends. Close cancels outstanding runs, so it never blocks on the fake.
func startTestManager(t *testing.T, exec ExecutorFunc, opts ...RunManagerOption) *RunManager {
	t.Helper()
	m := NewRunManager(append([]RunManagerOption{WithExecutor(exec)}, opts...)...)
	t.Cleanup(func() { _ = m.Close() })
	return m
}

// waitFor polls cond until it holds, failing the test after 5 seconds.
func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

func statusOf(t *testing.T, m *RunManager, id string) RunStatus {
	t.Helper()
	snap, ok := m.Get(id)
	if !ok {
		t.Fatalf("run %s is not known to the manager", id)
	}
	return snap.Status
}

func statusCount(m *RunManager, want RunStatus) int {
	n := 0
	for _, snap := range m.List(0) {
		if snap.Status == want {
			n++
		}
	}
	return n
}

// A: the concurrency cap holds, extra runs queue, releasing a slot promotes
// the queued run, and no permit leaks.
func TestRunManagerLimitsConcurrencyAndQueues(t *testing.T) {
	ex := newBlockingExecutor()
	m := startTestManager(t, ex.exec, WithMaxConcurrent(2))

	start := func(task string) string {
		t.Helper()
		run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: task})
		if err != nil {
			t.Fatalf("start %s: %v", task, err)
		}
		if !strings.HasPrefix(run.ID, "run_") || len(run.ID) != len("run_")+32 {
			t.Fatalf("run id %q is not run_<hex>", run.ID)
		}
		return run.ID
	}

	// Both slots first, so the next run has nowhere to go but the queue.
	first := start("task-0")
	waitFor(t, "the first run to start", func() bool { return statusOf(t, m, first) == RunRunning })
	second := start("task-1")
	waitFor(t, "the second run to start", func() bool { return statusOf(t, m, second) == RunRunning })

	third := start("task-2")
	waitFor(t, "the third run to queue", func() bool { return statusOf(t, m, third) == RunQueued })
	if snap, _ := m.Get(third); !snap.StartedAt.IsZero() {
		t.Fatalf("queued run must not have a start time yet: %v", snap.StartedAt)
	}
	if n := statusCount(m, RunRunning); n != 2 {
		t.Fatalf("running runs = %d, want the cap of 2", n)
	}

	ex.release("task-0")
	waitFor(t, "the queued run to start", func() bool { return statusOf(t, m, third) == RunRunning })
	ex.release("task-1")
	ex.release("task-2")

	waitFor(t, "all runs to finish", func() bool {
		for _, id := range []string{first, second, third} {
			if statusOf(t, m, id) != RunCompleted {
				return false
			}
		}
		return true
	})
	waitFor(t, "every permit to be released", func() bool { return len(m.sem) == 0 })

	// the freed slot is usable: a fourth run executes right away
	fourth := start("task-3")
	ex.release("task-3")
	waitFor(t, "the fourth run to finish", func() bool { return statusOf(t, m, fourth) == RunCompleted })
}

// B: cancelling a running run reaches the executor through its context and
// leaves the run terminal.
func TestRunManagerCancelRunning(t *testing.T) {
	ex := newBlockingExecutor()
	m := startTestManager(t, ex.exec)

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "long"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, "the run to start", func() bool { return statusOf(t, m, run.ID) == RunRunning })

	if err := m.Cancel(run.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := m.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if snap.Status != RunCancelled {
		t.Fatalf("status = %s, want %s", snap.Status, RunCancelled)
	}
	if snap.FinishedAt.IsZero() {
		t.Fatalf("terminal run is not stamped: %+v", snap)
	}
	waitFor(t, "the permit to be released", func() bool { return len(m.sem) == 0 })
}

// C: a queued run can be cancelled without touching the run that holds the
// only slot.
func TestRunManagerCancelQueuedKeepsRunningRun(t *testing.T) {
	ex := newBlockingExecutor()
	m := startTestManager(t, ex.exec, WithMaxConcurrent(1))

	first, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "first"})
	if err != nil {
		t.Fatalf("start first: %v", err)
	}
	waitFor(t, "the first run to start", func() bool { return statusOf(t, m, first.ID) == RunRunning })

	second, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "second"})
	if err != nil {
		t.Fatalf("start second: %v", err)
	}
	waitFor(t, "the second run to queue", func() bool { return statusOf(t, m, second.ID) == RunQueued })

	if err := m.Cancel(second.ID); err != nil {
		t.Fatalf("cancel queued run: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := m.Wait(waitCtx, second.ID)
	if err != nil {
		t.Fatalf("wait for cancelled run: %v", err)
	}
	if snap.Status != RunCancelled {
		t.Fatalf("queued run status = %s, want %s", snap.Status, RunCancelled)
	}
	if !snap.StartedAt.IsZero() {
		t.Fatalf("a cancelled queued run must never have started: %v", snap.StartedAt)
	}
	if got := statusOf(t, m, first.ID); got != RunRunning {
		t.Fatalf("first run status = %s, want %s", got, RunRunning)
	}

	ex.release("first")
	waitFor(t, "the first run to finish", func() bool { return statusOf(t, m, first.ID) == RunCompleted })
}

// D: events are per-run. A neighbour's lines must not reach another run's
// counter, last line or ring buffer.
func TestRunManagerEventIsolation(t *testing.T) {
	emit := func(_ context.Context, spec RunSpec, _ string, onEvent func(string)) (SubagentResult, error) {
		for i := 0; i < 5; i++ {
			onEvent(fmt.Sprintf("%s-%d", spec.Task, i))
			time.Sleep(2 * time.Millisecond)
		}
		return SubagentResult{Client: spec.Client, Status: "completed", Output: spec.Task + " done"}, nil
	}
	m := startTestManager(t, emit, WithMaxConcurrent(2), WithEventCapacity(8))

	a, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "AAA"})
	if err != nil {
		t.Fatalf("start A: %v", err)
	}
	b, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "BBB"})
	if err != nil {
		t.Fatalf("start B: %v", err)
	}
	waitFor(t, "both runs to finish", func() bool {
		return statusOf(t, m, a.ID) == RunCompleted && statusOf(t, m, b.ID) == RunCompleted
	})

	for _, tc := range []struct{ id, tag string }{{a.ID, "AAA"}, {b.ID, "BBB"}} {
		snap, ok := m.Get(tc.id)
		if !ok {
			t.Fatalf("run %s missing", tc.id)
		}
		if snap.EventCount != 5 {
			t.Fatalf("run %s event count = %d, want 5", tc.tag, snap.EventCount)
		}
		if snap.LastEvent != tc.tag+"-4" {
			t.Fatalf("run %s last event = %q, want %q", tc.tag, snap.LastEvent, tc.tag+"-4")
		}
		want := []string{tc.tag + "-0", tc.tag + "-1", tc.tag + "-2", tc.tag + "-3", tc.tag + "-4"}
		if got := m.Events(tc.id, 0); !slices.Equal(got, want) {
			t.Fatalf("run %s events = %v, want %v", tc.tag, got, want)
		}
		if tail := m.Events(tc.id, 2); !slices.Equal(tail, want[3:]) {
			t.Fatalf("run %s event tail = %v, want %v", tc.tag, tail, want[3:])
		}
	}

	// Once the ring is full it keeps only the newest lines, while the count
	// still reports every event.
	small := startTestManager(t, emit, WithMaxConcurrent(1), WithEventCapacity(3))
	run, err := small.Start(context.Background(), RunSpec{Workspace: "work", Task: "CCC"})
	if err != nil {
		t.Fatalf("start C: %v", err)
	}
	waitFor(t, "the ring-limited run to finish", func() bool { return statusOf(t, small, run.ID) == RunCompleted })
	if got, want := small.Events(run.ID, 0), []string{"CCC-2", "CCC-3", "CCC-4"}; !slices.Equal(got, want) {
		t.Fatalf("ring events = %v, want %v", got, want)
	}
	if snap, _ := small.Get(run.ID); snap.EventCount != 5 {
		t.Fatalf("event count = %d, want the total 5", snap.EventCount)
	}
}

// E0: the default/zero budget has no wall-clock deadline. A caller must
// explicitly set a positive timeout to create one.
func TestRunManagerZeroTimeoutHasNoDeadline(t *testing.T) {
	sawDeadline := make(chan bool, 1)
	exec := func(ctx context.Context, spec RunSpec, _ string, _ func(string)) (SubagentResult, error) {
		_, hasDeadline := ctx.Deadline()
		sawDeadline <- hasDeadline
		return SubagentResult{Client: spec.Client, Status: "completed", Output: "ok"}, nil
	}
	m := startTestManager(t, exec)

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "no-deadline"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := m.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if snap.Status != RunCompleted {
		t.Fatalf("status = %s, want %s", snap.Status, RunCompleted)
	}
	select {
	case hasDeadline := <-sawDeadline:
		if hasDeadline {
			t.Fatal("zero/omitted timeout unexpectedly installed a context deadline")
		}
	default:
		t.Fatal("executor did not report whether it received a deadline")
	}
}

// E: a run past its wall-clock budget ends as timeout.
func TestRunManagerTimeout(t *testing.T) {
	ex := newBlockingExecutor()
	m := startTestManager(t, ex.exec)

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "stuck", TimeoutSeconds: 1})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := m.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if snap.Status != RunTimeout {
		t.Fatalf("status = %s, want %s", snap.Status, RunTimeout)
	}
	if snap.ElapsedMS < 900 {
		t.Fatalf("elapsed = %dms, want about the 1s budget", snap.ElapsedMS)
	}
	waitFor(t, "the permit to be released", func() bool { return len(m.sem) == 0 })
}

// F: completed runs survive a restart, and a run left running by a dead
// process is reported as interrupted.
func TestRunManagerStorePersistsAndRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runs.db")
	exec := func(_ context.Context, spec RunSpec, _ string, onEvent func(string)) (SubagentResult, error) {
		onEvent("one line")
		return SubagentResult{Client: spec.Client, Status: "completed", Output: "persisted output"}, nil
	}

	m := NewRunManager(WithStore(path), WithExecutor(exec))
	run, err := m.Start(context.Background(), RunSpec{Workspace: "/work", Task: "persist", Client: "omp"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	snap, err := m.Wait(waitCtx, run.ID)
	if err != nil {
		t.Fatalf("wait: %v", err)
	}
	if snap.Status != RunCompleted || snap.Output != "persisted output" {
		t.Fatalf("run result = %+v", snap)
	}
	if snap.EventCount != 1 {
		t.Fatalf("event count = %d, want 1", snap.EventCount)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Seed the row a killed process would have left behind.
	st, err := openRunStore(path)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	if _, err := st.db.Exec(`INSERT OR REPLACE INTO agent_runs
		(id, workspace, client, agent, model, thinking, task, status, started_at, finished_at, elapsed_ms, output, error, event_count)
		VALUES ('run_deadbeef', '/work', 'omp', '', '', '', 'stale', 'running', ?, 0, 0, 'partial output', '', 0)`,
		time.Now().UnixMilli()); err != nil {
		t.Fatalf("seed stale run: %v", err)
	}
	if err := st.Close(); err != nil {
		t.Fatalf("close store: %v", err)
	}

	reopened := NewRunManager(WithStore(path))
	t.Cleanup(func() { _ = reopened.Close() })

	got, ok := reopened.Get(run.ID)
	if !ok {
		t.Fatalf("run %s is not known after restart", run.ID)
	}
	if got.Status != RunCompleted || got.Output != "persisted output" {
		t.Fatalf("recovered run = %+v, want the completed run", got)
	}
	if got.Workspace != "/work" || got.Client != "omp" || got.Task != "persist" {
		t.Fatalf("recovered run lost its spec: %+v", got)
	}

	dead, ok := reopened.Get("run_deadbeef")
	if !ok {
		t.Fatal("the stale run is not known after restart")
	}
	if dead.Status != RunInterrupted {
		t.Fatalf("stale run status = %s, want %s", dead.Status, RunInterrupted)
	}
	if dead.Output != "partial output" {
		t.Fatalf("stale run output = %q, want the stored tail", dead.Output)
	}

	all := reopened.List(0)
	if len(all) != 2 {
		t.Fatalf("List returned %d runs, want 2", len(all))
	}
	for i := 1; i < len(all); i++ {
		if all[i].StartedAt.After(all[i-1].StartedAt) {
			t.Fatalf("List is not newest first: %v then %v", all[i-1].StartedAt, all[i].StartedAt)
		}
	}
}

// Start must reject a spec that cannot execute, and register nothing.
func TestRunManagerStartValidation(t *testing.T) {
	m := NewRunManager(WithExecutor(func(context.Context, RunSpec, string, func(string)) (SubagentResult, error) {
		t.Error("the executor must not run for a rejected start")
		return SubagentResult{}, nil
	}))
	t.Cleanup(func() { _ = m.Close() })

	for _, tc := range []struct {
		name string
		spec RunSpec
	}{
		{"empty task", RunSpec{Workspace: "work", Task: "   "}},
		{"unsupported client", RunSpec{Workspace: "work", Task: "t", Client: "gemini"}},
		{"bad agent name", RunSpec{Workspace: "work", Task: "t", Agent: "bad agent!"}},
		{"bad thinking level", RunSpec{Workspace: "work", Task: "t", Thinking: "turbo"}},
	} {
		if run, err := m.Start(context.Background(), tc.spec); err == nil {
			t.Fatalf("%s: Start accepted the spec (%+v)", tc.name, run)
		}
	}
	if runs := m.List(0); len(runs) != 0 {
		t.Fatalf("rejected starts were registered: %+v", runs)
	}

	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "after close"}); err == nil {
		t.Fatal("Start accepted a run after Close")
	}
	if err := m.Close(); err != nil {
		t.Fatalf("Close must be idempotent: %v", err)
	}
}

// The state machine is one-way: no run may leave a terminal status.
func TestRunStatusMachineIsOneWay(t *testing.T) {
	legal := []struct{ from, to RunStatus }{
		{RunQueued, RunRunning}, {RunQueued, RunCancelled},
		{RunRunning, RunCompleted}, {RunRunning, RunFailed}, {RunRunning, RunCancelled},
		{RunRunning, RunTimeout}, {RunRunning, RunInterrupted},
	}
	for _, tc := range legal {
		if !canTransition(tc.from, tc.to) {
			t.Fatalf("canTransition(%s, %s) = false, want true", tc.from, tc.to)
		}
	}
	illegal := []struct{ from, to RunStatus }{
		{RunQueued, RunQueued}, {RunQueued, RunCompleted}, {RunQueued, RunFailed},
		{RunQueued, RunTimeout}, {RunQueued, RunInterrupted},
		{RunRunning, RunQueued}, {RunRunning, RunRunning},
		{RunCompleted, RunRunning}, {RunCompleted, RunFailed}, {RunCompleted, RunQueued},
		{RunFailed, RunRunning}, {RunFailed, RunCompleted},
		{RunCancelled, RunRunning}, {RunCancelled, RunCompleted},
		{RunTimeout, RunRunning}, {RunInterrupted, RunRunning}, {RunInterrupted, RunCompleted},
	}
	for _, tc := range illegal {
		if canTransition(tc.from, tc.to) {
			t.Fatalf("canTransition(%s, %s) = true, want false", tc.from, tc.to)
		}
	}
	for _, terminal := range []RunStatus{RunCompleted, RunFailed, RunCancelled, RunTimeout, RunInterrupted} {
		if !terminalRunStatus(terminal) {
			t.Fatalf("%s must be terminal", terminal)
		}
		if len(runTransitions[terminal]) != 0 {
			t.Fatalf("%s must have no successors", terminal)
		}
	}
}

// Preflight answers reach the caller unchanged, so a permission request id
// survives the trip.
func TestRunManagerStartPropagatesPreflightError(t *testing.T) {
	want := &PermissionNeededError{RequestID: "pr_test", Command: "omp subagent"}
	m := NewRunManager(
		WithExecutor(func(context.Context, RunSpec, string, func(string)) (SubagentResult, error) {
			t.Error("the executor must not run when preflight rejects the run")
			return SubagentResult{}, nil
		}),
		WithPreflight(func(spec RunSpec) error {
			if spec.Client == "omp" {
				return want
			}
			return nil
		}),
	)
	t.Cleanup(func() { _ = m.Close() })

	_, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "t", Client: "omp"})
	var needed *PermissionNeededError
	if !errors.As(err, &needed) {
		t.Fatalf("Start error = %v, want *PermissionNeededError", err)
	}
	if needed.RequestID != "pr_test" {
		t.Fatalf("request id = %q, want pr_test", needed.RequestID)
	}
	if runs := m.List(0); len(runs) != 0 {
		t.Fatalf("a rejected run was registered: %+v", runs)
	}
}

// Closing the manager cancels outstanding runs, waits for them, and refuses
// new work.
func TestRunManagerCloseCancelsRuns(t *testing.T) {
	ex := newBlockingExecutor()
	m := NewRunManager(WithExecutor(ex.exec))
	t.Cleanup(func() { _ = m.Close() })

	run, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "blocked"})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	waitFor(t, "the run to start", func() bool { return statusOf(t, m, run.ID) == RunRunning })

	if err := m.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if _, err := m.Start(context.Background(), RunSpec{Workspace: "work", Task: "after close"}); err == nil {
		t.Fatal("Start accepted a run after Close")
	}
	if snap, ok := m.Get(run.ID); !ok || snap.Status != RunCancelled {
		t.Fatalf("run at close = %+v (known=%v), want cancelled", snap, ok)
	}
}

// restorePermissions snapshots the active rules and returns a function that
// puts them back.
func restorePermissions(t *testing.T) func() {
	t.Helper()
	perms.mu.Lock()
	saved := append([]PermissionRule(nil), perms.rules...)
	perms.mu.Unlock()
	return func() { SetPermissions(saved) }
}

// RunSubagentSync must keep the agent tool's old shape: a finished run maps
// onto SubagentResult, with the executor seeing the workspace root.
func TestRunSubagentSyncMapsRunToResult(t *testing.T) {
	defer restorePermissions(t)()
	SetPermissions([]PermissionRule{{Effect: "allow", Tool: "agent", Pattern: "omp"}})

	root := t.TempDir()
	svc := &Service{Roots: map[string]string{"work": root}}
	svc.Runs = NewRunManager(
		WithExecutor(func(_ context.Context, _ RunSpec, gotRoot string, onEvent func(string)) (SubagentResult, error) {
			if gotRoot != root {
				t.Errorf("executor root = %q, want %q", gotRoot, root)
			}
			onEvent(`{"type":"text"}`)
			return SubagentResult{Client: "omp", Status: "completed", Output: "mapped output"}, nil
		}),
		WithPreflight(svc.subagentPreflight),
	)
	t.Cleanup(func() { _ = svc.Runs.Close() })

	res, err := svc.RunSubagentSync(context.Background(), root, "  do the thing  ", "omp", "", "", "", 0)
	if err != nil {
		t.Fatalf("RunSubagentSync: %v", err)
	}
	if res.Status != "completed" || res.Output != "mapped output" {
		t.Fatalf("result = %+v", res)
	}
	if res.Client != "omp" || res.Task != "do the thing" {
		t.Fatalf("result identity = %+v", res)
	}
	if res.Events != 1 {
		t.Fatalf("events = %d, want the 1 streamed line", res.Events)
	}
	if res.Elapsed == "" {
		t.Fatalf("elapsed is empty: %+v", res)
	}
}

// An "ask" harness parks a request and reports its id through the same
// PermissionNeededError the client already understands.
func TestRunSubagentSyncSurfacesPermissionRequest(t *testing.T) {
	defer restorePermissions(t)()
	SetPermissions(nil) // unmatched commands default to ask

	svc := &Service{Roots: map[string]string{"work": t.TempDir()}}
	svc.Runs = NewRunManager(
		WithExecutor(func(context.Context, RunSpec, string, func(string)) (SubagentResult, error) {
			t.Error("the executor must not run without a permission grant")
			return SubagentResult{}, nil
		}),
		WithPreflight(svc.subagentPreflight),
	)
	t.Cleanup(func() { _ = svc.Runs.Close() })

	_, err := svc.RunSubagentSync(context.Background(), svc.Roots["work"], "task", "omp", "", "", "", 0)
	var needed *PermissionNeededError
	if !errors.As(err, &needed) {
		t.Fatalf("error = %v, want *PermissionNeededError", err)
	}
	if needed.RequestID == "" {
		t.Fatal("the permission request id is empty")
	}
	if needed.Command != "omp subagent" {
		t.Fatalf("command = %q, want %q", needed.Command, "omp subagent")
	}
	if !strings.Contains(err.Error(), needed.RequestID) {
		t.Fatalf("error %q does not carry the request id %q", err, needed.RequestID)
	}
}
