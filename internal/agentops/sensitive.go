package agentops

import (
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
