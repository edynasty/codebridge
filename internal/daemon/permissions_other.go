//go:build !darwin || !cgo

package daemon

import "fmt"

func runDaemonPermissionProbe(probe string) (ProbeResult, error) {
	return ProbeResult{}, fmt.Errorf("%s requires macOS with CGO-enabled native APIs", probe)
}
