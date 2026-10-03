//go:build !darwin || !cgo

package daemon

// ComputerTCCStatus is the passive Computer-TCC posture of the daemon process.
type ComputerTCCStatus struct {
	ScreenPreflightGranted bool   `json:"screen_preflight_granted"`
	AXTrusted              bool   `json:"ax_trusted"`
	ListenPreflightGranted bool   `json:"listen_preflight_granted"`
	AnyComputerPrivilege   bool   `json:"any_computer_privilege"`
	Detail                 string `json:"detail,omitempty"`
}

// passiveComputerTCCStatus: on non-macOS/non-CGO builds the guard cannot run;
// report unavailable rather than pretending the daemon is safe.
func passiveComputerTCCStatus() ComputerTCCStatus {
	return ComputerTCCStatus{Detail: "passive TCC check requires macOS with CGO"}
}
