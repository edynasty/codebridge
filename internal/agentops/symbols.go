package agentops

import (
	"bufio"
	"context"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

const (
	maxSymbolResults   = 200
	maxSymbolFileBytes = 2 * 1024 * 1024
	maxReferenceLines  = 200
)

// Symbol is a language-neutral declaration extracted from a source file.
type Symbol struct {
	Name      string `json:"name"`
	Kind      string `json:"kind"`
	Path      string `json:"path"`
	Line      int    `json:"line"`
	EndLine   int    `json:"end_line,omitempty"`
	Signature string `json:"signature,omitempty"`
	Container string `json:"container,omitempty"`
	Language  string `json:"language,omitempty"`
}

// SymbolHit pairs a reference location with the line text.
type SymbolHit struct {
	Path string `json:"path"`
	Line int    `json:"line"`
	Text string `json:"text"`
}

type symbolMatch struct {
	name      string
	kind      string
	container string
	hasBody   bool
}

var (
	goFuncRe    = regexp.MustCompile(`^func\s+(?:\(\s*\w+\s+\*?\s*(\w+)\s*\)\s+)?(\w+)\s*\(`)
	goTypeRe    = regexp.MustCompile(`^type\s+(\w+)\s+(struct|interface)\s*\{`)
	goTypeAlias = regexp.MustCompile(`^type\s+(\w+)(?:\s+([^\s{=]+))?\s*(?:=|$)`)
	goConstRe   = regexp.MustCompile(`^(?:const|var)\s+(\w+)`)

	javaTypeRe = regexp.MustCompile(`^\s*(?:public\s+|private\s+|protected\s+|static\s+|final\s+|abstract\s+|sealed\s+|non-sealed\s+|strictfp\s+)*(class|interface|enum|record)\s+(\w+)`)
	javaMemRe  = regexp.MustCompile(`^\s+(?:public\s+|private\s+|protected\s+|static\s+|final\s+|abstract\s+|synchronized\s+|native\s+|default\s+|transient\s+|volatile\s+)*[\w<>[\],.?\s]+\s+(\w+)\s*\(`)

	tsDeclRe   = regexp.MustCompile(`^\s*(?:export\s+)?(?:default\s+)?(?:declare\s+)?(?:abstract\s+)?(async\s+)?(function\*?|class|interface|enum|type)\s+(\w+)`)
	tsConstRe  = regexp.MustCompile(`^\s*(?:export\s+)?(?:declare\s+)?const\s+(\w+)\s*=\s*(?:async\s*)?(?:\([^)]*\)|\w+)\s*=>`)
	tsMethodRe = regexp.MustCompile(`^\s+(?:public\s+|private\s+|protected\s+|static\s+|readonly\s+|async\s+|override\s+|abstract\s+)*(?:get\s+|set\s+)?\*?\s*(\w+)\s*(?:<[^>]*>)?\s*\(`)
)

func symbolLanguage(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".java":
		return "java"
	case ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs":
		return "typescript"
	default:
		return ""
	}
}

