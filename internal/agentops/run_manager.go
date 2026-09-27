package agentops

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"sort"
	"strings"
	"sync"
	"time"
)

// RunStatus is the lifecycle state of one managed agent run. Terminal
// statuses (completed, failed, cancelled, timeout, interrupted) never change
// again: every transition goes through RunManager, which serializes them
// under its own lock, so a run follows exactly one path.
type RunStatus string

const (
	// RunQueued: accepted, waiting for a concurrency slot.
	RunQueued RunStatus = "queued"
	// RunRunning: a slot is held and the harness is executing.
	RunRunning RunStatus = "running"
	// RunCompleted: the harness finished successfully.
	RunCompleted RunStatus = "completed"
	// RunFailed: the harness, or the executor itself, reported an error.
	RunFailed RunStatus = "failed"
	// RunCancelled: cancelled by the caller while queued or running.
	RunCancelled RunStatus = "cancelled"
	// RunTimeout: the run exceeded its wall-clock budget.
	RunTimeout RunStatus = "timeout"
	// RunInterrupted: a previous process died while the run was queued or
	// running; discovered when the run store is reopened.
	RunInterrupted RunStatus = "interrupted"
)

// runTransitions is the complete legal state machine. A status absent from
// the map is terminal.
var runTransitions = map[RunStatus][]RunStatus{
	RunQueued:  {RunRunning, RunCancelled},
	RunRunning: {RunCompleted, RunFailed, RunCancelled, RunTimeout, RunInterrupted},
}

// terminalRunStatus reports whether a status can never change again.
func terminalRunStatus(s RunStatus) bool {
	switch s {
	case RunCompleted, RunFailed, RunCancelled, RunTimeout, RunInterrupted:
		return true
	}
	return false
}

// Terminal reports whether a run can never change status again. Callers that
// project a run (agent_status, agent_result) use it to tell a live run from a
// finished one without duplicating the terminal set.
func (s RunStatus) Terminal() bool {
	return terminalRunStatus(s)
}

// canTransition reports whether a run may move from one status to another.
// Terminal statuses have no successors, so a finished run can never fall
// back into an earlier state.
func canTransition(from, to RunStatus) bool {
	for _, next := range runTransitions[from] {
		if next == to {
			return true
		}
	}
	return false
}

// RunSpec describes one subagent run. It is everything RunManager needs: the
// run does not depend on the context of the request that started it.
type RunSpec struct {
	// Workspace is the workspace root handed to the harness. It is also what
	// the executor receives as its root argument, so a caller that only has a
	// workspace name resolves it against Service.Roots first.
	Workspace      string
	Task           string
	Client         string // omp | opencode | codex
	Agent          string
	Model          string
	Thinking       string
	TimeoutSeconds int
}

// ExecutorFunc performs the work of one run. Tests inject fakes; production
// uses DefaultExecutor. onEvent receives every raw event line of the run and
// is never nil, though an executor may ignore it.
type ExecutorFunc func(ctx context.Context, spec RunSpec, root string, onEvent func(eventLine string)) (SubagentResult, error)

// AgentRun is an immutable snapshot of one run, as serialized by the
// agent_start / agent_status / agent_result / agent_runs tools.
type AgentRun struct {
	ID         string    `json:"id"`
	Workspace  string    `json:"workspace,omitempty"`
	Client     string    `json:"client,omitempty"`
	Agent      string    `json:"agent,omitempty"`
	Model      string    `json:"model,omitempty"`
	Thinking   string    `json:"thinking,omitempty"`
	Task       string    `json:"task"`
	Status     RunStatus `json:"status"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt time.Time `json:"finished_at"`
	ElapsedMS  int64     `json:"elapsed_ms"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	EventCount int       `json:"event_count"`
	LastEvent  string    `json:"last_event,omitempty"`
}

const (
	// defaultRunConcurrency is the number of subagent runs a client may
	// execute at once unless configured otherwise.
	defaultRunConcurrency = subagentMaxConcurrent
	// defaultRunEventCapacity is how many recent event lines a run keeps.
	defaultRunEventCapacity = 200
)

// RunManagerOption configures NewRunManager.
type RunManagerOption func(*RunManager)

// WithMaxConcurrent caps how many runs execute at once (default 2).
func WithMaxConcurrent(n int) RunManagerOption {
	return func(m *RunManager) {
		if n > 0 {
			m.maxConcurrent = n
		}
	}
}

