package agentops

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

const (
	bashTimeout      = 30 * time.Second
	bashMaxOutput    = 256 * 1024
	maxBashAllowlist = 200
)

// runBash executes an allowlisted command in the workspace root. Security
// model: the command must start with one of the locally configured prefix
// allowlist entries (CODEBRIDGE_BASH_ALLOWLIST, default empty = tool off),
// runs with the workspace as working directory, is killed after a timeout,
// never gets a TTY, and its combined output is bounded. Sensitive-path
// policy applies as everywhere else.
func (s *Service) runBash(ctx context.Context, root, command string) (map[string]any, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return nil, errors.New("command is required")
	}
	effect, pattern := permissionDecision("bash", command)
	if effect == "deny" {
		return nil, fmt.Errorf("command %q is denied by local permission rules", command)
	}
	if effect == "ask" {
		if consumeGrantOnce(command) {
			// A once-grant already approved this exact command; run it.
			rootReal, rootErr := s.resolveWorkspaceRoot(root)
			if rootErr != nil {
				return nil, rootErr
			}
			return s.execBash(ctx, rootReal, command)
		}
		requestID, _ := s.pendingBashRequest(command)
		if requestID == "" {
			requestID = parkRequest("bash", command, "")
		}
		return nil, &PermissionNeededError{RequestID: requestID, Command: command, Pattern: pattern}
	}
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	return s.execBash(ctx, rootReal, command)
}

func (s *Service) execBash(ctx context.Context, rootReal, command string) (map[string]any, error) {
	runCtx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, "bash", "-c", command)
	cmd.Dir = rootReal
	// GIT_DISCOVERY_ACROSS_FILESYSTEM lets allowlisted git commands walk out
	// of the workspace mount to enclosing repositories (macOS bind mounts
	// otherwise stop discovery at the mount boundary).
	cmd.Env = append(os.Environ(), "TERM=dumb", "PAGER=cat",
		"GIT_DISCOVERY_ACROSS_FILESYSTEM=1",
		// Read-only bind mounts present as root-owned trees; let allowlisted
		// git commands read them without dubious-ownership failures.
		"GIT_CONFIG_COUNT=1", "GIT_CONFIG_KEY_0=safe.directory", "GIT_CONFIG_VALUE_0=*")
	var stdout bytes.Buffer
	limited := &limitedBuffer{buf: &stdout, max: bashMaxOutput}
	cmd.Stdout = limited
	cmd.Stderr = limited
	runErr := cmd.Run()
	out := stdout.String()
	result := map[string]any{
		"command":   command,
		"output":    out,
		"truncated": stdout.Len() >= bashMaxOutput,
	}
	if runErr != nil {
		if runCtx.Err() == context.DeadlineExceeded {
			result["error"] = fmt.Sprintf("command exceeded %s and was killed", bashTimeout)
		} else {
			result["error"] = runErr.Error()
		}
		result["exit_code"] = -1
		if exit, ok := runErr.(*exec.ExitError); ok {
			result["exit_code"] = exit.ExitCode()
		}
	}
	return result, nil
}

// PermissionNeededError tells the MCP caller a permission_grant is
// required before the command runs. It carries the request id so the
// assistant can act on it without any UI.
type PermissionNeededError struct {
	RequestID string
	Command   string
	Pattern   string
}

func (e *PermissionNeededError) Error() string {
	return fmt.Sprintf("permission required: call permission_grant with request_id=%s decision=once|always|deny to approve command %q", e.RequestID, e.Command)
}

// pendingBashRequest returns an existing pending request id for the same
// command so a retried call does not mint a new request on every attempt.
func (s *Service) pendingBashRequest(command string) (string, string) {
	perms.mu.Lock()
	defer perms.mu.Unlock()
	for id, req := range perms.pending {
		if req.Tool == "bash" && req.Command == command && req.ExpiresAt.After(time.Now()) {
			return id, id
		}
	}
	return "", ""
}

// consumeGrantOnce checks (and consumes) a once-grant for a command, letting
// a single approved execution pass without a persistent rule.
func consumeGrantOnce(command string) bool {
	perms.mu.Lock()
	defer perms.mu.Unlock()
	for id, ok := range perms.granted {
		if !ok {
			continue
		}
		if req, exists := perms.pending[id]; exists && req.Command == command {
			delete(perms.granted, id)
			delete(perms.pending, id)
			return true
		}
	}
	return false
}