// parseSymbols extracts declarations from one source file using per-language
// heuristics with brace-depth tracking, so nested members (Java/TS class
// methods) are found too. It is the portable fallback when no LSP server is
// available; tree-sitter was deliberately not used to keep the client
// CGO-free.
func parseSymbols(rel string, data []byte) []Symbol {
	lang := symbolLanguage(rel)
	if lang == "" {
		return nil
	}
	lines := strings.Split(string(data), "\n")
	type openSymbol struct {
		sym        Symbol
		startDepth int
	}
	var (
		symbols []Symbol
		open    []openSymbol
		depth   int
	)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		isComment := strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") || strings.HasPrefix(trimmed, "/*")
		delta := braceDelta(line)
		if !isComment {
			if m := matchDeclaration(lang, line); m != nil && m.name != "" && !endsWithStatement(trimmed) {
				sym := Symbol{
					Name:      m.name,
					Kind:      m.kind,
					Path:      rel,
					Line:      i + 1,
					Signature: truncateSignature(trimmed),
					Language:  lang,
				}
				if m.container != "" {
					sym.Container = m.container
				} else if len(open) > 0 {
					sym.Container = open[len(open)-1].sym.Name
				}
				if m.hasBody && delta > 0 {
					open = append(open, openSymbol{sym: sym, startDepth: depth + delta})
				} else {
					sym.EndLine = i + 1
					symbols = append(symbols, sym)
				}
			}
		}
		depth += delta
		for len(open) > 0 && depth < open[len(open)-1].startDepth {
			finished := open[len(open)-1]
			open = open[:len(open)-1]
			finished.sym.EndLine = i + 1
			symbols = append(symbols, finished.sym)
		}
	}
	for _, o := range open {
		o.sym.EndLine = len(lines)
		symbols = append(symbols, o.sym)
	}
	sort.Slice(symbols, func(i, j int) bool {
		if symbols[i].Path != symbols[j].Path {
			return symbols[i].Path < symbols[j].Path
		}
		return symbols[i].Line < symbols[j].Line
	})
	return symbols
}

func truncateSignature(sig string) string {
	if len(sig) > 200 {
		return sig[:200]
	}
	return sig
}

// endsWithStatement rejects declaration-shaped lines that are actually call
// or return statements, which is the main false-positive source for the
// indentation-based Java/TS member patterns.
func endsWithStatement(trimmed string) bool {
	return strings.HasSuffix(trimmed, ";")
}

func matchDeclaration(lang, line string) *symbolMatch {
	switch lang {
	case "go":
		if m := goFuncRe.FindStringSubmatch(line); m != nil {
			if m[1] != "" {
				return &symbolMatch{name: m[2], kind: "method", container: m[1], hasBody: strings.Contains(line, "{")}
			}
			return &symbolMatch{name: m[2], kind: "function", hasBody: strings.Contains(line, "{")}
		}
		if m := goTypeRe.FindStringSubmatch(line); m != nil {
			return &symbolMatch{name: m[1], kind: m[2], hasBody: true}
		}
		if m := goTypeAlias.FindStringSubmatch(line); m != nil && m[2] != "" {
			return &symbolMatch{name: m[1], kind: "type"}
		}
		if m := goConstRe.FindStringSubmatch(line); m != nil {
			kind := "const"
			if strings.HasPrefix(strings.TrimSpace(line), "var") {
				kind = "var"
			}
			return &symbolMatch{name: m[1], kind: kind}
		}
	case "java":
		if m := javaTypeRe.FindStringSubmatch(line); m != nil {
			kind := m[1]
			if kind == "class" {
				kind = "class"
			}
			return &symbolMatch{name: m[2], kind: kind, hasBody: strings.Contains(line, "{") || strings.HasSuffix(strings.TrimSpace(line), ")")}
		}
		if m := javaMemRe.FindStringSubmatch(line); m != nil && !strings.Contains(line, "=") {
			return &symbolMatch{name: m[1], kind: "method", hasBody: true}
		}
	case "typescript":
		if m := tsDeclRe.FindStringSubmatch(line); m != nil {
			kind := m[2]
			kind = strings.TrimSuffix(kind, "*")
			return &symbolMatch{name: m[3], kind: kind, hasBody: kind == "class" || kind == "enum" || strings.Contains(line, "{")}
		}
		if m := tsConstRe.FindStringSubmatch(line); m != nil {
			return &symbolMatch{name: m[1], kind: "function", hasBody: false}
		}
		if m := tsMethodRe.FindStringSubmatch(line); m != nil && m[1] != "if" && m[1] != "for" && m[1] != "while" && m[1] != "switch" && m[1] != "catch" && m[1] != "return" && m[1] != "function" {
			return &symbolMatch{name: m[1], kind: "method", hasBody: true}
		}
	}
	return nil
}

// braceDelta counts net braces on a line after stripping string and char
// literals; good enough for finding declaration end lines.
func braceDelta(line string) int {
	delta := 0
	inString := byte(0)
	for i := 0; i < len(line); i++ {
		c := line[i]
		if inString != 0 {
			if c == '\\' {
				i++
			} else if c == inString {
				inString = 0
			}
			continue
		}
		switch c {
		case '"', '`', '\'':
			inString = c
		case '{':
			delta++
		case '}':
			delta--
		}
	}
	return delta
}

func (s *Service) findSymbol(ctx context.Context, root, workspace, pattern, kind string) ([]Symbol, error) {
	pattern = strings.TrimSpace(pattern)
	if pattern == "" {
		return nil, errSymbolRequired("pattern")
	}
	index := s.loadSymbolIndex(workspace, root)
	var out []Symbol
	err := s.walkCodeFiles(ctx, root, func(rel string, abs string, info fs.FileInfo) error {
		syms, cacheHit := index.lookup(rel, abs, info)
		if !cacheHit {
			data, readErr := os.ReadFile(abs)
			if readErr != nil {
				return nil
			}
			syms = parseSymbols(rel, data)
			index.store(rel, info, syms)
		}
		for _, sym := range syms {
			if kind != "" && sym.Kind != kind {
				continue
			}
			if strings.Contains(strings.ToLower(sym.Name), strings.ToLower(pattern)) {
				out = append(out, sym)
				if len(out) >= maxSymbolResults {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Path != out[j].Path {
			return out[i].Path < out[j].Path
		}
		return out[i].Line < out[j].Line
	})
	index.save(s.symbolIndexPath(workspace))
	return out, nil
}

func (s *Service) readSymbol(root, rel, name string, maxLines int) (map[string]any, error) {
	rel = strings.TrimSpace(rel)
	name = strings.TrimSpace(name)
	if rel == "" || name == "" {
		return nil, errSymbolRequired("path and symbol")
	}
	if symbolLanguage(rel) == "" {
		return nil, errUnsupportedSymbolFile(rel)
	}
	if maxLines <= 0 || maxLines > 512 {
		maxLines = 512
	}
	abs, err := s.resolveAllowedPath(root, rel)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, safePathError("read symbol", rel, err)
	}
	if len(data) > maxSymbolFileBytes {
		return nil, errSymbolFileTooLarge(rel)
	}
	syms := parseSymbols(logicalPath(rel), data)
	var chosen *Symbol
	for i := range syms {
		if syms[i].Name == name && (chosen == nil || syms[i].Line < chosen.Line) {
			chosen = &syms[i]
		}
	}
	if chosen == nil {
		return nil, errSymbolNotFound(name, rel)
	}
	lines := strings.Split(string(data), "\n")
	start := chosen.Line - 1
	end := chosen.EndLine
	if end > len(lines) {
		end = len(lines)
	}
	if end-start > maxLines {
		end = start + maxLines
	}
	content := strings.Join(lines[start:end], "\n")
	truncated := chosen.EndLine-chosen.Line+1 > maxLines
	return map[string]any{
		"symbol":    chosen,
		"content":   content,
		"truncated": truncated,
	}, nil
}

// findReferencesTextual is the bounded fallback used when no LSP server is
// available: word-boundary identifier matches across non-sensitive code files.
func (s *Service) findReferencesTextual(ctx context.Context, root, symbol string) ([]SymbolHit, error) {
	if !validIdentifier(symbol) {
		return nil, errSymbolRequired("symbol")
	}
	var out []SymbolHit
	err := s.walkCodeFiles(ctx, root, func(rel string, abs string, info fs.FileInfo) error {
		if info.Size() > maxSymbolFileBytes {
			return nil
		}
		f, err := os.Open(abs)
		if err != nil {
			return nil
		}
		defer f.Close()
		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			lineNo++
			if containsIdentifier(scanner.Text(), symbol) {
				out = append(out, SymbolHit{Path: rel, Line: lineNo, Text: strings.TrimSpace(scanner.Text())})
				if len(out) >= maxReferenceLines {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	return out, err
}

// walkCodeFiles walks code files under the workspace applying the same
// skip-directory and sensitive-path policy as search and discovery tools.
func (s *Service) walkCodeFiles(ctx context.Context, root string, fn func(rel, abs string, info fs.FileInfo) error) error {
	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return err
	}
	return filepath.WalkDir(rootReal, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
		rel, relErr := filepath.Rel(rootReal, path)
		if relErr != nil {
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
		if symbolLanguage(rel) == "" {
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || !info.Mode().IsRegular() || info.Size() > maxSymbolFileBytes {
			return nil
		}
		return fn(rel, path, info)
	})
}

// containsIdentifier reports whether the line contains the symbol as a whole
// identifier (word-boundary match on both sides).
func containsIdentifier(line, symbol string) bool {
	i := 0
	for i+len(symbol) <= len(line) {
		idx := strings.Index(line[i:], symbol)
		if idx < 0 {
			return false
		}
		pos := i + idx
		end := pos + len(symbol)
		if !identCharAt(line, pos-1) && !identCharAt(line, end) {
			return true
		}
		i = pos + 1
	}
	return false
}

// identCharAt reports whether the byte at pos is part of an identifier;
// out-of-range positions are word boundaries.
func identCharAt(line string, pos int) bool {
	if pos < 0 || pos >= len(line) {
		return false
	}
	return isIdentRune(line[pos])
}

func isIdentRune(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9')
}

func validIdentifier(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if !(r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)) {
			return false
		}
	}
	return true
}
