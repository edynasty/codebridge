package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/edynasty/codebridge/internal/agentops"
	"github.com/edynasty/codebridge/internal/protocol"
)

// Agent run tools. Unlike the other device tools they are served by the
// process-level RunManager, which owns runs independently of the gRPC session
// that started them: agent_start returns immediately, the run keeps going, and
// agent_status / agent_result / agent_cancel / agent_runs follow it up.
const (
	agentRunsDefaultLimit = 20
	agentRunsMaxLimit     = 100

	// runProgressInterval is how often a waiting caller polls a run for new
	// event lines. Runs already keep a ring buffer of recent lines; polling it
	// keeps progress best effort without wiring a per-request subscription
	// into the run manager.
	runProgressInterval = 250 * time.Millisecond

	// cancelSettleTimeout bounds how long agent_cancel waits for the
	// cancellation to land before answering with the state it already has.
	cancelSettleTimeout = 5 * time.Second
)

// agentRunToolNames is the family this file serves.
var agentRunToolNames = map[string]bool{
	"agent_start":  true,
	"agent_status": true,
	"agent_result": true,
	"agent_cancel": true,
	"agent_runs":   true,
}

// executeRequest routes one device request. The legacy agent tool drives a
// managed run with live progress; the agent_* tools are projections of that
// same manager; everything else goes through the service. onProgress may be
// nil and its failures are ignored by the caller, because progress is never
// allowed to fail a run.
func executeRequest(ctx context.Context, rt *runtime, req protocol.AgentRequest, onProgress func(string)) (any, error) {
	if req.Tool == "agent" && rt.runs != nil {
		return runAgentTool(ctx, rt.runs, req, onProgress)
	}
	if result, handled, err := handleAgentRunTool(rt.runs, req); handled {
		return result, err
	}
	return rt.service.Execute(ctx, req)
}

// handleAgentRunTool serves agent_start, agent_status, agent_result,
// agent_cancel and agent_runs. handled reports whether the tool belongs to
// this family; a false result means the caller falls through to the regular
// service dispatch.
func handleAgentRunTool(runs *agentops.RunManager, req protocol.AgentRequest) (any, bool, error) {
	if !agentRunToolNames[req.Tool] {
		return nil, false, nil
	}
	if runs == nil {
		return nil, true, errors.New("agent runs are unavailable: this client has no run manager")
	}
	switch req.Tool {
	case "agent_start":
		result, err := startAgentRun(runs, req)
		return result, true, err
	case "agent_status":
		result, err := agentStatus(runs, req)
		return result, true, err
	case "agent_result":
		result, err := agentResult(runs, req)
		return result, true, err
	case "agent_cancel":
		result, err := agentCancel(runs, req)
		return result, true, err
	case "agent_runs":
		return agentRuns(runs, req), true, nil
	}
	return nil, true, fmt.Errorf("unsupported tool %q", req.Tool)
}

// startAgentRun accepts a run and answers as soon as the manager has it.
// Admission is synchronous, so an invalid, unknown-workspace or denied call
// fails here — a denied harness answers with the permission request id it
// parked, exactly like the legacy agent tool. The run itself outlives this
// call, which is why the admission context is not the caller's.
func startAgentRun(runs *agentops.RunManager, req protocol.AgentRequest) (agentStartReply, error) {
	run, err := runs.Start(context.Background(), agentRunSpec(req))
	if err != nil {
		return agentStartReply{}, err
	}
	return agentStartReply{
		RunID:     run.ID,
		Status:    string(run.Status),
		Client:    run.Client,
		Agent:     run.Agent,
		Model:     run.Model,
		Thinking:  run.Thinking,
		Workspace: run.Workspace,
		StartedAt: formatRunTime(run.StartedAt),
	}, nil
}

func agentStatus(runs *agentops.RunManager, req protocol.AgentRequest) (agentStatusReply, error) {
	snap, err := lookupRun(runs, req)
	if err != nil {
		return agentStatusReply{}, err
	}
	return agentStatusReply{
		RunID:      snap.ID,
		Status:     string(snap.Status),
		Client:     snap.Client,
		Agent:      snap.Agent,
		Model:      snap.Model,
		Thinking:   snap.Thinking,
		Workspace:  snap.Workspace,
		StartedAt:  formatRunTime(snap.StartedAt),
		ElapsedMS:  runElapsedMS(snap),
		EventCount: snap.EventCount,
		LastEvent:  snap.LastEvent,
	}, nil
}

