// Package hostipc implements the frozen Host IPC v1 wire between codebridged
// (listener) and CodeBridge.app (connector): length-prefixed JSON-RPC 2.0 over
// a Unix domain socket with optional binary attachment frames.
//
// The language-neutral schema lives in schema/hostipc/v1 and is the source of
// truth; this package is the Go binding. Framing, version negotiation,
// authentication and the service/role matrix are documented there.
package hostipc

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

// Frame bounds (schema/hostipc/v1/README.md §2). Both peers must enforce them.
const (
	// MaxJSONFrame bounds a JSON control frame payload.
	MaxJSONFrame = 1 << 20 // 1 MiB
	// MaxAttachmentFrame bounds a binary attachment frame payload
	// (meta length prefix + meta JSON + raw bytes).
	MaxAttachmentFrame = 16 << 20 // 16 MiB
)

// attachmentFlag is bit 31 of the frame header; set marks an attachment frame.
const attachmentFlag uint32 = 1 << 31

// FrameKind distinguishes the two frame payload shapes.
type FrameKind int

const (
	// FrameJSON is a JSON control frame carrying one JSON-RPC 2.0 message.
	FrameJSON FrameKind = iota
	// FrameAttachment is a binary attachment frame.
	FrameAttachment
)

func (k FrameKind) String() string {
	switch k {
	case FrameJSON:
		return "json"
	case FrameAttachment:
		return "attachment"
	default:
		return fmt.Sprintf("kind(%d)", int(k))
	}
}

// Limits bounds inbound frames. Zero fields select the defaults above.
type Limits struct {
	MaxJSON       int
	MaxAttachment int
}

// WithDefaults returns l with every unset bound filled in.
func (l Limits) WithDefaults() Limits {
	if l.MaxJSON <= 0 {
		l.MaxJSON = MaxJSONFrame
	}
	if l.MaxAttachment <= 0 {
		l.MaxAttachment = MaxAttachmentFrame
	}
	return l
}

// ProtocolError is a framing/parsing failure that must close the connection.
// Code is one of the Host IPC v1 error codes; Reason is the stable machine
// readable string documented in schema/hostipc/v1/README.md §8.
type ProtocolError struct {
	Code   int
	Reason string
	Detail string
}

func (e *ProtocolError) Error() string {
	if e.Detail == "" {
		return fmt.Sprintf("hostipc: %s (%d)", e.Reason, e.Code)
	}
	return fmt.Sprintf("hostipc: %s (%d): %s", e.Reason, e.Code, e.Detail)
}

// NewProtocolError builds a ProtocolError.
func NewProtocolError(code int, reason, detail string) *ProtocolError {
	return &ProtocolError{Code: code, Reason: reason, Detail: detail}
}

// ErrFrameTooLarge reports a frame payload outside the negotiated bounds.
var ErrFrameTooLarge = &ProtocolError{Code: CodeFrameLimit, Reason: "frame_limit"}

// AttachmentMeta is the meta block of an attachment frame.
type AttachmentMeta struct {
	AttachmentID string `json:"attachment_id"`
	MIME         string `json:"mime"`
	Bytes        int    `json:"bytes"`
	SHA256       string `json:"sha256,omitempty"`
	Role         string `json:"role,omitempty"`
}

