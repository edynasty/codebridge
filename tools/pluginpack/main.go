package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const defaultVersion = "0.3.1"

var appIDPattern = regexp.MustCompile(`^(asdk_app_|connector_|templated_apps_)[A-Za-z0-9][A-Za-z0-9_-]*$`)

type options struct {
	MCPURL  string
	AppID   string
	Out     string
	Version string
}

func main() {
	var opts options
	flag.StringVar(&opts.MCPURL, "mcp-url", "", "public CodeBridge MCP endpoint, for example https://codebridge.example.com/mcp")
	flag.StringVar(&opts.AppID, "app-id", "", "optional ChatGPT registered MCP technical ID (plugin_asdk_app_... or asdk_app_...)")
	flag.StringVar(&opts.Out, "out", "dist/codebridge-plugin", "output directory or .zip file")
	flag.StringVar(&opts.Version, "version", defaultVersion, "plugin package version")
	flag.Parse()

	files, err := buildFiles(opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, "pluginpack:", err)
		os.Exit(2)
	}
	if err := writeFiles(opts.Out, files); err != nil {
		fmt.Fprintln(os.Stderr, "pluginpack:", err)
		os.Exit(1)
	}
	if strings.TrimSpace(opts.AppID) == "" {
		fmt.Printf("generated %s (portable/Codex MCP package; ChatGPT registered-app mapping not included)\n", opts.Out)
		return
	}
	fmt.Printf("generated %s (ChatGPT + Codex package)\n", opts.Out)
}

func buildFiles(opts options) (map[string][]byte, error) {
	mcpURL, oauthResource, err := validateMCPURL(opts.MCPURL)
	if err != nil {
		return nil, err
	}
	version := strings.TrimSpace(opts.Version)
	if version == "" {
		return nil, errors.New("version is required")
	}

	appID, err := normalizeAppID(opts.AppID)
	if err != nil {
		return nil, err
	}

	openAIExtension := map[string]any{
		"interface": map[string]any{
			"displayName":      "CodeBridge",
			"shortDescription": "Read source code from your registered devices",
			"longDescription":  "Connect ChatGPT or Codex to source-code workspaces exposed by your self-hosted CodeBridge Manager and local agents. CodeBridge provides bounded, read-only source inspection tools and does not expose arbitrary shell execution.",
			"developerName":    "CodeBridge",
			"category":         "Developer Tools",
			"capabilities":     []string{"Read"},
			"websiteURL":       "https://github.com/edynasty/codebridge",
			"defaultPrompt": []string{
				"List my connected devices and workspaces",
				"Inspect one of my CodeBridge workspaces without modifying files",
			},
		},
	}
	if appID != "" {
		openAIExtension["apps"] = "./.app.json"
	}

	portablePlugin := map[string]any{
		"$schema":     "https://agent-plugins.org/schemas/1.0.0/plugin.schema.json",
		"name":        "codebridge",
		"version":     version,
		"description": "Securely inspect source-code workspaces on your own registered devices through CodeBridge.",
		"author": map[string]any{
			"name": "CodeBridge",
			"url":  "https://github.com/edynasty/codebridge",
		},
		"homepage":   "https://github.com/edynasty/codebridge",
		"repository": "https://github.com/edynasty/codebridge",
		"license":    "MIT",
		"keywords":   []string{"mcp", "developer-tools", "source-code", "self-hosted", "chatgpt"},
		"extensions": map[string]any{
			"com.openai": openAIExtension,
		},
	}

	portableMCP := map[string]any{
		"$schema": "https://agent-plugins.org/schemas/1.0.0/mcp.schema.json",
		"mcpServers": map[string]any{
			"codebridge": map[string]any{
				"type": "streamable-http",
				"url":  mcpURL,
			},
		},
	}

	compatPlugin := map[string]any{
		"name":        "codebridge",
		"version":     version,
		"description": "Securely inspect source-code workspaces on your own registered devices through CodeBridge.",
		"author": map[string]any{
			"name": "CodeBridge",
			"url":  "https://github.com/edynasty/codebridge",
		},
		"homepage":   "https://github.com/edynasty/codebridge",
		"repository": "https://github.com/edynasty/codebridge",
		"license":    "MIT",
		"keywords":   []string{"mcp", "developer-tools", "source-code", "self-hosted", "chatgpt"},
		"mcpServers": "./.mcp.json",
		"interface":  openAIExtension["interface"],
	}
	if appID != "" {
		compatPlugin["apps"] = "./.app.json"
	}

	legacyMCP := map[string]any{
		"mcpServers": map[string]any{
			"codebridge": map[string]any{
				"type":           "http",
				"url":            mcpURL,
				"oauth_resource": oauthResource,
			},
		},
	}

	files := map[string][]byte{}
	for name, value := range map[string]any{
		"plugin.json":               portablePlugin,
		"mcp.json":                  portableMCP,
		".mcp.json":                 legacyMCP,
		".codex-plugin/plugin.json": compatPlugin,
	} {
		b, err := marshalJSON(value)
		if err != nil {
			return nil, fmt.Errorf("marshal %s: %w", name, err)
		}
		files[name] = b
	}
	if appID != "" {
		b, err := marshalJSON(map[string]any{
			"apps": map[string]any{
				"codebridge": map[string]any{"id": appID},
			},
		})
		if err != nil {
			return nil, fmt.Errorf("marshal .app.json: %w", err)
		}
		files[".app.json"] = b
	}
	return files, nil
}

func validateMCPURL(raw string) (string, string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", errors.New("--mcp-url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", "", errors.New("--mcp-url must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	if u.Scheme != "https" {
		return "", "", errors.New("--mcp-url must use https")
	}
	if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/mcp") {
		return "", "", errors.New("--mcp-url must point to the CodeBridge /mcp endpoint")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	origin := (&url.URL{Scheme: u.Scheme, Host: u.Host}).String()
	return u.String(), origin, nil
}

func normalizeAppID(raw string) (string, error) {
	id := strings.TrimSpace(raw)
	if id == "" {
		return "", nil
	}
	if strings.HasPrefix(id, "plugin_asdk_app_") {
		id = strings.TrimPrefix(id, "plugin_")
	}
	if !appIDPattern.MatchString(id) {
		return "", errors.New("--app-id must be plugin_asdk_app_..., asdk_app_..., connector_..., or templated_apps_...")
	}
	return id, nil
}

func marshalJSON(v any) ([]byte, error) {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}

func writeFiles(out string, files map[string][]byte) error {
	out = strings.TrimSpace(out)
	if out == "" {
		return errors.New("--out is required")
	}
	if strings.HasSuffix(strings.ToLower(out), ".zip") {
		return writeZip(out, files)
	}
	for name, data := range files {
		path := filepath.Join(out, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func writeZip(path string, files map[string][]byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil && filepath.Dir(path) != "." {
		return err
	}
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range []string{"plugin.json", "mcp.json", ".mcp.json", ".app.json", ".codex-plugin/plugin.json"} {
		data, ok := files[name]
		if !ok {
			continue
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, bytes.NewReader(data)); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
