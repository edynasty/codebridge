package agentops

import (
	"fmt"
	"path/filepath"
	"strings"
)

var sensitiveDirNames = map[string]struct{}{
	".git":     {},
	".ssh":     {},
	".gnupg":   {},
	".aws":     {},
	".azure":   {},
	".kube":    {},
	".docker":  {},
	".secrets": {},
}

var sensitiveBaseNames = map[string]struct{}{
	".envrc":           {},
	".npmrc":           {},
	".pypirc":          {},
	".netrc":           {},
	".git-credentials": {},
	"id_rsa":           {},
	"id_dsa":           {},
	"id_ecdsa":         {},
	"id_ed25519":       {},
}

func isSensitivePath(rel string) bool {
	clean := logicalPath(rel)
	if clean == "." {
		return false
	}

	parts := strings.Split(strings.ToLower(filepath.ToSlash(clean)), "/")
	for _, part := range parts {
		if _, ok := sensitiveDirNames[part]; ok {
			return true
		}
	}

	base := parts[len(parts)-1]
	if _, ok := sensitiveBaseNames[base]; ok {
		return true
	}
	if base == ".env" {
		return true
	}
	if strings.HasPrefix(base, ".env.") && !isSafeEnvExample(base) {
		return true
	}
	if strings.HasSuffix(base, ".tfstate") ||
		strings.Contains(base, ".tfstate.") ||
		strings.HasSuffix(base, ".tfvars") ||
		strings.HasSuffix(base, ".tfvars.json") {
		return true
	}

	switch strings.ToLower(filepath.Ext(base)) {
	case ".pem", ".key", ".p12", ".pfx", ".jks", ".keystore":
		return true
	default:
		return false
	}
}

func isSafeEnvExample(base string) bool {
	switch base {
	case ".env.example", ".env.sample", ".env.template", ".env.dist":
		return true
	default:
		return false
	}
}

func sensitivePathError(rel string) error {
	return fmt.Errorf("sensitive path %q is blocked by local policy", logicalPath(rel))
}

func (s *Service) resolveWorkspaceRoot(root string) (string, error) {
	rootReal, err := ResolveUnderRoot(root, ".")
	if err != nil {
		return "", err
	}
	if !s.AllowSensitiveFiles && isSensitivePath(filepath.Base(rootReal)) {
		return "", fmt.Errorf("workspace root is blocked by local sensitive-file policy")
	}
	return rootReal, nil
}

func (s *Service) resolveAllowedPath(root, rel string) (string, error) {
	if !s.AllowSensitiveFiles && isSensitivePath(rel) {
		return "", sensitivePathError(rel)
	}
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return "", err
	}
	target, err := ResolveUnderRoot(root, rel)
	if err != nil {
		return "", err
	}
	if s.AllowSensitiveFiles {
		return target, nil
	}
	resolvedRel, err := filepath.Rel(rootReal, target)
	if err != nil {
		return "", fmt.Errorf("resolve workspace-relative path failed")
	}
	resolvedRel = filepath.ToSlash(resolvedRel)
	if isSensitivePath(resolvedRel) {
		return "", sensitivePathError(rel)
	}
	return target, nil
}
