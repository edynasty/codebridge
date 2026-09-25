package agentops

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

const (
	subagentMaxOutput     = 256 * 1024
	subagentMaxConcurrent = 2
)

// Subagent harnesses. omp (oh-my-pi) is the default; opencode and codex stay
// selectable for callers that pin them.
const (
	SubagentClientOMP      = "omp"
	SubagentClientOpencode = "opencode"
	SubagentClientCodex    = "codex"
)

// DefaultSubagentClient is used when neither the call nor the profile names a
// harness.
const DefaultSubagentClient = SubagentClientOMP

func supportedSubagentClient(client string) bool {
	switch client {
	case SubagentClientOMP, SubagentClientOpencode, SubagentClientCodex:
		return true
	}
	return false
}

var subagentInflight atomic.Int32

// subagentPermit is a tiny semaphore honoring subagentMaxConcurrent.
type subagentPermit struct{}

func acquireSubagent(ctx context.Context) (subagentPermit, error) {
	for subagentInflight.Load() >= subagentMaxConcurrent {
		select {
		case <-ctx.Done():
			return subagentPermit{}, ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	subagentInflight.Add(1)
	return subagentPermit{}, nil
}

func (p subagentPermit) release() { subagentInflight.Add(-1) }

// SubagentResult is the outcome of one local subagent run.
type SubagentResult struct {
	Client   string `json:"client"`
	Agent    string `json:"agent,omitempty"`
	Model    string `json:"model,omitempty"`
	Thinking string `json:"thinking,omitempty"`
	Task     string `json:"task"`
	Status   string `json:"status"`
	Output   string `json:"output"`
	Events   int    `json:"events"`
	Elapsed  string `json:"elapsed"`
}

// runSubagent spawns a local coding-agent subagent inside the workspace root
// and returns its output. Supported clients: omp (default, `omp -p`),
// opencode (`opencode run`) and codex (`codex exec`). Gated by the same
// permission rules as bash: the operator must allow the client binary first.
// SubagentProfileConfig is the exported wire form of a profile (used by the
// UI config API); it mirrors clientconfig.SubagentProfile.
type SubagentProfileConfig struct {
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Client      string   `json:"client,omitempty"`
	Agent       string   `json:"agent,omitempty"`
	Model       string   `json:"model,omitempty"`
	Thinking    string   `json:"thinking,omitempty"`
	TimeoutSec  int      `json:"timeout_seconds,omitempty"`
	ExtraArgs   []string `json:"extra_args,omitempty"`
	// Tab names the configuration-UI tab this profile belongs to. Tabs are a
	// UI grouping only: the harness a profile runs on is always Client.
	Tab string `json:"tab,omitempty"`
}

// SubagentTabConfig is one configuration-UI tab: a display name bound to one
// harness. Several tabs may share a harness, so an operator can keep separate
// model/effort sets for the same tool.
type SubagentTabConfig struct {
	Name   string `json:"name"`
	Client string `json:"client"`
	// Locked freezes the tab's harness binding in the UI.
	Locked bool `json:"locked,omitempty"`
}

// SubagentProfiles is the profile table the client injects at boot; the
// MCP agent tool resolves its agent/agent_profile parameter against it.
var SubagentProfiles []SubagentProfile

// SubagentProfile mirrors clientconfig.SubagentProfile; duplicated to keep
// agentops free of a config dependency.
type SubagentProfile struct {
	Name        string
	Description string
	Client      string
	Agent       string
	Model       string
	Thinking    string
	TimeoutSec  int
	ExtraArgs   []string
	// Tab is the configuration-UI grouping this profile was edited under.
	Tab string
}

// AgentCatalogEntry is one entry of the agents_list answer.
type AgentCatalogEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Client      string `json:"client,omitempty"`
	Agent       string `json:"agent,omitempty"`
	Model       string `json:"model,omitempty"`
	Thinking    string `json:"thinking,omitempty"`
	Timeout     int    `json:"timeout_seconds,omitempty"`
	// Tab is the configuration-UI grouping the profile was defined under; it
	// does not affect how the subagent runs.
	Tab string `json:"tab,omitempty"`
}

// AgentCatalog returns the configured subagent profiles for AI clients. The
// harness is reported as the effective one, so a profile without an explicit
// client shows the default it will actually run on.
func AgentCatalog() []AgentCatalogEntry {
	out := make([]AgentCatalogEntry, 0, len(SubagentProfiles))
	for _, p := range SubagentProfiles {
		out = append(out, AgentCatalogEntry{
			Name: p.Name, Description: p.Description,
			Client: orDefault(p.Client, DefaultSubagentClient), Agent: p.Agent,
			Model: p.Model, Thinking: p.Thinking, Timeout: p.TimeoutSec,
			Tab: p.Tab,
		})
	}
	return out
}

// resolveSubagentCall merges an explicit profile selection with per-call
// overrides. Profile fields win over defaults; explicit call fields win
// over the profile so the caller can still narrow model/effort per task.
func resolveSubagentCall(client, agent, model, thinking string, timeoutSeconds int) (string, string, string, string, int, []string) {
	// The agent parameter doubles as the profile selector when it names a
	// configured profile.
	for _, p := range SubagentProfiles {
		if p.Name != "" && p.Name == strings.TrimSpace(agent) {
			if client == "" || client == p.Client {
				client = orDefault(p.Client, DefaultSubagentClient)
				// The profile name doubles as the agent selector when the
				// profile does not pin a separate agent explicitly.
				agent = orDefault(p.Agent, p.Name)
				if model == "" {
					model = p.Model
				}
				if thinking == "" {
					thinking = p.Thinking
				}
				if timeoutSeconds <= 0 {
					timeoutSeconds = p.TimeoutSec
				}
				return client, agent, model, thinking, timeoutSeconds, p.ExtraArgs
			}
		}
	}
	if client == "" {
		client = DefaultSubagentClient
	}
	return client, agent, model, thinking, timeoutSeconds, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

// OnSubagentEvent, when set, receives each raw JSONL event line while the
// subagent runs; the client uses it to stream progress to the manager.
var OnSubagentEvent func(tool, eventLine string)

func (s *Service) runSubagent(ctx context.Context, root, task, client, agent, model, thinking string, timeoutSeconds int) (*SubagentResult, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, errors.New("task is required")
	}
	var extraArgs []string
	client, agent, model, thinking, timeoutSeconds, extraArgs = resolveSubagentCall(client, agent, model, thinking, timeoutSeconds)
	client = strings.TrimSpace(client)
	if client == "" {
		client = DefaultSubagentClient
	}
	if !supportedSubagentClient(client) {
		return nil, fmt.Errorf("unsupported subagent client %q (use omp, opencode or codex)", client)
	}
	if agent != "" && !validIdentifierish(agent) {
		return nil, fmt.Errorf("invalid agent name %q", agent)
	}
	if thinking != "" {
		switch thinking {
		case "off", "minimal", "low", "medium", "high", "xhigh", "max":
		default:
			return nil, fmt.Errorf("unsupported thinking level %q (use off, minimal, low, medium, high, xhigh, max)", thinking)
		}
	}
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}

	effect, _ := permissionDecision("agent", client)
	if effect == "deny" {
		return nil, fmt.Errorf("subagent client %q is denied by local permission rules (add an allow rule for %s)", client, client)
	}
	if effect == "ask" {
		requestID, _ := s.pendingSubagentRequest(client)
		if requestID == "" {
			requestID = parkRequest("agent", client, "")
		}
		return nil, &PermissionNeededError{RequestID: requestID, Command: client + " subagent"}
	}

	// timeoutSeconds <= 0 means no wall-clock limit: the subagent runs
	// until its harness finishes (still bounded by the MCP request
	// context and the client-side agent budget).
	var timeout time.Duration
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}

	permit, err := acquireSubagent(ctx)
	if err != nil {
		return nil, err
	}
	defer permit.release()

	// A zero timeout leaves ctx unbounded; harness-level budgets still apply.
	var runCtx context.Context
	var cancel context.CancelFunc
	if timeout > 0 {
		runCtx, cancel = context.WithTimeout(ctx, timeout)
	} else {
		runCtx, cancel = context.WithCancel(ctx)
	}
	defer cancel()

	args := subagentArgs(client, rootReal, agent, model, thinking, extraArgs, task)

	started := time.Now()
	cmd := exec.CommandContext(runCtx, client, args...)
	cmd.Dir = rootReal
	var buf bytes.Buffer
	// Tee the subagent output: every line is forwarded live to the progress
	// callback (manager streams it as MCP progress notifications) and kept
	// in the bounded buffer for the final result.
	var events chan string
	if OnSubagentEvent != nil {
		events = make(chan string, 256)
		limited := &lineTee{buf: &buf, max: subagentMaxOutput, onLine: func(line string) {
			select {
			case events <- line:
			default: // drop on backpressure; the final result keeps everything
			}
		}}
		cmd.Stdout = limited
		cmd.Stderr = limited
		go func() {
			for line := range events {
				OnSubagentEvent("agent", line)
			}
		}()
	} else {
		limited := &limitedBuffer{buf: &buf, max: subagentMaxOutput}
		cmd.Stdout = limited
		cmd.Stderr = limited
	}
	runErr := cmd.Run()
	if events != nil {
		close(events)
	}

	result := &SubagentResult{
		Client: client, Agent: agent, Model: model, Thinking: thinking,
		Task: truncateText(task, 400), Elapsed: time.Since(started).Round(time.Millisecond).String(),
	}
	switch {
	case runCtx.Err() == context.DeadlineExceeded:
		result.Status = "timeout"
		result.Output = truncateTail(buf.String(), 2000) + "\n[subagent exceeded the time limit and was killed]"
	case runErr != nil:
		result.Status = "failed"
		result.Output = truncateTail(buf.String(), 2000) + fmt.Sprintf("\n[subagent exited: %v]", runErr)
	default:
		result.Status = "completed"
		result.Output = truncateTail(buf.String(), 4000)
	}
	// JSONL harnesses: lead with the final assistant message, which is what
	// the caller wants; the raw event tail stays available behind it.
	switch client {
	case SubagentClientOpencode:
		if final := lastAssistantMessage(result.Output); final != "" {
			result.Output = final + "\n\n--- raw events tail ---\n" + truncateTail(result.Output, 1200)
		}
	case SubagentClientOMP:
		if final := lastOMPMessage(result.Output); final != "" {
			result.Output = final + "\n\n--- raw events tail ---\n" + truncateTail(result.Output, 1200)
		}
	}
	result.Events = countJSONLines(result.Output)
	return result, nil
}

