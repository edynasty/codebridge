//go:build darwin && cgo

package daemon

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fblocks
#cgo LDFLAGS: -framework Foundation -framework ScreenCaptureKit -framework ApplicationServices -framework Carbon
#include <stdlib.h>
#include "permissions_darwin.h"
*/
import "C"

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/edynasty/codebridge/internal/hostipc"
)

// runDaemonPermissionProbe calls native APIs in this process, never a harness.
// Each asynchronous native call has a fixed deadline; timeout is not denial.
func runDaemonPermissionProbe(probe string) (ProbeResult, error) {
	foldersOnly := C.int(0)
	if probe == "daemon-files-folders" {
		foldersOnly = 1
	}
	signing, err := hostipc.PeerSignatureInfo(os.Getpid())
	if err != nil {
		return ProbeResult{}, err
	}
	identity := C.CString(signing)
	defer C.free(unsafe.Pointer(identity))
	data := C.cb_daemon_permissions(foldersOnly, identity)
	if data == nil {
		return ProbeResult{}, fmt.Errorf("daemon native permission report unavailable")
	}
	defer C.free(unsafe.Pointer(data))
	return ProbeResult{Probe: probe, Argv: []string{}, Stdout: C.GoString(data)}, nil
}

// ComputerTCCStatus is the passive Computer-TCC posture of the daemon process.
// It is gathered with preflight/status APIs ONLY — the daemon must never call
// CGRequestScreenCaptureAccess, AXIsProcessTrustedWithOptions(prompt:) or
// CGRequestListenEventAccess (invariant DAEMON_COMPUTER_TCC_MUST_BE_NONE).
type ComputerTCCStatus struct {
	ScreenPreflightGranted bool   `json:"screen_preflight_granted"`
	AXTrusted              bool   `json:"ax_trusted"`
	ListenPreflightGranted bool   `json:"listen_preflight_granted"`
	AnyComputerPrivilege   bool   `json:"any_computer_privilege"`
	Detail                 string `json:"detail,omitempty"`
}

// passiveComputerTCCStatus reads the daemon's own Computer-TCC posture using
// passive checks only (no prompts, no requests).
func passiveComputerTCCStatus() ComputerTCCStatus {
	data := C.cb_daemon_passive_tcc()
	if data == nil {
		return ComputerTCCStatus{Detail: "native passive check unavailable"}
	}
	defer C.free(unsafe.Pointer(data))
	jsonStr := C.GoString(data)
	var status ComputerTCCStatus
	if err := jsonUnmarshalStrict([]byte(jsonStr), &status); err != nil {
		return ComputerTCCStatus{Detail: fmt.Sprintf("parse passive report: %v", err)}
	}
	status.AnyComputerPrivilege = status.ScreenPreflightGranted || status.AXTrusted || status.ListenPreflightGranted
	return status
}
