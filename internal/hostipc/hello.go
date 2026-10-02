package hostipc

import "fmt"

// ProtocolMajor is the only Host IPC protocol major this daemon speaks.
const ProtocolMajor = 1

// DefaultProtocolMinor is the daemon's minor version in Phase 0.
const DefaultProtocolMinor = 0

// Protocol is the negotiated wire version.
type Protocol struct {
	Major int `json:"major"`
	Minor int `json:"minor"`
}

func (p Protocol) String() string { return fmt.Sprintf("%d.%d", p.Major, p.Minor) }

// Role is the identity a connection claims at handshake. Only "app" can ever
// reach local_ui or Computer services; "diagnostics" and "harness" are local
// CLI roles limited to host health and the fixed native-host smoke.
type Role string

const (
	// RoleApp is CodeBridge.app: requires a live, signature-verified peer.
	RoleApp Role = "app"
	// RoleDiagnostics is the daemon-local CLI (probe/ctl): peer UID only.
	RoleDiagnostics Role = "diagnostics"
	// RoleHarness is a harness-side CLI: peer UID only.
	RoleHarness Role = "harness"
)

// Known reports whether r is one of the three defined roles.
func (r Role) Known() bool {
	switch r {
	case RoleApp, RoleDiagnostics, RoleHarness:
		return true
	default:
		return false
	}
}

// CallerClass maps a role onto the Bridge caller class. An app connection is
// local_ui; every CLI role is local_mcp and can never approve or touch the
// computer.
func (r Role) CallerClass() string {
	if r == RoleApp {
		return "local_ui"
	}
	return "local_mcp"
}

// Capability is a named service group advertised in the handshake.
type Capability string

const (
	CapHost            Capability = "host"
	CapComputer        Capability = "computer"
	CapApproval        Capability = "approval"
	CapNotify          Capability = "notify"
	CapRuntime         Capability = "runtime"
	CapNativeHostSmoke Capability = "native_host_smoke"
)

// HelloParams is the host.hello request params.
type HelloParams struct {
	Protocol     Protocol     `json:"protocol"`
	AppVersion   string       `json:"app_version,omitempty"`
	Role         Role         `json:"role"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	// Diagnostic hints. Never used for authentication; the daemon reads the
	// live peer PID and validates that running process instead.
	BundleID string `json:"bundle_id,omitempty"`
	TeamID   string `json:"team_id,omitempty"`
	PID      int    `json:"pid,omitempty"`
}

// HelloResult is the host.hello response.
type HelloResult struct {
	Accepted          bool         `json:"accepted"`
	Protocol          Protocol     `json:"protocol"`
	DaemonVersion     string       `json:"daemon_version"`
	Capabilities      []Capability `json:"capabilities,omitempty"`
	Role              Role         `json:"role"`
	SignatureVerified bool         `json:"signature_verified"`
	PeerUID           uint32       `json:"peer_uid"`
}

// SupportedClientMinors returns the minor versions this daemon accepts from a
// peer: the daemon minor and the one below it (N, N-1).
func SupportedClientMinors(daemonMinor int) []int {
	out := []int{daemonMinor}
	if daemonMinor > 0 {
		out = append(out, daemonMinor-1)
	}
	return out
}

// Negotiate validates a client version against the daemon's and returns the
// negotiated protocol. Every rejection is a fail-closed *Error that requires
// closing the connection.
func Negotiate(daemon Protocol, client Protocol) (Protocol, *Error) {
	if client.Major != daemon.Major {
		return Protocol{}, errProtocolMajor(client.Major, daemon.Major)
	}
	supported := SupportedClientMinors(daemon.Minor)
	for _, m := range supported {
		if client.Minor == m {
			neg := Protocol{Major: daemon.Major, Minor: client.Minor}
			return neg, nil
		}
	}
	return Protocol{}, errProtocolMinor(client.Minor, supported)
}

// NegotiateCapabilities intersects the client's request with what the daemon
// actually offers. Unknown or unavailable capabilities are dropped rather than
// failing the handshake; a client must never assume a capability it was not
// granted.
func NegotiateCapabilities(daemon []Capability, client []Capability) []Capability {
	have := make(map[Capability]bool, len(daemon))
	for _, c := range daemon {
		have[c] = true
	}
	out := make([]Capability, 0, len(client))
	seen := map[Capability]bool{}
	for _, c := range client {
		if have[c] && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}
