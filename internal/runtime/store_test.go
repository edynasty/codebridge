package runtime

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// testNow has millisecond precision, which is the store's resolution.
var testNow = time.Date(2026, 10, 2, 12, 0, 0, 123_000_000, time.UTC)

func openAt(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatalf("Open(%s): %v", path, err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func openTemp(t *testing.T) *Store {
	t.Helper()
	return openAt(t, filepath.Join(t.TempDir(), "runtime.db"))
}

// seedGraph creates the project, session and provider session the run tests
// reference.
func seedGraph(t *testing.T, s *Store) {
	t.Helper()
	err := s.Update(func(tx *Tx) error {
		if err := tx.PutProject(Project{ID: "prj_t", Name: "t", CreatedAt: testNow, UpdatedAt: testNow}); err != nil {
			return err
		}
		if err := tx.PutSession(Session{
			ID: "ses_t", ProjectID: "prj_t", CallerClass: CallerLocalUI,
			Status: SessionOpen, CreatedAt: testNow, UpdatedAt: testNow,
		}); err != nil {
			return err
		}
		return tx.PutProviderSession(ProviderSession{
			ID: "psn_t", Provider: "omp", NativeID: "native-1",
			CreatedAt: testNow, UpdatedAt: testNow,
		})
	})
	if err != nil {
		t.Fatalf("seed graph: %v", err)
	}
}

// write runs one store transaction; the negative tests inspect the error.
func write(s *Store, fn func(*Tx) error) error { return s.Update(fn) }

// mustWrite runs one store transaction that has to succeed.
func mustWrite(t *testing.T, s *Store, fn func(*Tx) error) {
	t.Helper()
	if err := s.Update(fn); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func putRunOf(t *testing.T, s *Store, id string, status RunStatus, providerSessionID string) error {
	t.Helper()
	return write(s, func(tx *Tx) error {
		return tx.PutRun(Run{
			ID: id, SessionID: "ses_t", Kind: RunAgent, Status: status,
			ProviderSessionID: providerSessionID, CreatedAt: testNow, StartedAt: testNow,
		})
	})
}

func TestOpenAppliesDurabilityPragmas(t *testing.T) {
	s := openTemp(t)

	// Architecture §8: a terminal transition or approval decision is durable
	// before it is acknowledged, so the store commits with synchronous=FULL on a
	// WAL database with enforced foreign keys.
	var (
		synchronous int
		foreignKeys int
	)
	if err := s.db.QueryRow(`PRAGMA synchronous`).Scan(&synchronous); err != nil {
		t.Fatalf("read synchronous: %v", err)
	}
	if synchronous != 2 { // 2 = FULL
		t.Fatalf("synchronous: want FULL (2), got %d", synchronous)
	}
	if err := s.db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("read foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys: want 1, got %d", foreignKeys)
	}
	var journalMode string
	if err := s.db.QueryRow(`PRAGMA journal_mode`).Scan(&journalMode); err != nil {
		t.Fatalf("read journal_mode: %v", err)
	}
	if journalMode != "wal" {
		t.Fatalf("journal_mode: want wal, got %q", journalMode)
	}
}

func TestOpenCreatesSchemaAndReopens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	s := openAt(t, path)

	version, err := s.readVersion()
	if err != nil {
		t.Fatalf("readVersion: %v", err)
	}
	if version != SchemaVersion {
		t.Fatalf("version after create: want %d, got %d", SchemaVersion, version)
	}
	mustWrite(t, s, func(tx *Tx) error {
		return tx.PutProject(Project{ID: "prj_t", Name: "kept", CreatedAt: testNow, UpdatedAt: testNow})
	})
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened := openAt(t, path)
	project, err := reopened.GetProject("prj_t")
	if err != nil {
		t.Fatalf("GetProject after reopen: %v", err)
	}
	if project.Name != "kept" {
		t.Fatalf("project name after reopen: want %q, got %q", "kept", project.Name)
	}
}

func TestOpenPathWithSpaces(t *testing.T) {
	// Application Support contains a space; the connection string must survive it.
	path := filepath.Join(t.TempDir(), "Application Support", "CodeBridge", "runtime.db")
	s := openAt(t, path)
	if s.Path() != path {
		t.Fatalf("store path: want %q, got %q", path, s.Path())
	}
	mustWrite(t, s, func(tx *Tx) error {
		return tx.PutProject(Project{ID: "prj_space", Name: "spaces", CreatedAt: testNow, UpdatedAt: testNow})
	})
	if _, err := s.GetProject("prj_space"); err != nil {
		t.Fatalf("GetProject: %v", err)
	}
}

func TestUpdateRollsBackOnError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "runtime.db")
	s := openAt(t, path)
	seedGraph(t, s)

	boom := errors.New("boom")
	err := s.Update(func(tx *Tx) error {
		if err := tx.PutRun(Run{ID: "run_rollback", SessionID: "ses_t", Kind: RunAgent,
			Status: RunRunning, CreatedAt: testNow}); err != nil {
			return err
		}
		if _, err := tx.AppendEvent(Event{Stream: StreamRun("run_rollback"), Kind: "run.running"}); err != nil {
			return err
		}
		return boom
	})
	if !errors.Is(err, boom) {
		t.Fatalf("Update error: want %v, got %v", boom, err)
	}
	if _, err := s.GetRun("run_rollback"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("run after rollback: want ErrNotFound, got %v", err)
	}
	if head, err := s.JournalHead(); err != nil || head != 0 {
		t.Fatalf("journal head after rollback: want 0, got %d (err %v)", head, err)
	}

	// The rollback is durable, not just invisible in this process.
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	reopened := openAt(t, path)
	if _, err := reopened.GetRun("run_rollback"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("run after reopen: want ErrNotFound, got %v", err)
	}
	if head, err := reopened.JournalHead(); err != nil || head != 0 {
		t.Fatalf("journal head after reopen: want 0, got %d (err %v)", head, err)
	}
}

