package main

import (
	"context"
	"encoding/json"
	"errors"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/authstore"
	"github.com/edynasty/codebridge/internal/manager"
	"github.com/edynasty/codebridge/internal/protocol"
)

// End-to-end tests for the agent run tools: a real manager registry, a real
// gRPC session and the client's real wiring, with only the harness faked.
// They exist to prove the property the run manager was built for — a gRPC
// session is not the owner of a run, so nothing that ends a session (a network
// drop, an abandoned request, a cancel) may end local work silently.
const chainDeviceID = "chain-device"

// chainHarness is one in-process deployment: the manager side (auth store,
// registry, gRPC server) plus the client side (service and the process-level
// run manager), wired exactly like main wires them.
type chainHarness struct {
	t          *testing.T
	registry   *manager.Registry
	service    *agentops.Service
	live       *atomic.Pointer[agentops.Service]
	runs       *agentops.RunManager
	processCtx context.Context
	target     string
	workspace  string
	enroll     string
}

// newChainHarness builds the deployment around a fake harness. The run
// manager is created through the client's own constructor, so workspace
// mapping, admission, persistence and the live-runtime lookup under test are
// the production ones.
func newChainHarness(t *testing.T, exec agentops.ExecutorFunc) *chainHarness {
	t.Helper()

	// The service admission check applies the agent permission rules, so the
	// harness grants the agent tool the way a client administrator would. The
	// rules are process-global; no other test in this package reads them.
	agentops.SetPermissions([]agentops.PermissionRule{{Effect: "allow", Tool: "agent", Pattern: "*"}})
	t.Cleanup(func() { agentops.SetPermissions(nil) })

	store, err := authstore.Open(filepath.Join(t.TempDir(), "auth.json"))
	if err != nil {
		t.Fatalf("open auth store: %v", err)
	}
	enrollment, _, err := store.CreateEnrollment(5*time.Minute, authstore.DefaultAccount)
	if err != nil {
		t.Fatalf("create enrollment: %v", err)
	}
	registry := manager.NewRegistry(4)
	grpcSrv, listener, err := manager.ServeGRPC("127.0.0.1:0", &manager.GRPCAgentServer{Registry: registry, Auth: store})
	if err != nil {
		t.Fatalf("serve grpc: %v", err)
	}
	t.Cleanup(grpcSrv.Stop)

	// The process context is the run manager's root, not any session's: it
	// outlives every session the test opens and closes.
	processCtx, stopProcess := context.WithCancel(context.Background())
	t.Cleanup(stopProcess)

	workspace := t.TempDir()
	live := &atomic.Pointer[agentops.Service]{}
	runs := newRunManagerWith(processCtx, t.TempDir(), live, func(*agentops.Service) agentops.ExecutorFunc { return exec })
	t.Cleanup(func() { _ = runs.Close() })

	return &chainHarness{
		t:          t,
		registry:   registry,
		service:    &agentops.Service{Roots: map[string]string{"work": workspace}},
		live:       live,
		runs:       runs,
		processCtx: processCtx,
		target:     listener.Addr().String(),
		workspace:  workspace,
		enroll:     enrollment,
	}
}

// sessionRuntime returns the connection settings for one session: a fresh
// runtime sharing the harness service and run manager, which is what main
// does when a configuration reload replaces the runtime but never the manager.
func (h *chainHarness) sessionRuntime(reg protocol.RegisterRequest) *runtime {
	rt := &runtime{grpcTarget: h.target, service: h.service, reg: reg}
	activateRuntime(rt, h.runs, h.live)
	return rt
}

// registerEnrollment is the registration of a first session: the harness
// enrollment code plus the advertised workspace.
func (h *chainHarness) registerEnrollment() protocol.RegisterRequest {
	return protocol.RegisterRequest{
		EnrollmentCode: h.enroll,
		DeviceID:       chainDeviceID,
		DeviceName:     "Chain Test Device",
		Version:        "test",
		Workspaces:     []protocol.Workspace{{Name: "work"}},
	}
}

// registerCredential is the registration of a reconnect: the device
// credential the first session was issued, with no enrollment code.
func (h *chainHarness) registerCredential(credential string) protocol.RegisterRequest {
	reg := h.registerEnrollment()
	reg.EnrollmentCode = ""
	reg.DeviceCredential = credential
	reg.Version = "test-reconnect"
	return reg
}

