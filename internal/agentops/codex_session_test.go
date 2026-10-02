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

func TestCodexSessionAdapterListDiscoverReadAndIncrementalAppend(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "workspace")
	base := time.Date(2026, 9, 30, 8, 0, 0, 0, time.UTC)
	path := writeCodexFixture(t, home, "2026/09/30", "rollout-main.jsonl", []string{
		codexSessionMetaFixture("thread-main", root, base),
		codexTurnContextFixture("gpt-6-sol", "high", base.Add(time.Second)),
		codexMessageFixture("user", "do the thing", base.Add(2*time.Second)),
		codexEventFixture("task_started", base.Add(3*time.Second)),
		codexFunctionCallFixture("fc-1", "shell", "call-1", `{"command":"go test ./..."}`, base.Add(4*time.Second)),
		codexFunctionOutputFixture("call-1", "ok", base.Add(5*time.Second)),
		codexCustomCallFixture("ctc-1", "exec", "call-2", "console.log('ok')", base.Add(6*time.Second)),
		codexCustomOutputFixture("call-2", "custom ok", base.Add(7*time.Second)),
		codexReasoningFixture(base.Add(8 * time.Second)),
		codexMessageFixture("assistant", "done", base.Add(9*time.Second)),
		codexEventFixture("task_complete", base.Add(10*time.Second)),
	})
	writeCodexFixture(t, home, "2026/09/30", "rollout-other.jsonl", []string{
		codexSessionMetaFixture("thread-other", filepath.Join(t.TempDir(), "other"), base.Add(time.Minute)),
		codexTurnContextFixture("gpt-5.6-luna", "medium", base.Add(time.Minute+time.Second)),
		codexMessageFixture("user", "other task", base.Add(time.Minute+2*time.Second)),
	})

	adapter := NewCodexSessionAdapter(home)
	list, err := adapter.List(context.Background(), SessionListRequest{Root: root, Limit: 10})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("list count = %d, want 1: %#v", len(list), list)
	}
	if list[0].ID != "thread-main" || list[0].Model != "gpt-6-sol" || list[0].Thinking != "high" {
		t.Fatalf("unexpected session metadata: %#v", list[0])
	}
	if list[0].Title != "do the thing" {
		t.Fatalf("title = %q", list[0].Title)
	}

	got, err := adapter.Discover(context.Background(), SessionDiscoverRequest{
		Root: root, NativeID: "thread-main", StartedAt: base.Add(20 * time.Second),
	})
	if err != nil {
		t.Fatalf("discover by native id: %v", err)
	}
	if got.ID != "thread-main" {
		t.Fatalf("discover by native id = %q", got.ID)
	}

	got, err = adapter.Discover(context.Background(), SessionDiscoverRequest{
		Root: root, Task: "do the thing", StartedAt: base.Add(3 * time.Second),
	})
	if err != nil {
		t.Fatalf("discover by task: %v", err)
	}
	if got.ID != "thread-main" {
		t.Fatalf("discover by task = %q", got.ID)
	}

	page, err := adapter.Read(context.Background(), list[0], SessionReadRequest{Limit: 100})
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(page.RawRecords) != 11 || page.NextCursor != 11 || !page.EOF {
		t.Fatalf("page raw=%d cursor=%d eof=%v", len(page.RawRecords), page.NextCursor, page.EOF)
	}

	var sawUser, sawAssistant, sawFuncCall, sawFuncResult, sawCustomCall, sawCustomResult bool
	for _, entry := range page.Entries {
		switch {
		case entry.Kind == SessionEntryMessage && entry.Role == "user" && entry.Text == "do the thing":
			sawUser = true
		case entry.Kind == SessionEntryMessage && entry.Role == "assistant" && entry.Text == "done":
			sawAssistant = true
		case entry.Kind == SessionEntryToolCall && entry.ToolName == "shell" && entry.ToolCall == "call-1":
			sawFuncCall = true
			if string(entry.Arguments) != `{"command":"go test ./..."}` {
				t.Fatalf("function args = %s", entry.Arguments)
			}
		case entry.Kind == SessionEntryToolResult && entry.ToolCall == "call-1" && entry.Text == "ok":
			sawFuncResult = true
		case entry.Kind == SessionEntryToolCall && entry.ToolName == "exec" && entry.ToolCall == "call-2":
			sawCustomCall = true
		case entry.Kind == SessionEntryToolResult && entry.ToolCall == "call-2" && entry.Text == "custom ok":
			sawCustomResult = true
		}
		if entry.RawType == "response_item/reasoning" {
			t.Fatalf("reasoning unexpectedly projected: %#v", entry)
		}
	}
	if !sawUser || !sawAssistant || !sawFuncCall || !sawFuncResult || !sawCustomCall || !sawCustomResult {
		t.Fatalf("missing projected entries: %#v", page.Entries)
	}

	before := len(adapter.indexes[path].records)
	appendCodexFixture(t, path, codexMessageFixture("assistant", "appended", base.Add(11*time.Second)))
	next, err := adapter.Read(context.Background(), list[0], SessionReadRequest{After: page.NextCursor, Limit: 100})
	if err != nil {
		t.Fatalf("incremental read: %v", err)
	}
	if len(next.RawRecords) != 1 || next.NextCursor != 12 || len(next.Entries) != 1 || next.Entries[0].Text != "appended" {
		t.Fatalf("incremental page: %#v", next)
	}
	after := len(adapter.indexes[path].records)
	if before != 11 || after != 12 {
		t.Fatalf("index size %d -> %d, want 11 -> 12", before, after)
	}
}

