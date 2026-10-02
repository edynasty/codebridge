package agentops

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	defaultOpenCodeSessionLimit = 50
	maxOpenCodeSessionLimit     = 500
	defaultOpenCodeReadLimit    = 200
	maxOpenCodeReadLimit        = 2000
)

type OpenCodeSessionAdapter struct {
	dataHome string
	dbPath   string
}

type openCodeSessionRow struct {
	ID, Directory, Title, Model, Agent string
	TimeCreated, TimeUpdated           int64
}

type openCodeMessageData struct {
	Role string `json:"role"`
}

type openCodePartData struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Synthetic bool            `json:"synthetic"`
	Tool      string          `json:"tool"`
	CallID    string          `json:"callID"`
	State     json.RawMessage `json:"state"`
}

type openCodeToolState struct {
	Status string          `json:"status"`
	Input  json.RawMessage `json:"input"`
	Output json.RawMessage `json:"output"`
	Error  json.RawMessage `json:"error"`
}

type openCodeLiveEnvelope struct {
	Type      string          `json:"type"`
	Timestamp json.RawMessage `json:"timestamp"`
	SessionID string          `json:"sessionID"`
	Part      json.RawMessage `json:"part"`
}

func NewOpenCodeSessionAdapter(dataHome string) *OpenCodeSessionAdapter {
	dataHome = strings.TrimSpace(dataHome)
	if dataHome == "" {
		dataHome = strings.TrimSpace(os.Getenv("CODEBRIDGE_OPENCODE_DATA_HOME"))
	}
	if dataHome == "" {
		dataHome = strings.TrimSpace(os.Getenv("XDG_DATA_HOME"))
	}
	if dataHome == "" {
		if home, err := os.UserHomeDir(); err == nil && home != "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
	}
	return &OpenCodeSessionAdapter{dataHome: dataHome, dbPath: filepath.Join(dataHome, "opencode", "opencode.db")}
}

func (a *OpenCodeSessionAdapter) Harness() string { return SubagentClientOpencode }

func (a *OpenCodeSessionAdapter) List(ctx context.Context, req SessionListRequest) ([]HarnessSessionRef, error) {
	db, err := a.openReadOnly(ctx)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer db.Close()
	limit := req.Limit
	if limit <= 0 {
		limit = defaultOpenCodeSessionLimit
	}
	if limit > maxOpenCodeSessionLimit {
		limit = maxOpenCodeSessionLimit
	}
	query := `SELECT id,directory,title,model,agent,time_created,time_updated FROM session`
	args := []any{}
	if strings.TrimSpace(req.Root) != "" {
		query += " WHERE directory=?"
		args = append(args, canonicalOpenCodePath(req.Root))
	}
	query += " ORDER BY time_updated DESC,id DESC LIMIT ?"
	args = append(args, limit)
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]HarnessSessionRef, 0, limit)
	for rows.Next() {
		row, err := scanOpenCodeSession(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, openCodeSessionRef(row, a.dbPath))
	}
	return out, rows.Err()
}

