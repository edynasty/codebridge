//go:build !darwin || !cgo

package hostipc

import "fmt"

// SignatureError reports an unavailable or failed live peer signature check.
type SignatureError struct {
	PID    int
	Status int32
	Reason string
	Detail string
}

func (e *SignatureError) Error() string {
	return fmt.Sprintf("hostipc: %s (pid %d): %s", e.Reason, e.PID, e.Detail)
}

// AppRequirement builds the code requirement string (see the cgo build).
func AppRequirement(teamID, bundleID string) string {
	return fmt.Sprintf("anchor apple generic and certificate leaf[subject.OU] = %q and identifier %q", teamID, bundleID)
}

// VerifyPeerCodeSignature fails closed when the build cannot reach the Security
// framework (non-Darwin, or Darwin without cgo). role "app" is therefore
// refused rather than trusted; there is no bypass.
func VerifyPeerCodeSignature(pid int, requirement string) error {
	return &SignatureError{
		PID:    pid,
		Reason: "app_signature_unavailable",
		Detail: "this build was compiled without the Security framework (need darwin + cgo)",
	}
}

// PeerSignatureInfo is unavailable without the Security framework.
func PeerSignatureInfo(pid int) (string, error) {
	return "", &SignatureError{PID: pid, Reason: "app_signature_unavailable", Detail: "no Security framework in this build"}
}