// WithEventCapacity sets how many recent event lines each run retains for
// progress reporting (default 200).
func WithEventCapacity(n int) RunManagerOption {
	return func(m *RunManager) {
		if n > 0 {
			m.eventCapacity = n
		}
	}
}

// WithExecutor installs the executor that performs a run. Without it every
// run fails, because performing real work requires a Service (see
// DefaultExecutor) that only the client can assemble.
func WithExecutor(exec ExecutorFunc) RunManagerOption {
	return func(m *RunManager) {
		if exec != nil {
			m.exec = exec
		}
	}
}

// WithRootContext sets the context runs derive from. It must be the client
// process root context, never a per-request one: a run outlives the gRPC
// call that started it. Defaults to context.Background().
func WithRootContext(ctx context.Context) RunManagerOption {
	return func(m *RunManager) {
		if ctx != nil {
			m.rootCtx = ctx
		}
	}
}

// WithPreflight installs a synchronous admission check. A non-nil error
// rejects the run before it is queued, so validation and permission answers
// reach the caller immediately; a *PermissionNeededError passes through
// unchanged, letting the tool report the request id it parked.
func WithPreflight(fn func(RunSpec) error) RunManagerOption {
	return func(m *RunManager) {
		m.preflight = fn
	}
}

// WithStore persists runs to a SQLite database at path so history survives a
// client restart. An empty path keeps the manager in memory only.
func WithStore(path string) RunManagerOption {
	return func(m *RunManager) {
		m.storePath = path
	}
}

// RunManager owns the lifecycle of local agent runs independently of the
// caller that started them. It caps concurrency, records every status change
// (in memory and, when configured, in SQLite), and keeps the most recent
// event lines of each run.
type RunManager struct {
	rootCtx       context.Context
	rootStop      context.CancelFunc
	exec          ExecutorFunc
	preflight     func(RunSpec) error
	maxConcurrent int
	eventCapacity int
	storePath     string
	sem           chan struct{}
	store         *runStore

	mu     sync.RWMutex
	runs   map[string]*runState
	closed bool
	wg     sync.WaitGroup
}

// runState is the mutable run record; every field is guarded by the owning
// RunManager mutex, except the context and done channel, which never change
// after creation.
type runState struct {
	id   string
	spec RunSpec

	status     RunStatus
	startedAt  time.Time
	finishedAt time.Time
	elapsedMS  int64
	output     string
	errText    string

	eventTotal int
	lastEvent  string
	events     []string // ring buffer, len == manager event capacity
	eventNext  int      // next ring slot to write
	eventKept  int      // valid entries in the ring

	lifeCtx  context.Context
	lifeStop context.CancelFunc
	done     chan struct{}
}

// NewRunManager builds a run manager. Options configure concurrency, event
// retention, the executor, the root context and persistence.
func NewRunManager(opts ...RunManagerOption) *RunManager {
	m := &RunManager{
		rootCtx: context.Background(),
		exec: func(context.Context, RunSpec, string, func(string)) (SubagentResult, error) {
			return SubagentResult{}, errors.New("this run manager has no executor configured")
		},
		maxConcurrent: defaultRunConcurrency,
		eventCapacity: defaultRunEventCapacity,
		runs:          map[string]*runState{},
	}
	for _, opt := range opts {
		if opt != nil {
			opt(m)
		}
	}
	m.sem = make(chan struct{}, m.maxConcurrent)
	m.rootCtx, m.rootStop = context.WithCancel(m.rootCtx)

	store, err := openRunStore(m.storePath)
	if err != nil {
		log.Printf("agent runs: persistence disabled: %v", err)
	} else {
		m.store = store
	}
	if m.store != nil {
		history, err := m.store.all()
		if err != nil {
			log.Printf("agent runs: load history: %v", err)
		}
		for _, snap := range history {
			m.runs[snap.ID] = loadedRunState(snap, m.eventCapacity)
		}
	}
	return m
}

