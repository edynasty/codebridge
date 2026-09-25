package main

import (
	"archive/zip"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packageDir is the checked-in package the tests build from, relative to this
// test's working directory.
const packageDir = "../../deploy/agent-plugin"

func testOptions() options {
	return options{PackageDir: packageDir, Version: "0.1.0"}
}

func TestBuildFilesWithChatGPTApp(t *testing.T) {
	opts := testOptions()
	opts.MCPURL = "https://codebridge.example.com/mcp"
	opts.AppID = "plugin_asdk_app_abc123"
	files, err := buildFiles(opts)
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
	opts := testOptions()
	opts.MCPURL = "https://codebridge.example.com/mcp"
	files, err := buildFiles(opts)
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
	var compat map[string]any
	if err := json.Unmarshal(files[".codex-plugin/plugin.json"], &compat); err != nil {
		t.Fatal(err)
	}
	if _, ok := compat["apps"]; ok {
		t.Fatal("codex manifest must not declare missing .app.json")
	}
}

// The generated package must carry the checked-in package's metadata rather
// than a second copy of it: the shipped interface text and every non-manifest
// file (skills, assets, scripts) come from deploy/agent-plugin.
func TestBuildFilesCarriesCheckedInPackage(t *testing.T) {
	opts := testOptions()
	opts.MCPURL = "https://codebridge.example.com/mcp"
	files, err := buildFiles(opts)
	if err != nil {
		t.Fatal(err)
	}

	shipped, err := os.ReadFile(filepath.Join(packageDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(shipped, &want); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(files["plugin.json"], &got); err != nil {
		t.Fatal(err)
	}
	wantExt := want["extensions"].(map[string]any)["com.openai"].(map[string]any)["interface"].(map[string]any)
	gotExt := got["extensions"].(map[string]any)["com.openai"].(map[string]any)["interface"].(map[string]any)
	for _, key := range []string{"displayName", "longDescription", "capabilities"} {
		if gotExt[key] == nil {
			t.Fatalf("generated interface lost %s", key)
		}
		if wantJSON, gotJSON := mustJSON(t, wantExt[key]), mustJSON(t, gotExt[key]); wantJSON != gotJSON {
			t.Fatalf("interface %s = %s, want %s", key, gotJSON, wantJSON)
		}
	}
	if got["version"] != "0.1.0" {
		t.Fatalf("version = %#v", got["version"])
	}

	for _, name := range []string{"README.md", "assets/logo.png", "skills/codebridge-ops/SKILL.md", "skills/codebridge-subagent/SKILL.md", "scripts/verify-plugin.sh"} {
		if _, ok := files[name]; !ok {
			t.Fatalf("package missing %s", name)
		}
	}
}

func TestBuildFilesDefaultsToCheckedInVersion(t *testing.T) {
	opts := testOptions()
	opts.MCPURL = "https://codebridge.example.com/mcp"
	opts.Version = ""
	files, err := buildFiles(opts)
	if err != nil {
		t.Fatal(err)
	}
	shipped, err := os.ReadFile(filepath.Join(packageDir, "plugin.json"))
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(shipped, &want); err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(files["plugin.json"], &got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != want["version"] {
		t.Fatalf("version = %#v, want the checked-in %#v", got["version"], want["version"])
	}
	var compat map[string]any
	if err := json.Unmarshal(files[".codex-plugin/plugin.json"], &compat); err != nil {
		t.Fatal(err)
	}
	if compat["version"] != want["version"] {
		t.Fatalf("codex version = %#v, want %#v", compat["version"], want["version"])
	}
}

func TestMCPManifestsKeepTheirSpecTransports(t *testing.T) {
	opts := testOptions()
	opts.MCPURL = "https://codebridge.example.com/mcp"
	files, err := buildFiles(opts)
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
	if server["url"] != "https://codebridge.example.com/mcp" {
		t.Fatalf("portable url = %#v", server["url"])
	}

	var legacy map[string]any
	if err := json.Unmarshal(files[".mcp.json"], &legacy); err != nil {
		t.Fatal(err)
	}
	legacyServer := legacy["mcpServers"].(map[string]any)["codebridge"].(map[string]any)
	if legacyServer["type"] != "http" {
		t.Fatalf("legacy type = %#v", legacyServer["type"])
	}
	if legacyServer["url"] != "https://codebridge.example.com/mcp" {
		t.Fatalf("legacy url = %#v", legacyServer["url"])
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
			if _, err := validateMCPURL(raw); err == nil {
				t.Fatalf("validateMCPURL(%q) succeeded", raw)
			}
		})
	}
}

func TestWriteZipContainsExpectedFiles(t *testing.T) {
	opts := testOptions()
	opts.MCPURL = "https://codebridge.example.com/mcp"
	opts.AppID = "plugin_asdk_app_zip123"
	files, err := buildFiles(opts)
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
	for name := range files {
		if !seen[name] {
			t.Fatalf("zip missing %s", name)
		}
	}
	if info, err := os.Stat(path); err != nil || info.Size() == 0 {
		t.Fatalf("zip not written: info=%v err=%v", info, err)
	}
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}
