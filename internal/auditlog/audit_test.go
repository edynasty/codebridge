package auditlog

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestLogContainsMetadataOnly(t *testing.T) {
	var buf bytes.Buffer
	l := &Logger{writer: &buf}
	err := l.Log(Event{
		Time:       time.Unix(1, 0).UTC(),
		Event:      "mcp.tool",
		RequestID:  "req-1",
		ActorID:    "user-1",
		Tool:       "read_file",
		DeviceID:   "mac-1",
		Workspace:  "pms",
		Success:    Bool(true),
		DurationMS: 12,
	})
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &got); err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"path", "query", "pattern", "arguments", "result", "content", "token"} {
		if _, exists := got[forbidden]; exists {
			t.Fatalf("audit log contains forbidden field %q", forbidden)
		}
	}
	if got["tool"] != "read_file" || got["workspace"] != "pms" {
		t.Fatalf("unexpected audit event: %#v", got)
	}
}

func TestFileMode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "audit", "audit.jsonl")
	l, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	defer l.Close()
	if err := l.Log(Event{Event: "test"}); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		st, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if st.Mode().Perm() != 0o600 {
			t.Fatalf("mode=%o, want 600", st.Mode().Perm())
		}
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), `"event":"test"`) {
		t.Fatalf("missing audit line: %s", b)
	}
}

func TestRequestIDsDiffer(t *testing.T) {
	a, b := NewRequestID(), NewRequestID()
	if a == "" || b == "" || a == b {
		t.Fatalf("bad request ids: %q %q", a, b)
	}
}
