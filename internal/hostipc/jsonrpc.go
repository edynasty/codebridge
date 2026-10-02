package hostipc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
)

// JSONRPCVersion is the only accepted value of the "jsonrpc" field.
const JSONRPCVersion = "2.0"

// Message is one JSON-RPC 2.0 message. Exactly one of the request,
// notification and response shapes is populated; Kind reports which.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *Error          `json:"error,omitempty"`
}

// MessageKind classifies a decoded message.
type MessageKind int

const (
	// KindUnknown is an unclassifiable message.
	KindUnknown MessageKind = iota
	// KindRequest expects a response.
	KindRequest
	// KindNotification expects no response.
	KindNotification
	// KindResponse answers an earlier request.
	KindResponse
)

func (k MessageKind) String() string {
	switch k {
	case KindRequest:
		return "request"
	case KindNotification:
		return "notification"
	case KindResponse:
		return "response"
	default:
		return "unknown"
	}
}

// Kind classifies the message after decoding.
func (m *Message) Kind() MessageKind {
	switch {
	case m.Method != "" && len(m.ID) > 0:
		return KindRequest
	case m.Method != "":
		return KindNotification
	case len(m.ID) > 0 && (len(m.Result) > 0 || m.Error != nil):
		return KindResponse
	default:
		return KindUnknown
	}
}

// NewRequest builds a request message.
func NewRequest(id json.RawMessage, method string, params any) (*Message, error) {
	raw, err := marshalOptional(params)
	if err != nil {
		return nil, err
	}
	if len(id) == 0 {
		id = json.RawMessage(`"1"`)
	}
	return &Message{JSONRPC: JSONRPCVersion, ID: id, Method: method, Params: raw}, nil
}

// NewNotification builds a notification message.
func NewNotification(method string, params any) (*Message, error) {
	raw, err := marshalOptional(params)
	if err != nil {
		return nil, err
	}
	return &Message{JSONRPC: JSONRPCVersion, Method: method, Params: raw}, nil
}

// NewResponse builds a successful response.
func NewResponse(id json.RawMessage, result any) (*Message, error) {
	raw, err := marshalOptional(result)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	return &Message{JSONRPC: JSONRPCVersion, ID: id, Result: raw}, nil
}

// NewErrorResponse builds an error response.
func NewErrorResponse(id json.RawMessage, e *Error) *Message {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	return &Message{JSONRPC: JSONRPCVersion, ID: id, Error: e}
}

// DecodeMessage parses frame payload bytes into a Message. It distinguishes
// -32700 (bytes are not JSON) from -32600 (JSON that is not a JSON-RPC 2.0
// object) so the peer gets the documented code.
func DecodeMessage(payload []byte) (*Message, *Error) {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || !json.Valid(trimmed) {
		return nil, NewError(CodeParse, "parse", "frame payload is not valid JSON")
	}
	if trimmed[0] != '{' {
		return nil, NewError(CodeInvalidRequest, "invalid_request", "message is not a JSON object")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &probe); err != nil {
		return nil, NewError(CodeInvalidRequest, "invalid_request", "message is not a JSON object")
	}
	var m Message
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, NewError(CodeInvalidRequest, "invalid_request", "message fields are malformed: "+err.Error())
	}
	if m.JSONRPC != JSONRPCVersion {
		return nil, NewError(CodeInvalidRequest, "invalid_request", fmt.Sprintf("jsonrpc must be %q", JSONRPCVersion))
	}
	if len(m.Method) == 0 && len(m.ID) == 0 {
		return nil, NewError(CodeInvalidRequest, "invalid_request", "message has neither method nor id")
	}
	return &m, nil
}

// DecodeParams decodes params into v. An empty params block is treated as the
// zero value so a no-argument request is valid.
func DecodeParams(params json.RawMessage, v any) *Error {
	trimmed := bytes.TrimSpace(params)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}
	// Unknown fields are ignored on the wire (schema/hostipc/v1/README.md §4),
	// so this decoder is deliberately lenient; a future minor version may add
	// fields without breaking an older peer.
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return errInvalidParams(err.Error())
	}
	if dec.More() {
		return errInvalidParams("trailing data after params object")
	}
	return nil
}

func marshalMessage(v any) ([]byte, error) {
	return json.Marshal(v)
}

func marshalOptional(v any) (json.RawMessage, error) {
	if v == nil {
		return nil, nil
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return raw, nil
}

// unmarshalStrict decodes payload into v, requiring a JSON object and
// rejecting trailing bytes.
func unmarshalStrict(payload []byte, v any) error {
	trimmed := bytes.TrimSpace(payload)
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return fmt.Errorf("payload is not a JSON object")
	}
	dec := json.NewDecoder(bytes.NewReader(trimmed))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if _, err := dec.Token(); err != io.EOF {
		return fmt.Errorf("trailing data after JSON object")
	}
	return nil
}
