package agentops

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOMPSessionAdapterListAndDiscover(t *testing.T) {
	agentDir := t.TempDir()
	root := filepath.Join(t.TempDir(), "workspace")
	otherRoot := filepath.Join(t.TempDir(), "other")
	base := time.Date(2026, 9, 30, 2, 0, 0, 0, time.UTC)

	writeOMPFixture(t, agentDir, "bucket-a", "older.jsonl", []string{
		ompTitleFixture("Older", base),
		ompSessionFixture("sess-old", root, base),
		ompModelFixture("local/glm-5.3", base.Add(time.Millisecond)),
		ompThinkingFixture("max", base.Add(2*time.Millisecond)),
		ompUserFixture("older task", base.Add(time.Second)),
	})
	writeOMPFixture(t, agentDir, "bucket-b", "target.jsonl", []string{
		ompTitleFixture("Target", base.Add(2*time.Minute)),
		ompSessionFixture("sess-target", root, base.Add(2*time.Minute)),
		ompModelFixture("sub/gpt-6-sol", base.Add(2*time.Minute+time.Millisecond)),
		ompThinkingFixture("xhigh", base.Add(2*time.Minute+2*time.Millisecond)),
		ompUserFixture("target task", base.Add(2*time.Minute+time.Second)),
	})
	writeOMPFixture(t, agentDir, "bucket-c", "other.jsonl", []string{
		ompSessionFixture("sess-other", otherRoot, base.Add(3*time.Minute)),
		ompUserFixture("target task", base.Add(3*time.Minute+time.Second)),
	})

	adapter := NewOMPSessionAdapter(agentDir)
	list, err := adapter.List(context.Background(), SessionListRequest{Root: root, Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 2 {
		t.Fatalf("list count = %d, want 2: %#v", len(list), list)
	}
	if list[0].ID != "sess-target" || list[1].ID != "sess-old" {
		t.Fatalf("unexpected order: %s, %s", list[0].ID, list[1].ID)
	}
	if list[0].Model != "sub/gpt-6-sol" || list[0].Thinking != "xhigh" {
		t.Fatalf("session metadata not projected: %#v", list[0])
	}

	got, err := adapter.Discover(context.Background(), SessionDiscoverRequest{
		Root:      root,
		Task:      "target task",
		StartedAt: base.Add(2*time.Minute + 5*time.Second),
	})
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if got.ID != "sess-target" {
		t.Fatalf("discover selected %q, want sess-target", got.ID)
	}
	if got.Locator == "" {
		t.Fatal("discover did not retain local locator")
	}
}

func TestOMPSessionAdapterIncrementalReadAndNormalization(t *testing.T) {
	agentDir := t.TempDir()
	root := filepath.Join(t.TempDir(), "workspace")
	base := time.Date(2026, 9, 30, 3, 0, 0, 0, time.UTC)
	path := writeOMPFixture(t, agentDir, "bucket", "session.jsonl", []string{
		ompSessionFixture("sess-1", root, base),
		ompUserFixture("please test", base.Add(time.Second)),
		ompAssistantToolCallFixture("call-1", "bash", base.Add(2*time.Second)),
		ompToolStartFixture("call-1", "bash", base.Add(3*time.Second)),
		ompToolResultFixture("call-1", "bash", "ok", false, base.Add(4*time.Second)),
		ompAssistantTextFixture("done", base.Add(5*time.Second)),
	})

	adapter := NewOMPSessionAdapter(agentDir)
	ref := HarnessSessionRef{Harness: SubagentClientOMP, ID: "sess-1", Locator: path}
	first, err := adapter.Read(context.Background(), ref, SessionReadRequest{Limit: 100})
	if err != nil {
		t.Fatalf("first read: %v", err)
	}
	if len(first.RawRecords) != 6 || first.NextCursor != 6 || !first.EOF {
		t.Fatalf("first page = raw:%d cursor:%d eof:%v", len(first.RawRecords), first.NextCursor, first.EOF)
	}

	var (
		sawUser, sawToolCall, sawToolStart, sawToolResult, sawAssistant bool
	)
	for _, entry := range first.Entries {
		switch {
		case entry.Kind == SessionEntryMessage && entry.Role == "user" && entry.Text == "please test":
			sawUser = true
		case entry.Kind == SessionEntryToolCall && entry.ToolName == "bash" && entry.ToolCall == "call-1":
			sawToolCall = true
			if string(entry.Arguments) != `{"command":"echo ok"}` {
				t.Fatalf("tool args = %s", entry.Arguments)
			}
		case entry.Kind == SessionEntryActivity && entry.Status == "started" && entry.ToolCall == "call-1":
			sawToolStart = true
		case entry.Kind == SessionEntryToolResult && entry.Status == "completed" && entry.Text == "ok":
			sawToolResult = true
		case entry.Kind == SessionEntryMessage && entry.Role == "assistant" && entry.Text == "done":
			sawAssistant = true
		}
	}
	if !sawUser || !sawToolCall || !sawToolStart || !sawToolResult || !sawAssistant {
		t.Fatalf("missing normalized entries: %#v", first.Entries)
	}

	before := len(adapter.indexes[path].records)
	appendOMPFixture(t, path, ompAssistantTextFixture("appended", base.Add(6*time.Second)))
	second, err := adapter.Read(context.Background(), ref, SessionReadRequest{After: first.NextCursor, Limit: 100})
	if err != nil {
		t.Fatalf("second read: %v", err)
	}
	if len(second.RawRecords) != 1 || second.NextCursor != 7 {
		t.Fatalf("second page = raw:%d cursor:%d", len(second.RawRecords), second.NextCursor)
	}
	if len(second.Entries) != 1 || second.Entries[0].Text != "appended" {
		t.Fatalf("second entries = %#v", second.Entries)
	}
	after := len(adapter.indexes[path].records)
	if before != 6 || after != 7 {
		t.Fatalf("incremental index size %d -> %d, want 6 -> 7", before, after)
	}
}

func TestOMPSessionAdapterNormalizeLive(t *testing.T) {
	adapter := NewOMPSessionAdapter(t.TempDir())

	line := []byte(`{"type":"message_end","message":{"role":"assistant","content":[{"type":"text","text":"finished"}],"timestamp":1790733916383}}`)
	event, err := adapter.NormalizeLive(line)
	if err != nil {
		t.Fatalf("normalize message_end: %v", err)
	}
	if event.Record.RawType != "message_end" || string(event.Record.Payload) != string(line) {
		t.Fatalf("raw live event not preserved: %#v", event.Record)
	}
	if len(event.Entries) != 1 || event.Entries[0].Kind != SessionEntryMessage || event.Entries[0].Text != "finished" {
		t.Fatalf("live projection = %#v", event.Entries)
	}
	if event.Entries[0].At.IsZero() {
		t.Fatal("numeric message timestamp was not parsed")
	}

	end, err := adapter.NormalizeLive([]byte(`{"type":"agent_end","messages":[]}`))
	if err != nil {
		t.Fatalf("normalize agent_end: %v", err)
	}
	if end.Record.RawType != "agent_end" || len(end.Entries) != 0 {
		t.Fatalf("agent_end projection = %#v", end)
	}

	if _, err := adapter.NormalizeLive([]byte("{not-json")); err == nil {
		t.Fatal("malformed live event unexpectedly accepted")
	}
}

func TestOMPSessionAdapterRejectsEscapingLocator(t *testing.T) {
	agentDir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.jsonl")
	if err := os.WriteFile(outside, []byte(ompSessionFixture("escape", "/tmp", time.Now())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewOMPSessionAdapter(agentDir)
	_, err := adapter.Read(context.Background(), HarnessSessionRef{
		Harness: SubagentClientOMP,
		ID:      "escape",
		Locator: outside,
	}, SessionReadRequest{})
	if err == nil || !strings.Contains(err.Error(), "escapes session root") {
		t.Fatalf("escaping locator error = %v", err)
	}
}

func TestReadOMPSessionMetaMatchesObservedSchemas(t *testing.T) {
	base := time.Date(2026, 9, 25, 9, 48, 10, 631000000, time.UTC)
	cases := []struct {
		name  string
		cwd   string
		id    string
		title string
	}{
		{name: "project", cwd: "/Users/test/IdeaProjects/me/codebridge", id: "01a0d7f7-project", title: "project session"},
		{name: "downloads", cwd: "/Users/test/Downloads", id: "01a0d899-downloads", title: "downloads session"},
		{name: "tmp", cwd: "/tmp/ompprobe", id: "01a0d89d-tmp", title: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			agentDir := t.TempDir()
			path := writeOMPFixture(t, agentDir, "encoded-directory-name-is-not-authoritative", tc.name+".jsonl", []string{
				ompTitleFixture(tc.title, base),
				ompSessionFixture(tc.id, tc.cwd, base),
				ompModelFixture("sub/gpt-6-sol", base.Add(time.Millisecond)),
				ompThinkingFixture("medium", base.Add(2*time.Millisecond)),
				ompUserFixture("hello", base.Add(time.Second)),
			})
			meta, err := readOMPSessionMeta(path)
			if err != nil {
				t.Fatalf("read meta: %v", err)
			}
			if meta.ref.ID != tc.id || meta.cwd != tc.cwd {
				t.Fatalf("meta = id:%q cwd:%q", meta.ref.ID, meta.cwd)
			}
			if meta.ref.Model != "sub/gpt-6-sol" || meta.ref.Thinking != "medium" {
				t.Fatalf("model/thinking = %q/%q", meta.ref.Model, meta.ref.Thinking)
			}
			if meta.firstUser != "hello" {
				t.Fatalf("first user = %q", meta.firstUser)
			}
		})
	}
}

func writeOMPFixture(t *testing.T, agentDir, bucket, name string, lines []string) string {
	t.Helper()
	dir := filepath.Join(agentDir, "sessions", bucket)
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendOMPFixture(t *testing.T, path, line string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(line + "\n"); err != nil {
		t.Fatal(err)
	}
}

func ompTitleFixture(title string, at time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"type": "title", "v": 1, "title": title, "updatedAt": at.Format(time.RFC3339Nano),
	})
	return string(b)
}

func ompSessionFixture(id, cwd string, at time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"type": "session", "version": 3, "id": id,
		"timestamp": at.Format(time.RFC3339Nano), "cwd": cwd,
	})
	return string(b)
}

