package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/edynasty/codebridge/internal/protocol"
)

// ParseWorkspaces parses CODEBRIDGE_WORKSPACES in the form:
//
//	pms=/Users/me/code/pms,portal=/Users/me/code/portal
//
// writable names mark workspaces with explicit local write opt-in.
func ParseWorkspaces(raw string, writable map[string]bool) (map[string]string, []protocol.Workspace, error) {
	entries := map[string]string{}
	if strings.TrimSpace(raw) == "" {
		return nil, nil, fmt.Errorf("CODEBRIDGE_WORKSPACES is required")
	}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, nil, fmt.Errorf("invalid workspace entry %q; expected name=/absolute/path", item)
		}
		name := strings.TrimSpace(parts[0])
		if _, exists := entries[name]; exists {
			return nil, nil, fmt.Errorf("duplicate workspace name %q", name)
		}
		entries[name] = strings.TrimSpace(parts[1])
	}
	return ParseWorkspaceMap(entries, writable)
}

// ParseWorkspaceMap validates workspace names and local roots while advertising
// only logical names upstream. JSON config files use this form so filesystem
// paths can safely contain commas. writable names mark workspaces with
// explicit local write opt-in.
func ParseWorkspaceMap(entries map[string]string, writable map[string]bool) (map[string]string, []protocol.Workspace, error) {
	if writable == nil {
		writable = map[string]bool{}
	}
	if len(entries) == 0 {
		return nil, nil, fmt.Errorf("at least one workspace is required")
	}
	for name := range writable {
		if _, exists := entries[name]; !exists {
			return nil, nil, fmt.Errorf("writable workspace %q is not a configured workspace", name)
		}
	}
	names := make([]string, 0, len(entries))
	for rawName := range entries {
		names = append(names, rawName)
	}
	sort.Strings(names)

	roots := make(map[string]string, len(entries))
	advertised := make([]protocol.Workspace, 0, len(entries))
	for _, rawName := range names {
		name := strings.TrimSpace(rawName)
		path := strings.TrimSpace(entries[rawName])
		if name == "" || path == "" {
			return nil, nil, fmt.Errorf("workspace name and path are required")
		}
		if len(name) > 128 {
			return nil, nil, fmt.Errorf("workspace name %q exceeds 128 bytes", name)
		}
		if name != rawName {
			if _, exists := entries[name]; exists {
				return nil, nil, fmt.Errorf("duplicate workspace name %q", name)
			}
		}
		abs, err := filepath.Abs(path)
		if err != nil {
			return nil, nil, fmt.Errorf("workspace %s: %w", name, err)
		}
		st, err := os.Stat(abs)
		if err != nil {
			return nil, nil, fmt.Errorf("workspace %s: %w", name, err)
		}
		if !st.IsDir() {
			return nil, nil, fmt.Errorf("workspace %s path is not a directory", name)
		}
		if _, exists := roots[name]; exists {
			return nil, nil, fmt.Errorf("duplicate workspace name %q", name)
		}
		roots[name] = abs
		advertised = append(advertised, protocol.Workspace{Name: name, Writable: writable[name]})
	}
	return roots, advertised, nil
}