func (a *OpenCodeSessionAdapter) Discover(ctx context.Context, req SessionDiscoverRequest) (HarnessSessionRef, error) {
	db, err := a.openReadOnly(ctx)
	if err != nil {
		return HarnessSessionRef{}, err
	}
	defer db.Close()
	root := canonicalOpenCodePath(req.Root)
	if id := strings.TrimSpace(req.NativeID); id != "" {
		row, err := queryOpenCodeSession(ctx, db, id)
		if err == nil && (root == "" || canonicalOpenCodePath(row.Directory) == root) {
			return openCodeSessionRef(row, a.dbPath), nil
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return HarnessSessionRef{}, err
		}
	}
	query := `SELECT id,directory,title,model,agent,time_created,time_updated FROM session`
	args := []any{}
	if root != "" {
		query += " WHERE directory=?"
		args = append(args, root)
	}
	query += " ORDER BY time_created DESC,id DESC LIMIT 200"
	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return HarnessSessionRef{}, err
	}
	defer rows.Close()
	task := strings.TrimSpace(req.Task)
	var nearest *openCodeSessionRow
	var nearestDelta time.Duration
	for rows.Next() {
		row, err := scanOpenCodeSession(rows)
		if err != nil {
			return HarnessSessionRef{}, err
		}
		delta := openCodeTimeDistance(time.UnixMilli(row.TimeCreated).UTC(), req.StartedAt)
		if task != "" {
			first, err := firstOpenCodeUserText(ctx, db, row.ID)
			if err != nil {
				return HarnessSessionRef{}, err
			}
			if strings.TrimSpace(first) == task {
				return openCodeSessionRef(row, a.dbPath), nil
			}
		}
		if req.StartedAt.IsZero() || delta <= 30*time.Second {
			if nearest == nil || delta < nearestDelta {
				cp := row
				nearest = &cp
				nearestDelta = delta
			}
		}
	}
	if err := rows.Err(); err != nil {
		return HarnessSessionRef{}, err
	}
	if nearest != nil {
		return openCodeSessionRef(*nearest, a.dbPath), nil
	}
	return HarnessSessionRef{}, ErrHarnessSessionNotFound
}

func (a *OpenCodeSessionAdapter) Read(ctx context.Context, ref HarnessSessionRef, req SessionReadRequest) (SessionPage, error) {
	if ref.Harness != "" && ref.Harness != SubagentClientOpencode {
		return SessionPage{}, fmt.Errorf("session %q belongs to harness %q", ref.ID, ref.Harness)
	}
	if strings.TrimSpace(ref.ID) == "" {
		return SessionPage{}, ErrHarnessSessionNotFound
	}
	db, err := a.openReadOnly(ctx)
	if err != nil {
		return SessionPage{}, err
	}
	defer db.Close()
	srow, err := queryOpenCodeSession(ctx, db, ref.ID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return SessionPage{}, ErrHarnessSessionNotFound
		}
		return SessionPage{}, err
	}
	limit := req.Limit
	if limit <= 0 {
		limit = defaultOpenCodeReadLimit
	}
	if limit > maxOpenCodeReadLimit {
		limit = maxOpenCodeReadLimit
	}
	offset := req.After
	if offset < 0 {
		offset = 0
	}
	rows, err := db.QueryContext(ctx, `
SELECT p.id,p.message_id,p.time_created,p.data,m.data
FROM part p JOIN message m ON m.id=p.message_id
WHERE p.session_id=?
ORDER BY p.time_created ASC,p.id ASC
LIMIT ? OFFSET ?`, ref.ID, limit+1, offset)
	if err != nil {
		return SessionPage{}, err
	}
	defer rows.Close()
	type partRow struct {
		ID, MessageID         string
		TimeCreated           int64
		PartData, MessageData string
	}
	parts := make([]partRow, 0, limit+1)
	for rows.Next() {
		var r partRow
		if err := rows.Scan(&r.ID, &r.MessageID, &r.TimeCreated, &r.PartData, &r.MessageData); err != nil {
			return SessionPage{}, err
		}
		parts = append(parts, r)
	}
	if err := rows.Err(); err != nil {
		return SessionPage{}, err
	}
	eof := len(parts) <= limit
	if len(parts) > limit {
		parts = parts[:limit]
	}
	page := SessionPage{
		Session: openCodeSessionRef(srow, a.dbPath),
		Entries: []SessionEntry{}, RawRecords: []HarnessRawRecord{},
		NextCursor: offset, EOF: eof,
	}
	for i, r := range parts {
		seq := offset + int64(i) + 1
		raw, entries, err := parseOpenCodeDurablePart([]byte(r.PartData), []byte(r.MessageData), seq, r.TimeCreated)
		if err != nil {
			return SessionPage{}, fmt.Errorf("parse OpenCode session %s part %s: %w", ref.ID, r.ID, err)
		}
		page.RawRecords = append(page.RawRecords, raw)
		page.Entries = append(page.Entries, entries...)
		page.NextCursor = seq
	}
	return page, nil
}

