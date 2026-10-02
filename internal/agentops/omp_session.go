package agentops

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	defaultOMPSessionLimit = 50
	maxOMPSessionLimit     = 500
	defaultOMPReadLimit    = 200
	maxOMPReadLimit        = 2000
	maxOMPRecordBytes      = 4 << 20
)

type OMPSessionAdapter struct {
	agentDir    string
	sessionsDir string

	mu      sync.Mutex
	indexes map[string]*ompFileIndex
}

type ompFileIndex struct {
	offset  int64
	records []ompRecordPos
}

type ompRecordPos struct {
	seq    int64
	offset int64
	length int
}

type ompSessionMeta struct {
	ref       HarnessSessionRef
	cwd       string
	firstUser string
}

type ompEnvelope struct {
	Type          string            `json:"type"`
	Timestamp     json.RawMessage   `json:"timestamp"`
	UpdatedAt     json.RawMessage   `json:"updatedAt"`
	Title         string            `json:"title"`
	ID            string            `json:"id"`
	CWD           string            `json:"cwd"`
	Model         string            `json:"model"`
	ThinkingLevel string            `json:"thinkingLevel"`
	Message       json.RawMessage   `json:"message"`
	Messages      []json.RawMessage `json:"messages"`
	CustomType    string            `json:"customType"`
	Data          json.RawMessage   `json:"data"`
}

type ompMessage struct {
	Role         string           `json:"role"`
	Content      []ompContentPart `json:"content"`
	ToolCallID   string           `json:"toolCallId"`
	ToolName     string           `json:"toolName"`
	IsError      bool             `json:"isError"`
	ErrorMessage string           `json:"errorMessage"`
	Timestamp    json.RawMessage  `json:"timestamp"`
}