func TestActiveProviderRunConstraint(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)

	if err := putRunOf(t, s, "run_a", RunRunning, "psn_t"); err != nil {
		t.Fatalf("first active run: %v", err)
	}
	if err := putRunOf(t, s, "run_b", RunRunning, "psn_t"); err == nil {
		t.Fatal("second running run for the same provider session was accepted")
	}
	if err := putRunOf(t, s, "run_c", RunQueued, "psn_t"); err == nil {
		t.Fatal("queued run for the same provider session was accepted")
	}

	if _, _, err := s.TransitionRun(RunTransition{RunID: "run_a", Status: RunCompleted, At: testNow}); err != nil {
		t.Fatalf("TransitionRun: %v", err)
	}
	if err := putRunOf(t, s, "run_b", RunRunning, "psn_t"); err != nil {
		t.Fatalf("run after the first terminated: %v", err)
	}
	// A run without a provider session is not constrained.
	if err := putRunOf(t, s, "run_d", RunRunning, ""); err != nil {
		t.Fatalf("run without provider session: %v", err)
	}
}

func TestEntityRoundTrip(t *testing.T) {
	s := openTemp(t)

	project := Project{
		ID:             "prj_rt",
		Name:           "round trip",
		Roots:          []ProjectRoot{{Path: "/tmp/a", Writable: true}, {Path: "/tmp/b"}},
		SensitivePaths: []string{"**/.env", "**/id_rsa"},
		CreatedAt:      testNow,
		UpdatedAt:      testNow,
	}
	session := Session{
		ID: "ses_rt", ProjectID: "prj_rt", Title: "session", CallerClass: CallerRemoteAI,
		ExternalConversation: "conv-1", Status: SessionOpen, CreatedAt: testNow, UpdatedAt: testNow,
	}
	providerSession := ProviderSession{
		ID: "psn_rt", Provider: "codex", NativeID: "thread-1", Locator: "/tmp/rollout.jsonl",
		ProjectID: "prj_rt", CanResume: true, CanHistory: true, CreatedAt: testNow, UpdatedAt: testNow,
	}
	run := Run{
		ID: "run_rt", SessionID: "ses_rt", ProjectID: "prj_rt", Kind: RunCloud,
		Provider: "codex", Model: "gpt-5", Autonomy: "workspace-write", Role: "worker",
		ContinuesRunID: "run_parent", ProviderSessionID: "psn_rt", Status: RunQueued,
		ProcessPID: 4242, ProcessGroupID: 4242, ProcessStartedAt: testNow, OutputRef: "artifact://out",
		ErrorCategory: "transient", CreatedAt: testNow, StartedAt: testNow, FinishedAt: testNow,
	}
	computerSession := ComputerSession{
		ID: "cmp_rt", SessionID: "ses_rt", RunID: "run_rt", Target: "display:2",
		State: ComputerActive, Controller: Controller{Kind: ControllerHuman, Holder: "user", Channel: ChannelLocal},
		ControllerEpoch: 7, ArbiterInstance: "arb-1", LeaseHolder: "run_rt", LeaseExpiresAt: testNow,
		CanObserve: true, CanInput: false, CanPreview: true,
		GrantedVerbs: []Verb{VerbComputerObserve, VerbComputerPreview}, AppSelectors: []string{"com.apple.Safari"},
		LastFrameID: "frame-9", GeometryGeneration: 3, CreatedAt: testNow, UpdatedAt: testNow,
	}
	artifact := Artifact{
		ID: "art_rt", ProducerKind: ProducedByRun, ProducerID: "run_rt", Kind: ArtifactPatch,
		Reference: "/tmp/change.patch", Digest: "sha256:abc", SizeBytes: 1234,
		Sensitivity: "normal", Retention: "keep", CreatedAt: testNow,
	}
	rule := PolicyRule{ID: "rule-1", Effect: EffectDeny, Verb: VerbShellExecute, Selector: "rm -rf*", CreatedAt: testNow, UpdatedAt: testNow}
	grant := Grant{
		ID: "grant-1", Verb: VerbComputerInput, Selector: "com.apple.TextEdit", Scope: ScopeSession,
		SessionID: "ses_rt", RunID: "run_rt", ApprovalID: "approval-1", Channel: ApprovalLocalUI,
		ExpiresAt: testNow, CreatedAt: testNow, RevokedAt: testNow,
	}
	approval := Approval{
		ID: "approval-1", RunID: "run_rt", SessionID: "ses_rt", ProjectID: "prj_rt",
		Verb: VerbComputerInput, Selector: "com.apple.TextEdit", Status: ApprovalApproved,
		Channel: ApprovalRemoteHuman, Reason: "widget", RequestedAt: testNow, DecidedAt: testNow,
	}

	err := s.Update(func(tx *Tx) error {
		if err := tx.PutProject(project); err != nil {
			return err
		}
		if err := tx.PutSession(session); err != nil {
			return err
		}
		if err := tx.PutProviderSession(providerSession); err != nil {
			return err
		}
		// The run references a continuation run; insert that first.
		parent := run
		parent.ID = "run_parent"
		parent.ContinuesRunID = ""
		parent.Status = RunInterrupted
		if err := tx.PutRun(parent); err != nil {
			return err
		}
		if err := tx.PutRun(run); err != nil {
			return err
		}
		if err := tx.PutComputerSession(computerSession); err != nil {
			return err
		}
		if err := tx.PutArtifact(artifact); err != nil {
			return err
		}
		if err := tx.PutPolicyRule(rule); err != nil {
			return err
		}
		if err := tx.PutApproval(approval); err != nil {
			return err
		}
		return tx.PutGrant(grant)
	})
	if err != nil {
		t.Fatalf("seed entities: %v", err)
	}

	gotProject, err := s.GetProject("prj_rt")
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if !reflect.DeepEqual(*gotProject, project) {
		t.Fatalf("project round trip:\n got %+v\nwant %+v", *gotProject, project)
	}
	gotSession, err := s.GetSession("ses_rt")
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if !reflect.DeepEqual(*gotSession, session) {
		t.Fatalf("session round trip:\n got %+v\nwant %+v", *gotSession, session)
	}
	gotProviderSession, err := s.GetProviderSession("psn_rt")
	if err != nil {
		t.Fatalf("GetProviderSession: %v", err)
	}
	if !reflect.DeepEqual(*gotProviderSession, providerSession) {
		t.Fatalf("provider session round trip:\n got %+v\nwant %+v", *gotProviderSession, providerSession)
	}
	gotRun, err := s.GetRun("run_rt")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if !reflect.DeepEqual(*gotRun, run) {
		t.Fatalf("run round trip:\n got %+v\nwant %+v", *gotRun, run)
	}
	gotComputerSession, err := s.GetComputerSession("cmp_rt")
	if err != nil {
		t.Fatalf("GetComputerSession: %v", err)
	}
	if !reflect.DeepEqual(*gotComputerSession, computerSession) {
		t.Fatalf("computer session round trip:\n got %+v\nwant %+v", *gotComputerSession, computerSession)
	}
	gotArtifact, err := s.GetArtifact("art_rt")
	if err != nil {
		t.Fatalf("GetArtifact: %v", err)
	}
	if !reflect.DeepEqual(*gotArtifact, artifact) {
		t.Fatalf("artifact round trip:\n got %+v\nwant %+v", *gotArtifact, artifact)
	}
	gotRule, err := s.GetPolicyRule("rule-1")
	if err != nil {
		t.Fatalf("GetPolicyRule: %v", err)
	}
	if !reflect.DeepEqual(*gotRule, rule) {
		t.Fatalf("policy rule round trip:\n got %+v\nwant %+v", *gotRule, rule)
	}
	gotGrant, err := s.GetGrant("grant-1")
	if err != nil {
		t.Fatalf("GetGrant: %v", err)
	}
	if !reflect.DeepEqual(*gotGrant, grant) {
		t.Fatalf("grant round trip:\n got %+v\nwant %+v", *gotGrant, grant)
	}
	gotApproval, err := s.GetApproval("approval-1")
	if err != nil {
		t.Fatalf("GetApproval: %v", err)
	}
	if !reflect.DeepEqual(*gotApproval, approval) {
		t.Fatalf("approval round trip:\n got %+v\nwant %+v", *gotApproval, approval)
	}
}

