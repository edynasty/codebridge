package clientconfig

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathFlagWins(t *testing.T) {
	t.Setenv("CODEBRIDGE_CONFIG", "/env/config.json")
	path, required, err := ResolvePath([]string{"--manager", "wss://x", "--config", "/flag/config.json"})
	if err != nil {
		t.Fatal(err)
	}
	if path != "/flag/config.json" || !required {
		t.Fatalf("path=%q required=%v", path, required)
	}
}

func TestResolvePathEnv(t *testing.T) {
	t.Setenv("CODEBRIDGE_CONFIG", "/env/config.json")
	path, required, err := ResolvePath(nil)
	if err != nil {
		t.Fatal(err)
	}
	if path != "/env/config.json" || !required {
		t.Fatalf("path=%q required=%v", path, required)
	}
}

func TestLoadMissingDefaultIsAllowed(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"), false)
	if err != nil {
		t.Fatal(err)
	}
}

func TestLoadExplicitMissingFails(t *testing.T) {
	_, err := Load(filepath.Join(t.TempDir(), "missing.json"), true)
	if err == nil {
		t.Fatal("explicit missing config was accepted")
	}
}

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "client.json")
	content := []byte(`{
	  "manager_host": " codebridge.example.com:8081 ",
	  "device_id": " mac-1 ",
	  "device_name": " Mac ",
	  "workspaces": {
	    " pms ": "/tmp/pms"
	  },
	  "credential_file": "/tmp/credential.json"
	}`)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path, true)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ManagerHost != "codebridge.example.com:8081" || cfg.DeviceID != "mac-1" || cfg.DeviceName != "Mac" {
		t.Fatalf("unexpected config: %#v", cfg)
	}
	if cfg.Workspaces["pms"] != "/tmp/pms" {
		t.Fatalf("workspaces=%#v", cfg.Workspaces)
	}
}