// Start accepts a run and returns as soon as it is registered: the returned
// snapshot is usually queued, and callers follow the run through Get, List or
// Wait. Validation and preflight errors (including *PermissionNeededError)
// come back immediately and nothing is registered.
//
// ctx gates admission only; the run's lifetime derives from the manager root
// context, so a cancelled caller never kills a long local run.
func (m *RunManager) Start(ctx context.Context, spec RunSpec) (*AgentRun, error) {
	if ctx != nil {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
	}
	spec = normalizeRunSpec(spec)
	if err := validateRunSpec(spec); err != nil {
		return nil, err
	}
	if m.preflight != nil {
		if err := m.preflight(spec); err != nil {
			return nil, err
		}
	}

	id := newRunID()
	lifeCtx, lifeStop := context.WithCancel(m.rootCtx)
	rs := &runState{
		id:       id,
		spec:     spec,
		status:   RunQueued,
		events:   make([]string, m.eventCapacity),
		lifeCtx:  lifeCtx,
		lifeStop: lifeStop,
		done:     make(chan struct{}),
	}

	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		lifeStop()
		return nil, errors.New("run manager is closed")
	}
	m.runs[id] = rs
	snap := m.snapshotLocked(rs)
	m.persistLocked(rs)
	m.wg.Add(1)
	m.mu.Unlock()

	go func() {
		defer m.wg.Done()
		m.run(rs)
	}()
	return snap, nil
}

// Get returns the run with the given id, falling back to the store so runs
// from previous processes stay visible.
func (m *RunManager) Get(id string) (*AgentRun, bool) {
	m.mu.RLock()
	rs, ok := m.runs[id]
	if ok {
		snap := m.snapshotLocked(rs)
		m.mu.RUnlock()
		return snap, true
	}
	m.mu.RUnlock()

	stored, err := m.store.get(id)
	if err != nil {
		log.Printf("agent runs: load run %s: %v", id, err)
		return nil, false
	}
	if stored == nil {
		return nil, false
	}
	m.cacheLoaded(stored)
	return stored, true
}

// List returns every known run, newest first, capped at limit (limit <= 0
// means all). Persisted history is loaded at startup, so the in-memory map is
// the full picture.
func (m *RunManager) List(limit int) []*AgentRun {
	m.mu.RLock()
	out := make([]*AgentRun, 0, len(m.runs))
	for _, rs := range m.runs {
		out = append(out, m.snapshotLocked(rs))
	}
	m.mu.RUnlock()

	sort.Slice(out, func(i, j int) bool {
		if out[i].StartedAt.Equal(out[j].StartedAt) {
			return out[i].ID > out[j].ID
		}
		return out[i].StartedAt.After(out[j].StartedAt)
	})
	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}
	return out
}

// Events returns up to limit of the most recent event lines of a run, oldest
// first. limit <= 0 means the whole retained ring.
func (m *RunManager) Events(id string, limit int) []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rs, ok := m.runs[id]
	if !ok {
		return nil
	}
	n := rs.eventKept
	if limit > 0 && limit < n {
		n = limit
	}
	out := make([]string, 0, n)
	for i := rs.eventTotal - n; i < rs.eventTotal; i++ {
		out = append(out, rs.events[i%len(rs.events)])
	}
	return out
}

// Cancel stops a run: a queued run is cancelled before it starts, a running
// one sees its context cancelled. Cancelling a finished run is a no-op.
func (m *RunManager) Cancel(id string) error {
	m.mu.RLock()
	rs, ok := m.runs[id]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("run %q not found", id)
	}
	rs.lifeStop()
	return nil
}

// Wait blocks until the run reaches a terminal status and returns its final
// snapshot. ctx bounds the wait only; the run keeps going when ctx expires.
func (m *RunManager) Wait(ctx context.Context, id string) (*AgentRun, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	m.mu.RLock()
	rs, ok := m.runs[id]
	m.mu.RUnlock()
	if !ok {
		stored, err := m.store.get(id)
		if err != nil {
			return nil, err
		}
		if stored == nil {
			return nil, fmt.Errorf("run %q not found", id)
		}
		return stored, nil
	}
	select {
	case <-rs.done:
	case <-ctx.Done():
		return nil, ctx.Err()
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.snapshotLocked(rs), nil
}

// Close cancels every outstanding run, waits for their goroutines and closes
// the store. It is safe to call more than once.
func (m *RunManager) Close() error {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return nil
	}
	m.closed = true
	m.mu.Unlock()

	m.rootStop()
	m.wg.Wait()
	return m.store.Close()
}

