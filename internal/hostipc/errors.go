package hostipc

import "fmt"

// Host IPC v1 error codes (schema/hostipc/v1/README.md §8).
const (
	CodeParse                    = -32700
	CodeInvalidRequest           = -32600
	CodeUnsupported              = -32601
	CodeInvalidParams            = -32602
	CodeInternal                 = -32603
	CodeProtocolMajorMismatch    = -32000
	CodeProtocolMinorUnsupported = -32001
	CodeNotInitialized           = -32002
	CodeUnauthorizedPeer         = -32010
	CodeRoleForbidden            = -32011
	CodeFrameLimit               = -32020
)

// Error is the JSON-RPC 2.0 error object. Data carries the stable reason
// string so a peer can branch on machine-readable values without parsing
// messages.
type Error struct {
	Code    int      `json:"code"`
	Message string   `json:"message"`
	Data    *ErrData `json:"data,omitempty"`
}

// ErrData is the error detail block.
type ErrData struct {
	Reason string `json:"reason,omitempty"`
	Detail string `json:"detail,omitempty"`
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Data != nil && e.Data.Reason != "" {
		return fmt.Sprintf("%s (%d): %s", e.Data.Reason, e.Code, e.Message)
	}
	return fmt.Sprintf("hostipc error (%d): %s", e.Code, e.Message)
}

// NewError builds an *Error with a stable reason.
func NewError(code int, reason, message string) *Error {
	return &Error{Code: code, Message: message, Data: &ErrData{Reason: reason}}
}

// Errorf builds an *Error with a formatted message.
func Errorf(code int, reason, format string, args ...any) *Error {
	return NewError(code, reason, fmt.Sprintf(format, args...))
}

// Convenience constructors for the fixed protocol failures.
func errProtocolMajor(got, want int) *Error {
	return Errorf(CodeProtocolMajorMismatch, "protocol_major_mismatch",
		"protocol major %d is not supported (daemon major %d)", got, want)
}

func errProtocolMinor(got int, supported []int) *Error {
	return Errorf(CodeProtocolMinorUnsupported, "protocol_minor_unsupported",
		"protocol minor %d is not supported (daemon supports %v)", got, supported)
}

func errNotInitialized() *Error {
	return NewError(CodeNotInitialized, "not_initialized", "host.hello must be the first message")
}

func errUnauthorized(reason string) *Error {
	// Data.Reason carries the specific cause (which auth check failed) so a peer
	// can branch on it without parsing the message.
	return NewError(CodeUnauthorizedPeer, reason, "peer is not authorized: "+reason)
}

func errRoleForbidden(method string, role Role) *Error {
	return Errorf(CodeRoleForbidden, "role_forbidden", "role %q may not call %s", role, method)
}

func errUnsupported(method string) *Error {
	return NewError(CodeUnsupported, "unsupported", "unsupported: no handler for "+method)
}

func errInvalidParams(detail string) *Error {
	return NewError(CodeInvalidParams, "invalid_params", detail)
}

func errInternal(detail string) *Error {
	return NewError(CodeInternal, "internal", detail)
}