// ReadFrame reads exactly one frame. It returns io.EOF when the peer closed the
// connection cleanly on a frame boundary. Any other read failure is
// io.ErrUnexpectedEOF or an *ProtocolError that requires closing the
// connection.
//
// For FrameJSON the returned payload is the raw JSON bytes (validation that it
// is a single JSON object is the caller's job, so it can answer -32700 vs
// -32600 correctly). For FrameAttachment the payload is the meta JSON and data
// is the raw attachment bytes.
func ReadFrame(r io.Reader, lim Limits) (kind FrameKind, payload, data []byte, err error) {
	lim = lim.WithDefaults()
	var header [4]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		if errors.Is(err, io.EOF) {
			return 0, nil, nil, io.EOF
		}
		if errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil, err
	}
	raw := binary.BigEndian.Uint32(header[:])
	kind = FrameJSON
	if raw&attachmentFlag != 0 {
		kind = FrameAttachment
	}
	n := int(raw &^ attachmentFlag)
	max := lim.MaxJSON
	if kind == FrameAttachment {
		max = lim.MaxAttachment
	}
	if n <= 0 {
		return 0, nil, nil, NewProtocolError(CodeFrameLimit, "frame_limit", "empty frame")
	}
	if n > max {
		return 0, nil, nil, NewProtocolError(CodeFrameLimit, "frame_limit",
			fmt.Sprintf("%s frame of %d bytes exceeds %d", kind, n, max))
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(r, buf); err != nil {
		if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
			return 0, nil, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil, err
	}
	if kind == FrameJSON {
		return kind, buf, nil, nil
	}
	if n < 4 {
		return 0, nil, nil, NewProtocolError(CodeInvalidRequest, "invalid_request", "attachment frame without meta length")
	}
	metaLen := int(binary.BigEndian.Uint32(buf[:4]))
	if metaLen <= 0 || metaLen > n-4 {
		return 0, nil, nil, NewProtocolError(CodeInvalidRequest, "invalid_request", "attachment meta length out of range")
	}
	return kind, buf[4 : 4+metaLen], buf[4+metaLen:], nil
}

// WriteFrame writes one framed payload with the given kind. It enforces the
// same bounds as ReadFrame so a local bug cannot emit an unreadable frame.
func WriteFrame(w io.Writer, kind FrameKind, payload []byte, lim Limits) error {
	lim = lim.WithDefaults()
	max := lim.MaxJSON
	if kind == FrameAttachment {
		max = lim.MaxAttachment
	}
	if len(payload) == 0 {
		return NewProtocolError(CodeFrameLimit, "frame_limit", "empty frame")
	}
	if len(payload) > max {
		return NewProtocolError(CodeFrameLimit, "frame_limit",
			fmt.Sprintf("%s frame of %d bytes exceeds %d", kind, len(payload), max))
	}
	header := uint32(len(payload))
	if kind == FrameAttachment {
		header |= attachmentFlag
	}
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], header)
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}

// WriteJSONFrame marshals msg and writes it as one JSON control frame.
func WriteJSONFrame(w io.Writer, msg any, lim Limits) error {
	payload, err := marshalMessage(msg)
	if err != nil {
		return err
	}
	return WriteFrame(w, FrameJSON, payload, lim)
}

// WriteAttachmentFrame writes one attachment frame. meta.Bytes must equal
// len(data); the meta block is emitted verbatim apart from that check.
func WriteAttachmentFrame(w io.Writer, meta AttachmentMeta, data []byte, lim Limits) error {
	if meta.Bytes != len(data) {
		return fmt.Errorf("hostipc: attachment meta bytes=%d but data is %d", meta.Bytes, len(data))
	}
	metaJSON, err := marshalMessage(meta)
	if err != nil {
		return err
	}
	payload := make([]byte, 4+len(metaJSON)+len(data))
	binary.BigEndian.PutUint32(payload[:4], uint32(len(metaJSON)))
	copy(payload[4:], metaJSON)
	copy(payload[4+len(metaJSON):], data)
	return WriteFrame(w, FrameAttachment, payload, lim)
}

// DecodeAttachmentMeta decodes the meta block returned by ReadFrame for a
// FrameAttachment frame.
func DecodeAttachmentMeta(payload []byte) (AttachmentMeta, error) {
	var meta AttachmentMeta
	if err := unmarshalStrict(payload, &meta); err != nil {
		return AttachmentMeta{}, NewProtocolError(CodeInvalidRequest, "invalid_request", "attachment meta is not a JSON object")
	}
	if meta.AttachmentID == "" || meta.MIME == "" {
		return AttachmentMeta{}, NewProtocolError(CodeInvalidRequest, "invalid_request", "attachment meta missing attachment_id or mime")
	}
	return meta, nil
}