// lastOMPMessage scans omp JSONL event lines for the last assistant text.
// omp emits {"type":"message_end","message":{"role":"assistant","content":
// [{"type":"text","text":"..."}]}} per turn and a final
// {"type":"agent_end","messages":[...]} for the whole run.
func lastOMPMessage(out string) string {
	final := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var ev struct {
			Type     string            `json:"type"`
			Message  json.RawMessage   `json:"message"`
			Messages []json.RawMessage `json:"messages"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		switch ev.Type {
		case "message_end", "turn_end":
			if text := assistantMessageText(ev.Message); text != "" {
				final = text
			}
		case "agent_end":
			for _, raw := range ev.Messages {
				if text := assistantMessageText(raw); text != "" {
					final = text
				}
			}
		}
	}
	return final
}

// assistantMessageText returns the joined text parts of one assistant message
// and an empty string for every other role or malformed payload.
func assistantMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var msg struct {
		Role    string `json:"role"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if json.Unmarshal(raw, &msg) != nil || msg.Role != "assistant" {
		return ""
	}
	parts := make([]string, 0, len(msg.Content))
	for _, part := range msg.Content {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			parts = append(parts, part.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// subagentArgs builds the headless command line for one harness. The agent
// name only reaches harnesses that have a CLI selector for it (opencode
// --agent, codex --profile); omp picks its working agent from its own
// configuration, so the value is not passed there.
func subagentArgs(client, rootReal, agent, model, thinking string, extraArgs []string, task string) []string {
	var args []string
	switch client {
	case SubagentClientOMP:
		// -p is omp's non-interactive print mode and --mode json emits one
		// JSONL event per line, which the caller streams as progress.
		// --allow-home keeps omp working in the requested directory when that
		// directory is the home tree (the "host" workspace) instead of
		// relocating itself to a temp dir.
		args = []string{"-p", "--mode", "json", "--cwd", rootReal, "--auto-approve", "--allow-home"}
		if model != "" {
			args = append(args, "--model", model)
		}
		if thinking != "" {
			args = append(args, "--thinking", thinking)
		}
		args = append(args, extraArgs...)
		args = append(args, "--", task)
	case SubagentClientOpencode:
		args = []string{"run", "--format", "json", "--auto", "--dir", rootReal}
		if agent != "" {
			args = append(args, "--agent", agent)
		}
		if model != "" {
			args = append(args, "--model", model)
		}
		if thinking != "" && thinking != "off" {
			args = append(args, "--variant", thinking)
		}
		args = append(args, extraArgs...)
		args = append(args, "--", task)
	case SubagentClientCodex:
		args = []string{"exec"}
		if agent != "" {
			args = append(args, "--profile", agent)
		}
		if model != "" {
			args = append(args, "--model", model)
		}
		if thinking != "" && thinking != "off" {
			args = append(args, "-c", "model_reasoning_effort="+thinking)
		}
		args = append(args, extraArgs...)
		args = append(args, task)
	}
	return args
}

// lastAssistantMessage scans opencode JSONL event lines for the last text
// part; opencode emits {"type":"text","part":{"type":"text","text":"..."}}.
func lastAssistantMessage(out string) string {
	final := ""
	for _, line := range strings.Split(out, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] != '{' {
			continue
		}
		var ev struct {
			Type string `json:"type"`
			Part struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"part"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil {
			continue
		}
		if ev.Type == "text" && ev.Part.Type == "text" && strings.TrimSpace(ev.Part.Text) != "" {
			final = ev.Part.Text
		}
	}
	return final
}

func (s *Service) pendingSubagentRequest(client string) (string, string) {
	perms.mu.Lock()
	defer perms.mu.Unlock()
	for id, req := range perms.pending {
		if req.Tool == "agent" && req.Command == client && req.ExpiresAt.After(time.Now()) {
			return id, id
		}
	}
	return "", ""
}

// lineTee is a bounded buffer that also emits each written line to a
// callback, used for live subagent progress streaming.
type lineTee struct {
	buf    *bytes.Buffer
	max    int
	onLine func(string)
	carry  []byte
}

func (t *lineTee) Write(p []byte) (int, error) {
	t.carry = append(t.carry, p...)
	for {
		idx := bytes.IndexByte(t.carry, '\n')
		if idx < 0 {
			break
		}
		line := string(t.carry[:idx])
		t.carry = t.carry[idx+1:]
		if t.onLine != nil && strings.TrimSpace(line) != "" {
			t.onLine(line)
		}
	}
	if t.buf.Len() < t.max {
		remaining := t.max - t.buf.Len()
		if len(p) <= remaining {
			t.buf.Write(p)
		} else {
			t.buf.Write(p[:remaining])
		}
	}
	return len(p), nil
}

func countJSONLines(s string) int {
	n := 0
	for _, line := range strings.Split(s, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var probe json.RawMessage
		if json.Unmarshal([]byte(line), &probe) == nil {
			n++
		}
	}
	return n
}

func validIdentifierish(s string) bool {
	if s == "" {
		return true
	}
	if len(s) > 64 {
		return false
	}
	for _, r := range s {
		if !(r == '_' || r == '-' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')) {
			return false
		}
	}
	return true
}

func truncateText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max] + "…"
}

func truncateTail(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return "…(truncated)\n" + s[len(s)-max:]
}
