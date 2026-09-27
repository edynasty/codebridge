package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/protocol"
)

// testExecutor is a fake ExecutorFunc: it emits its event lines, then blocks
// until the test releases it or the run's context ends. The context arm keeps
// a manager close fast, which is what the cleanup of every test relies on.
type testExecutor struct {
	once    sync.Once
	release chan struct{}
	events  []string
}

func newTestExecutor(events ...string) *testExecutor {
	return &testExecutor{release: make(chan struct{}), events: events}
}

// finish lets the blocked run complete successfully.
func (t *testExecutor) finish() {
	t.once.Do(func() { close(t.release) })
}

func (t *testExecutor) exec(ctx context.Context, spec agentops.RunSpec, _ string, onEvent func(string)) (agentops.SubagentResult, error) {
	for _, line := range t.events {
		onEvent(line)
	}
	select {
	case <-t.release:
		return agentops.SubagentResult{Client: spec.Client, Status: "completed", Output: spec.Task + " done"}, nil
	case <-ctx.Done():
		return agentops.SubagentResult{Client: spec.Client, Status: "cancelled"}, ctx.Err()
	}
}

// newTestRunManager builds an in-memory manager around exec and closes it when
// the test ends.
func newTestRunManager(t *testing.T, exec agentops.ExecutorFunc) *agentops.RunManager {
	t.Helper()
	runs := agentops.NewRunManager(agentops.WithExecutor(exec))
	t.Cleanup(func() { _ = runs.Close() })
	return runs
}

// callTool runs one agent_* tool and returns its payload.
func callTool(t *testing.T, runs *agentops.RunManager, tool string, args map[string]any) any {
	t.Helper()
	result, handled, err := handleAgentRunTool(runs, protocol.AgentRequest{Tool: tool, Workspace: "work", Args: args})
	if !handled {
		t.Fatalf("%s was not handled by the agent run tools", tool)
	}
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	return result
}

// startRun starts one run through agent_start and returns its reply.
func startRun(t *testing.T, runs *agentops.RunManager, task string) agentStartReply {
	t.Helper()
	result := callTool(t, runs, "agent_start", map[string]any{"task": task})
	reply, ok := result.(agentStartReply)
	if !ok {
		t.Fatalf("agent_start returned %T", result)
	}
	return reply
}

// jsonKeys marshals v and returns its top-level JSON keys.
func jsonKeys(t *testing.T, v any) map[string]bool {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal %T: %v", v, err)
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		t.Fatalf("unmarshal %s: %v", raw, err)
	}
	keys := map[string]bool{}
	for key := range obj {
		keys[key] = true
	}
	return keys
}

// wantJSONKeys asserts the marshalled value carries exactly these keys.
func wantJSONKeys(t *testing.T, got map[string]bool, want ...string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("json keys = %v, want exactly %v", got, want)
	}
	for _, key := range want {
		if !got[key] {
			t.Fatalf("json keys = %v, want key %q", got, key)
		}
	}
}

// awaitStatus waits for a run to reach want, failing the test after 5 seconds.
func awaitStatus(t *testing.T, runs *agentops.RunManager, id string, want agentops.RunStatus) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		snap, known := runs.Get(id)
		if known && snap.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("run %s = %+v (known=%v), want %s", id, snap, known, want)
		}
		time.Sleep(2 * time.Millisecond)
	}
}

// agent_start answers with a run the caller can follow, and reports the
// harness the run will actually use rather than the empty parameter it got.
func TestAgentStartReturnsFollowableRun(t *testing.T) {
	runs := newTestRunManager(t, newTestExecutor().exec)

	result, handled, err := handleAgentRunTool(runs, protocol.AgentRequest{
		Tool:      "agent_start",
		Workspace: "work",
		Args:      map[string]any{"task": "summarize the repo", "agent": "task"},
	})
	if !handled {
		t.Fatal("agent_start was not handled")
	}
	if err != nil {
		t.Fatalf("agent_start: %v", err)
	}
	wantJSONKeys(t, jsonKeys(t, result),
		"run_id", "status", "client", "agent", "model", "thinking", "workspace", "started_at")

	reply, ok := result.(agentStartReply)
	if !ok {
		t.Fatalf("agent_start returned %T", result)
	}
	if !strings.HasPrefix(reply.RunID, "run_") || len(reply.RunID) != len("run_")+32 {
		t.Fatalf("run_id %q is not run_<hex>", reply.RunID)
	}
	if reply.Status != string(agentops.RunQueued) && reply.Status != string(agentops.RunRunning) {
		t.Fatalf("status = %q, want queued or running", reply.Status)
	}
	if reply.Client != "omp" {
		t.Fatalf("client = %q, want the default harness omp", reply.Client)
	}
	if reply.Agent != "task" || reply.Workspace != "work" {
		t.Fatalf("identity = %+v, want agent task in workspace work", reply)
	}
	if _, known := runs.Get(reply.RunID); !known {
		t.Fatalf("run %s is not known to the manager", reply.RunID)
	}
}

