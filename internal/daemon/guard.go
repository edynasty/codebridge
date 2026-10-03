package daemon

import (
	"encoding/json"
	"fmt"
	"time"
)

// jsonUnmarshalStrict is a tiny helper shared by the passive-TCC parser.
func jsonUnmarshalStrict(data []byte, v any) error { return json.Unmarshal(data, v) }

// computerTCCGuard evaluates the DAEMON_COMPUTER_TCC_MUST_BE_NONE invariant with
// passive checks only. The result is cached per call site (each boundary performs
// a fresh check; no high-frequency polling).
//
// Safe=false means the daemon process itself holds a Computer privilege the user
// granted to it. In that state the daemon must fail closed: no agent/harness
// spawning, no Computer broker remote execution, health reports degraded.
func (d *Daemon) computerTCCGuard() *ComputerTCCGuardState {
	d.computerTCCOnce.Do(func() {
		d.computerTCCState = computeGuard()
	})
	return d.computerTCCState
}

// ComputerTCCBoundaryCheck forces a fresh passive evaluation (used at runtime
// boundaries: Run creation, harness spawn, ComputerSession setup, restart).
func (d *Daemon) ComputerTCCBoundaryCheck() *ComputerTCCGuardState {
	state := computeGuard()
	d.computerTCCState = state
	return state
}

// GuardUnsafe reports whether the daemon must fail closed right now.
func (d *Daemon) GuardUnsafe() bool {
	if g := d.ComputerTCCBoundaryCheck(); g != nil && !g.Safe {
		return true
	}
	return false
}

func computeGuard() *ComputerTCCGuardState {
	status := passiveComputerTCCStatus()
	state := &ComputerTCCGuardState{
		Screen:    status.ScreenPreflightGranted,
		AXTrusted: status.AXTrusted,
		Listen:    status.ListenPreflightGranted,
		Safe:      !status.AnyComputerPrivilege,
		CheckedAt: time.Now().UTC().Format(time.RFC3339),
	}
	if !state.Safe {
		state.Reason = fmt.Sprintf(
			"host_permission_conflict: the daemon holds a Computer privilege (screen=%v ax=%v input=%v); "+
				"remove the daemon's Screen Recording / Accessibility / Input Monitoring grants in System Settings",
			state.Screen, state.AXTrusted, state.Listen)
	}
	return state
}