// run executes one accepted run: it waits for a slot (cancellable), reports
// the run as running, calls the executor and lands on a terminal status. The
// permit is released on every path, including panics.
func (m *RunManager) run(rs *runState) {
	select {
	case m.sem <- struct{}{}:
	case <-rs.lifeCtx.Done():
		m.finish(rs, RunCancelled, SubagentResult{}, nil)
		return
	}
	defer func() { <-m.sem }()

	if rs.lifeCtx.Err() != nil {
		m.finish(rs, RunCancelled, SubagentResult{}, nil)
		return
	}

	m.mu.Lock()
	started := m.transitionLocked(rs, RunRunning)
	m.mu.Unlock()
	if !started {
		return
	}

	execCtx, cancel := runExecContext(rs.lifeCtx, rs.spec.TimeoutSeconds)
	defer cancel()

	res, execErr := m.execute(execCtx, rs)
	m.finishOutcome(rs, execCtx, res, execErr)
}

// execute calls the executor, converting a panic into an ordinary failure so
// the run still reaches a terminal status and releases its slot.
func (m *RunManager) execute(ctx context.Context, rs *runState) (res SubagentResult, err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("subagent executor panicked: %v", r)
		}
	}()
	return m.exec(ctx, rs.spec, rs.spec.Workspace, func(line string) {
		m.recordEvent(rs, line)
	})
}

// finishOutcome decides the terminal status of a run that left the executor.
// A cancelled or expired run context wins over what the executor reported.
func (m *RunManager) finishOutcome(rs *runState, execCtx context.Context, res SubagentResult, execErr error) {
	switch {
	case errors.Is(execCtx.Err(), context.DeadlineExceeded):
		m.finish(rs, RunTimeout, res, nil)
	case errors.Is(execCtx.Err(), context.Canceled):
		m.finish(rs, RunCancelled, res, nil)
	case execErr != nil:
		m.finish(rs, RunFailed, res, execErr)
	default:
		status, err := statusFromResult(res.Status)
		m.finish(rs, status, res, err)
	}
}

// statusFromResult maps a harness status onto the run state machine.
func statusFromResult(s string) (RunStatus, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "", "completed":
		return RunCompleted, nil
	case "failed":
		return RunFailed, nil
	case "timeout":
		return RunTimeout, nil
	case "cancelled", "canceled":
		return RunCancelled, nil
	}
	return RunFailed, fmt.Errorf("subagent reported unknown status %q", s)
}

// finish applies the single terminal transition of a run and records its
// outcome. It is a no-op when the run is already terminal, so exactly one
// path decides the result.
func (m *RunManager) finish(rs *runState, status RunStatus, res SubagentResult, runErr error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if terminalRunStatus(rs.status) {
		return
	}
	if !canTransition(rs.status, status) {
		// Queue-time cancellations are the only out-of-band arrivals; land
		// them on cancelled rather than leaving a run stuck non-terminal.
		log.Printf("agent runs: illegal transition %s -> %s for run %s, cancelling instead", rs.status, status, rs.id)
		status = RunCancelled
		if !canTransition(rs.status, status) {
			return
		}
	}
	rs.output = truncateText(res.Output, subagentMaxOutput)
	rs.errText = ""
	if runErr != nil {
		rs.errText = runErr.Error()
	}
	m.transitionLocked(rs, status)
	rs.lifeStop()
}

// transitionLocked moves a run to its next status and persists the result.
// The caller holds the manager write lock.
func (m *RunManager) transitionLocked(rs *runState, to RunStatus) bool {
	if !canTransition(rs.status, to) {
		return false
	}
	rs.status = to
	switch {
	case to == RunRunning:
		rs.startedAt = time.Now()
	case terminalRunStatus(to):
		rs.finishedAt = time.Now()
		if !rs.startedAt.IsZero() {
			rs.elapsedMS = rs.finishedAt.Sub(rs.startedAt).Milliseconds()
		}
		close(rs.done)
	}
	m.persistLocked(rs)
	return true
}

// recordEvent appends one event line to the run's ring buffer. Lines that
// arrive after the run finished are dropped so the snapshot stays consistent.
func (m *RunManager) recordEvent(rs *runState, line string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if terminalRunStatus(rs.status) {
		return
	}
	rs.events[rs.eventNext%len(rs.events)] = line
	rs.eventNext++
	if rs.eventKept < len(rs.events) {
		rs.eventKept++
	}
	rs.eventTotal++
	rs.lastEvent = line
}

