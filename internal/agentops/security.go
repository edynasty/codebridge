package agentops

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ResolveUnderRoot resolves a workspace-relative path and rejects lexical or
// symlink-based escapes. Existing targets are evaluated through symlinks before
// the final containment check.
//
// Errors returned from this function intentionally never include the physical
// workspace root. The caller-facing security boundary is the logical workspace
// name plus workspace-relative paths.
func ResolveUnderRoot(root, rel string) (string, error) {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", errors.New("resolve workspace root failed")
	}
	rootReal, err := filepath.EvalSymlinks(rootAbs)
	if err != nil {
		return "", errors.New("workspace root is unavailable")
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
		return "", fmt.Errorf("resolve path %q failed", logicalPath(rel))
	}
	candidate := joinedAbs
	if _, statErr := os.Lstat(joinedAbs); statErr == nil {
		real, evalErr := filepath.EvalSymlinks(joinedAbs)
		if evalErr != nil {
			return "", safePathError("resolve path", rel, evalErr)
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

func safePathError(action, rel string, err error) error {
	path := logicalPath(rel)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("%s %q: not found", action, path)
	case errors.Is(err, fs.ErrPermission):
		return fmt.Errorf("%s %q: permission denied", action, path)
	default:
		return fmt.Errorf("%s %q failed", action, path)
	}
}

func logicalPath(rel string) string {
	rel = strings.TrimSpace(rel)
	if rel == "" {
		return "."
	}
	clean := filepath.Clean(rel)
	if clean == "." {
		return "."
	}
	return filepath.ToSlash(clean)
}

func logicalChildPath(dir, name string) string {
	dir = logicalPath(dir)
	if dir == "." {
		return filepath.ToSlash(name)
	}
	return filepath.ToSlash(filepath.Join(dir, name))
}

func openWorkspaceRoot(root string) (*os.Root, error) {
	handle, err := os.OpenRoot(root)
	if err != nil {
		return nil, errors.New("workspace root is unavailable")
	}
	return handle, nil
}