// TestGrantScopeBoundaries covers the frozen grant rules the schema enforces:
// a computer verb never carries the project scope (architecture §13.1), a
// session-scoped grant names its session, and the opt-in remote_human channel is
// limited to once/session and never covers secret.read or cloud.upload
// (architecture §13.2).
func TestGrantScopeBoundaries(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)

	put := func(g Grant) error {
		return write(s, func(tx *Tx) error { return tx.PutGrant(g) })
	}

	rejected := []struct {
		name  string
		grant Grant
	}{
		{"computer verb with project scope", Grant{
			ID: "grant-1", Verb: VerbComputerInput, Scope: ScopeProject, Channel: ApprovalLocalUI, CreatedAt: testNow}},
		{"session scope without a session", Grant{
			ID: "grant-2", Verb: VerbComputerInput, Scope: ScopeSession, Channel: ApprovalLocalUI, CreatedAt: testNow}},
		{"remote_human with project scope", Grant{
			ID: "grant-3", Verb: VerbFilesystemWrite, Scope: ScopeProject, ProjectID: "prj_t", Channel: ApprovalRemoteHuman, CreatedAt: testNow}},
		{"remote_human with always scope", Grant{
			ID: "grant-4", Verb: VerbFilesystemRead, Scope: ScopeAlways, Channel: ApprovalRemoteHuman, CreatedAt: testNow}},
		{"remote_human covering secret.read", Grant{
			ID: "grant-5", Verb: VerbSecretRead, Scope: ScopeOnce, Channel: ApprovalRemoteHuman, CreatedAt: testNow}},
		{"remote_human covering cloud.upload", Grant{
			ID: "grant-6", Verb: VerbCloudUpload, Scope: ScopeOnce, Channel: ApprovalRemoteHuman, CreatedAt: testNow}},
	}
	for _, tc := range rejected {
		if err := put(tc.grant); err == nil {
			t.Fatalf("%s: grant was accepted", tc.name)
		}
	}

	accepted := []struct {
		name  string
		grant Grant
	}{
		{"computer verb with session scope", Grant{
			ID: "grant-7", Verb: VerbComputerInput, Scope: ScopeSession, SessionID: "ses_t", Channel: ApprovalLocalUI, CreatedAt: testNow}},
		{"filesystem verb with project scope", Grant{
			ID: "grant-8", Verb: VerbFilesystemWrite, Scope: ScopeProject, ProjectID: "prj_t", Channel: ApprovalLocalUI, CreatedAt: testNow}},
		{"remote_human computer grant with once scope", Grant{
			ID: "grant-9", Verb: VerbComputerObserve, Scope: ScopeOnce, Channel: ApprovalRemoteHuman, CreatedAt: testNow}},
		{"local_ui secret.read with always scope", Grant{
			ID: "grant-10", Verb: VerbSecretRead, Scope: ScopeAlways, Channel: ApprovalLocalUI, CreatedAt: testNow}},
	}
	for _, tc := range accepted {
		if err := put(tc.grant); err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
	}
}