func (a *OpenCodeSessionAdapter) NormalizeLive(line []byte) (HarnessLiveEvent, error) {
	line = bytes.TrimSpace(line)
	if len(line) == 0 {
		return HarnessLiveEvent{}, errors.New("empty OpenCode live event")
	}
	var env openCodeLiveEnvelope
	if err := json.Unmarshal(line, &env); err != nil {
		return HarnessLiveEvent{}, fmt.Errorf("invalid OpenCode live event: %w", err)
	}
	out := HarnessLiveEvent{Record: HarnessRawRecord{
		At: parseOpenCodeLiveTime(env.Timestamp), RawType: env.Type,
		Payload: append(json.RawMessage(nil), line...),
	}}
	if len(env.Part) == 0 || bytes.Equal(bytes.TrimSpace(env.Part), []byte("null")) {
		return out, nil
	}
	var part openCodePartData
	if err := json.Unmarshal(env.Part, &part); err != nil {
		return HarnessLiveEvent{}, err
	}
	out.Entries = normalizeOpenCodePart(part, "assistant", 0, out.Record.At, env.Type)
	return out, nil
}

func (a *OpenCodeSessionAdapter) openReadOnly(ctx context.Context) (*sql.DB, error) {
	if _, err := os.Stat(a.dbPath); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", a.dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	if _, err := db.ExecContext(ctx, "PRAGMA query_only=ON"); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}

type openCodeScanner interface{ Scan(dest ...any) error }

func scanOpenCodeSession(s openCodeScanner) (openCodeSessionRow, error) {
	var r openCodeSessionRow
	err := s.Scan(&r.ID, &r.Directory, &r.Title, &r.Model, &r.Agent, &r.TimeCreated, &r.TimeUpdated)
	return r, err
}

func queryOpenCodeSession(ctx context.Context, db *sql.DB, id string) (openCodeSessionRow, error) {
	return scanOpenCodeSession(db.QueryRowContext(ctx,
		`SELECT id,directory,title,model,agent,time_created,time_updated FROM session WHERE id=?`, id))
}

func openCodeSessionRef(r openCodeSessionRow, locator string) HarnessSessionRef {
	model, thinking := parseOpenCodeModel(r.Model)
	return HarnessSessionRef{
		Harness: SubagentClientOpencode, ID: r.ID, Title: r.Title,
		Model: model, Thinking: thinking,
		StartedAt: time.UnixMilli(r.TimeCreated).UTC(),
		UpdatedAt: time.UnixMilli(r.TimeUpdated).UTC(),
		Locator:   locator, root: r.Directory,
	}
}

func parseOpenCodeModel(raw string) (string, string) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ""
	}
	var m struct {
		ID         string `json:"id"`
		ProviderID string `json:"providerID"`
		Variant    string `json:"variant"`
	}
	if json.Unmarshal([]byte(raw), &m) != nil {
		return raw, ""
	}
	name := m.ID
	if m.ProviderID != "" && m.ID != "" {
		name = m.ProviderID + "/" + m.ID
	}
	thinking := m.Variant
	if thinking == "default" {
		thinking = ""
	}
	return name, thinking
}

