package authstore

import (
	"path/filepath"
	"testing"
	"time"
)

func TestEnrollmentCredentialLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	code, _, err := s.CreateEnrollment(time.Minute)
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
	code, _, _ := s.CreateEnrollment(time.Minute)
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
}
