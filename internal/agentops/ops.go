package agentops

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/edynasty/codebridge/internal/protocol"
)

const (
	maxReadBytes        = 256 * 1024
	maxOutputBytes      = 512 * 1024
	maxFindResults      = 500
	maxSearchLines      = 200
	maxDirectoryEntries = 1000
)

type Service struct {
	Roots map[string]string
	// AllowSensitiveFiles is a local-only relaxation of the sensitive-file
	// read policy; it never enables writes to sensitive paths.
	AllowSensitiveFiles bool
	// Writable lists workspace names with explicit local write opt-in.
	Writable map[string]bool
	// IndexDir/CheckpointDir are local-only caches on the agent machine.
	IndexDir      string
	CheckpointDir string
	// EnableLSP opts into local language-server usage for symbol tools.
	EnableLSP bool
	// BashAllowlist is retained for backward compatibility; permission rules
	// are the authoritative surface now.
	BashAllowlist []string

	// onRulePersisted persists permission rules after an always-grant.
	onRulePersisted func(rules []PermissionRule)

	lspMu      sync.Mutex
	lspClients map[string]*lspClient
}

type DirEntry struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Type string `json:"type"`
	Size int64  `json:"size,omitempty"`
}

type ReadFileResult struct {
	Path      string `json:"path"`
	Content   string `json:"content"`
	Truncated bool   `json:"truncated"`
	BytesRead int    `json:"bytes_read"`
}

type SearchMatch struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

// SetRulePersister installs the callback persisting permission rules after
// an always-grant; the client wires it to its config file.
func (s *Service) SetRulePersister(fn func(rules []PermissionRule)) {
	s.onRulePersisted = fn
}

func (s *Service) Execute(ctx context.Context, req protocol.AgentRequest) (any, error) {
	// Catalog lookups are device-wide and need no workspace.
	if req.Tool == "agents_list" {
		return map[string]any{"agents": AgentCatalog()}, nil
	}
	root, ok := s.Roots[req.Workspace]
	if !ok {
		return nil, fmt.Errorf("unknown workspace %q", req.Workspace)
	}
	switch req.Tool {
	case "list", "list_directory":
		return s.listDirectory(root, stringArg(req.Args, "path", "."))
	case "read", "read_file":
		maxBytes := intArg(req.Args, "max_bytes", maxReadBytes)
		if maxBytes <= 0 {
			maxBytes = maxReadBytes
		}
		return s.readFile(root, stringArg(req.Args, "path", ""), maxBytes)
	case "bash":
		return s.runBash(ctx, root, stringArg(req.Args, "command", ""))
	case "edit":
		return s.applyPatch(ctx, req.Workspace, root, []FileEdit{{
			Path:    stringArg(req.Args, "path", ""),
			OldText: stringArg(req.Args, "old_text", ""),
			NewText: stringArg(req.Args, "new_text", ""),
		}}, boolArg(req.Args, "preview", false), boolArg(req.Args, "confirm", false))
	case "write":
		return s.applyPatch(ctx, req.Workspace, root, []FileEdit{{
			Path:    stringArg(req.Args, "path", ""),
			NewText: stringArg(req.Args, "content", ""),
		}}, boolArg(req.Args, "preview", false), boolArg(req.Args, "confirm", false))
	case "permission_grant":
		return s.permissionGrant(stringArg(req.Args, "request_id", ""), stringArg(req.Args, "decision", ""))
	case "agent":
		// The client parameter may name a subagent profile configured in the
		// local UI (extra CLI flags, model, effort); profiles resolve in
		// resolveSubagentCall.
		return s.runSubagent(ctx, root,
			stringArg(req.Args, "task", ""),
			stringArg(req.Args, "client", ""),
			stringArg(req.Args, "agent", ""),
			stringArg(req.Args, "model", ""),
			stringArg(req.Args, "thinking", ""),
			intArg(req.Args, "timeout_seconds", 0))

	// Legacy aliases: no longer advertised by the manager but still routed so
	// existing custom tool wrappers and boundary tests keep working.
	case "find_files":
		return s.findFiles(root, stringArg(req.Args, "pattern", ""))
	case "search_code":
		return s.searchCode(ctx, root, stringArg(req.Args, "query", ""), stringArg(req.Args, "path", "."))
	case "git_status":
		return s.gitStatus(ctx, root)
	case "git_diff":
		return s.gitDiff(ctx, root, stringArg(req.Args, "path", ""))
	case "project_info":
		return s.projectInfo(root)
	case "find_symbol":
		return s.findSymbol(ctx, root, req.Workspace, stringArg(req.Args, "pattern", ""), stringArg(req.Args, "kind", ""))
	case "find_references":
		return s.findReferences(ctx, root, req.Workspace, stringArg(req.Args, "symbol", ""), stringArg(req.Args, "path", ""))
	case "read_symbol":
		return s.readSymbol(root, stringArg(req.Args, "path", ""), stringArg(req.Args, "symbol", ""), intArg(req.Args, "max_lines", 512))
	case "dependency_graph":
		return s.dependencyGraph(ctx, root)
	case "apply_patch":
		return s.applyPatch(ctx, req.Workspace, root, parseFileEdits(req.Args["edits"]), boolArg(req.Args, "preview", false), boolArg(req.Args, "confirm", false))
	case "rollback_patch":
		return s.rollbackPatch(ctx, req.Workspace, root, stringArg(req.Args, "checkpoint_id", ""))
	default:
		return nil, fmt.Errorf("unsupported tool %q", req.Tool)
	}
}