// connect starts one client gRPC session and waits until the manager has the
// device. The session derives from the process context, as in main, so
// cancelling it never touches a run.
func (h *chainHarness) connect(reg protocol.RegisterRequest) (context.CancelFunc, <-chan error, <-chan string) {
	t := h.t
	t.Helper()
	sessionCtx, cancel := context.WithCancel(h.processCtx)
	done := make(chan error, 1)
	issued := make(chan string, 1)
	rt := h.sessionRuntime(reg)
	go func() {
		done <- runGRPCSession(sessionCtx, rt, &clientState{}, func(credential string) error {
			issued <- credential
			return nil
		})
	}()
	waitForDeviceState(t, h.registry, reg.DeviceID, true)
	t.Cleanup(cancel)
	return cancel, done, issued
}

// callCtx invokes one tool over the live session with a caller-chosen
// deadline, for the tests that deliberately outlive their own wait.
func (h *chainHarness) callCtx(tool string, args map[string]any, timeout time.Duration) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	return h.registry.Call(ctx, authstore.DefaultAccount, chainDeviceID, protocol.AgentRequest{
		Tool:      tool,
		Workspace: "work",
		Args:      args,
	})
}

// call is callCtx with a deadline far beyond anything the test waits for.
func (h *chainHarness) call(t *testing.T, tool string, args map[string]any) json.RawMessage {
	t.Helper()
	raw, err := h.callCtx(tool, args, 10*time.Second)
	if err != nil {
		t.Fatalf("%s over the session: %v", tool, err)
	}
	return raw
}

// statusOverSession reads one run's status through the live session, the way
// a manager asking about a run does.
func (h *chainHarness) statusOverSession(id string) runAnswer {
	return decode[runAnswer](h.t, h.call(h.t, "agent_status", map[string]any{"run_id": id}))
}

// awaitTerminal polls agent_result over the session until the run reports a
// terminal status.
func (h *chainHarness) awaitTerminal(id string) runAnswer {
	t := h.t
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		answer := decode[runAnswer](t, h.call(t, "agent_result", map[string]any{"run_id": id}))
		if agentops.RunStatus(answer.Status).Terminal() {
			return answer
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s = %q, want a terminal status", id, answer.Status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// endSession closes one session the way the client loop does when a reload,
// a manual disconnect or a dropped connection ends it.
func endSession(t *testing.T, cancel context.CancelFunc, done <-chan error) {
	t.Helper()
	cancel()
	if err := waitSession(done); !errors.Is(err, context.Canceled) {
		t.Fatalf("session ended with %v, want context.Canceled", err)
	}
}

// chainExecutor is the fake harness these tests inject. It streams its tag
// once on entry and then every beat until the test releases it, so a run stays
// visibly alive while a session dies, a cancel lands or another run races it.
type chainExecutor struct {
	tag     string        // event line; the run's task when empty
	beat    time.Duration // streaming interval
	release chan struct{}
	once    sync.Once

	entered  sync.Once
	started  chan struct{}
	ctxOnce  sync.Once
	ctxEnded chan struct{}
	retOnce  sync.Once
	returned chan struct{}

	mu    sync.Mutex
	roots []string
}

func newChainExecutor(tag string) *chainExecutor {
	return &chainExecutor{
		tag:      tag,
		beat:     100 * time.Millisecond,
		release:  make(chan struct{}),
		started:  make(chan struct{}),
		ctxEnded: make(chan struct{}),
		returned: make(chan struct{}),
	}
}

// finish releases the blocked run, which then completes with "<task> done".
func (e *chainExecutor) finish() { e.once.Do(func() { close(e.release) }) }

func (e *chainExecutor) exec(ctx context.Context, spec agentops.RunSpec, root string, onEvent func(string)) (agentops.SubagentResult, error) {
	e.entered.Do(func() { close(e.started) })
	defer e.retOnce.Do(func() { close(e.returned) })
	e.mu.Lock()
	e.roots = append(e.roots, root)
	e.mu.Unlock()

	tag := e.tag
	if tag == "" {
		tag = spec.Task
	}
	onEvent(tag)
	ticker := time.NewTicker(e.beat)
	defer ticker.Stop()
	for {
		select {
		case <-e.release:
			return agentops.SubagentResult{Client: spec.Client, Status: "completed", Output: spec.Task + " done"}, nil
		case <-ctx.Done():
			e.ctxOnce.Do(func() { close(e.ctxEnded) })
			return agentops.SubagentResult{Client: spec.Client, Status: "cancelled"}, ctx.Err()
		case <-ticker.C:
			onEvent(tag)
		}
	}
}

// rootsSeen reports the workspace roots the manager handed the harness, in
// call order: the executor receives the resolved root, never the workspace
// name a request carried.
func (e *chainExecutor) rootsSeen() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.roots...)
}

