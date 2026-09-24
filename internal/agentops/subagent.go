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
	subagentDefaultTimeout = 10 * time.Minute
	subagentMaxTimeout     = 30 * time.Minute
	subagentMaxOutput      = 256 * 1024
	subagentMaxConcurrent  = 2
)

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
// and returns its output. Supported clients: opencode (opencode run) and
// codex (codex exec). Gated by the same permission rules as bash: the
// operator must allow the client binary first.
// SubagentProfileConfig is the exported wire form of a profile (used by the
// UI config API); it mirrors clientconfig.SubagentProfile.
type SubagentProfileConfig struct {
	Name       string   `json:"name"`
	Client     string   `json:"client,omitempty"`
	Agent      string   `json:"agent,omitempty"`
	Model      string   `json:"model,omitempty"`
	Thinking   string   `json:"thinking,omitempty"`
	TimeoutSec int      `json:"timeout_seconds,omitempty"`
	ExtraArgs  []string `json:"extra_args,omitempty"`
}

// SubagentProfiles is the profile table the client injects at boot; the
// MCP agent tool resolves its agent/agent_profile parameter against it.
var SubagentProfiles []SubagentProfile

// SubagentProfile mirrors clientconfig.SubagentProfile; duplicated to keep
// agentops free of a config dependency.
type SubagentProfile struct {
	Name       string
	Client     string
	Agent      string
	Model      string
	Thinking   string
	TimeoutSec int
	ExtraArgs  []string
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
				client = orDefault(p.Client, "opencode")
				agent = p.Agent
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
		client = "opencode"
	}
	return client, agent, model, thinking, timeoutSeconds, nil
}

func orDefault(v, def string) string {
	if strings.TrimSpace(v) == "" {
		return def
	}
	return v
}

func (s *Service) runSubagent(ctx context.Context, root, task, client, agent, model, thinking string, timeoutSeconds int) (*SubagentResult, error) {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil, errors.New("task is required")
	}
	var extraArgs []string
	client, agent, model, thinking, timeoutSeconds, extraArgs = resolveSubagentCall(client, agent, model, thinking, timeoutSeconds)
	client = strings.TrimSpace(client)
	if client == "" {
		client = "opencode"
	}
	switch client {
	case "opencode", "codex":
	default:
		return nil, fmt.Errorf("unsupported subagent client %q (use opencode or codex)", client)
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

	timeout := subagentDefaultTimeout
	if timeoutSeconds > 0 {
		timeout = time.Duration(timeoutSeconds) * time.Second
	}
	if timeout > subagentMaxTimeout {
		timeout = subagentMaxTimeout
	}

	permit, err := acquireSubagent(ctx)
	if err != nil {
		return nil, err
	}
	defer permit.release()

	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	var args []string
	switch client {
	case "opencode":
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
	case "codex":
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

	started := time.Now()
	cmd := exec.CommandContext(runCtx, client, args...)
	cmd.Dir = rootReal
	var buf bytes.Buffer
	limited := &limitedBuffer{buf: &buf, max: subagentMaxOutput}
	cmd.Stdout = limited
	cmd.Stderr = limited
	runErr := cmd.Run()

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
	// opencode --format json emits JSONL events; keep the raw tail but lead
	// with the final assistant message, which is what the caller wants.
	if client == "opencode" {
		if final := lastAssistantMessage(result.Output); final != "" {
			result.Output = final + "\n\n--- raw events tail ---\n" + truncateTail(result.Output, 1200)
		}
	}
	result.Events = countJSONLines(result.Output)
	return result, nil
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
