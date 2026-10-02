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
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultCodexSessionLimit = 50
	maxCodexSessionLimit     = 500
	defaultCodexReadLimit    = 200
	maxCodexReadLimit        = 2000
	maxCodexRecordBytes      = 8 << 20
)

// CodexSessionAdapter indexes durable Codex rollout JSONL under CODEX_HOME.
// Native rollout records remain local; only explicit conversation/tool records
// are projected into SessionEntry.
type CodexSessionAdapter struct {
	homeDir     string
	sessionsDir string
	archivedDir string

	mu      sync.Mutex
	indexes map[string]*codexFileIndex
}

type codexFileIndex struct {
	offset  int64
	records []codexRecordPos
}

type codexRecordPos struct {
	seq    int64
	offset int64
	length int
}

type codexSessionMeta struct {
	ref       HarnessSessionRef
	cwd       string
	userTexts []string
}

type codexRolloutEnvelope struct {
	Timestamp string          `json:"timestamp"`
	Ordinal   int64           `json:"ordinal"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

type codexSessionMetaPayload struct {
	SessionID     string `json:"session_id"`
	ID            string `json:"id"`
	Timestamp     string `json:"timestamp"`
	CWD           string `json:"cwd"`
	Originator    string `json:"originator"`
	CLIVersion    string `json:"cli_version"`
	ModelProvider string `json:"model_provider"`
}

type codexTurnContextPayload struct {
	Model             string `json:"model"`
	Effort            string `json:"effort"`
	CollaborationMode struct {
		Settings struct {
			ReasoningEffort string `json:"reasoning_effort"`
		} `json:"settings"`
	} `json:"collaboration_mode"`
}

type codexResponseItem struct {
	Type      string             `json:"type"`
	ID        string             `json:"id"`
	Role      string             `json:"role"`
	Name      string             `json:"name"`
	CallID    string             `json:"call_id"`
	Input     string             `json:"input"`
	Status    string             `json:"status"`
	Arguments json.RawMessage    `json:"arguments"`
	Output    json.RawMessage    `json:"output"`
	Content   []codexContentPart `json:"content"`
}

type codexContentPart struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type codexEventMsg struct {
	Type             string `json:"type"`
	TurnID           string `json:"turn_id"`
	LastAgentMessage string `json:"last_agent_message"`
	Message          string `json:"message"`
}

type codexLiveEnvelope struct {
	Type     string          `json:"type"`
	ThreadID string          `json:"thread_id"`
	Item     codexLiveItem   `json:"item"`
	Error    json.RawMessage `json:"error"`
}

type codexLiveItem struct {
	ID               string          `json:"id"`
	Type             string          `json:"type"`
	Text             string          `json:"text"`
	Command          string          `json:"command"`
	AggregatedOutput string          `json:"aggregated_output"`
	Status           string          `json:"status"`
	Server           string          `json:"server"`
	Tool             string          `json:"tool"`
	Arguments        json.RawMessage `json:"arguments"`
	Result           json.RawMessage `json:"result"`
	Error            json.RawMessage `json:"error"`
	ExitCode         *int            `json:"exit_code"`
}

func NewCodexSessionAdapter(homeDir string) *CodexSessionAdapter {
	homeDir = strings.TrimSpace(homeDir)
	if homeDir == "" {
		homeDir = strings.TrimSpace(os.Getenv("CODEX_HOME"))
	}
	if homeDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			homeDir = filepath.Join(home, ".codex")
		}
	}
	return &CodexSessionAdapter{
		homeDir:     homeDir,
		sessionsDir: filepath.Join(homeDir, "sessions"),
		archivedDir: filepath.Join(homeDir, "archived_sessions"),
		indexes:     map[string]*codexFileIndex{},
	}
}

func (a *CodexSessionAdapter) Harness() string { return SubagentClientCodex }

func (a *CodexSessionAdapter) List(ctx context.Context, req SessionListRequest) ([]HarnessSessionRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultCodexSessionLimit
	}
	if limit > maxCodexSessionLimit {
		limit = maxCodexSessionLimit
	}
	files, err := a.sessionFiles(ctx)
	if err != nil {
		return nil, err
	}
	root := canonicalCodexPath(req.Root)
	metas := make([]codexSessionMeta, 0, len(files))
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		meta, err := readCodexSessionMeta(path)
		if err != nil {
			continue
		}
		if root != "" && canonicalCodexPath(meta.cwd) != root {
			continue
		}
		metas = append(metas, meta)
	}
	sort.Slice(metas, func(i, j int) bool {
		if metas[i].ref.StartedAt.Equal(metas[j].ref.StartedAt) {
			return metas[i].ref.ID > metas[j].ref.ID
		}
		return metas[i].ref.StartedAt.After(metas[j].ref.StartedAt)
	})
	if len(metas) > limit {
		metas = metas[:limit]
	}
	out := make([]HarnessSessionRef, 0, len(metas))
	for _, meta := range metas {
		out = append(out, meta.ref)
	}
	return out, nil
}

func (a *CodexSessionAdapter) Discover(ctx context.Context, req SessionDiscoverRequest) (HarnessSessionRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	files, err := a.sessionFiles(ctx)
	if err != nil {
		return HarnessSessionRef{}, err
	}
	root := canonicalCodexPath(req.Root)
	task := strings.TrimSpace(req.Task)
	nativeID := strings.TrimSpace(req.NativeID)

	var (
		exact      *codexSessionMeta
		exactDelta time.Duration
		nearest    *codexSessionMeta
		nearDelta  time.Duration
	)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return HarnessSessionRef{}, err
		}
		meta, err := readCodexSessionMeta(path)
		if err != nil {
			continue
		}
		if root != "" && canonicalCodexPath(meta.cwd) != root {
			continue
		}
		if nativeID != "" && meta.ref.ID == nativeID {
			return meta.ref, nil
		}
		delta := codexTimeDistance(meta.ref.StartedAt, req.StartedAt)
		if task != "" && codexHasTask(meta.userTexts, task) {
			if exact == nil || delta < exactDelta {
				copyMeta := meta
				exact = &copyMeta
				exactDelta = delta
			}
			continue
		}
		if req.StartedAt.IsZero() || delta <= 30*time.Second {
			if nearest == nil || delta < nearDelta {
				copyMeta := meta
				nearest = &copyMeta
				nearDelta = delta
			}
		}
	}
	if exact != nil {
		return exact.ref, nil
	}
	if nearest != nil {
		return nearest.ref, nil
	}
	return HarnessSessionRef{}, ErrHarnessSessionNotFound
}

func (a *CodexSessionAdapter) Read(ctx context.Context, ref HarnessSessionRef, req SessionReadRequest) (SessionPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ref.Harness != "" && ref.Harness != SubagentClientCodex {
		return SessionPage{}, fmt.Errorf("session %q belongs to harness %q", ref.ID, ref.Harness)
	}
	path, err := a.resolveLocator(ctx, ref)
	if err != nil {
		return SessionPage{}, err
	}
	if err := a.validateLocator(path); err != nil {
		return SessionPage{}, err
	}
	positions, err := a.indexSnapshot(path)
	if err != nil {
		return SessionPage{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultCodexReadLimit
	}
	if limit > maxCodexReadLimit {
		limit = maxCodexReadLimit
	}
	start := sort.Search(len(positions), func(i int) bool {
		return positions[i].seq > req.After
	})
	end := start + limit
	if end > len(positions) {
		end = len(positions)
	}

	f, err := os.Open(path)
	if err != nil {
		return SessionPage{}, err
	}
	defer f.Close()

	page := SessionPage{
		Session:    ref,
		Entries:    make([]SessionEntry, 0),
		RawRecords: make([]HarnessRawRecord, 0, end-start),
		NextCursor: req.After,
		EOF:        end == len(positions),
	}
	if page.Session.Locator == "" {
		if meta, metaErr := readCodexSessionMeta(path); metaErr == nil {
			page.Session = meta.ref
		}
	}
	for _, pos := range positions[start:end] {
		if err := ctx.Err(); err != nil {
			return SessionPage{}, err
		}
		line := make([]byte, pos.length)
		if _, err := f.ReadAt(line, pos.offset); err != nil && !errors.Is(err, io.EOF) {
			return SessionPage{}, err
		}
		line = bytes.TrimSpace(line)
		raw, entries, err := parseCodexDurableRecord(line, pos.seq)
		if err != nil {
			return SessionPage{}, fmt.Errorf("parse Codex session %s record %d: %w", ref.ID, pos.seq, err)
		}
		page.RawRecords = append(page.RawRecords, raw)
		page.Entries = append(page.Entries, entries...)
		page.NextCursor = pos.seq
	}
	return page, nil
}

func (a *CodexSessionAdapter) NormalizeLive(line []byte) (HarnessLiveEvent, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return HarnessLiveEvent{}, errors.New("empty Codex live event")
	}
	var ev codexLiveEnvelope
	if err := json.Unmarshal(line, &ev); err != nil {
		return HarnessLiveEvent{}, fmt.Errorf("invalid Codex live event: %w", err)
	}
	out := HarnessLiveEvent{
		Record: HarnessRawRecord{
			RawType: ev.Type,
			Payload: append(json.RawMessage(nil), line...),
		},
	}
	switch ev.Type {
	case "thread.started":
		if ev.ThreadID != "" {
			out.Entries = []SessionEntry{{
				Kind: SessionEntryRuntime, Status: "started",
				Text: "thread " + ev.ThreadID, RawType: ev.Type,
			}}
		}
	case "turn.started":
		out.Entries = []SessionEntry{{
			Kind: SessionEntryActivity, Status: "started", RawType: ev.Type,
		}}
	case "item.started":
		if entry, ok := normalizeCodexLiveToolCall(ev.Item, ev.Type); ok {
			out.Entries = []SessionEntry{entry}
		}
	case "item.completed":
		switch ev.Item.Type {
		case "agent_message":
			if strings.TrimSpace(ev.Item.Text) != "" {
				out.Entries = []SessionEntry{{
					Kind: SessionEntryMessage, Role: "assistant",
					Text: ev.Item.Text, RawType: ev.Type,
				}}
			}
		default:
			if entry, ok := normalizeCodexLiveToolResult(ev.Item, ev.Type); ok {
				out.Entries = []SessionEntry{entry}
			}
		}
	case "turn.completed":
		out.Entries = []SessionEntry{{
			Kind: SessionEntryRuntime, Status: "completed", RawType: ev.Type,
		}}
	case "turn.failed", "error":
		out.Entries = []SessionEntry{{
			Kind: SessionEntryError, Status: "error",
			Text: codexJSONText(ev.Error), RawType: ev.Type,
		}}
	}
	return out, nil
}

func (a *CodexSessionAdapter) sessionFiles(ctx context.Context) ([]string, error) {
	var files []string
	for _, root := range []string{a.sessionsDir, a.archivedDir} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				if errors.Is(walkErr, os.ErrNotExist) {
					return nil
				}
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.HasPrefix(d.Name(), ".") || !strings.HasSuffix(d.Name(), ".jsonl") {
				return nil
			}
			files = append(files, path)
			return nil
		})
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
	}
	return files, nil
}

func (a *CodexSessionAdapter) resolveLocator(ctx context.Context, ref HarnessSessionRef) (string, error) {
	if strings.TrimSpace(ref.Locator) != "" {
		return ref.Locator, nil
	}
	if strings.TrimSpace(ref.ID) == "" {
		return "", ErrHarnessSessionNotFound
	}
	files, err := a.sessionFiles(ctx)
	if err != nil {
		return "", err
	}
	for _, path := range files {
		meta, err := readCodexSessionMeta(path)
		if err == nil && meta.ref.ID == ref.ID {
			return path, nil
		}
	}
	return "", ErrHarnessSessionNotFound
}

func (a *CodexSessionAdapter) validateLocator(path string) error {
	for _, root := range []string{a.sessionsDir, a.archivedDir} {
		ok, err := pathWithinCodexRoot(root, path)
		if err != nil {
			return err
		}
		if ok {
			if !strings.HasSuffix(strings.ToLower(filepath.Base(path)), ".jsonl") {
				return errors.New("Codex session locator is not a rollout JSONL")
			}
			return nil
		}
	}
	return errors.New("Codex session locator escapes CODEX_HOME session roots")
}

func pathWithinCodexRoot(root, target string) (bool, error) {
	base, err := filepath.Abs(root)
	if err != nil {
		return false, err
	}
	path, err := filepath.Abs(target)
	if err != nil {
		return false, err
	}
	if realBase, err := filepath.EvalSymlinks(base); err == nil {
		base = realBase
	}
	if realPath, err := filepath.EvalSymlinks(path); err == nil {
		path = realPath
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false, err
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel), nil
}

func (a *CodexSessionAdapter) indexSnapshot(path string) ([]codexRecordPos, error) {
	a.mu.Lock()
	defer a.mu.Unlock()

	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	idx := a.indexes[path]
	if idx == nil || stat.Size() < idx.offset {
		idx = &codexFileIndex{}
		a.indexes[path] = idx
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if _, err := f.Seek(idx.offset, io.SeekStart); err != nil {
		return nil, err
	}
	reader := bufio.NewReaderSize(f, 64*1024)
	cursor := idx.offset
	for {
		line, readErr := reader.ReadBytes('\n')
		if len(line) > maxCodexRecordBytes {
			return nil, fmt.Errorf("Codex rollout record exceeds %d bytes", maxCodexRecordBytes)
		}
		if len(line) > 0 {
			trimmed := bytes.TrimSpace(line)
			complete := readErr == nil
			if errors.Is(readErr, io.EOF) && len(trimmed) > 0 && json.Valid(trimmed) {
				complete = true
			}
			if complete {
				if len(trimmed) > 0 {
					if !json.Valid(trimmed) {
						return nil, fmt.Errorf("invalid Codex JSONL record at byte %d", cursor)
					}
					idx.records = append(idx.records, codexRecordPos{
						seq:    int64(len(idx.records) + 1),
						offset: cursor,
						length: len(line),
					})
				}
				cursor += int64(len(line))
				idx.offset = cursor
			}
		}
		switch {
		case readErr == nil:
			continue
		case errors.Is(readErr, io.EOF):
			return append([]codexRecordPos(nil), idx.records...), nil
		default:
			return nil, readErr
		}
	}
}

func readCodexSessionMeta(path string) (codexSessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return codexSessionMeta{}, err
	}
	defer f.Close()
	stat, _ := f.Stat()
	meta := codexSessionMeta{
		ref: HarnessSessionRef{Harness: SubagentClientCodex, Locator: path},
	}
	if stat != nil {
		meta.ref.UpdatedAt = stat.ModTime().UTC()
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxCodexRecordBytes)
	for scanned := 0; scanner.Scan() && scanned < 512; scanned++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env codexRolloutEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return codexSessionMeta{}, err
		}
		switch env.Type {
		case "session_meta":
			var payload codexSessionMetaPayload
			if json.Unmarshal(env.Payload, &payload) != nil {
				continue
			}
			meta.ref.ID = firstNonEmptyString(payload.SessionID, payload.ID)
			meta.cwd = payload.CWD
			meta.ref.root = payload.CWD
			meta.ref.StartedAt = parseCodexTime(firstNonEmptyString(payload.Timestamp, env.Timestamp))
		case "turn_context":
			var payload codexTurnContextPayload
			if json.Unmarshal(env.Payload, &payload) != nil {
				continue
			}
			if meta.ref.Model == "" {
				meta.ref.Model = payload.Model
			}
			if meta.ref.Thinking == "" {
				meta.ref.Thinking = firstNonEmptyString(payload.Effort, payload.CollaborationMode.Settings.ReasoningEffort)
			}
		case "response_item":
			var item codexResponseItem
			if json.Unmarshal(env.Payload, &item) != nil || item.Type != "message" || item.Role != "user" {
				continue
			}
			for _, text := range codexContentTexts(item.Content) {
				if strings.TrimSpace(text) != "" {
					meta.userTexts = append(meta.userTexts, text)
				}
			}
		}
		if meta.ref.ID != "" && meta.cwd != "" && meta.ref.Model != "" && len(meta.userTexts) > 0 {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return codexSessionMeta{}, err
	}
	if meta.ref.ID == "" || meta.cwd == "" {
		return codexSessionMeta{}, errors.New("Codex session metadata record not found")
	}
	if meta.ref.StartedAt.IsZero() && stat != nil {
		meta.ref.StartedAt = stat.ModTime().UTC()
	}
	if len(meta.userTexts) > 0 {
		meta.ref.Title = truncateText(strings.TrimSpace(meta.userTexts[len(meta.userTexts)-1]), 120)
	}
	return meta, nil
}

func parseCodexDurableRecord(line []byte, seq int64) (HarnessRawRecord, []SessionEntry, error) {
	var env codexRolloutEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return HarnessRawRecord{}, nil, err
	}
	at := parseCodexTime(env.Timestamp)
	raw := HarnessRawRecord{
		SourceSeq: seq,
		At:        at,
		RawType:   env.Type,
		Payload:   append(json.RawMessage(nil), line...),
	}

	switch env.Type {
	case "response_item":
		var item codexResponseItem
		if err := json.Unmarshal(env.Payload, &item); err != nil {
			return HarnessRawRecord{}, nil, err
		}
		entries := normalizeCodexResponseItem(item, seq, at)
		for i := range entries {
			entries[i].RawType = "response_item/" + item.Type
		}
		return raw, entries, nil
	case "event_msg":
		var msg codexEventMsg
		if err := json.Unmarshal(env.Payload, &msg); err != nil {
			return HarnessRawRecord{}, nil, err
		}
		switch msg.Type {
		case "task_started":
			return raw, []SessionEntry{{
				SourceSeq: seq, At: at, Kind: SessionEntryActivity,
				Status: "started", RawType: "event_msg/task_started",
			}}, nil
		case "task_complete":
			return raw, []SessionEntry{{
				SourceSeq: seq, At: at, Kind: SessionEntryActivity,
				Status: "completed", RawType: "event_msg/task_complete",
			}}, nil
		case "error":
			return raw, []SessionEntry{{
				SourceSeq: seq, At: at, Kind: SessionEntryError,
				Status: "error", Text: msg.Message, RawType: "event_msg/error",
			}}, nil
		}
	}
	return raw, nil, nil
}

func normalizeCodexResponseItem(item codexResponseItem, seq int64, at time.Time) []SessionEntry {
	switch item.Type {
	case "message":
		if item.Role != "user" && item.Role != "assistant" {
			return nil
		}
		text := strings.Join(codexContentTexts(item.Content), "\n")
		if strings.TrimSpace(text) == "" {
			return nil
		}
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryMessage,
			Role: item.Role, Text: text,
		}}
	case "function_call":
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryToolCall,
			Role: "assistant", ToolName: item.Name, ToolCall: item.CallID,
			Status: "requested", Arguments: normalizeCodexArguments(item.Arguments),
		}}
	case "function_call_output":
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryToolResult,
			Role: "tool", ToolCall: item.CallID, Status: "completed",
			Text: codexJSONText(item.Output),
		}}
	case "custom_tool_call":
		args, _ := json.Marshal(map[string]any{"input": item.Input})
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryToolCall,
			Role: "assistant", ToolName: item.Name, ToolCall: item.CallID,
			Status: firstNonEmptyString(item.Status, "requested"), Arguments: args,
		}}
	case "custom_tool_call_output":
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryToolResult,
			Role: "tool", ToolCall: item.CallID, Status: firstNonEmptyString(item.Status, "completed"),
			Text: codexJSONText(item.Output),
		}}
	}
	return nil
}

func normalizeCodexLiveToolCall(item codexLiveItem, rawType string) (SessionEntry, bool) {
	switch item.Type {
	case "command_execution":
		args, _ := json.Marshal(map[string]any{"command": item.Command})
		return SessionEntry{
			Kind: SessionEntryToolCall, Role: "assistant",
			ToolName: "command_execution", ToolCall: item.ID,
			Status:    firstNonEmptyString(item.Status, "started"),
			Arguments: args, RawType: rawType,
		}, true
	case "mcp_tool_call":
		name := strings.Trim(strings.TrimSpace(item.Server)+"/"+strings.TrimSpace(item.Tool), "/")
		return SessionEntry{
			Kind: SessionEntryToolCall, Role: "assistant",
			ToolName: firstNonEmptyString(name, "mcp_tool_call"), ToolCall: item.ID,
			Status:    firstNonEmptyString(item.Status, "started"),
			Arguments: cloneJSON(item.Arguments), RawType: rawType,
		}, true
	}
	return SessionEntry{}, false
}

func normalizeCodexLiveToolResult(item codexLiveItem, rawType string) (SessionEntry, bool) {
	switch item.Type {
	case "command_execution":
		status := firstNonEmptyString(item.Status, "completed")
		if item.ExitCode != nil && *item.ExitCode != 0 {
			status = "error"
		}
		return SessionEntry{
			Kind: SessionEntryToolResult, Role: "tool",
			ToolName: "command_execution", ToolCall: item.ID,
			Status: status, Text: item.AggregatedOutput, RawType: rawType,
		}, true
	case "mcp_tool_call":
		name := strings.Trim(strings.TrimSpace(item.Server)+"/"+strings.TrimSpace(item.Tool), "/")
		text := codexJSONText(item.Result)
		status := firstNonEmptyString(item.Status, "completed")
		if len(item.Error) > 0 && !bytes.Equal(bytes.TrimSpace(item.Error), []byte("null")) {
			status = "error"
			if text == "" {
				text = codexJSONText(item.Error)
			}
		}
		return SessionEntry{
			Kind: SessionEntryToolResult, Role: "tool",
			ToolName: firstNonEmptyString(name, "mcp_tool_call"), ToolCall: item.ID,
			Status: status, Text: text, RawType: rawType,
		}, true
	}
	return SessionEntry{}, false
}

func codexContentTexts(parts []codexContentPart) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		switch part.Type {
		case "input_text", "output_text", "text":
			if strings.TrimSpace(part.Text) != "" {
				out = append(out, part.Text)
			}
		}
	}
	return out
}

func normalizeCodexArguments(raw json.RawMessage) json.RawMessage {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return nil
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if candidate := bytes.TrimSpace([]byte(s)); json.Valid(candidate) {
				return append(json.RawMessage(nil), candidate...)
			}
			b, _ := json.Marshal(map[string]any{"value": s})
			return b
		}
	}
	if json.Valid(raw) {
		return append(json.RawMessage(nil), raw...)
	}
	return nil
}

func codexJSONText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var parts []codexContentPart
	if json.Unmarshal(raw, &parts) == nil {
		return strings.Join(codexContentTexts(parts), "\n")
	}
	var obj map[string]any
	if json.Unmarshal(raw, &obj) == nil {
		for _, key := range []string{"text", "message", "output", "content"} {
			if v, ok := obj[key]; ok {
				switch x := v.(type) {
				case string:
					return x
				default:
					if b, err := json.Marshal(x); err == nil {
						return string(b)
					}
				}
			}
		}
	}
	return string(raw)
}

func codexHasTask(texts []string, task string) bool {
	task = strings.TrimSpace(task)
	for _, text := range texts {
		if strings.TrimSpace(text) == task {
			return true
		}
	}
	return false
}

func parseCodexTime(s string) time.Time {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}
	}
	if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
		return t.UTC()
	}
	return time.Time{}
}

func codexTimeDistance(a, b time.Time) time.Duration {
	if b.IsZero() {
		return 0
	}
	d := a.Sub(b)
	if d < 0 {
		return -d
	}
	return d
}

func canonicalCodexPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	path = filepath.Clean(path)
	if real, err := filepath.EvalSymlinks(path); err == nil {
		path = filepath.Clean(real)
	}
	return path
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