// snapshotLocked copies a run into its exported, read-only form. The caller
// holds the manager lock.
func (m *RunManager) snapshotLocked(rs *runState) *AgentRun {
	return &AgentRun{
		ID:         rs.id,
		Workspace:  rs.spec.Workspace,
		Client:     rs.spec.Client,
		Agent:      rs.spec.Agent,
		Model:      rs.spec.Model,
		Thinking:   rs.spec.Thinking,
		Task:       rs.spec.Task,
		Status:     rs.status,
		StartedAt:  rs.startedAt,
		FinishedAt: rs.finishedAt,
		ElapsedMS:  rs.elapsedMS,
		Output:     rs.output,
		Error:      rs.errText,
		EventCount: rs.eventTotal,
		LastEvent:  rs.lastEvent,
	}
}

// persistLocked writes the run through to the store, if any. A failing store
// must never stall the state machine, so errors are logged only.
func (m *RunManager) persistLocked(rs *runState) {
	if err := m.store.upsert(m.snapshotLocked(rs)); err != nil {
		log.Printf("agent runs: persist run %s: %v", rs.id, err)
	}
}

// cacheLoaded mirrors a store row back into memory, keeping memory and store
// in agreement after a lookup that had to hit the database.
func (m *RunManager) cacheLoaded(snap *AgentRun) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.runs[snap.ID]; !ok {
		m.runs[snap.ID] = loadedRunState(snap, m.eventCapacity)
	}
}

// loadedRunState rebuilds the state of a run loaded from the store. A row
// still marked queued or running belongs to a dead process and is reported as
// interrupted.
func loadedRunState(snap *AgentRun, eventCapacity int) *runState {
	status := snap.Status
	if !terminalRunStatus(status) {
		status = RunInterrupted
	}
	done := make(chan struct{})
	close(done)
	return &runState{
		id: snap.ID,
		spec: RunSpec{
			Workspace: snap.Workspace, Task: snap.Task, Client: snap.Client,
			Agent: snap.Agent, Model: snap.Model, Thinking: snap.Thinking,
		},
		status:     status,
		startedAt:  snap.StartedAt,
		finishedAt: snap.FinishedAt,
		elapsedMS:  snap.ElapsedMS,
		output:     snap.Output,
		errText:    snap.Error,
		eventTotal: snap.EventCount,
		lastEvent:  snap.LastEvent,
		events:     make([]string, eventCapacity),
		done:       done,
	}
}

// newRunID returns an unpredictable run identifier.
func newRunID() string {
	var b [16]byte
	_, _ = rand.Read(b[:])
	return "run_" + hex.EncodeToString(b[:])
}

// runExecContext bounds a run by its configured wall-clock budget, if any.
func runExecContext(parent context.Context, timeoutSeconds int) (context.Context, context.CancelFunc) {
	if timeoutSeconds > 0 {
		return context.WithTimeout(parent, time.Duration(timeoutSeconds)*time.Second)
	}
	return context.WithCancel(parent)
}

// normalizeRunSpec resolves profile selection and per-call defaults up front,
// so the snapshot of a run reports the harness it will actually use instead of
// the empty strings an omitted parameter left behind. Start applies it before
// validation; runSubagent resolves the same call again, which is idempotent.
func normalizeRunSpec(spec RunSpec) RunSpec {
	client, agent, model, thinking, timeoutSeconds, _ := resolveSubagentCall(spec.Client, spec.Agent, spec.Model, spec.Thinking, spec.TimeoutSeconds)
	if client = strings.TrimSpace(client); client != "" {
		spec.Client = client
	}
	spec.Agent = agent
	spec.Model = model
	spec.Thinking = thinking
	if timeoutSeconds > 0 {
		spec.TimeoutSeconds = timeoutSeconds
	}
	return spec
}

// validateRunSpec rejects a run that cannot possibly execute: the caller
// passes a spec that normalizeRunSpec has already resolved, so a profile that
// pins a harness or effort validates exactly like a direct call.
func validateRunSpec(spec RunSpec) error {
	if strings.TrimSpace(spec.Task) == "" {
		return errors.New("task is required")
	}
	return validateSubagentCall(spec.Client, spec.Agent, spec.Thinking)
}