// The status, result and cancel answers keep the key set their MCP schemas
// advertise, whatever state the run is in.
func TestAgentRunToolReplies(t *testing.T) {
	runs := newTestRunManager(t, newTestExecutor().exec)
	reply := startRun(t, runs, "pending task")

	status := callTool(t, runs, "agent_status", map[string]any{"run_id": reply.RunID})
	wantJSONKeys(t, jsonKeys(t, status),
		"run_id", "status", "client", "agent", "model", "thinking", "workspace",
		"started_at", "elapsed_ms", "event_count", "last_event")
	if got := status.(agentStatusReply); got.RunID != reply.RunID || got.Status != reply.Status {
		t.Fatalf("agent_status = %+v, want the run agent_start returned", got)
	}

	// A run that has not finished has no result to report.
	pending := callTool(t, runs, "agent_result", map[string]any{"run_id": reply.RunID})
	wantJSONKeys(t, jsonKeys(t, pending), "run_id", "status")

	cancelled := callTool(t, runs, "agent_cancel", map[string]any{"run_id": reply.RunID})
	wantJSONKeys(t, jsonKeys(t, cancelled), "run_id", "status", "cancelled")
	cancelReply := cancelled.(agentCancelReply)
	if !cancelReply.Cancelled || cancelReply.Status != string(agentops.RunCancelled) {
		t.Fatalf("agent_cancel = %+v, want cancelled", cancelReply)
	}
	awaitStatus(t, runs, reply.RunID, agentops.RunCancelled)

	// Cancelling a finished run reports the terminal state it already has.
	again := callTool(t, runs, "agent_cancel", map[string]any{"run_id": reply.RunID})
	if repeated := again.(agentCancelReply); repeated.Cancelled || repeated.Status != string(agentops.RunCancelled) {
		t.Fatalf("second agent_cancel = %+v, want a no-op on a finished run", repeated)
	}

	finished := callTool(t, runs, "agent_result", map[string]any{"run_id": reply.RunID})
	wantJSONKeys(t, jsonKeys(t, finished), "run_id", "status", "output", "error", "elapsed_ms", "finished_at")
	if terminal := finished.(agentResultTerminal); terminal.Status != string(agentops.RunCancelled) {
		t.Fatalf("agent_result = %+v, want the cancelled run", terminal)
	}
}

// A completed run reports its output, and agent_runs lists it.
func TestAgentResultAndRunsListAfterCompletion(t *testing.T) {
	exec := newTestExecutor("event-1")
	runs := newTestRunManager(t, exec.exec)
	reply := startRun(t, runs, "finish me")
	exec.finish()
	awaitStatus(t, runs, reply.RunID, agentops.RunCompleted)

	finished := callTool(t, runs, "agent_result", map[string]any{"run_id": reply.RunID})
	wantJSONKeys(t, jsonKeys(t, finished), "run_id", "status", "output", "error", "elapsed_ms", "finished_at")
	terminal := finished.(agentResultTerminal)
	if terminal.Status != string(agentops.RunCompleted) || terminal.Output != "finish me done" {
		t.Fatalf("agent_result = %+v, want the completed run's output", terminal)
	}
	if terminal.FinishedAt == "" {
		t.Fatalf("agent_result = %+v, want a finish timestamp", terminal)
	}

	list := callTool(t, runs, "agent_runs", nil).(agentRunsReply)
	if list.Count != 1 || len(list.Runs) != 1 {
		t.Fatalf("agent_runs = count %d, runs %d; want the one run", list.Count, len(list.Runs))
	}
	wantJSONKeys(t, jsonKeys(t, list.Runs[0]),
		"run_id", "status", "workspace", "client", "agent", "started_at", "elapsed_ms", "event_count")
	if list.Runs[0].RunID != reply.RunID || list.Runs[0].Status != string(agentops.RunCompleted) {
		t.Fatalf("agent_runs row = %+v, want the completed run", list.Runs[0])
	}
	if list.Runs[0].EventCount != 1 {
		t.Fatalf("event_count = %d, want the 1 streamed line", list.Runs[0].EventCount)
	}
}

// agent_runs defaults its limit and clamps it, newest first.
func TestAgentRunsLimits(t *testing.T) {
	runs := newTestRunManager(t, newTestExecutor().exec)
	for i := 0; i < agentRunsMaxLimit+1; i++ {
		startRun(t, runs, fmt.Sprintf("task-%d", i))
	}

	cases := []struct {
		name string
		args map[string]any
		want int
	}{
		{"omitted limit uses the default", nil, agentRunsDefaultLimit},
		{"explicit limit is honoured", map[string]any{"limit": float64(5)}, 5},
		{"the maximum passes through", map[string]any{"limit": float64(agentRunsMaxLimit)}, agentRunsMaxLimit},
		{"a larger limit is clamped", map[string]any{"limit": float64(1000)}, agentRunsMaxLimit},
		{"a negative limit uses the default", map[string]any{"limit": float64(-3)}, agentRunsDefaultLimit},
	}
	for _, tc := range cases {
		got := callTool(t, runs, "agent_runs", tc.args).(agentRunsReply)
		if got.Count != tc.want || len(got.Runs) != tc.want {
			t.Fatalf("%s: agent_runs(%v) = count %d, runs %d; want %d", tc.name, tc.args, got.Count, len(got.Runs), tc.want)
		}
	}
}