func agentResult(runs *agentops.RunManager, req protocol.AgentRequest) (any, error) {
	snap, err := lookupRun(runs, req)
	if err != nil {
		return nil, err
	}
	if !snap.Status.Terminal() {
		return agentResultPending{RunID: snap.ID, Status: string(snap.Status)}, nil
	}
	return agentResultTerminal{
		RunID:      snap.ID,
		Status:     string(snap.Status),
		Output:     snap.Output,
		Error:      snap.Error,
		ElapsedMS:  snap.ElapsedMS,
		FinishedAt: formatRunTime(snap.FinishedAt),
	}, nil
}

func agentCancel(runs *agentops.RunManager, req protocol.AgentRequest) (agentCancelReply, error) {
	snap, err := lookupRun(runs, req)
	if err != nil {
		return agentCancelReply{}, err
	}
	if snap.Status.Terminal() {
		return agentCancelReply{RunID: snap.ID, Status: string(snap.Status), Cancelled: false}, nil
	}
	if err := runs.Cancel(snap.ID); err != nil {
		return agentCancelReply{}, err
	}
	// Wait for the transition to land so the answer is the run's real final
	// status, not the state the cancel raced against. A cancellation always
	// lands on cancelled, so a run that outlasts the settle window is still
	// reported as cancelled: that is where it is headed.
	ctx, cancel := context.WithTimeout(context.Background(), cancelSettleTimeout)
	defer cancel()
	final, err := runs.Wait(ctx, snap.ID)
	if err != nil || final == nil {
		return agentCancelReply{RunID: snap.ID, Status: string(agentops.RunCancelled), Cancelled: true}, nil
	}
	return agentCancelReply{RunID: final.ID, Status: string(final.Status), Cancelled: final.Status == agentops.RunCancelled}, nil
}

// agentRuns lists the runs this device knows, newest first. History survives a
// client restart through the run store, so the list is the full picture.
func agentRuns(runs *agentops.RunManager, req protocol.AgentRequest) agentRunsReply {
	limit := intArgAny(req.Args, "limit", 0)
	if limit <= 0 {
		limit = agentRunsDefaultLimit
	}
	if limit > agentRunsMaxLimit {
		limit = agentRunsMaxLimit
	}
	known := runs.List(limit)
	rows := make([]agentRunRow, 0, len(known))
	for _, snap := range known {
		rows = append(rows, agentRunRow{
			RunID:      snap.ID,
			Status:     string(snap.Status),
			Workspace:  snap.Workspace,
			Client:     snap.Client,
			Agent:      snap.Agent,
			StartedAt:  formatRunTime(snap.StartedAt),
			ElapsedMS:  runElapsedMS(snap),
			EventCount: snap.EventCount,
		})
	}
	return agentRunsReply{Runs: rows, Count: len(rows)}
}

// runAgentTool runs the legacy agent tool through the process-level run
// manager: start, then wait on ctx while forwarding the run's event lines as
// progress. ctx bounds the wait only, so a lost gRPC session ends this call
// while the run keeps going for agent_result to collect.
func runAgentTool(ctx context.Context, runs *agentops.RunManager, req protocol.AgentRequest, onEvent func(string)) (any, error) {
	run, err := runs.Start(ctx, agentRunSpec(req))
	if err != nil {
		return nil, err
	}
	snap, err := waitRun(ctx, runs, run.ID, onEvent)
	if err != nil {
		return nil, fmt.Errorf("agent run %s is still going; fetch it with agent_result: %w", run.ID, err)
	}
	return agentops.SubagentResultFromRun(snap), nil
}

// waitRun blocks until a run reaches a terminal status, forwarding the event
// lines it records on the way. The wait runs in its own goroutine so the
// caller can poll for progress; when ctx ends the wait returns its error and
// the run keeps going.
func waitRun(ctx context.Context, runs *agentops.RunManager, id string, onEvent func(string)) (*agentops.AgentRun, error) {
	type waitResult struct {
		snap *agentops.AgentRun
		err  error
	}
	waited := make(chan waitResult, 1)
	go func() {
		snap, err := runs.Wait(ctx, id)
		waited <- waitResult{snap: snap, err: err}
	}()

	sent := 0
	flush := func() { sent += forwardRunEvents(runs, id, sent, onEvent) }
	ticker := time.NewTicker(runProgressInterval)
	defer ticker.Stop()
	for {
		select {
		case res := <-waited:
			flush()
			return res.snap, res.err
		case <-ticker.C:
			flush()
		}
	}
}

