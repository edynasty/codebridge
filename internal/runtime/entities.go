package runtime

import (
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// Identifier prefixes frozen by the V2 architecture (docs/v2/architecture.md
// §7.1). The store validates them so a mistyped id fails at the boundary
// instead of creating an unreachable row.
const (
	idProject         = "prj_"
	idSession         = "ses_"
	idRun             = "run_"
	idProviderSession = "psn_"
	idComputerSession = "cmp_"
	idArtifact        = "art_"
)

// CallerClass is a Bridge caller class (architecture §6).
type CallerClass string

const (
	CallerRemoteAI      CallerClass = "remote_ai"
	CallerLocalUI       CallerClass = "local_ui"
	CallerLocalMCP      CallerClass = "local_mcp"
	CallerAgentInternal CallerClass = "agent_internal"
)

// SessionStatus is a Session's lifecycle. A Session never completes; it groups
// Runs and ComputerSessions and is the scope of `session` grants.
type SessionStatus string

const (
	SessionOpen     SessionStatus = "open"
	SessionArchived SessionStatus = "archived"
)

// RunStatus is the V1 run state machine, unchanged in V2 (architecture §7.7).
type RunStatus string

const (
	RunQueued      RunStatus = "queued"
	RunRunning     RunStatus = "running"
	RunCompleted   RunStatus = "completed"
	RunFailed      RunStatus = "failed"
	RunCancelled   RunStatus = "cancelled"
	RunTimeout     RunStatus = "timeout"
	RunInterrupted RunStatus = "interrupted"
)

// Terminal reports whether the status ends the Run.
func (s RunStatus) Terminal() bool {
	switch s {
	case RunCompleted, RunFailed, RunCancelled, RunTimeout, RunInterrupted:
		return true
	default:
		return false
	}
}

// valid reports whether the status is part of the V1 state machine.
func (s RunStatus) valid() bool {
	switch s {
	case RunQueued, RunRunning, RunCompleted, RunFailed, RunCancelled, RunTimeout, RunInterrupted:
		return true
	default:
		return false
	}
}

// RunKind distinguishes a local agent run from a cloud continuation. A Run is
// never migrated between the two; a handoff creates a new Run.
type RunKind string

const (
	RunAgent RunKind = "agent"
	RunCloud RunKind = "cloud"
)

// ComputerSessionState is the leased-resource state machine (architecture §11.4).
type ComputerSessionState string

const (
	ComputerStarting  ComputerSessionState = "starting"
	ComputerActive    ComputerSessionState = "active"
	ComputerSuspended ComputerSessionState = "suspended"
	ComputerClosed    ComputerSessionState = "closed"
	ComputerFailed    ComputerSessionState = "failed"
)

// ControllerKind is who holds input for a ComputerSession.
type ControllerKind string

const (
	ControllerAgent ControllerKind = "agent"
	ControllerHuman ControllerKind = "human"
	ControllerNone  ControllerKind = "none"
)

// ControllerChannel is how a human controller reaches the session.
type ControllerChannel string

const (
	ChannelNone   ControllerChannel = ""
	ChannelLocal  ControllerChannel = "local"
	ChannelRemote ControllerChannel = "remote"
)

// EventSource is the event-emission contract's source field (§8).
type EventSource string

const (
	SourceBridge   EventSource = "bridge"
	SourceRuntime  EventSource = "runtime"
	SourceAgent    EventSource = "agent"
	SourceComputer EventSource = "computer"
	SourcePolicy   EventSource = "policy"
	SourceSystem   EventSource = "system"
)

// PolicyEffect is a policy rule's action; deny always wins and an unmatched
// request means `ask` (architecture §13.1).
type PolicyEffect string

const (
	EffectAllow PolicyEffect = "allow"
	EffectDeny  PolicyEffect = "deny"
	EffectAsk   PolicyEffect = "ask"
)

// GrantScope bounds how long a grant survives.
type GrantScope string

const (
	ScopeOnce    GrantScope = "once"
	ScopeSession GrantScope = "session"
	ScopeProject GrantScope = "project"
	ScopeAlways  GrantScope = "always"
)

// ApprovalChannel is the channel a decision came through. No model-visible path
// can create or widen a grant (architecture §13.2).
type ApprovalChannel string

const (
	ApprovalLocalUI     ApprovalChannel = "local_ui"
	ApprovalRemoteHuman ApprovalChannel = "remote_human"
)

// ApprovalStatus is an approval request's state.
type ApprovalStatus string

const (
	ApprovalPending  ApprovalStatus = "pending"
	ApprovalApproved ApprovalStatus = "approved"
	ApprovalDenied   ApprovalStatus = "denied"
	ApprovalExpired  ApprovalStatus = "expired"
)

// Verb is a permission verb (architecture §13.1).
type Verb string

const (
	VerbFilesystemRead  Verb = "filesystem.read"
	VerbFilesystemWrite Verb = "filesystem.write"
	VerbShellExecute    Verb = "shell.execute"
	VerbAgentStart      Verb = "agent.start"
	VerbAgentCancel     Verb = "agent.cancel"
	VerbComputerObserve Verb = "computer.observe"
	VerbComputerInput   Verb = "computer.input"
	VerbComputerPreview Verb = "computer.preview"
	VerbGitPush         Verb = "git.push"
	VerbCloudUpload     Verb = "cloud.upload"
	VerbCloudExecute    Verb = "cloud.execute"
	VerbNetworkConnect  Verb = "network.connect"
	VerbSecretRead      Verb = "secret.read"
)

// ArtifactKind lists the durable output kinds (provider-contracts.md §10).
type ArtifactKind string

const (
	ArtifactFile            ArtifactKind = "file"
	ArtifactPatch           ArtifactKind = "patch"
	ArtifactCommit          ArtifactKind = "commit"
	ArtifactBranchRef       ArtifactKind = "branch_ref"
	ArtifactReport          ArtifactKind = "report"
	ArtifactTestResult      ArtifactKind = "test_result"
	ArtifactLogBundle       ArtifactKind = "log_bundle"
	ArtifactCloudResult     ArtifactKind = "cloud_result"
	ArtifactSavedScreenshot ArtifactKind = "saved_screenshot"
)

// ProducerKind is what produced an Artifact.
type ProducerKind string

const (
	ProducedByRun             ProducerKind = "run"
	ProducedByComputerSession ProducerKind = "computer_session"
)

// Journal streams. A stream is `run:<id>`, `computer:<id>`, `session:<id>` or
// `system` (architecture §8).
const (
	streamRunPrefix      = "run:"
	streamComputerPrefix = "computer:"
	streamSessionPrefix  = "session:"

	// StreamSystem is the daemon-wide journal stream.
	StreamSystem = "system"
)

// StreamRun returns the journal stream of one Run.
func StreamRun(id string) string { return streamRunPrefix + id }

// StreamComputer returns the journal stream of one ComputerSession.
func StreamComputer(id string) string { return streamComputerPrefix + id }

// StreamSession returns the journal stream of one Session.
func StreamSession(id string) string { return streamSessionPrefix + id }

func runIDFromStream(stream string) (string, bool) {
	if id, ok := strings.CutPrefix(stream, streamRunPrefix); ok && id != "" {
		return id, true
	}
	return "", false
}

// Project is the authorization boundary for files, shell and agents. Roots and
// sensitive paths are ordered typed lists, stored in child tables.
type Project struct {
	ID             string
	Name           string
	Roots          []ProjectRoot
	SensitivePaths []string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// ProjectRoot is one authorized root of a Project.
type ProjectRoot struct {
	Path     string
	Writable bool
}

// Session is durable working context: zero or one Project, no running state and
// no result.
type Session struct {
	ID                   string
	ProjectID            string
	Title                string
	CallerClass          CallerClass
	ExternalConversation string
	Status               SessionStatus
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

// Run is one execution with a state machine and a terminal result.
type Run struct {
	ID                string
	SessionID         string
	ProjectID         string
	Kind              RunKind
	Provider          string
	Model             string
	Autonomy          string
	Role              string
	ParentRunID       string
	ContinuesRunID    string
	ProviderSessionID string
	Status            RunStatus
	ProcessPID        int
	ProcessStartedAt  time.Time
	ProcessGroupID    int
	OutputRef         string
	ErrorCategory     string
	LastSeq           int64
	CreatedAt         time.Time
	StartedAt         time.Time
	FinishedAt        time.Time
}

// ProviderSession is a harness-native conversation binding. The locator is
// local only and never exported.
type ProviderSession struct {
	ID         string
	Provider   string
	NativeID   string
	Locator    string
	ProjectID  string
	CanResume  bool
	CanHistory bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

// ComputerSession is leased control of one computer target. It is not a Run.
type ComputerSession struct {
	ID                 string
	SessionID          string
	RunID              string
	Target             string
	State              ComputerSessionState
	Controller         Controller
	ControllerEpoch    int64
	ArbiterInstance    string
	LeaseHolder        string
	LeaseExpiresAt     time.Time
	CanObserve         bool
	CanInput           bool
	CanPreview         bool
	GrantedVerbs       []Verb
	AppSelectors       []string
	LastFrameID        string
	GeometryGeneration int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Controller is who holds input for a ComputerSession.
type Controller struct {
	Kind    ControllerKind
	Holder  string
	Channel ControllerChannel
}

// Artifact is metadata for a durable output; payloads live outside the store.
type Artifact struct {
	ID           string
	ProducerKind ProducerKind
	ProducerID   string
	Kind         ArtifactKind
	Reference    string
	Digest       string
	SizeBytes    int64
	Sensitivity  string
	Retention    string
	CreatedAt    time.Time
}

// PolicyRule is one verb + selector matched against a request.
type PolicyRule struct {
	ID        string
	Effect    PolicyEffect
	Verb      Verb
	Selector  string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// Grant is a persisted permission. It is never created or widened by a
// model-visible tool.
type Grant struct {
	ID         string
	Verb       Verb
	Selector   string
	Scope      GrantScope
	SessionID  string
	ProjectID  string
	RunID      string
	ApprovalID string
	Channel    ApprovalChannel
	ExpiresAt  time.Time
	CreatedAt  time.Time
	RevokedAt  time.Time
}

// Approval is one approval request and its decision.
type Approval struct {
	ID          string
	RunID       string
	SessionID   string
	ProjectID   string
	Verb        Verb
	Selector    string
	Status      ApprovalStatus
	Channel     ApprovalChannel
	Reason      string
	RequestedAt time.Time
	DecidedAt   time.Time
}

// Event is one journal entry (architecture §8).
type Event struct {
	Pos           int64
	Stream        string
	Seq           int64
	At            time.Time
	SessionID     string
	ProjectID     string
	Source        EventSource
	Kind          string
	TurnRef       string
	CorrelationID string
	Payload       string
	RawRef        string
}

// ValidateContiguous reports the first gap when events, ordered by seq, do not
// continue immediately after afterSeq. Subscribers use it to detect dropped or
// missing events before replaying.
func ValidateContiguous(evs []Event, afterSeq int64) error {
	want := afterSeq + 1
	for _, ev := range evs {
		if ev.Seq != want {
			return fmt.Errorf("runtime: event gap on stream %s: want seq %d, got %d", ev.Stream, want, ev.Seq)
		}
		want++
	}
	return nil
}

// ms renders a timestamp for the store; the zero time stays zero.
func ms(t time.Time) int64 {
	if t.IsZero() {
		return 0
	}
	return t.UnixMilli()
}

// timeFromMS is the inverse of ms.
func timeFromMS(v int64) time.Time {
	if v == 0 {
		return time.Time{}
	}
	return time.UnixMilli(v).UTC()
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// nullText stores an absent optional reference as NULL so foreign keys and
// partial indexes see it as unset.
func nullText(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func text(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}

// checkID requires a non-empty identifier with the frozen entity prefix.
func checkID(id, prefix string) error {
	switch {
	case id == "":
		return fmt.Errorf("runtime: identifier is required")
	case !strings.HasPrefix(id, prefix):
		return fmt.Errorf("runtime: identifier %q must start with %q", id, prefix)
	default:
		return nil
	}
}

// requireText requires a non-empty free-form identifier or label.
func requireText(label, v string) error {
	if v == "" {
		return fmt.Errorf("runtime: %s is required", label)
	}
	return nil
}