// runAnswer is the subset agent_status and agent_result share: pending and
// terminal answers differ only in which of these fields they fill in.
type runAnswer struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	FinishedAt string `json:"finished_at"`
	EventCount int    `json:"event_count"`
	LastEvent  string `json:"last_event"`
}

// rawKeys returns the top-level JSON keys of a tool answer exactly as it
// crossed the wire.
func rawKeys(t *testing.T, raw json.RawMessage) map[string]bool {
	t.Helper()
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("decode keys of %s: %v", raw, err)
	}
	keys := map[string]bool{}
	for key := range obj {
		keys[key] = true
	}
	return keys
}

// decode unmarshals a tool answer into the type the tool promises.
func decode[T any](t *testing.T, raw json.RawMessage) T {
	t.Helper()
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decode %s: %v", raw, err)
	}
	return out
}

// awaitCredential waits for the first session to be issued its device
// credential, which is what a reconnect authenticates with.
func awaitCredential(t *testing.T, issued <-chan string) string {
	t.Helper()
	select {
	case credential := <-issued:
		return credential
	case <-time.After(5 * time.Second):
		t.Fatal("the first session was not issued a device credential")
		return ""
	}
}

// awaitCondition polls cond until it holds, failing the test after timeout.
func awaitCondition(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		if cond() {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// isClosed reports whether a signal channel has been closed.
func isClosed(ch chan struct{}) bool {
	select {
	case <-ch:
		return true
	default:
		return false
	}
}

// assertOnlyStatuses fails when a run was ever seen in a status outside want:
// a session lifecycle must never produce a status of its own.
func assertOnlyStatuses(t *testing.T, seen []agentops.RunStatus, want ...agentops.RunStatus) {
	t.Helper()
	for _, status := range seen {
		allowed := false
		for _, candidate := range want {
			if status == candidate {
				allowed = true
			}
		}
		if !allowed {
			t.Fatalf("run reported %q, want only %v", status, want)
		}
	}
}

// A. A session that dies takes its requests with it, never its runs. The run
// keeps executing while the device is offline and is still there — with the
// events it recorded in the meantime — when the device reconnects.
func TestGRPCDisconnectKeepsAgentRunning(t *testing.T) {
	exec := newChainExecutor("tick")
	h := newChainHarness(t, exec.exec)
	disconnect, done, issued := h.connect(h.registerEnrollment())
	credential := awaitCredential(t, issued)
	t.Logf("session 1 registered device %s with a freshly issued credential", chainDeviceID)

	start := decode[agentStartReply](t, h.call(t, "agent_start", map[string]any{"task": "long task"}))
	if start.Status != string(agentops.RunQueued) && start.Status != string(agentops.RunRunning) {
		t.Fatalf("agent_start status = %q, want queued or running", start.Status)
	}
	if start.Workspace != "work" {
		t.Fatalf("agent_start workspace = %q, want the workspace name the caller passed", start.Workspace)
	}
	awaitStatus(t, h.runs, start.RunID, agentops.RunRunning)
	awaitCondition(t, 5*time.Second, "the harness to start", func() bool { return isClosed(exec.started) })
	if roots := exec.rootsSeen(); len(roots) != 1 || roots[0] != h.workspace {
		t.Fatalf("harness roots = %v, want the resolved root %q", roots, h.workspace)
	}
	before := decode[runAnswer](t, h.call(t, "agent_status", map[string]any{"run_id": start.RunID}))
	if before.Status != string(agentops.RunRunning) {
		t.Fatalf("agent_status over the session = %q, want running", before.Status)
	}
	seen := []agentops.RunStatus{agentops.RunQueued, agentops.RunRunning}
	t.Logf("run %s running over the session with %d events", start.RunID, before.EventCount)

	// The session ends the way a network drop ends it: the client cancels the
	// session context mid-run and the manager drops the device.
	endSession(t, disconnect, done)
	waitForDeviceState(t, h.registry, chainDeviceID, false)
	disconnectedAt := time.Now()

	// A session-bound run would already be dead. Instead it must still be
	// running one second later, with events recorded after the disconnect.
	awaitCondition(t, 5*time.Second, "the offline run to keep streaming", func() bool {
		snap, known := h.runs.Get(start.RunID)
		return known && snap.Status == agentops.RunRunning && snap.EventCount > before.EventCount &&
			time.Since(disconnectedAt) >= time.Second
	})
	offline, _ := h.runs.Get(start.RunID)
	seen = append(seen, offline.Status)
	if offline.LastEvent != "tick" {
		t.Fatalf("offline run last event = %q, want the harness line", offline.LastEvent)
	}
	t.Logf("one second after the session died, run %s is %s with %d events (was %d)",
		start.RunID, offline.Status, offline.EventCount, before.EventCount)

	// Reconnect: a new session for the same device reaches the same runs.
	cancelReconnect, reconnectDone, _ := h.connect(h.registerCredential(credential))
	resumed := h.statusOverSession(start.RunID)
	if resumed.Status != string(agentops.RunRunning) {
		t.Fatalf("agent_status over the new session = %q, want the still-running run", resumed.Status)
	}
	seen = append(seen, agentops.RunStatus(resumed.Status))
	if resumed.EventCount < offline.EventCount {
		t.Fatalf("event count after reconnect = %d, want at least the %d recorded offline", resumed.EventCount, offline.EventCount)
	}
	// The run is not merely visible through the new session: it is still
	// producing events while that session serves it.
	awaitCondition(t, 5*time.Second, "the reconnected session to see new events", func() bool {
		return h.statusOverSession(start.RunID).EventCount > resumed.EventCount
	})
	grown := h.statusOverSession(start.RunID)
	t.Logf("the reconnected session found run %s %s: %d events when it connected, %d now",
		start.RunID, grown.Status, resumed.EventCount, grown.EventCount)

	exec.finish()
	finished := h.awaitTerminal(start.RunID)
	if finished.Status != string(agentops.RunCompleted) || finished.Output != "long task done" {
		t.Fatalf("agent_result = %+v, want the completed run and its output", finished)
	}
	seen = append(seen, agentops.RunStatus(finished.Status))
	snap, known := h.runs.Get(start.RunID)
	if !known || snap.Status != agentops.RunCompleted || snap.Error != "" {
		t.Fatalf("run in the manager = %+v (known=%v), want completed without error", snap, known)
	}
	assertOnlyStatuses(t, seen, agentops.RunQueued, agentops.RunRunning, agentops.RunCompleted)
	t.Logf("run %s completed with output %q after %d events", start.RunID, finished.Output, snap.EventCount)

	endSession(t, cancelReconnect, reconnectDone)
}

// B. The legacy agent tool keeps the answer shape MCP clients have always
// seen, even though the run behind it is a managed one.
func TestLegacyAgentToolKeepsResultShape(t *testing.T) {
	exec := newChainExecutor("line")
	h := newChainHarness(t, exec.exec)
	disconnect, done, issued := h.connect(h.registerEnrollment())
	awaitCredential(t, issued)
	exec.finish() // the harness answers at once: this is the synchronous path

	raw := h.call(t, "agent", map[string]any{"task": "legacy task"})
	wantJSONKeys(t, rawKeys(t, raw), "client", "task", "status", "output", "events", "elapsed")
	result := decode[agentops.SubagentResult](t, raw)
	if result.Status != "completed" || result.Client != "omp" {
		t.Fatalf("agent result = %+v, want a completed omp run", result)
	}
	if result.Task != "legacy task" || result.Output != "legacy task done" {
		t.Fatalf("agent result identity = %+v", result)
	}
	if result.Events != 1 || result.Elapsed == "" {
		t.Fatalf("agent result progress = %+v, want one streamed line and an elapsed time", result)
	}
	if roots := exec.rootsSeen(); len(roots) != 1 || roots[0] != h.workspace {
		t.Fatalf("harness roots = %v, want the resolved root %q", roots, h.workspace)
	}

	// The answer above came from a managed run the caller can still query.
	rows := decode[agentRunsReply](t, h.call(t, "agent_runs", nil)).Runs
	if len(rows) != 1 || rows[0].Status != string(agentops.RunCompleted) || rows[0].EventCount != 1 {
		t.Fatalf("agent_runs = %+v, want the one completed run with one event", rows)
	}
	t.Logf("legacy agent call returned %+v from managed run %s", result, rows[0].RunID)

	endSession(t, disconnect, done)
}

// B. A caller that gives up waiting leaves the run intact and collectable.
// That is what makes the legacy agent tool safe over an unreliable link: the
// wait is bounded, the work is not.
func TestLegacyAgentWaitTimeoutLeavesRunRunning(t *testing.T) {
	exec := newChainExecutor("tick")
	h := newChainHarness(t, exec.exec)
	cancel, done, issued := h.connect(h.registerEnrollment())
	awaitCredential(t, issued)

	// The caller's request context expires while the harness is still working.
	if _, err := h.callCtx("agent", map[string]any{"task": "slow task"}, 400*time.Millisecond); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("agent call error = %v, want context.DeadlineExceeded", err)
	}
	rows := decode[agentRunsReply](t, h.call(t, "agent_runs", nil)).Runs
	if len(rows) != 1 || rows[0].Status != string(agentops.RunRunning) {
		t.Fatalf("agent_runs after the abandoned call = %+v, want the one running run", rows)
	}
	pending := decode[runAnswer](t, h.call(t, "agent_result", map[string]any{"run_id": rows[0].RunID}))
	if pending.Status != string(agentops.RunRunning) {
		t.Fatalf("agent_result after the abandoned call = %+v, want the pending run", pending)
	}
	t.Logf("run %s survived the abandoned call: %s with %d events", rows[0].RunID, pending.Status, pending.EventCount)

	// The error a caller sees names the run and says where to collect it, and
	// the run it names is still going.
	waitCtx, stopWait := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer stopWait()
	result, err := executeRequest(waitCtx, h.sessionRuntime(h.registerEnrollment()), protocol.AgentRequest{
		Tool:      "agent",
		Workspace: "work",
		Args:      map[string]any{"task": "second slow task"},
	}, nil)
	if err == nil {
		t.Fatalf("agent with an expired wait returned %v, want the run to be reported as still going", result)
	}
	if !strings.Contains(err.Error(), "still going") || !strings.Contains(err.Error(), "agent_result") {
		t.Fatalf("agent error = %q, want it to name agent_result as the way to collect the run", err)
	}
	abandoned := regexp.MustCompile(`run_[0-9a-f]{32}`).FindString(err.Error())
	if abandoned == "" {
		t.Fatalf("agent error = %q, want it to name the run", err)
	}
	if snap, known := h.runs.Get(abandoned); !known || snap.Status != agentops.RunRunning {
		t.Fatalf("run %s named by the error = %+v (known=%v), want running", abandoned, snap, known)
	}
	t.Logf("expired wait reported %q while run %s kept running", err, abandoned)

	// Both runs finish the work their callers stopped waiting for.
	exec.finish()
	for _, want := range []struct{ id, output string }{
		{rows[0].RunID, "slow task done"},
		{abandoned, "second slow task done"},
	} {
		finished := h.awaitTerminal(want.id)
		if finished.Status != string(agentops.RunCompleted) || finished.Output != want.output {
			t.Fatalf("run %s = %+v, want completed with %q", want.id, finished, want.output)
		}
	}

	endSession(t, cancel, done)
}

// C. agent_cancel is real: the harness context ends, not merely the label.
func TestAgentCancelStopsExecutorContext(t *testing.T) {
	exec := newChainExecutor("tick")
	h := newChainHarness(t, exec.exec)
	cancel, done, issued := h.connect(h.registerEnrollment())
	awaitCredential(t, issued)

	start := decode[agentStartReply](t, h.call(t, "agent_start", map[string]any{"task": "cancel me"}))
	awaitStatus(t, h.runs, start.RunID, agentops.RunRunning)
	awaitCondition(t, 5*time.Second, "the harness to start", func() bool { return isClosed(exec.started) })

	cancelled := decode[agentCancelReply](t, h.call(t, "agent_cancel", map[string]any{"run_id": start.RunID}))
	if !cancelled.Cancelled || cancelled.Status != string(agentops.RunCancelled) {
		t.Fatalf("agent_cancel = %+v, want the run reported as cancelled", cancelled)
	}
	awaitCondition(t, 5*time.Second, "the harness context to end", func() bool { return isClosed(exec.ctxEnded) })
	awaitCondition(t, 5*time.Second, "the harness to return", func() bool { return isClosed(exec.returned) })
	awaitStatus(t, h.runs, start.RunID, agentops.RunCancelled)

	// The cancelled run stays cancelled: the executor's own return value
	// cannot flip it back.
	time.Sleep(150 * time.Millisecond)
	snap, known := h.runs.Get(start.RunID)
	if !known || snap.Status != agentops.RunCancelled || snap.Error != "" {
		t.Fatalf("run after cancel = %+v (known=%v), want cancelled without error", snap, known)
	}
	t.Logf("run %s: harness context ended, harness returned, status %s", start.RunID, snap.Status)

	endSession(t, cancel, done)
}

// D. Runs never share events: two concurrent runs streaming their own tags
// keep their event rings, counts and last-event markers to themselves.
func TestConcurrentRunsKeepEventsSeparate(t *testing.T) {
	exec := newChainExecutor("") // every run streams its own task as its tag
	h := newChainHarness(t, exec.exec)
	cancel, done, issued := h.connect(h.registerEnrollment())
	awaitCredential(t, issued)

	type stream struct{ id, tag string }
	streams := []stream{
		{decode[agentStartReply](t, h.call(t, "agent_start", map[string]any{"task": "AAA"})).RunID, "AAA"},
		{decode[agentStartReply](t, h.call(t, "agent_start", map[string]any{"task": "BBB"})).RunID, "BBB"},
	}
	// Both runs must be streaming at the same time, or there is nothing to
	// interleave and the check would pass trivially.
	awaitCondition(t, 5*time.Second, "both runs to stream concurrently", func() bool {
		a, aKnown := h.runs.Get(streams[0].id)
		b, bKnown := h.runs.Get(streams[1].id)
		return aKnown && bKnown && a.Status == agentops.RunRunning && b.Status == agentops.RunRunning &&
			a.EventCount >= 3 && b.EventCount >= 3
	})
	exec.finish()
	for _, s := range streams {
		if finished := h.awaitTerminal(s.id); finished.Status != string(agentops.RunCompleted) || finished.Output != s.tag+" done" {
			t.Fatalf("run %s = %+v, want completed with %q", s.id, finished, s.tag+" done")
		}
	}

	for _, s := range streams {
		snap, known := h.runs.Get(s.id)
		if !known {
			t.Fatalf("run %s is unknown to the manager", s.id)
		}
		lines := h.runs.Events(s.id, 0)
		if snap.EventCount != len(lines) {
			t.Fatalf("run %s counted %d events but retained %d lines", s.id, snap.EventCount, len(lines))
		}
		for _, line := range lines {
			if line != s.tag {
				t.Fatalf("run %s recorded %q, want only its own %q", s.id, line, s.tag)
			}
		}
		other := "AAA"
		if s.tag == "AAA" {
			other = "BBB"
		}
		if strings.Contains(strings.Join(lines, "\n"), other) {
			t.Fatalf("run %s retained a line tagged %q: %v", s.id, other, lines)
		}
		if snap.LastEvent != s.tag {
			t.Fatalf("run %s last event = %q, want %q", s.id, snap.LastEvent, s.tag)
		}
		t.Logf("run %s kept %d of %d events, all tagged %s", s.id, len(lines), snap.EventCount, s.tag)
	}

	endSession(t, cancel, done)
}