// forwardRunEvents hands the event lines recorded after the first sent ones to
// onEvent and reports how many are now accounted for. A run keeps at most its
// event capacity lines, so a poll that fell too far behind skips whatever the
// ring already dropped: progress is best effort, the result is not.
func forwardRunEvents(runs *agentops.RunManager, id string, sent int, onEvent func(string)) int {
	if onEvent == nil {
		return 0
	}
	snap, ok := runs.Get(id)
	if !ok || snap.EventCount <= sent {
		return 0
	}
	pending := snap.EventCount - sent
	for _, line := range runs.Events(id, pending) {
		onEvent(line)
	}
	return pending
}

// lookupRun reads the run a request names. The not-found wording matches the
// run manager's own so a caller cannot tell the layers apart.
func lookupRun(runs *agentops.RunManager, req protocol.AgentRequest) (*agentops.AgentRun, error) {
	id := strings.TrimSpace(stringArg(req.Args, "run_id", ""))
	if id == "" {
		return nil, errors.New("run_id is required")
	}
	snap, ok := runs.Get(id)
	if !ok {
		return nil, fmt.Errorf("run %q not found", id)
	}
	return snap, nil
}

// agentRunSpec projects the arguments shared by agent_start and the legacy
// agent tool onto a RunSpec. Workspace stays the workspace name; the client's
// executor resolves it against the live roots when the run executes.
func agentRunSpec(req protocol.AgentRequest) agentops.RunSpec {
	return agentops.RunSpec{
		Workspace:      req.Workspace,
		Task:           stringArg(req.Args, "task", ""),
		Client:         stringArg(req.Args, "client", ""),
		Agent:          stringArg(req.Args, "agent", ""),
		Model:          stringArg(req.Args, "model", ""),
		Thinking:       stringArg(req.Args, "thinking", ""),
		TimeoutSeconds: intArgAny(req.Args, "timeout_seconds", 0),
	}
}

// runElapsedMS reports how long a run has been going: the final duration once
// it finished, the live elapsed time while it runs, and nothing before it
// started.
func runElapsedMS(snap *agentops.AgentRun) int64 {
	if !snap.FinishedAt.IsZero() {
		return snap.ElapsedMS
	}
	if snap.StartedAt.IsZero() {
		return 0
	}
	return time.Since(snap.StartedAt).Milliseconds()
}

// formatRunTime renders a run timestamp for JSON; a run that has not started
// yet reports an empty string instead of the zero time.
func formatRunTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

// stringArg reads a string argument from a device request.
func stringArg(args map[string]any, key, def string) string {
	if v, ok := args[key].(string); ok {
		return v
	}
	return def
}

// agentStartReply is the answer to agent_start: enough to follow the run.
type agentStartReply struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Client    string `json:"client"`
	Agent     string `json:"agent"`
	Model     string `json:"model"`
	Thinking  string `json:"thinking"`
	Workspace string `json:"workspace"`
	StartedAt string `json:"started_at"`
}

// agentStatusReply is the answer to agent_status.
type agentStatusReply struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Client     string `json:"client"`
	Agent      string `json:"agent"`
	Model      string `json:"model"`
	Thinking   string `json:"thinking"`
	Workspace  string `json:"workspace"`
	StartedAt  string `json:"started_at"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	EventCount int    `json:"event_count"`
	LastEvent  string `json:"last_event"`
}

// agentRunRow is one entry of agent_runs: the subset of a run a list needs.
type agentRunRow struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Workspace  string `json:"workspace"`
	Client     string `json:"client"`
	Agent      string `json:"agent"`
	StartedAt  string `json:"started_at"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	EventCount int    `json:"event_count"`
}

type agentRunsReply struct {
	Runs  []agentRunRow `json:"runs"`
	Count int           `json:"count"`
}

// agentResultPending answers agent_result for a run that has not finished:
// only identity and state, because there is no result to report yet.
type agentResultPending struct {
	RunID  string `json:"run_id"`
	Status string `json:"status"`
}

// agentResultTerminal answers agent_result for a finished run.
type agentResultTerminal struct {
	RunID      string `json:"run_id"`
	Status     string `json:"status"`
	Output     string `json:"output"`
	Error      string `json:"error"`
	ElapsedMS  int64  `json:"elapsed_ms"`
	FinishedAt string `json:"finished_at"`
}

type agentCancelReply struct {
	RunID     string `json:"run_id"`
	Status    string `json:"status"`
	Cancelled bool   `json:"cancelled"`
}
