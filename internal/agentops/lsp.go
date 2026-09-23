package agentops

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"
)

// lspServerConfig describes a locally installed language server. Servers run
// only on the agent machine; nothing about them is exposed remotely.
type lspServerConfig struct {
	Command string
	Args    []string
}

var lspServers = map[string]lspServerConfig{
	"go":         {Command: "gopls"},
	"typescript": {Command: "typescript-language-server", Args: []string{"--stdio"}},
	"java":       {Command: "jdtls"},
}

const (
	lspStartupTimeout = 20 * time.Second
	lspRequestTimeout = 15 * time.Second
)

// lspClient is a minimal JSON-RPC-over-stdio client for the subset of LSP
// CodeBridge uses: initialize, textDocument/documentSymbol and
// textDocument/references. On any failure callers fall back to the portable
// heuristic symbol engine.
type lspClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Scanner
	mu     sync.Mutex
	nextID atomic.Int64
}

type lspMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      *int64          `json:"id,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

func startLSPClient(ctx context.Context, workspaceRoot, lang string) (*lspClient, error) {
	cfg, ok := lspServers[lang]
	if !ok {
		return nil, fmt.Errorf("no language server configured for %s", lang)
	}
	if _, err := exec.LookPath(cfg.Command); err != nil {
		return nil, fmt.Errorf("language server %s is not installed", cfg.Command)
	}
	rootAbs, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, err
	}
	rootURI := "file://" + rootAbs

	startCtx, cancel := context.WithTimeout(ctx, lspStartupTimeout)
	defer cancel()
	cmd := exec.CommandContext(startCtx, cfg.Command, cfg.Args...)
	cmd.Dir = rootAbs
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	client := &lspClient{cmd: cmd, stdin: stdin, stdout: bufio.NewScanner(stdout)}
	client.stdout.Buffer(make([]byte, 0, 256*1024), 16*1024*1024)

	initParams := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   rootURI,
		"workspaceFolders": []map[string]string{
			{"uri": rootURI, "name": filepath.Base(rootAbs)},
		},
		"capabilities": map[string]any{},
	}
	if _, err := client.request(startCtx, "initialize", initParams); err != nil {
		_ = cmd.Process.Kill()
		return nil, fmt.Errorf("language server did not initialize: %w", err)
	}
	if err := client.notify("initialized", map[string]any{}); err != nil {
		_ = cmd.Process.Kill()
		return nil, err
	}
	return client, nil
}

func (c *lspClient) request(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.cmd.ProcessState != nil && c.cmd.ProcessState.Exited() {
		return nil, fmt.Errorf("language server exited")
	}
	id := c.nextID.Add(1)
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params})
	if err != nil {
		return nil, err
	}
	if _, err := c.stdin.Write(append(b, '\n')); err != nil {
		return nil, fmt.Errorf("language server write failed")
	}
	deadline := time.Now().Add(lspRequestTimeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for {
		if !c.stdout.Scan() {
			return nil, fmt.Errorf("language server connection closed")
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("language server request timed out")
		}
		line := strings.TrimSpace(c.stdout.Text())
		if line == "" || strings.HasPrefix(line, "Content-Length:") {
			continue
		}
		var resp lspMessage
		if err := json.Unmarshal([]byte(line), &resp); err != nil || resp.ID == nil || *resp.ID != id {
			continue
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("language server error: %s", resp.Error.Message)
		}
		return resp.Result, nil
	}
}

func (c *lspClient) notify(method string, params any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	b, err := json.Marshal(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
	if err != nil {
		return err
	}
	if _, err := c.stdin.Write(append(b, '\n')); err != nil {
		return fmt.Errorf("language server write failed")
	}
	return nil
}

type lspDocumentSymbol struct {
	Name     string              `json:"name"`
	Kind     int                 `json:"kind"`
	Range    lspRange            `json:"range"`
	Children []lspDocumentSymbol `json:"children,omitempty"`
}

type lspRange struct {
	Start lspPosition `json:"start"`
	End   lspPosition `json:"end"`
}

type lspPosition struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type lspLocation struct {
	URI   string   `json:"uri"`
	Range lspRange `json:"range"`
}

// documentSymbols flattens the hierarchical symbol tree into our neutral form.
func (c *lspClient) documentSymbols(ctx context.Context, absPath, rel string) ([]Symbol, error) {
	result, err := c.request(ctx, "textDocument/documentSymbol", map[string]any{
		"textDocument": map[string]string{"uri": "file://" + absPath},
	})
	if err != nil {
		return nil, err
	}
	var hierarchical []lspDocumentSymbol
	if err := json.Unmarshal(result, &hierarchical); err == nil && len(hierarchical) > 0 {
		var out []Symbol
		var flatten func(syms []lspDocumentSymbol, container string)
		flatten = func(syms []lspDocumentSymbol, container string) {
			for _, s := range syms {
				out = append(out, Symbol{
					Name:      s.Name,
					Kind:      lspSymbolKind(s.Kind),
					Path:      rel,
					Line:      s.Range.Start.Line + 1,
					EndLine:   s.Range.End.Line + 1,
					Container: container,
				})
				flatten(s.Children, s.Name)
			}
		}
		flatten(hierarchical, "")
		return out, nil
	}
	var flat []struct {
		Name     string      `json:"name"`
		Kind     int         `json:"kind"`
		Location lspLocation `json:"location"`
	}
	if err := json.Unmarshal(result, &flat); err != nil {
		return nil, err
	}
	out := make([]Symbol, 0, len(flat))
	for _, s := range flat {
		out = append(out, Symbol{
			Name:    s.Name,
			Kind:    lspSymbolKind(s.Kind),
			Path:    rel,
			Line:    s.Location.Range.Start.Line + 1,
			EndLine: s.Location.Range.End.Line + 1,
		})
	}
	return out, nil
}

func (c *lspClient) references(ctx context.Context, absPath string, line, character int, includeDeclaration bool) ([]lspLocation, error) {
	result, err := c.request(ctx, "textDocument/references", map[string]any{
		"textDocument": map[string]string{"uri": "file://" + absPath},
		"position":     map[string]int{"line": line, "character": character},
		"context":      map[string]bool{"includeDeclaration": includeDeclaration},
	})
	if err != nil {
		return nil, err
	}
	var locations []lspLocation
	if err := json.Unmarshal(result, &locations); err != nil {
		return nil, err
	}
	return locations, nil
}

func lspSymbolKind(kind int) string {
	switch kind {
	case 1, 4, 5, 22:
		return "class"
	case 2:
		return "method"
	case 3, 12:
		return "function"
	case 6, 7, 9, 13:
		return "var"
	case 8:
		return "struct"
	case 10:
		return "enum"
	case 11, 23:
		return "interface"
	case 14:
		return "const"
	case 26:
		return "type"
	default:
		return "symbol"
	}
}

// languageForFile maps a logical path to an LSP language key.
func languageForFile(rel string) string {
	return symbolLanguage(rel)
}

// lspClientFor returns a cached running language server for the workspace, or
// starts one. A nil return means "unavailable"; callers fall back to the
// portable heuristic engine.
func (s *Service) lspClientFor(ctx context.Context, workspace, root, lang string) *lspClient {
	if !s.EnableLSP {
		return nil
	}
	key := workspace + "\x00" + lang
	s.lspMu.Lock()
	defer s.lspMu.Unlock()
	if s.lspClients == nil {
		s.lspClients = map[string]*lspClient{}
	}
	if client, ok := s.lspClients[key]; ok {
		return client
	}
	client, err := startLSPClient(ctx, root, lang)
	if err != nil {
		return nil
	}
	s.lspClients[key] = client
	return client
}

// utf16Column converts a byte offset into the UTF-16 column LSP expects.
func utf16Column(line string, byteOffset int) int {
	col := 0
	for i := 0; i < byteOffset && i < len(line); {
		r, size := utf8.DecodeRuneInString(line[i:])
		i += size
		if r > 0xFFFF {
			col += 2
		} else {
			col++
		}
	}
	return col
}