// findReferences resolves references to a symbol. It prefers a local LSP
// server when enabled and available, and falls back to bounded word-boundary
// textual matching otherwise.
func (s *Service) findReferences(ctx context.Context, root, workspace, symbol, rel string) (any, error) {
	symbol = strings.TrimSpace(symbol)
	if !validIdentifier(symbol) {
		return nil, errSymbolRequired("symbol")
	}
	if rel != "" {
		hits, useLSP := s.referencesViaLSP(ctx, root, workspace, rel, symbol)
		if len(hits) > 0 {
			return map[string]any{"engine": refEngine(useLSP), "references": hits}, nil
		}
	}
	hits, err := s.findReferencesTextual(ctx, root, symbol)
	if err != nil {
		return nil, err
	}
	return map[string]any{"engine": "textual", "references": hits}, nil
}

func (s *Service) referencesViaLSP(ctx context.Context, root, workspace, rel, symbol string) ([]SymbolHit, bool) {
	lang := languageForFile(rel)
	if lang == "" {
		return nil, false
	}
	client := s.lspClientFor(ctx, workspace, root, lang)
	if client == nil {
		return nil, false
	}
	abs, err := s.resolveAllowedPath(root, rel)
	if err != nil {
		return nil, false
	}
	data, err := os.ReadFile(abs)
	if err != nil || len(data) > maxSymbolFileBytes {
		return nil, false
	}
	syms := parseSymbols(logicalPath(rel), data)
	lines := strings.Split(string(data), "\n")
	for _, sym := range syms {
		if sym.Name != symbol || sym.Line-1 >= len(lines) {
			continue
		}
		byteIdx := strings.Index(lines[sym.Line-1], symbol)
		if byteIdx < 0 {
			continue
		}
		col := utf16Column(lines[sym.Line-1], byteIdx)
		locations, err := client.references(ctx, abs, sym.Line-1, col, true)
		if err != nil {
			return nil, false
		}
		hits := make([]SymbolHit, 0, len(locations))
		for _, loc := range locations {
			absRef := strings.TrimPrefix(loc.URI, "file://")
			refRel, relErr := filepath.Rel(root, absRef)
			if relErr != nil || (!s.AllowSensitiveFiles && isSensitivePath(filepath.ToSlash(refRel))) {
				continue
			}
			hits = append(hits, SymbolHit{Path: filepath.ToSlash(refRel), Line: loc.Range.Start.Line + 1})
		}
		return hits, true
	}
	return nil, false
}