// An unknown or absent run id is an error, not an empty answer.
func TestAgentRunToolsRejectUnknownRuns(t *testing.T) {
	runs := newTestRunManager(t, newTestExecutor().exec)
	for _, tool := range []string{"agent_status", "agent_result", "agent_cancel"} {
		_, handled, err := handleAgentRunTool(runs, protocol.AgentRequest{Tool: tool, Args: map[string]any{"run_id": "run_missing"}})
		if !handled {
			t.Fatalf("%s was not handled", tool)
		}
		if err == nil || !strings.Contains(err.Error(), `run "run_missing" not found`) {
			t.Fatalf("%s error = %v, want run not found", tool, err)
		}
	}
	_, _, err := handleAgentRunTool(runs, protocol.AgentRequest{Tool: "agent_status"})
	if err == nil || !strings.Contains(err.Error(), "run_id is required") {
		t.Fatalf("missing run_id error = %v", err)
	}
}

// Tools outside the family pass through, and the family refuses to run without
// a manager instead of silently dropping the request.
func TestHandleAgentRunToolLeavesOtherToolsAlone(t *testing.T) {
	runs := newTestRunManager(t, newTestExecutor().exec)
	if result, handled, err := handleAgentRunTool(runs, protocol.AgentRequest{Tool: "read"}); handled || err != nil || result != nil {
		t.Fatalf("handleAgentRunTool(read) = %v, %v, %v; want a pass-through", result, handled, err)
	}
	_, handled, err := handleAgentRunTool(nil, protocol.AgentRequest{Tool: "agent_start", Args: map[string]any{"task": "t"}})
	if !handled || err == nil {
		t.Fatalf("agent_start without a run manager = handled %v, err %v; want a handled failure", handled, err)
	}
}

// The legacy agent tool keeps its old answer shape while running through the
// run manager, and forwards the run's event lines as progress.
func TestRunAgentToolMapsResultAndStreamsProgress(t *testing.T) {
	exec := newTestExecutor("line-1", "line-2")
	runs := newTestRunManager(t, exec.exec)
	go exec.finish()

	var progress []string
	result, err := runAgentTool(context.Background(), runs, protocol.AgentRequest{
		Tool:      "agent",
		Workspace: "work",
		Args:      map[string]any{"task": "do it"},
	}, func(line string) { progress = append(progress, line) })
	if err != nil {
		t.Fatalf("runAgentTool: %v", err)
	}
	res, ok := result.(*agentops.SubagentResult)
	if !ok {
		t.Fatalf("runAgentTool returned %T, want the legacy SubagentResult", result)
	}
	if res.Status != "completed" || res.Client != "omp" || res.Output != "do it done" {
		t.Fatalf("result = %+v", res)
	}
	if res.Task != "do it" || res.Events != 2 || res.Elapsed == "" {
		t.Fatalf("result identity = %+v", res)
	}
	if len(progress) != 2 || progress[0] != "line-1" || progress[1] != "line-2" {
		t.Fatalf("progress = %v, want both event lines", progress)
	}
}

// A wait that ends without the run finishing is an error, and the run is
// untouched: that is what keeps a lost gRPC session from killing local work.
func TestWaitRunLeavesRunRunningWhenWaitExpires(t *testing.T) {
	runs := newTestRunManager(t, newTestExecutor().exec)
	reply := startRun(t, runs, "long task")
	awaitStatus(t, runs, reply.RunID, agentops.RunRunning)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if _, err := waitRun(ctx, runs, reply.RunID, nil); err == nil {
		t.Fatal("waitRun returned no error after the wait expired")
	}
	snap, known := runs.Get(reply.RunID)
	if !known || snap.Status != agentops.RunRunning {
		t.Fatalf("run after the wait expired = %+v (known=%v), want running", snap, known)
	}
}

func TestNextBackoffResetsAfterStableSession(t *testing.T) {
	cases := []struct {
		name    string
		current time.Duration
		session time.Duration
		want    time.Duration
	}{
		{"a stable session resets the ladder", 15 * time.Second, stableConnectionThreshold, time.Second},
		{"a long session resets the ladder", time.Second, 2 * time.Hour, time.Second},
		{"a short session doubles", time.Second, 30 * time.Second, 2 * time.Second},
		{"doubling continues", 4 * time.Second, 5 * time.Second, 8 * time.Second},
		{"the delay is capped", 8 * time.Second, time.Second, 15 * time.Second},
		{"the cap holds", 15 * time.Second, time.Second, 15 * time.Second},
	}
	for _, tc := range cases {
		if got := nextBackoff(tc.current, tc.session); got != tc.want {
			t.Fatalf("%s: nextBackoff(%s, %s) = %s, want %s", tc.name, tc.current, tc.session, got, tc.want)
		}
	}
}
