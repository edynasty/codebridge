package manager

import (
	"strings"
	"testing"

	"github.com/edynasty/codebridge/internal/protocol"
)

func TestSanitizeRegistrationRemovesPhysicalPaths(t *testing.T) {
	reg := protocol.RegisterRequest{
		DeviceID:   " mac-1 ",
		DeviceName: " Mac ",
		Version:    " 0.3.0 ",
		Workspaces: []protocol.Workspace{{Name: " pms ", Path: "/Users/me/secret/pms"}},
	}
	if err := sanitizeRegistration(&reg); err != nil {
		t.Fatal(err)
	}
	if reg.DeviceID != "mac-1" || reg.DeviceName != "Mac" || reg.Version != "0.3.0" {
		t.Fatalf("registration was not normalized: %#v", reg)
	}
	if got := reg.Workspaces[0]; got.Name != "pms" || got.Path != "" {
		t.Fatalf("workspace physical path leaked: %#v", got)
	}
}

func TestSanitizeRegistrationRejectsDuplicateWorkspaces(t *testing.T) {
	reg := protocol.RegisterRequest{
		DeviceID: "mac-1", DeviceName: "Mac",
		Workspaces: []protocol.Workspace{{Name: "pms"}, {Name: "pms"}},
	}
	if err := sanitizeRegistration(&reg); err == nil {
		t.Fatal("duplicate workspaces accepted")
	}
}

func TestSanitizeRegistrationBoundsMetadata(t *testing.T) {
	reg := protocol.RegisterRequest{DeviceID: strings.Repeat("x", 129), DeviceName: "Mac"}
	if err := sanitizeRegistration(&reg); err == nil {
		t.Fatal("oversized device id accepted")
	}

	reg = protocol.RegisterRequest{DeviceID: "mac", DeviceName: "Mac", Workspaces: make([]protocol.Workspace, 65)}
	for i := range reg.Workspaces {
		reg.Workspaces[i].Name = "w"
	}
	if err := sanitizeRegistration(&reg); err == nil {
		t.Fatal("too many workspaces accepted")
	}
}