func refEngine(useLSP bool) string {
	if useLSP {
		return "lsp"
	}
	return "textual"
}

func parseFileEdits(raw any) []FileEdit {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	edits := make([]FileEdit, 0, len(items))
	for _, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		edit := FileEdit{
			Path:    stringArg(m, "path", ""),
			OldText: stringArg(m, "old_text", ""),
			NewText: stringArg(m, "new_text", ""),
		}
		if edit.Path != "" {
			edits = append(edits, edit)
		}
	}
	return edits
}

func boolArg(m map[string]any, key string, def bool) bool {
	if v, ok := m[key].(bool); ok {
		return v
	}
	return def
}

func (s *Service) listDirectory(root, rel string) ([]DirEntry, error) {
	if _, err := s.resolveAllowedPath(root, rel); err != nil {
		return nil, err
	}
	rootHandle, err := openWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	defer rootHandle.Close()
	dir, err := rootHandle.Open(logicalPath(rel))
	if err != nil {
		return nil, safePathError("list directory", rel, err)
	}
	defer dir.Close()
	entries, err := dir.ReadDir(-1)
	if err != nil {
		return nil, safePathError("list directory", rel, err)
	}
	if len(entries) > maxDirectoryEntries {
		return nil, fmt.Errorf("directory contains %d entries; limit is %d, narrow the path", len(entries), maxDirectoryEntries)
	}
	out := make([]DirEntry, 0, len(entries))
	for _, e := range entries {
		info, err := e.Info()
		if err != nil {
			continue
		}
		childPath := logicalChildPath(rel, e.Name())
		if !s.AllowSensitiveFiles && isSensitivePath(childPath) {
			continue
		}
		typ := "file"
		if e.IsDir() {
			typ = "dir"
		} else if info.Mode()&os.ModeSymlink != 0 {
			typ = "symlink"
		}
		out = append(out, DirEntry{Name: e.Name(), Path: childPath, Type: typ, Size: info.Size()})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type == "dir"
		}
		return strings.ToLower(out[i].Name) < strings.ToLower(out[j].Name)
	})
	return out, nil
}

func (s *Service) readFile(root, rel string, limit int) (ReadFileResult, error) {
	if rel == "" {
		return ReadFileResult{}, errors.New("path is required")
	}
	if limit <= 0 || limit > maxReadBytes {
		limit = maxReadBytes
	}
	if _, err := s.resolveAllowedPath(root, rel); err != nil {
		return ReadFileResult{}, err
	}
	rootHandle, err := openWorkspaceRoot(root)
	if err != nil {
		return ReadFileResult{}, err
	}
	defer rootHandle.Close()
	f, err := rootHandle.Open(logicalPath(rel))
	if err != nil {
		return ReadFileResult{}, safePathError("read file", rel, err)
	}
	defer f.Close()
	buf, err := io.ReadAll(io.LimitReader(f, int64(limit+1)))
	if err != nil {
		return ReadFileResult{}, fmt.Errorf("read file %q failed", logicalPath(rel))
	}
	truncated := len(buf) > limit
	if truncated {
		buf = buf[:limit]
	}
	return ReadFileResult{Path: logicalPath(rel), Content: string(buf), Truncated: truncated, BytesRead: len(buf)}, nil
}

func (s *Service) findFiles(root, pattern string) ([]string, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errors.New("pattern is required")
	}
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(pattern)
	var out []string
	err = filepath.WalkDir(rootReal, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		rel, err := filepath.Rel(rootReal, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != rootReal && (shouldSkipDir(d.Name()) || (!s.AllowSensitiveFiles && isSensitivePath(rel))) {
				return filepath.SkipDir
			}
			return nil
		}
		if !s.AllowSensitiveFiles && isSensitivePath(rel) {
			return nil
		}
		matched := strings.Contains(strings.ToLower(rel), needle)
		if strings.Contains(pattern, "*") || strings.Contains(pattern, "?") {
			matched, _ = filepath.Match(pattern, d.Name())
		}
		if matched {
			out = append(out, rel)
			if len(out) >= maxFindResults {
				return fs.SkipAll
			}
		}
		return nil
	})
	return out, err
}

