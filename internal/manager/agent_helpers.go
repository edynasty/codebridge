package manager

import (
	"fmt"
	"strings"

	"github.com/edynasty/codebridge/internal/protocol"
)

// maxAgentPayloadBytes bounds a single tool response from an agent.
const maxAgentPayloadBytes = 1024 * 1024

func sanitizeRegistration(reg *protocol.RegisterRequest) error {
	reg.DeviceID = strings.TrimSpace(reg.DeviceID)
	reg.DeviceName = strings.TrimSpace(reg.DeviceName)
	reg.Version = strings.TrimSpace(reg.Version)
	if reg.DeviceID == "" || reg.DeviceName == "" {
		return fmt.Errorf("device_id and device_name are required")
	}
	if len(reg.DeviceID) > 128 {
		return fmt.Errorf("device_id exceeds 128 bytes")
	}
	if len(reg.DeviceName) > 256 {
		return fmt.Errorf("device_name exceeds 256 bytes")
	}
	if len(reg.Version) > 64 {
		return fmt.Errorf("version exceeds 64 bytes")
	}
	if len(reg.Workspaces) > 64 {
		return fmt.Errorf("too many workspaces; maximum is 64")
	}
	seen := map[string]bool{}
	for i := range reg.Workspaces {
		name := strings.TrimSpace(reg.Workspaces[i].Name)
		if name == "" || len(name) > 128 {
			return fmt.Errorf("workspace name must be 1-128 bytes")
		}
		if seen[name] {
			return fmt.Errorf("duplicate workspace %q", name)
		}
		seen[name] = true
		reg.Workspaces[i].Name = name
		// Never trust or forward a physical path supplied by an agent.
		reg.Workspaces[i].Path = ""
	}
	return nil
}