func firstOpenCodeUserText(ctx context.Context, db *sql.DB, sessionID string) (string, error) {
	rows, err := db.QueryContext(ctx, `SELECT id,data FROM message WHERE session_id=? ORDER BY time_created ASC,id ASC`, sessionID)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	for rows.Next() {
		var id, raw string
		if err := rows.Scan(&id, &raw); err != nil {
			return "", err
		}
		var msg openCodeMessageData
		if json.Unmarshal([]byte(raw), &msg) != nil || msg.Role != "user" {
			continue
		}
		pr, err := db.QueryContext(ctx, `SELECT data FROM part WHERE message_id=? ORDER BY time_created ASC,id ASC`, id)
		if err != nil {
			return "", err
		}
		var texts []string
		for pr.Next() {
			var pRaw string
			if err := pr.Scan(&pRaw); err != nil {
				pr.Close()
				return "", err
			}
			var p openCodePartData
			if json.Unmarshal([]byte(pRaw), &p) == nil && p.Type == "text" && !p.Synthetic && strings.TrimSpace(p.Text) != "" {
				texts = append(texts, p.Text)
			}
		}
		err = pr.Err()
		pr.Close()
		if err != nil {
			return "", err
		}
		if len(texts) > 0 {
			return strings.Join(texts, "\n"), nil
		}
	}
	return "", rows.Err()
}

func parseOpenCodeDurablePart(partRaw, messageRaw []byte, seq, timeCreated int64) (HarnessRawRecord, []SessionEntry, error) {
	var p openCodePartData
	if err := json.Unmarshal(partRaw, &p); err != nil {
		return HarnessRawRecord{}, nil, err
	}
	var msg openCodeMessageData
	if len(messageRaw) > 0 {
		if err := json.Unmarshal(messageRaw, &msg); err != nil {
			return HarnessRawRecord{}, nil, err
		}
	}
	at := time.UnixMilli(timeCreated).UTC()
	raw := HarnessRawRecord{SourceSeq: seq, At: at, RawType: "part/" + p.Type, Payload: append(json.RawMessage(nil), partRaw...)}
	return raw, normalizeOpenCodePart(p, msg.Role, seq, at, raw.RawType), nil
}

func normalizeOpenCodePart(p openCodePartData, role string, seq int64, at time.Time, rawType string) []SessionEntry {
	switch p.Type {
	case "text":
		if p.Synthetic || strings.TrimSpace(p.Text) == "" {
			return nil
		}
		if role == "" {
			role = "assistant"
		}
		if role != "user" && role != "assistant" {
			return nil
		}
		return []SessionEntry{{SourceSeq: seq, At: at, Kind: SessionEntryMessage, Role: role, Text: p.Text, RawType: rawType}}
	case "tool":
		var st openCodeToolState
		if len(p.State) > 0 {
			_ = json.Unmarshal(p.State, &st)
		}
		status := strings.TrimSpace(st.Status)
		if status == "" {
			status = "requested"
		}
		out := []SessionEntry{{SourceSeq: seq, At: at, Kind: SessionEntryToolCall, Role: "assistant", ToolName: p.Tool, ToolCall: p.CallID, Status: status, Arguments: cloneJSON(st.Input), RawType: rawType}}
		if openCodeToolTerminal(status) {
			text := openCodeJSONText(st.Output)
			if text == "" {
				text = openCodeJSONText(st.Error)
			}
			rs := status
			if rs == "success" {
				rs = "completed"
			}
			out = append(out, SessionEntry{SourceSeq: seq, At: at, Kind: SessionEntryToolResult, Role: "tool", ToolName: p.Tool, ToolCall: p.CallID, Status: rs, Text: text, RawType: rawType})
		}
		return out
	}
	return nil
}

func openCodeToolTerminal(status string) bool {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case "completed", "success", "error", "failed", "cancelled":
		return true
	default:
		return false
	}
}

func openCodeJSONText(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func parseOpenCodeLiveTime(raw json.RawMessage) time.Time {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || bytes.Equal(raw, []byte("null")) {
		return time.Time{}
	}
	var ms int64
	if json.Unmarshal(raw, &ms) == nil && ms > 0 {
		return time.UnixMilli(ms).UTC()
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
			return t.UTC()
		}
	}
	return time.Time{}
}

func canonicalOpenCodePath(path string) string {
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

func openCodeTimeDistance(a, b time.Time) time.Duration {
	if b.IsZero() {
		return 0
	}
	d := a.Sub(b)
	if d < 0 {
		return -d
	}
	return d
}