func (s *Service) searchCode(ctx context.Context, root, query, rel string) ([]SearchMatch, error) {
	if strings.TrimSpace(query) == "" {
		return nil, errors.New("query is required")
	}
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return nil, err
	}
	target, err := s.resolveAllowedPath(root, rel)
	if err != nil {
		return nil, err
	}
	if _, err := exec.LookPath("rg"); err == nil {
		ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, "rg", "--fixed-strings", "--line-number", "--no-heading", "--color", "never", "--max-count", "20", "--", query, target)
		cmd.Dir = rootReal
		var stdout bytes.Buffer
		cmd.Stdout = &limitedBuffer{buf: &stdout, max: maxOutputBytes}
		cmd.Stderr = io.Discard
		err := cmd.Run()
		if err != nil {
			if exit, ok := err.(*exec.ExitError); ok && exit.ExitCode() == 1 {
				return []SearchMatch{}, nil
			}
			return nil, err
		}
		return parseRG(rootReal, stdout.String(), s.AllowSensitiveFiles), nil
	}
	return fallbackSearch(ctx, rootReal, target, query, s.AllowSensitiveFiles)
}

func parseRG(root, output string, allowSensitive bool) []SearchMatch {
	var out []SearchMatch
	s := bufio.NewScanner(strings.NewReader(output))
	for s.Scan() {
		line := s.Text()
		parts := strings.SplitN(line, ":", 3)
		if len(parts) != 3 {
			continue
		}
		n, _ := strconv.Atoi(parts[1])
		path := parts[0]
		rel, err := filepath.Rel(root, path)
		if err != nil {
			continue
		}
		rel = filepath.ToSlash(rel)
		if !allowSensitive && isSensitivePath(rel) {
			continue
		}
		out = append(out, SearchMatch{Path: rel, Line: n, Text: parts[2]})
		if len(out) >= maxSearchLines {
			break
		}
	}
	return out
}

