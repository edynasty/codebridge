package agentops

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

// PermissionRule is one persisted permission decision. Pattern is matched
// against the command's first word plus optional suffix glob:
//   - "" or "*"        -> every command
//   - "git"            -> commands whose first word is git
//   - "git status*"    -> git status ... only
//
// Deny beats allow beats ask (the default for unmatched commands).
type PermissionRule struct {
	Effect  string `json:"effect"` // allow | deny
	Tool    string `json:"tool"`   // "bash" today
	Pattern string `json:"pattern,omitempty"`
	Reason  string `json:"reason,omitempty"`
}

// PermissionRequest is a parked command waiting for permission_grant.
type PermissionRequest struct {
	ID        string    `json:"id"`
	Tool      string    `json:"tool"`
	Command   string    `json:"command"`
	Workspace string    `json:"workspace"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

const permissionRequestTTL = 5 * time.Minute

type permissionStore struct {
	mu      sync.Mutex
	rules   []PermissionRule
	pending map[string]*PermissionRequest
	granted map[string]bool
}

var perms permissionStore

// SetPermissions replaces the active rule set (called on client boot and
// whenever the configuration hot-reloads).
func SetPermissions(rules []PermissionRule) {
	perms.mu.Lock()
	defer perms.mu.Unlock()
	perms.rules = append([]PermissionRule(nil), rules...)
}

func permissionDecision(tool, command string) (effect string, pattern string) {
	fields := strings.Fields(command)
	first := ""
	if len(fields) > 0 {
		first = fields[0]
	}
	effect = "ask"
	perms.mu.Lock()
	defer perms.mu.Unlock()
	for _, rule := range perms.rules {
		if rule.Tool != "" && rule.Tool != tool {
			continue
		}
		if ruleMatches(rule.Pattern, command, first) {
			// Deny wins over allow; first matching rule in file order wins
			// within the same effect.
			if rule.Effect == "deny" {
				return "deny", rule.Pattern
			}
			if effect != "deny" {
				effect = "allow"
				pattern = rule.Pattern
			}
		}
	}
	return effect, pattern
}

// ruleMatches reports whether a glob-ish pattern covers the command. A bare
// word matches the first word; a trailing "*" allows any arguments after the
// prefix.
func ruleMatches(pattern, command, firstWord string) bool {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" || pattern == "*" {
		return true
	}
	prefix := strings.TrimSuffix(pattern, "*")
	if prefix == pattern {
		return firstWord == pattern
	}
	return strings.HasPrefix(command, prefix)
}

// parkRequest registers a pending permission request and returns its ID.
func parkRequest(tool, command, workspace string) string {
	var b [8]byte
	_, _ = rand.Read(b[:])
	id := "pr_" + hex.EncodeToString(b[:])
	perms.mu.Lock()
	defer perms.mu.Unlock()
	if perms.pending == nil {
		perms.pending = map[string]*PermissionRequest{}
	}
	now := time.Now()
	perms.pending[id] = &PermissionRequest{
		ID: id, Tool: tool, Command: command, Workspace: workspace,
		CreatedAt: now, ExpiresAt: now.Add(permissionRequestTTL),
	}
	// Opportunistic cleanup of expired requests.
	for k, v := range perms.pending {
		if v.ExpiresAt.Before(now) {
			delete(perms.pending, k)
		}
	}
	return id
}

func lookupRequest(id string) (*PermissionRequest, error) {
	perms.mu.Lock()
	defer perms.mu.Unlock()
	req, ok := perms.pending[id]
	if !ok {
		return nil, fmt.Errorf("permission request %q not found or expired", id)
	}
	if req.ExpiresAt.Before(time.Now()) {
		delete(perms.pending, id)
		return nil, fmt.Errorf("permission request %q expired", id)
	}
	return req, nil
}

// PermissionRuleForRequest lets the caller persist an always-rule after a
// grant; exposed for the config layer to save.
func PermissionRuleForRequest(req *PermissionRequest) PermissionRule {
	first := strings.Fields(req.Command)
	pattern := "*"
	if len(first) > 0 {
		pattern = first[0] + " *"
	}
	return PermissionRule{Effect: "allow", Tool: req.Tool, Pattern: pattern}
}

// permissionGrant executes a grant decision: once re-runs nothing (the
// caller retries the tool call and finds a one-shot grant), always persists
// a rule, deny clears the request.
func (s *Service) permissionGrant(requestID, decision string) (map[string]any, error) {
	if requestID == "" {
		return nil, errors.New("request_id is required")
	}
	req, err := lookupRequest(requestID)
	if err != nil {
		return nil, err
	}
	switch strings.TrimSpace(strings.ToLower(decision)) {
	case "once":
		perms.mu.Lock()
		if perms.granted == nil {
			perms.granted = map[string]bool{}
		}
		perms.granted[requestID] = true
		perms.mu.Unlock()
		return map[string]any{"request_id": requestID, "granted": "once", "message": "retry the blocked tool call now"}, nil
	case "always":
		rule := PermissionRuleForRequest(req)
		perms.mu.Lock()
		perms.rules = append(perms.rules, rule)
		delete(perms.pending, requestID)
		perms.mu.Unlock()
		if s.onRulePersisted != nil {
			s.onRulePersisted(append([]PermissionRule(nil), perms.rules...))
		}
		return map[string]any{"request_id": requestID, "granted": "always", "rule": rule, "message": "rule persisted; retry the blocked tool call now"}, nil
	case "deny":
		perms.mu.Lock()
		delete(perms.pending, requestID)
		delete(perms.granted, requestID)
		perms.mu.Unlock()
		return map[string]any{"request_id": requestID, "granted": "denied"}, nil
	default:
		return nil, fmt.Errorf("decision must be once, always, or deny")
	}
}

// consumeGrant reports whether a once-granted request has been used, and
// consumes it.
func consumeGrant(requestID string) bool {
	perms.mu.Lock()
	defer perms.mu.Unlock()
	if perms.granted[requestID] {
		delete(perms.granted, requestID)
		return true
	}
	return false
}