// DefaultExecutor adapts a Service to the ExecutorFunc RunManager calls. The
// root argument arrives from RunSpec.Workspace and runSubagent resolves it
// against the Service's workspace table, so the manager itself needs no
// workspace knowledge.
func DefaultExecutor(service *Service) ExecutorFunc {
	return func(ctx context.Context, spec RunSpec, root string, onEvent func(string)) (SubagentResult, error) {
		res, err := service.runSubagent(ctx, root, spec.Task, spec.Client, spec.Agent, spec.Model, spec.Thinking, spec.TimeoutSeconds, onEvent)
		if res == nil {
			return SubagentResult{}, err
		}
		return *res, err
	}
}

// runManager returns the Service's run manager, creating it on first use. The
// client may install its own (with a store path and its own root context)
// before the first run.
func (s *Service) runManager() *RunManager {
	s.runsOnce.Do(func() {
		if s.Runs == nil {
			s.Runs = NewRunManager(
				WithExecutor(DefaultExecutor(s)),
				WithPreflight(s.subagentPreflight),
			)
		}
	})
	return s.Runs
}

// RunSubagentSync runs one subagent to completion and blocks until it
// finishes: the compatibility path for the agent tool. The run is owned by
// the Service's RunManager and its root context, so ctx bounds the wait only,
// not the run.
func (s *Service) RunSubagentSync(ctx context.Context, root, task, client, agent, model, thinking string, timeoutSeconds int) (*SubagentResult, error) {
	manager := s.runManager()
	run, err := manager.Start(ctx, RunSpec{
		Workspace:      root,
		Task:           task,
		Client:         client,
		Agent:          agent,
		Model:          model,
		Thinking:       thinking,
		TimeoutSeconds: timeoutSeconds,
	})
	if err != nil {
		return nil, err
	}
	snap, err := manager.Wait(ctx, run.ID)
	if err != nil {
		return nil, err
	}
	return SubagentResultFromRun(snap), nil
}

// SubagentResultFromRun maps a finished run back onto the result shape the
// agent tool has always returned. A failure that carries no harness output
// (for example, no executor configured) falls back to the run's error text so
// the caller is not left with an empty result. It is exported for the client,
// which drives its own runs through Start and Wait and still owes MCP callers
// the legacy shape.
func SubagentResultFromRun(snap *AgentRun) *SubagentResult {
	elapsed := snap.ElapsedMS
	if !snap.StartedAt.IsZero() && !snap.FinishedAt.IsZero() {
		elapsed = snap.FinishedAt.Sub(snap.StartedAt).Milliseconds()
	}
	output := snap.Output
	if output == "" {
		output = snap.Error
	}
	return &SubagentResult{
		Client:   snap.Client,
		Agent:    snap.Agent,
		Model:    snap.Model,
		Thinking: snap.Thinking,
		Task:     truncateText(strings.TrimSpace(snap.Task), 400),
		Status:   string(snap.Status),
		Output:   output,
		Events:   snap.EventCount,
		Elapsed:  (time.Duration(elapsed) * time.Millisecond).String(),
	}
}

// subagentPreflight is the admission check the Service installs on its run
// manager: it resolves the workspace and applies the agent permission rules
// synchronously, so a denied harness fails immediately and an "ask" harness
// answers with the request id it parked.
func (s *Service) subagentPreflight(spec RunSpec) error {
	if _, err := s.resolveWorkspaceRoot(spec.Workspace); err != nil {
		return err
	}
	client, _, _, _, _, _ := resolveSubagentCall(spec.Client, spec.Agent, spec.Model, spec.Thinking, spec.TimeoutSeconds)
	if client = strings.TrimSpace(client); client == "" {
		client = DefaultSubagentClient
	}
	effect, _ := permissionDecision("agent", client)
	switch effect {
	case "deny":
		return fmt.Errorf("subagent client %q is denied by local permission rules (add an allow rule for %s)", client, client)
	case "ask":
		requestID, _ := s.pendingSubagentRequest(client)
		if requestID == "" {
			requestID = parkRequest("agent", client, "")
		}
		return &PermissionNeededError{RequestID: requestID, Command: client + " subagent"}
	}
	return nil
}

// SubagentPreflight runs the run manager's admission check on demand: it
// resolves the workspace and applies the agent permission rules synchronously,
// so a client that assembles its own RunManager rejects a denied harness (or
// answers with the request id it parked) before the run is queued.
func (s *Service) SubagentPreflight(spec RunSpec) error {
	return s.subagentPreflight(spec)
}