func fallbackSearch(ctx context.Context, root, target, query string, allowSensitive bool) ([]SearchMatch, error) {
	var out []SearchMatch
	err := filepath.WalkDir(target, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		if d.IsDir() {
			if path != target && (shouldSkipDir(d.Name()) || (!allowSensitive && isSensitivePath(rel))) {
				return filepath.SkipDir
			}
			return nil
		}
		if !allowSensitive && isSensitivePath(rel) {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > 2*1024*1024 {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		s := bufio.NewScanner(f)
		lineNo := 0
		for s.Scan() {
			lineNo++
			if strings.Contains(s.Text(), query) {
				out = append(out, SearchMatch{Path: rel, Line: lineNo, Text: s.Text()})
				if len(out) >= maxSearchLines {
					_ = f.Close()
					return fs.SkipAll
				}
			}
		}
		_ = f.Close()
		return nil
	})
	return out, err
}

func (s *Service) gitStatus(ctx context.Context, root string) (map[string]any, error) {
	result, err := gitRun(ctx, root, "status", "--short", "--branch")
	if err != nil || s.AllowSensitiveFiles {
		return result, err
	}
	output, _ := result["output"].(string)
	result["output"] = filterGitStatus(output)
	return result, nil
}

func filterGitStatus(output string) string {
	lines := strings.Split(output, "\n")
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "## ") {
			filtered = append(filtered, line)
			continue
		}
		if len(line) < 3 {
			filtered = append(filtered, line)
			continue
		}
		pathField := strings.TrimSpace(line[3:])
		sensitive := false
		for _, path := range strings.Split(pathField, " -> ") {
			path = strings.Trim(path, "\"")
			if isSensitivePath(path) {
				sensitive = true
				break
			}
		}
		if !sensitive {
			filtered = append(filtered, line)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return strings.Join(filtered, "\n") + "\n"
}

func (s *Service) gitDiff(ctx context.Context, root, rel string) (map[string]any, error) {
	if _, err := s.resolveWorkspaceRoot(root); err != nil {
		return nil, err
	}
	if rel != "" {
		if _, err := s.resolveAllowedPath(root, rel); err != nil {
			return nil, err
		}
	}

	nameArgs := []string{"diff", "--name-only", "-z", "--"}
	if rel != "" {
		nameArgs = append(nameArgs, rel)
	}
	names, err := gitOutput(ctx, root, nameArgs...)
	if err != nil {
		return nil, err
	}

	var safe []string
	for _, name := range strings.Split(names, "\x00") {
		if name == "" {
			continue
		}
		if !s.AllowSensitiveFiles && isSensitivePath(name) {
			continue
		}
		safe = append(safe, name)
	}
	if len(safe) == 0 {
		return map[string]any{"output": "", "truncated": false}, nil
	}

	args := []string{"diff", "--no-ext-diff", "--no-textconv", "--"}
	args = append(args, safe...)
	return gitRun(ctx, root, args...)
}

func gitOutput(ctx context.Context, root string, args ...string) (string, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return "", fmt.Errorf("git not found in PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	gitArgs := append([]string{"-c", "core.fsmonitor=false", "-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat")
	var stdout bytes.Buffer
	cmd.Stdout = &limitedBuffer{buf: &stdout, max: maxOutputBytes}
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git command failed: %v", err)
	}
	return stdout.String(), nil
}

func (s *Service) projectInfo(root string) (map[string]any, error) {
	markers := []string{"go.mod", "pom.xml", "build.gradle", "build.gradle.kts", "package.json", "pyproject.toml", "Cargo.toml", "requirements.txt", "Makefile", "Dockerfile", "docker-compose.yml", "compose.yml"}
	found := []string{}
	for _, name := range markers {
		if _, err := os.Stat(filepath.Join(root, name)); err == nil {
			found = append(found, name)
		}
	}
	return map[string]any{"markers": found, "has_git": dirExists(filepath.Join(root, ".git"))}, nil
}

func gitRun(ctx context.Context, root string, args ...string) (map[string]any, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, fmt.Errorf("git not found in PATH")
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	gitArgs := append([]string{"-c", "core.fsmonitor=false", "-C", root}, args...)
	cmd := exec.CommandContext(ctx, "git", gitArgs...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0", "GIT_PAGER=cat")
	var stdout bytes.Buffer
	cmd.Stdout = &limitedBuffer{buf: &stdout, max: maxOutputBytes}
	cmd.Stderr = io.Discard
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("git command failed: %v", err)
	}
	return map[string]any{"output": stdout.String(), "truncated": stdout.Len() >= maxOutputBytes}, nil
}

type limitedBuffer struct {
	buf *bytes.Buffer
	max int
}

func (w *limitedBuffer) Write(p []byte) (int, error) {
	orig := len(p)
	remaining := w.max - w.buf.Len()
	if remaining > 0 {
		if len(p) > remaining {
			p = p[:remaining]
		}
		_, _ = w.buf.Write(p)
	}
	return orig, nil
}

func stringArg(m map[string]any, key, def string) string {
	if v, ok := m[key].(string); ok {
		return v
	}
	return def
}

func intArg(m map[string]any, key string, def int) int {
	switch v := m[key].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		i, _ := v.Int64()
		return int(i)
	default:
		return def
	}
}

// shouldSkipDir lists directories the walk-based tools never descend into:
// dependency/build caches and, for whole-host workspaces, OS system trees
// that are huge, slow to scan, and never contain user source code.
func shouldSkipDir(name string) bool {
	switch name {
	case ".git", "node_modules", "target", "dist", "build", ".idea", ".vscode", ".next", ".gradle",
		"Library", "System", "Applications", "usr", "var", "opt", "proc", "sys", "dev", "run", "Volumes", ".Trash":
		return true
	default:
		return false
	}
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