type ompContentPart struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	ID        string          `json:"id"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type ompToolStart struct {
	ToolCallID string          `json:"toolCallId"`
	ToolName   string          `json:"toolName"`
	StartedAt  json.RawMessage `json:"startedAt"`
	Args       json.RawMessage `json:"args"`
	Intent     string          `json:"intent"`
}

func NewOMPSessionAdapter(agentDir string) *OMPSessionAdapter {
	agentDir = strings.TrimSpace(agentDir)
	if agentDir == "" {
		agentDir = strings.TrimSpace(os.Getenv("PI_CODING_AGENT_DIR"))
	}
	if agentDir == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			agentDir = filepath.Join(home, ".omp", "agent")
		}
	}
	return &OMPSessionAdapter{
		agentDir:    agentDir,
		sessionsDir: filepath.Join(agentDir, "sessions"),
		indexes:     map[string]*ompFileIndex{},
	}
}

func (a *OMPSessionAdapter) Harness() string { return SubagentClientOMP }

func (a *OMPSessionAdapter) List(ctx context.Context, req SessionListRequest) ([]HarnessSessionRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultOMPSessionLimit
	}
	if limit > maxOMPSessionLimit {
		limit = maxOMPSessionLimit
	}
	files, err := a.sessionFiles(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	root := canonicalOMPPath(req.Root)
	metas := make([]ompSessionMeta, 0, len(files))
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		meta, err := readOMPSessionMeta(path)
		if err != nil {
			continue
		}
		if root != "" && canonicalOMPPath(meta.cwd) != root {
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

func (a *OMPSessionAdapter) Discover(ctx context.Context, req SessionDiscoverRequest) (HarnessSessionRef, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	files, err := a.sessionFiles(ctx)
	if err != nil {
		return HarnessSessionRef{}, err
	}
	root := canonicalOMPPath(req.Root)
	task := strings.TrimSpace(req.Task)
	var (
		exact      *ompSessionMeta
		exactDelta time.Duration
		nearest    *ompSessionMeta
		nearDelta  time.Duration
	)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return HarnessSessionRef{}, err
		}
		meta, err := readOMPSessionMeta(path)
		if err != nil {
			continue
		}
		if root != "" && canonicalOMPPath(meta.cwd) != root {
			continue
		}
		delta := ompTimeDistance(meta.ref.StartedAt, req.StartedAt)
		if task != "" && strings.TrimSpace(meta.firstUser) == task {
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

func (a *OMPSessionAdapter) Read(ctx context.Context, ref HarnessSessionRef, req SessionReadRequest) (SessionPage, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if ref.Harness != "" && ref.Harness != SubagentClientOMP {
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
		limit = defaultOMPReadLimit
	}
	if limit > maxOMPReadLimit {
		limit = maxOMPReadLimit
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
		meta, metaErr := readOMPSessionMeta(path)
		if metaErr == nil {
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
		raw, entries, err := parseOMPDurableRecord(line, pos.seq)
		if err != nil {
			return SessionPage{}, fmt.Errorf("parse OMP session %s record %d: %w", ref.ID, pos.seq, err)
		}
		page.RawRecords = append(page.RawRecords, raw)
		page.Entries = append(page.Entries, entries...)
		page.NextCursor = pos.seq
	}
	return page, nil
}

func (a *OMPSessionAdapter) NormalizeLive(line []byte) (HarnessLiveEvent, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return HarnessLiveEvent{}, errors.New("empty OMP live event")
	}
	var env ompEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return HarnessLiveEvent{}, fmt.Errorf("invalid OMP live event: %w", err)
	}
	raw := HarnessRawRecord{
		RawType: env.Type,
		At:      ompEnvelopeTime(env),
		Payload: append(json.RawMessage(nil), line...),
	}
	out := HarnessLiveEvent{Record: raw}
	switch env.Type {
	case "message_end", "turn_end":
		entries, err := normalizeOMPMessage(env.Message, 0, raw.At)
		if err != nil {
			return HarnessLiveEvent{}, err
		}
		for i := range entries {
			entries[i].RawType = env.Type
		}
		out.Entries = entries
	case "agent_end":
	case "tool_execution_start":
		if entry, ok := normalizeOMPToolStart(env.Data, 0, raw.At); ok {
			entry.RawType = env.Type
			out.Entries = []SessionEntry{entry}
		}
	}
	return out, nil
}

func (a *OMPSessionAdapter) sessionFiles(ctx context.Context) ([]string, error) {
	entries, err := os.ReadDir(a.sessionsDir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		path := filepath.Join(a.sessionsDir, entry.Name())
		if entry.IsDir() {
			children, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, child := range children {
				if child.IsDir() || !isOMPSessionJSONL(child.Name()) {
					continue
				}
				files = append(files, filepath.Join(path, child.Name()))
			}
			continue
		}
		if isOMPSessionJSONL(entry.Name()) {
			files = append(files, path)
		}
	}
	return files, nil
}

func isOMPSessionJSONL(name string) bool {
	return strings.HasSuffix(name, ".jsonl") && !strings.HasPrefix(name, ".")
}

func (a *OMPSessionAdapter) resolveLocator(ctx context.Context, ref HarnessSessionRef) (string, error) {
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
		meta, err := readOMPSessionMeta(path)
		if err == nil && meta.ref.ID == ref.ID {
			return path, nil
		}
	}
	return "", ErrHarnessSessionNotFound
}

func (a *OMPSessionAdapter) validateLocator(path string) error {
	base, err := filepath.Abs(a.sessionsDir)
	if err != nil {
		return err
	}
	target, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if realBase, err := filepath.EvalSymlinks(base); err == nil {
		base = realBase
	}
	if realTarget, err := filepath.EvalSymlinks(target); err == nil {
		target = realTarget
	}
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return err
	}
	if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return fmt.Errorf("OMP session locator escapes session root")
	}
	if !isOMPSessionJSONL(filepath.Base(target)) {
		return fmt.Errorf("OMP session locator is not a session JSONL")
	}
	return nil
}

func (a *OMPSessionAdapter) indexSnapshot(path string) ([]ompRecordPos, error) {
	a.mu.Lock()
	defer a.mu.Unlock()
	stat, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	idx := a.indexes[path]
	if idx == nil || stat.Size() < idx.offset {
		idx = &ompFileIndex{}
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
		if len(line) > maxOMPRecordBytes {
			return nil, fmt.Errorf("OMP session record exceeds %d bytes", maxOMPRecordBytes)
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
						return nil, fmt.Errorf("invalid OMP JSONL record at byte %d", cursor)
					}
					idx.records = append(idx.records, ompRecordPos{
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
			out := append([]ompRecordPos(nil), idx.records...)
			return out, nil
		default:
			return nil, readErr
		}
	}
}

func readOMPSessionMeta(path string) (ompSessionMeta, error) {
	f, err := os.Open(path)
	if err != nil {
		return ompSessionMeta{}, err
	}
	defer f.Close()
	stat, _ := f.Stat()
	meta := ompSessionMeta{
		ref: HarnessSessionRef{Harness: SubagentClientOMP, Locator: path},
	}
	if stat != nil {
		meta.ref.UpdatedAt = stat.ModTime().UTC()
	}
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), maxOMPRecordBytes)
	for scanned := 0; scanner.Scan() && scanned < 256; scanned++ {
		line := bytes.TrimSpace(scanner.Bytes())
		if len(line) == 0 {
			continue
		}
		var env ompEnvelope
		if err := json.Unmarshal(line, &env); err != nil {
			return ompSessionMeta{}, err
		}
		switch env.Type {
		case "title":
			if strings.TrimSpace(env.Title) != "" {
				meta.ref.Title = env.Title
			}
			if t := parseOMPTime(env.UpdatedAt); !t.IsZero() && t.After(meta.ref.UpdatedAt) {
				meta.ref.UpdatedAt = t
			}
		case "session":
			meta.ref.ID = env.ID
			meta.cwd = env.CWD
			meta.ref.root = env.CWD
			if meta.ref.Title == "" {
				meta.ref.Title = env.Title
			}
			meta.ref.StartedAt = parseOMPTime(env.Timestamp)
		case "model_change":
			if meta.ref.Model == "" && env.Model != "" {
				meta.ref.Model = env.Model
			}
		case "thinking_level_change":
			if meta.ref.Thinking == "" && env.ThinkingLevel != "" {
				meta.ref.Thinking = env.ThinkingLevel
			}
		case "message":
			if meta.firstUser == "" {
				var msg ompMessage
				if json.Unmarshal(env.Message, &msg) == nil && strings.EqualFold(msg.Role, "user") {
					meta.firstUser = ompTextContent(msg.Content)
				}
			}
		}
		if meta.ref.ID != "" && meta.firstUser != "" && meta.ref.Model != "" && meta.ref.Thinking != "" {
			break
		}
	}
	if err := scanner.Err(); err != nil {
		return ompSessionMeta{}, err
	}
	if meta.ref.ID == "" || meta.cwd == "" {
		return ompSessionMeta{}, errors.New("OMP session metadata record not found")
	}
	if meta.ref.StartedAt.IsZero() && stat != nil {
		meta.ref.StartedAt = stat.ModTime().UTC()
	}
	return meta, nil
}

func parseOMPDurableRecord(line []byte, seq int64) (HarnessRawRecord, []SessionEntry, error) {
	var env ompEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return HarnessRawRecord{}, nil, err
	}
	at := ompEnvelopeTime(env)
	raw := HarnessRawRecord{
		SourceSeq: seq,
		At:        at,
		RawType:   env.Type,
		Payload:   append(json.RawMessage(nil), line...),
	}
	var entries []SessionEntry
	switch env.Type {
	case "message":
		var err error
		entries, err = normalizeOMPMessage(env.Message, seq, at)
		if err != nil {
			return HarnessRawRecord{}, nil, err
		}
	case "custom":
		if env.CustomType == "tool_execution_start" {
			if entry, ok := normalizeOMPToolStart(env.Data, seq, at); ok {
				entry.RawType = env.CustomType
				entries = []SessionEntry{entry}
			}
		}
	}
	for i := range entries {
		if entries[i].RawType == "" {
			entries[i].RawType = env.Type
		}
	}
	return raw, entries, nil
}

func normalizeOMPMessage(raw json.RawMessage, seq int64, fallbackAt time.Time) ([]SessionEntry, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	var msg ompMessage
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, err
	}
	at := parseOMPTime(msg.Timestamp)
	if at.IsZero() {
		at = fallbackAt
	}
	switch strings.ToLower(msg.Role) {
	case "user":
		text := ompTextContent(msg.Content)
		if strings.TrimSpace(text) == "" {
			return nil, nil
		}
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryMessage,
			Role: "user", Text: text,
		}}, nil
	case "assistant":
		out := make([]SessionEntry, 0)
		text := ompTextContent(msg.Content)
		if strings.TrimSpace(text) != "" {
			out = append(out, SessionEntry{
				SourceSeq: seq, At: at, Kind: SessionEntryMessage,
				Role: "assistant", Text: text,
			})
		}
		for _, part := range msg.Content {
			if part.Type != "toolCall" {
				continue
			}
			out = append(out, SessionEntry{
				SourceSeq: seq, At: at, Kind: SessionEntryToolCall,
				Role: "assistant", ToolName: part.Name, ToolCall: part.ID,
				Status: "requested", Arguments: cloneJSON(part.Arguments),
			})
		}
		if len(out) == 0 && strings.TrimSpace(msg.ErrorMessage) != "" {
			out = append(out, SessionEntry{
				SourceSeq: seq, At: at, Kind: SessionEntryError,
				Role: "assistant", Text: msg.ErrorMessage, Status: "error",
			})
		}
		return out, nil
	case "toolresult":
		status := "completed"
		if msg.IsError {
			status = "error"
		}
		return []SessionEntry{{
			SourceSeq: seq, At: at, Kind: SessionEntryToolResult,
			Role: "tool", Text: ompTextContent(msg.Content),
			ToolName: msg.ToolName, ToolCall: msg.ToolCallID, Status: status,
		}}, nil
	}
	return nil, nil
}

func normalizeOMPToolStart(raw json.RawMessage, seq int64, fallbackAt time.Time) (SessionEntry, bool) {
	if len(raw) == 0 {
		return SessionEntry{}, false
	}
	var start ompToolStart
	if json.Unmarshal(raw, &start) != nil || start.ToolName == "" {
		return SessionEntry{}, false
	}
	at := parseOMPTime(start.StartedAt)
	if at.IsZero() {
		at = fallbackAt
	}
	return SessionEntry{
		SourceSeq: seq,
		At:        at,
		Kind:      SessionEntryActivity,
		Text:      start.Intent,
		ToolName:  start.ToolName,
		ToolCall:  start.ToolCallID,
		Status:    "started",
		Arguments: cloneJSON(start.Args),
	}, true
}

func ompTextContent(parts []ompContentPart) string {
	texts := make([]string, 0, len(parts))
	for _, part := range parts {
		if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
			texts = append(texts, part.Text)
		}
	}
	return strings.Join(texts, "\n")
}

func cloneJSON(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	return append(json.RawMessage(nil), raw...)
}

func ompEnvelopeTime(env ompEnvelope) time.Time {
	if t := parseOMPTime(env.Timestamp); !t.IsZero() {
		return t
	}
	return parseOMPTime(env.UpdatedAt)
}

func parseOMPTime(raw json.RawMessage) time.Time {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return time.Time{}
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
				return t.UTC()
			}
		}
		return time.Time{}
	}
	var ms float64
	if json.Unmarshal(raw, &ms) == nil && ms > 0 {
		return time.UnixMilli(int64(ms)).UTC()
	}
	return time.Time{}
}

func ompTimeDistance(a, b time.Time) time.Duration {
	if b.IsZero() {
		return 0
	}
	d := a.Sub(b)
	if d < 0 {
		return -d
	}
	return d
}

func canonicalOMPPath(path string) string {
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