func TestCodexSessionAdapterReadsArchivedSessions(t *testing.T) {
	home := t.TempDir()
	root := filepath.Join(t.TempDir(), "workspace")
	base := time.Date(2026, 8, 2, 9, 0, 0, 0, time.UTC)
	path := filepath.Join(home, "archived_sessions", "rollout-old.jsonl")
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	body := strings.Join([]string{
		codexSessionMetaFixture("thread-old", root, base),
		codexTurnContextFixture("gpt-5.6-luna", "medium", base.Add(time.Second)),
		codexMessageFixture("user", "old task", base.Add(2*time.Second)),
	}, "\n") + "\n"
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewCodexSessionAdapter(home)
	list, err := adapter.List(context.Background(), SessionListRequest{Root: root})
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "thread-old" {
		t.Fatalf("archived list = %#v", list)
	}
}

func TestCodexSessionAdapterNormalizeLive(t *testing.T) {
	adapter := NewCodexSessionAdapter(t.TempDir())

	threadLine := []byte(`{"type":"thread.started","thread_id":"thread-123"}`)
	thread, err := adapter.NormalizeLive(threadLine)
	if err != nil {
		t.Fatal(err)
	}
	if thread.Record.RawType != "thread.started" || string(thread.Record.Payload) != string(threadLine) {
		t.Fatalf("thread raw not preserved: %#v", thread.Record)
	}
	if len(thread.Entries) != 1 || !strings.Contains(thread.Entries[0].Text, "thread-123") {
		t.Fatalf("thread projection = %#v", thread.Entries)
	}

	start, err := adapter.NormalizeLive([]byte(`{"type":"item.started","item":{"id":"item-1","type":"command_execution","command":"go test ./...","status":"in_progress"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(start.Entries) != 1 || start.Entries[0].Kind != SessionEntryToolCall || start.Entries[0].ToolCall != "item-1" {
		t.Fatalf("command start = %#v", start.Entries)
	}

	done, err := adapter.NormalizeLive([]byte(`{"type":"item.completed","item":{"id":"item-1","type":"command_execution","command":"go test ./...","aggregated_output":"ok\n","exit_code":0,"status":"completed"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(done.Entries) != 1 || done.Entries[0].Kind != SessionEntryToolResult || done.Entries[0].Text != "ok\n" {
		t.Fatalf("command completed = %#v", done.Entries)
	}

	msg, err := adapter.NormalizeLive([]byte(`{"type":"item.completed","item":{"id":"item-2","type":"agent_message","text":"final answer"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(msg.Entries) != 1 || msg.Entries[0].Role != "assistant" || msg.Entries[0].Text != "final answer" {
		t.Fatalf("agent message = %#v", msg.Entries)
	}

	reasoning, err := adapter.NormalizeLive([]byte(`{"type":"item.completed","item":{"id":"item-3","type":"reasoning","text":"hidden"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(reasoning.Entries) != 0 {
		t.Fatalf("reasoning must not be projected: %#v", reasoning.Entries)
	}
}

func TestCodexSessionAdapterRealSessionSmoke(t *testing.T) {
	home := strings.TrimSpace(os.Getenv("CODEBRIDGE_CODEX_REAL_HOME"))
	if home == "" {
		t.Skip("set CODEBRIDGE_CODEX_REAL_HOME to run against a real Codex home")
	}
	adapter := NewCodexSessionAdapter(home)
	list, err := adapter.List(context.Background(), SessionListRequest{Limit: 5})
	if err != nil {
		t.Fatalf("real list: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("real Codex home has no readable sessions")
	}
	page, err := adapter.Read(context.Background(), list[0], SessionReadRequest{Limit: 50})
	if err != nil {
		t.Fatalf("real read: %v", err)
	}
	if page.Session.ID == "" || len(page.RawRecords) == 0 {
		t.Fatalf("real session incomplete: id=%q raw=%d", page.Session.ID, len(page.RawRecords))
	}
}

func TestCodexSessionAdapterRejectsEscapingLocator(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "rollout-outside.jsonl")
	if err := os.WriteFile(outside, []byte(codexSessionMetaFixture("escape", "/tmp", time.Now())+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	adapter := NewCodexSessionAdapter(home)
	_, err := adapter.Read(context.Background(), HarnessSessionRef{
		Harness: SubagentClientCodex, ID: "escape", Locator: outside,
	}, SessionReadRequest{})
	if err == nil || !strings.Contains(err.Error(), "escapes CODEX_HOME") {
		t.Fatalf("escaping locator error = %v", err)
	}
}

func writeCodexFixture(t *testing.T, home, dayPath, name string, lines []string) string {
	t.Helper()
	dir := filepath.Join(home, "sessions", filepath.FromSlash(dayPath))
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func appendCodexFixture(t *testing.T, path, line string) {
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

func codexEnvelopeFixture(kind string, payload any, at time.Time) string {
	b, _ := json.Marshal(map[string]any{
		"timestamp": at.Format(time.RFC3339Nano),
		"ordinal":   at.UnixNano(),
		"type":      kind,
		"payload":   payload,
	})
	return string(b)
}

func codexSessionMetaFixture(id, cwd string, at time.Time) string {
	return codexEnvelopeFixture("session_meta", map[string]any{
		"session_id":     id,
		"id":             id,
		"timestamp":      at.Format(time.RFC3339Nano),
		"cwd":            cwd,
		"originator":     "codex_exec",
		"cli_version":    "0.159.2",
		"model_provider": "openai",
	}, at)
}

func codexTurnContextFixture(model, effort string, at time.Time) string {
	return codexEnvelopeFixture("turn_context", map[string]any{
		"model":  model,
		"effort": effort,
		"collaboration_mode": map[string]any{
			"settings": map[string]any{"reasoning_effort": effort},
		},
	}, at)
}

func codexMessageFixture(role, text string, at time.Time) string {
	contentType := "input_text"
	if role == "assistant" {
		contentType = "output_text"
	}
	return codexEnvelopeFixture("response_item", map[string]any{
		"type":    "message",
		"id":      "msg-" + role,
		"role":    role,
		"content": []map[string]any{{"type": contentType, "text": text}},
	}, at)
}

func codexFunctionCallFixture(id, name, callID, args string, at time.Time) string {
	return codexEnvelopeFixture("response_item", map[string]any{
		"type": "function_call", "id": id, "name": name, "call_id": callID, "arguments": args,
	}, at)
}

func codexFunctionOutputFixture(callID, output string, at time.Time) string {
	return codexEnvelopeFixture("response_item", map[string]any{
		"type": "function_call_output", "call_id": callID, "output": output,
	}, at)
}

func codexCustomCallFixture(id, name, callID, input string, at time.Time) string {
	return codexEnvelopeFixture("response_item", map[string]any{
		"type": "custom_tool_call", "id": id, "name": name, "call_id": callID,
		"status": "completed", "input": input,
	}, at)
}

func codexCustomOutputFixture(callID, output string, at time.Time) string {
	return codexEnvelopeFixture("response_item", map[string]any{
		"type": "custom_tool_call_output", "call_id": callID,
		"output": []map[string]any{{"type": "input_text", "text": output}},
	}, at)
}

func codexReasoningFixture(at time.Time) string {
	return codexEnvelopeFixture("response_item", map[string]any{
		"type": "reasoning", "summary": []any{}, "content": nil,
	}, at)
}

func codexEventFixture(kind string, at time.Time) string {
	return codexEnvelopeFixture("event_msg", map[string]any{"type": kind, "turn_id": "turn-1"}, at)
}
