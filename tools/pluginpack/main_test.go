package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildFilesWithChatGPTApp(t *testing.T) {
	files, err := buildFiles(options{
		MCPURL:  "https://codebridge.example.com/mcp",
		AppID:   "plugin_asdk_app_abc123",
		Out:     "unused",
		Version: "0.1.0",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files[".app.json"]; !ok {
		t.Fatal("expected .app.json")
	}

	var app struct {
		Apps map[string]struct {
			ID string `json:"id"`
		} `json:"apps"`
	}
	if err := json.Unmarshal(files[".app.json"], &app); err != nil {
		t.Fatal(err)
	}
	if got := app.Apps["codebridge"].ID; got != "asdk_app_abc123" {
		t.Fatalf("app id = %q, want asdk_app_abc123", got)
	}

	var plugin map[string]any
	if err := json.Unmarshal(files["plugin.json"], &plugin); err != nil {
		t.Fatal(err)
	}
	ext := plugin["extensions"].(map[string]any)["com.openai"].(map[string]any)
	if got := ext["apps"]; got != "./.app.json" {
		t.Fatalf("portable apps path = %#v", got)
	}

	var compat map[string]any
	if err := json.Unmarshal(files[".codex-plugin/plugin.json"], &compat); err != nil {
		t.Fatal(err)
	}
	if got := compat["apps"]; got != "./.app.json" {
		t.Fatalf("compat apps path = %#v", got)
	}
	if got := compat["mcpServers"]; got != "./.mcp.json" {
		t.Fatalf("compat mcpServers path = %#v", got)
	}
}

func TestBuildFilesWithoutAppOmitsChatGPTMapping(t *testing.T) {
	files, err := buildFiles(options{MCPURL: "https://codebridge.example.com/mcp", Version: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := files[".app.json"]; ok {
		t.Fatal("did not expect .app.json")
	}
	var plugin map[string]any
	if err := json.Unmarshal(files["plugin.json"], &plugin); err != nil {
		t.Fatal(err)
	}
	ext := plugin["extensions"].(map[string]any)["com.openai"].(map[string]any)
	if _, ok := ext["apps"]; ok {
		t.Fatal("portable manifest must not declare missing .app.json")
	}
}

func TestMCPManifestsUseExpectedTransports(t *testing.T) {
	files, err := buildFiles(options{MCPURL: "https://codebridge.example.com/mcp", Version: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	var portable map[string]any
	if err := json.Unmarshal(files["mcp.json"], &portable); err != nil {
		t.Fatal(err)
	}
	server := portable["mcpServers"].(map[string]any)["codebridge"].(map[string]any)
	if server["type"] != "streamable-http" {
		t.Fatalf("portable type = %#v", server["type"])
	}

	var legacy map[string]any
	if err := json.Unmarshal(files[".mcp.json"], &legacy); err != nil {
		t.Fatal(err)
	}
	legacyServer := legacy["mcpServers"].(map[string]any)["codebridge"].(map[string]any)
	if legacyServer["type"] != "http" {
		t.Fatalf("legacy type = %#v", legacyServer["type"])
	}
	if legacyServer["oauth_resource"] != "https://codebridge.example.com" {
		t.Fatalf("oauth_resource = %#v", legacyServer["oauth_resource"])
	}
}

func TestRejectsUnsafeOrWrongMCPURL(t *testing.T) {
	for _, raw := range []string{
		"",
		"http://codebridge.example.com/mcp",
		"https://user:pass@codebridge.example.com/mcp",
		"https://codebridge.example.com/agent",
		"https://codebridge.example.com/mcp?token=secret",
	} {
		t.Run(strings.ReplaceAll(raw, "/", "_"), func(t *testing.T) {
			if _, _, err := validateMCPURL(raw); err == nil {
				t.Fatalf("validateMCPURL(%q) succeeded", raw)
			}
		})
	}
}

func TestWriteZipContainsExpectedFiles(t *testing.T) {
	files, err := buildFiles(options{MCPURL: "https://codebridge.example.com/mcp", AppID: "plugin_asdk_app_zip123", Version: "0.1.0"})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "codebridge-plugin.zip")
	if err := writeFiles(path, files); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	seen := map[string]bool{}
	for _, f := range zr.File {
		seen[f.Name] = true
	}
	for _, name := range []string{"plugin.json", "mcp.json", ".mcp.json", ".app.json", ".codex-plugin/plugin.json"} {
		if !seen[name] {
			t.Fatalf("zip missing %s", name)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("zip not written: info=%v err=%v", info, err)
	}
}
