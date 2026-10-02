package agentops

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestHarnessSessionRefJSONOmitsLocalLocator(t *testing.T) {
	ref := HarnessSessionRef{
		Harness: "omp",
		ID:      "sess_opaque_123",
		Title:   "demo",
		Locator: "/Users/alice/.omp/sessions/private/session.jsonl",
		root:    "/Users/alice/work/private-repo",
	}

	b, err := json.Marshal(ref)
	if err != nil {
		t.Fatalf("marshal session ref: %v", err)
	}
	if bytes.Contains(b, []byte("/Users/alice")) {
		t.Fatalf("serialized session ref leaked a physical path: %s", b)
	}
	if bytes.Contains(b, []byte("Locator")) || bytes.Contains(b, []byte("locator")) {
		t.Fatalf("serialized session ref exposed locator field: %s", b)
	}
	if !bytes.Contains(b, []byte("sess_opaque_123")) {
		t.Fatalf("serialized session ref lost opaque id: %s", b)
	}
}

func TestHarnessRawRecordPreservesNativePayloadLocally(t *testing.T) {
	raw := json.RawMessage(`{"type":"message","cwd":"/Users/alice/private","text":"hello"}`)
	event := HarnessLiveEvent{
		Record: HarnessRawRecord{
			SourceSeq: 7,
			At:        time.Unix(1_700_000_000, 0).UTC(),
			RawType:   "message",
			Payload:   append(json.RawMessage(nil), raw...),
		},
		Entries: []SessionEntry{{
			SourceSeq: 7,
			Kind:      SessionEntryMessage,
			Role:      "assistant",
			Text:      "hello",
		}},
	}

	if !bytes.Equal(event.Record.Payload, raw) {
		t.Fatalf("native payload changed: got %q want %q", event.Record.Payload, raw)
	}

	// Default JSON serialization is safe for upstream use: the native record
	// (which may carry physical paths) is local-only unless a later API builds
	// an explicit, reviewed raw projection.
	b, err := json.Marshal(event)
	if err != nil {
		t.Fatalf("marshal live event: %v", err)
	}
	if bytes.Contains(b, []byte("/Users/alice")) || bytes.Contains(b, []byte("cwd")) {
		t.Fatalf("serialized live event leaked native payload: %s", b)
	}
	if !bytes.Contains(b, []byte(`"entries"`)) {
		t.Fatalf("serialized live event lost presentation projection: %s", b)
	}
}