func TestIllegalRunTransitions(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)
	if err := putRunOf(t, s, "run_a", RunQueued, "psn_t"); err != nil {
		t.Fatalf("seed run_a: %v", err)
	}

	// The V1 state machine allows queued -> running -> completed.
	if _, _, err := s.TransitionRun(RunTransition{RunID: "run_a", Status: RunRunning, At: testNow}); err != nil {
		t.Fatalf("queued -> running: %v", err)
	}
	if _, _, err := s.TransitionRun(RunTransition{RunID: "run_a", Status: RunCompleted, At: testNow}); err != nil {
		t.Fatalf("running -> completed: %v", err)
	}

	// A terminal status has no successors: a finished run never falls back.
	for _, to := range []RunStatus{RunRunning, RunQueued, RunCompleted, RunCancelled} {
		if _, _, err := s.TransitionRun(RunTransition{RunID: "run_a", Status: to, At: testNow}); !errors.Is(err, ErrIllegalTransition) {
			t.Fatalf("completed -> %s: want ErrIllegalTransition, got %v", to, err)
		}
	}
	// queued -> completed and running -> queued are not legal either.
	if err := putRunOf(t, s, "run_b", RunQueued, ""); err != nil {
		t.Fatalf("seed run_b: %v", err)
	}
	if _, _, err := s.TransitionRun(RunTransition{RunID: "run_b", Status: RunCompleted, At: testNow}); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("queued -> completed: want ErrIllegalTransition, got %v", err)
	}
	if err := putRunOf(t, s, "run_c", RunRunning, ""); err != nil {
		t.Fatalf("seed run_c: %v", err)
	}
	if _, _, err := s.TransitionRun(RunTransition{RunID: "run_c", Status: RunQueued, At: testNow}); !errors.Is(err, ErrIllegalTransition) {
		t.Fatalf("running -> queued: want ErrIllegalTransition, got %v", err)
	}

	// A rejected transition changes no state and appends no event.
	for _, id := range []string{"run_a", "run_b", "run_c"} {
		run, err := s.GetRun(id)
		if err != nil {
			t.Fatalf("GetRun(%s): %v", id, err)
		}
		want := RunCompleted
		if id != "run_a" {
			want = RunQueued
		}
		if id == "run_c" {
			want = RunRunning
		}
		if run.Status != want {
			t.Fatalf("%s status after rejected transitions: want %s, got %s", id, want, run.Status)
		}
	}
	head, err := s.StreamHead(StreamRun("run_a"))
	if err != nil {
		t.Fatalf("StreamHead: %v", err)
	}
	if head != 2 {
		t.Fatalf("run_a events: want the 2 legal transitions, got %d", head)
	}
}

