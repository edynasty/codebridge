package agentops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ResolveUnderRoot resolves a workspace-relative path and rejects lexical or
// symlink-based escapes. Existing targets are evaluated through symlinks before
// the final containment check.
func ResolveUnderRoot(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}

	if rel == "" || rel == "." {
		return rootReal, nil
	}
	if filepath.IsAbs(rel) {
		return "", fmt.Errorf("absolute paths are not allowed")
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes workspace")
	}

	joined := filepath.Join(rootReal, clean)
	joinedAbs, err := filepath.Abs(joined)
	if err != nil {
		return "", err
	}
	candidate := joinedAbs
	if _, statErr := os.Lstat(joinedAbs); statErr == nil {
		real, evalErr := filepath.EvalSymlinks(joinedAbs)
		if evalErr != nil {
			return "", evalErr
		}
		candidate = real
	}
	if !contained(rootReal, candidate) {
		return "", fmt.Errorf("path escapes workspace")
	}
	return candidate, nil
}

func contained(root, target string) bool {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
