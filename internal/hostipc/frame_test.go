package hostipc

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"errors"
	"io"
	"testing"
)

func TestFrameJSONRoundTrip(t *testing.T) {
	var buf bytes.Buffer
	msg := map[string]any{"jsonrpc": "2.0", "id": "1", "method": "host.health"}
	if err := WriteJSONFrame(&buf, msg, Limits{}); err != nil {
		t.Fatalf("WriteJSONFrame: %v", err)
	}
	kind, payload, data, err := ReadFrame(&buf, Limits{})
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if kind != FrameJSON {
		t.Fatalf("kind = %v, want json", kind)
	}
	if data != nil {
		t.Fatalf("attachment data = %v, want nil", data)
	}
	var got map[string]any
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatalf("payload is not JSON: %v", err)
	}
	if got["method"] != "host.health" {
		t.Fatalf("method = %v", got["method"])
	}
}

func TestFrameAttachmentRoundTrip(t *testing.T) {
	raw := []byte{0x89, 'P', 'N', 'G', 0x00, 0x01}
	meta := AttachmentMeta{AttachmentID: "frame-1", MIME: "image/png", Bytes: len(raw)}
	var buf bytes.Buffer
	if err := WriteAttachmentFrame(&buf, meta, raw, Limits{}); err != nil {
		t.Fatalf("WriteAttachmentFrame: %v", err)
	}
	kind, payload, data, err := ReadFrame(&buf, Limits{})
	if err != nil {
		t.Fatalf("ReadFrame: %v", err)
	}
	if kind != FrameAttachment {
		t.Fatalf("kind = %v, want attachment", kind)
	}
	if !bytes.Equal(data, raw) {
		t.Fatalf("data = %v, want %v", data, raw)
	}
	got, err := DecodeAttachmentMeta(payload)
	if err != nil {
		t.Fatalf("DecodeAttachmentMeta: %v", err)
	}
	if got != meta {
		t.Fatalf("meta = %+v, want %+v", got, meta)
	}
}

func TestFramePartialReadIsUnexpectedEOF(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteJSONFrame(&buf, map[string]any{"jsonrpc": "2.0", "id": "1", "method": "x"}, Limits{}); err != nil {
		t.Fatalf("WriteJSONFrame: %v", err)
	}
	full := buf.Bytes()
	for _, n := range []int{1, 2, 3, 4, 5, len(full) - 1} {
		if n >= len(full) {
			continue
		}
		_, _, _, err := ReadFrame(bytes.NewReader(full[:n]), Limits{})
		if err == nil {
			t.Fatalf("ReadFrame(%d bytes) succeeded, want error", n)
		}
		if !errors.Is(err, io.ErrUnexpectedEOF) {
			t.Fatalf("ReadFrame(%d bytes) error = %v, want io.ErrUnexpectedEOF", n, err)
		}
	}
}

func TestFrameCleanEOFOnBoundary(t *testing.T) {
	_, _, _, err := ReadFrame(bytes.NewReader(nil), Limits{})
	if !errors.Is(err, io.EOF) {
		t.Fatalf("err = %v, want io.EOF", err)
	}
}

func TestFrameBoundsRejected(t *testing.T) {
	cases := []struct {
		name    string
		header  uint32
		payload []byte
		limits  Limits
		want    string
	}{
		{
			name:   "zero length json",
			header: 0,
			want:   "frame_limit",
		},
		{
			name:   "zero length attachment",
			header: attachmentFlag,
			want:   "frame_limit",
		},
		{
			name:   "json over limit",
			header: uint32(MaxJSONFrame) + 1,
			want:   "frame_limit",
		},
		{
			name:   "json over custom limit",
			header: 1024,
			limits: Limits{MaxJSON: 16},
			want:   "frame_limit",
		},
		{
			name:   "attachment over limit",
			header: attachmentFlag | uint32(MaxAttachmentFrame) + 1,
			want:   "frame_limit",
		},
		{
			name:    "attachment meta length out of range",
			header:  attachmentFlag | 6,
			payload: append(binary.BigEndian.AppendUint32(nil, 100), '{', '}'),
			want:    "invalid_request",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			_ = binary.Write(&buf, binary.BigEndian, tc.header)
			buf.Write(tc.payload)
			_, _, _, err := ReadFrame(&buf, tc.limits)
			var perr *ProtocolError
			if !errors.As(err, &perr) {
				t.Fatalf("err = %v, want *ProtocolError", err)
			}
			if perr.Reason != tc.want {
				t.Fatalf("reason = %q, want %q", perr.Reason, tc.want)
			}
		})
	}
}

func TestWriteFrameRejectsOversize(t *testing.T) {
	var buf bytes.Buffer
	big := make([]byte, MaxJSONFrame+1)
	if err := WriteFrame(&buf, FrameJSON, big, Limits{}); err == nil {
		t.Fatal("WriteFrame accepted an oversize JSON frame")
	}
	if err := WriteFrame(&buf, FrameJSON, nil, Limits{}); err == nil {
		t.Fatal("WriteFrame accepted an empty frame")
	}
}

func TestWriteAttachmentMetaMismatch(t *testing.T) {
	var buf bytes.Buffer
	meta := AttachmentMeta{AttachmentID: "a", MIME: "image/png", Bytes: 99}
	if err := WriteAttachmentFrame(&buf, meta, []byte("short"), Limits{}); err == nil {
		t.Fatal("WriteAttachmentFrame accepted a meta/data size mismatch")
	}
}

func TestDecodeMessageCodes(t *testing.T) {
	cases := []struct {
		name string
		in   string
		code int
	}{
		{"not json", "not-json", CodeParse},
		{"json array", "[1,2]", CodeInvalidRequest},
		{"json scalar", "\"hello\"", CodeInvalidRequest},
		{"wrong version", `{"jsonrpc":"1.0","id":"1","method":"x"}`, CodeInvalidRequest},
		{"no method or id", `{"jsonrpc":"2.0"}`, CodeInvalidRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeMessage([]byte(tc.in))
			if err == nil {
				t.Fatal("expected error")
			}
			if err.Code != tc.code {
				t.Fatalf("code = %d, want %d (%s)", err.Code, tc.code, tc.in)
			}
		})
	}
}

func TestDecodeMessageKinds(t *testing.T) {
	req, err := DecodeMessage([]byte(`{"jsonrpc":"2.0","id":"7","method":"host.health"}`))
	if err != nil {
		t.Fatalf("request decode: %v", err)
	}
	if req.Kind() != KindRequest {
		t.Fatalf("kind = %v, want request", req.Kind())
	}
	notif, derr := DecodeMessage([]byte(`{"jsonrpc":"2.0","method":"notify.post","params":{}}`))
	if derr != nil {
		t.Fatalf("notification decode: %v", derr)
	}
	if notif.Kind() != KindNotification {
		t.Fatalf("kind = %v, want notification", notif.Kind())
	}
	resp, derr := DecodeMessage([]byte(`{"jsonrpc":"2.0","id":"7","result":{}}`))
	if derr != nil {
		t.Fatalf("response decode: %v", derr)
	}
	if resp.Kind() != KindResponse {
		t.Fatalf("kind = %v, want response", resp.Kind())
	}
}