func TestIdentifierPrefixValidation(t *testing.T) {
	s := openTemp(t)

	if err := write(s, func(tx *Tx) error {
		return tx.PutProject(Project{ID: "ses_wrong", CreatedAt: testNow})
	}); err == nil {
		t.Fatal("project with a session prefix was accepted")
	}
	if err := write(s, func(tx *Tx) error {
		return tx.PutRun(Run{ID: "run_none", SessionID: "ses_t", Kind: RunAgent, Status: RunQueued})
	}); err == nil {
		t.Fatal("run referencing an unknown session was accepted")
	}
	if err := write(s, func(tx *Tx) error {
		_, err := tx.AppendEvent(Event{Stream: StreamSystem})
		return err
	}); err == nil {
		t.Fatal("event without a kind was accepted")
	}
}

func TestComputerControllerValidation(t *testing.T) {
	s := openTemp(t)
	seedGraph(t, s)
	mustWrite(t, s, func(tx *Tx) error {
		if err := tx.PutSession(Session{ID: "ses_t", Status: SessionOpen, CallerClass: CallerLocalUI}); err != nil {
			return err
		}
		return tx.PutComputerSession(ComputerSession{
			ID: "cmp_t", SessionID: "ses_t", Target: "display:1", State: ComputerStarting,
			Controller: Controller{Kind: ControllerNone}, CreatedAt: testNow, UpdatedAt: testNow,
		})
	})

	// A human controller must carry a channel, and an agent controller must not.
	if _, _, err := s.SetComputerController(ControllerChange{
		ComputerSessionID: "cmp_t", Controller: Controller{Kind: ControllerHuman}, At: testNow,
	}); err == nil {
		t.Fatal("human controller without a channel was accepted")
	}
	if _, _, err := s.SetComputerController(ControllerChange{
		ComputerSessionID: "cmp_t", Controller: Controller{Kind: ControllerAgent, Channel: ChannelRemote}, At: testNow,
	}); err == nil {
		t.Fatal("agent controller with a channel was accepted")
	}

	session, event, err := s.SetComputerController(ControllerChange{
		ComputerSessionID: "cmp_t", Controller: Controller{Kind: ControllerHuman, Holder: "user", Channel: ChannelRemote},
		At: testNow,
	})
	if err != nil {
		t.Fatalf("SetComputerController: %v", err)
	}
	if session.ControllerEpoch != 1 || event.Kind != "computer.controller.changed" {
		t.Fatalf("controller change: epoch %d, kind %q", session.ControllerEpoch, event.Kind)
	}
	next, _, err := s.SetComputerController(ControllerChange{
		ComputerSessionID: "cmp_t", Controller: Controller{Kind: ControllerNone}, At: testNow,
	})
	if err != nil {
		t.Fatalf("second SetComputerController: %v", err)
	}
	if next.ControllerEpoch != 2 {
		t.Fatalf("controller epoch after the second change: want 2, got %d", next.ControllerEpoch)
	}
}
