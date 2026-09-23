package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/edynasty/codebridge/internal/protocol"
)

// ParseWorkspaces parses CODEBRIDGE_WORKSPACES in the form:
//
//	pms=/Users/me/code/pms,portal=/Users/me/code/portal
func ParseWorkspaces(raw string) (map[string]string, []protocol.Workspace, error) {
	roots := map[string]string{}
	var advertised []protocol.Workspace
	if strings.TrimSpace(raw) == "" {
		return nil, nil, fmt.Errorf("CODEBRIDGE_WORKSPACES is required")
	}
	for _, item := range strings.Split(raw, ",") {
		parts := strings.SplitN(strings.TrimSpace(item), "=", 2)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return nil, nil, fmt.Errorf("invalid workspace entry %q; expected name=/absolute/path", item)
		}
		name := strings.TrimSpace(parts[0])
		path := strings.TrimSpace(parts[1])
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
		// Do not expose the physical local path to ChatGPT/Manager users.
		advertised = append(advertised, protocol.Workspace{Name: name})
	}
	return roots, advertised, nil
}
