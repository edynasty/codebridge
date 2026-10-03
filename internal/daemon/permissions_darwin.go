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