func ompModelFixture(model string, at time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"type": "model_change", "timestamp": at.Format(time.RFC3339Nano), "model": model,
	})
	return string(b)
}

func ompThinkingFixture(level string, at time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"type": "thinking_level_change", "timestamp": at.Format(time.RFC3339Nano), "thinkingLevel": level,
	})
	return string(b)
}

func ompUserFixture(text string, at time.Time) string {
	return ompMessageFixture("user", []map[string]any{{"type": "text", "text": text}}, "", "", false, at)
}

func ompAssistantTextFixture(text string, at time.Time) string {
	return ompMessageFixture("assistant", []map[string]any{{"type": "text", "text": text}}, "", "", false, at)
}

func ompAssistantToolCallFixture(callID, tool string, at time.Time) string {
	return ompMessageFixture("assistant", []map[string]any{{
		"type": "toolCall", "id": callID, "name": tool,
		"arguments": map[string]any{"command": "echo ok"},
	}}, "", "", false, at)
}

func ompToolResultFixture(callID, tool, text string, isError bool, at time.Time) string {
	return ompMessageFixture("toolResult", []map[string]any{{"type": "text", "text": text}}, callID, tool, isError, at)
}

func ompMessageFixture(role string, content []map[string]any, callID, tool string, isError bool, at time.Time) string {
	message := map[string]any{
		"role": role, "content": content, "timestamp": at.UnixMilli(),
	}
	if callID != "" {
		message["toolCallId"] = callID
	}
	if tool != "" {
		message["toolName"] = tool
	}
	if isError {
		message["isError"] = true
	}
	b, _ := json.Marshal(map[string]any{
		"type": "message", "timestamp": at.Format(time.RFC3339Nano), "message": message,
	})
	return string(b)
}

func ompToolStartFixture(callID, tool string, at time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"type": "custom", "customType": "tool_execution_start",
		"timestamp": at.Format(time.RFC3339Nano),
		"data": map[string]any{
			"toolCallId": callID, "toolName": tool,
			"startedAt": at.Format(time.RFC3339Nano),
			"args":      map[string]any{"command": "echo ok"},
			"intent":    "run test command",
		},
	})
	return string(b)
}
