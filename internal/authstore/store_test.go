package authstore

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEnrollmentCredentialLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := s.CreateEnrollment(time.Minute, DefaultAccount)
	if err != nil {
		t.Fatal(err)
	}
	cred, err := s.EnrollDevice(code, "mac-1", "Mac")
	if err != nil {
		t.Fatal(err)
	}
	if cred == "" || !s.VerifyDevice("mac-1", cred) {
		t.Fatal("issued credential did not verify")
	}
	if s.VerifyDevice("mac-1", "wrong") {
		t.Fatal("wrong credential verified")
	}
	if _, err := s.EnrollDevice(code, "mac-2", "Mac 2"); err == nil {
		t.Fatal("one-time enrollment code was accepted twice")
	}

	rotated, err := s.RotateDevice("mac-1")
	if err != nil {
		t.Fatal(err)
	}
	if s.VerifyDevice("mac-1", cred) {
		t.Fatal("old credential remained valid after rotation")
	}
	if !s.VerifyDevice("mac-1", rotated) {
		t.Fatal("rotated credential did not verify")
	}
	if err := s.RevokeDevice("mac-1"); err != nil {
		t.Fatal(err)
	}
	if s.VerifyDevice("mac-1", rotated) {
		t.Fatal("credential remained valid after revoke")
	}
}

func TestStateReload(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s, _ := Open(path)
	code, _, _ := s.CreateEnrollment(time.Minute, "team-a")
	cred, err := s.EnrollDevice(code, "linux-1", "Linux")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !s2.VerifyDevice("linux-1", cred) {
		t.Fatal("persisted credential did not verify after reload")
	}
	if account, ok := s2.DeviceAccount("linux-1"); !ok || account != "team-a" {
		t.Fatalf("persisted account not reloaded: %q %v", account, ok)
	}
}

func TestCreateEnrollmentRequiresValidAccount(t *testing.T) {
	s, _ := Open("")
	if _, _, err := s.CreateEnrollment(time.Minute, "  "); err == nil {
		t.Fatal("whitespace-only account accepted")
	}
	if _, _, err := s.CreateEnrollment(time.Minute, strings.Repeat("x", maxAccountIDBytes+1)); err == nil {
		t.Fatal("oversized account accepted")
	}
	if _, _, err := s.CreateEnrollment(time.Minute, "team-a"); err != nil {
		t.Fatalf("valid account rejected: %v", err)
	}
}

func TestEnrollDeviceInheritsEnrollmentAccount(t *testing.T) {
	s, _ := Open("")
	code, _, err := s.CreateEnrollment(time.Minute, "team-a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.EnrollDevice(code, "dev-1", "Dev"); err != nil {
		t.Fatal(err)
	}
	account, ok := s.DeviceAccount("dev-1")
	if !ok || account != "team-a" {
		t.Fatalf("device did not inherit enrollment account: %q %v", account, ok)
	}
	devices := s.ListDevices()
	if len(devices) != 1 || devices[0].AccountID != "team-a" {
		t.Fatalf("ListDevices did not expose account: %#v", devices)
	}
	if _, ok := s.DeviceAccount("missing"); ok {
		t.Fatal("unknown device returned an account")
	}
}

func TestV1StateMigratesToDefaultAccount(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	credential := "dev_legacy"
	v1 := persistedState{
		Version: 1,
		Devices: map[string]Device{
			"legacy-1": {ID: "legacy-1", Name: "Legacy", CredentialHash: digest(credential)},
		},
		Enrollments: map[string]Enrollment{
			digest("enr_legacy"): {Hash: digest("enr_legacy"), ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	b, err := json.Marshal(v1)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatalf("open v1 state: %v", err)
	}
	if !s.VerifyDevice("legacy-1", credential) {
		t.Fatal("migrated device credential did not verify")
	}
	if account, ok := s.DeviceAccount("legacy-1"); !ok || account != DefaultAccount {
		t.Fatalf("v1 device not migrated to default account: %q %v", account, ok)
	}
	if _, _, err := s.CreateEnrollment(time.Minute, "team-a"); err != nil {
		t.Fatal(err)
	}
	s2, err := Open(path)
	if err != nil {
		t.Fatalf("reopen migrated state: %v", err)
	}
	if s2.state.Version != stateVersion {
		t.Fatalf("state version not persisted as %d: %d", stateVersion, s2.state.Version)
	}
}

func TestUnsupportedStateVersionRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	if err := os.WriteFile(path, []byte(`{"version":99,"devices":{},"enrollments":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Fatal("unsupported state version accepted")
	}
}
