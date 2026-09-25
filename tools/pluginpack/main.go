// Command pluginpack assembles the installable CodeBridge plugin package from
// the checked-in package in deploy/agent-plugin, injecting the deployment's MCP
// endpoint, app binding and version.
//
// The checked-in manifests stay the single source of truth for everything that
// is not deployment-specific (interface text, branding, transports), so
// regenerating a package can never silently revert the shipped metadata.
package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const defaultPackageDir = "deploy/agent-plugin"

// manifests are the files pluginpack regenerates; every other file in the
// package directory is copied verbatim.
var manifests = map[string]bool{
	"plugin.json":               true,
	"mcp.json":                  true,
	".mcp.json":                 true,
	".app.json":                 true,
	".codex-plugin/plugin.json": true,
}

var appIDPattern = regexp.MustCompile(`^(asdk_app_|connector_|templated_apps_)[A-Za-z0-9][A-Za-z0-9_-]*$`)

type options struct {
	MCPURL     string
	AppID      string
	Out        string
	Version    string
	PackageDir string
}

func main() {
	var opts options
	flag.StringVar(&opts.MCPURL, "mcp-url", "", "public CodeBridge MCP endpoint, for example https://codebridge.example.com/mcp")
	flag.StringVar(&opts.AppID, "app-id", "", "optional ChatGPT registered MCP technical ID (plugin_asdk_app_... or asdk_app_...)")
	flag.StringVar(&opts.Out, "out", "dist/codebridge-plugin", "output directory or .zip file")
	flag.StringVar(&opts.Version, "version", "", "plugin package version; defaults to the checked-in package version")
	flag.StringVar(&opts.PackageDir, "package-dir", defaultPackageDir, "checked-in plugin package to build from")
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

// buildFiles returns the package as a path -> content map keyed by the path
// inside the package root.
func buildFiles(opts options) (map[string][]byte, error) {
	mcpURL, err := validateMCPURL(opts.MCPURL)
	if err != nil {
		return nil, err
	}
	version := strings.TrimSpace(opts.Version)
	appID, err := normalizeAppID(opts.AppID)
	if err != nil {
		return nil, err
	}
	dir := strings.TrimSpace(opts.PackageDir)
	if dir == "" {
		return nil, errors.New("--package-dir is required")
	}
	if version == "" {
		// Default to the version the checked-in package already carries, so a
		// rebuild never silently downgrades a released package.
		doc, err := readManifest(dir, "plugin.json")
		if err != nil {
			return nil, err
		}
		version, _ = doc["version"].(string)
		version = strings.TrimSpace(version)
		if version == "" {
			return nil, errors.New("version is required")
		}
	}

	files := map[string][]byte{}
	err = filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if entry.IsDir() {
			return nil
		}
		if entry.Name() == ".DS_Store" || manifests[rel] {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[rel] = data
		return nil
	})
	if err != nil {
		return nil, err
	}

	// plugin.json and .codex-plugin/plugin.json carry the version; the app
	// binding is added only when the caller supplied a registered app id, so a
	// portable package never references a missing .app.json.
	for _, name := range []string{"plugin.json", ".codex-plugin/plugin.json"} {
		doc, err := readManifest(dir, name)
		if err != nil {
			return nil, err
		}
		doc["version"] = version
		if appID == "" {
			delete(doc, "apps")
			if ext, ok := doc["extensions"].(map[string]any); ok {
				if openai, ok := ext["com.openai"].(map[string]any); ok {
					delete(openai, "apps")
				}
			}
		} else {
			if name == "plugin.json" {
				openai, err := nestedObject(doc, "extensions", "com.openai")
				if err != nil {
					return nil, fmt.Errorf("%s: %w", name, err)
				}
				openai["apps"] = "./.app.json"
			} else {
				doc["apps"] = "./.app.json"
			}
		}
		if files[name], err = marshalJSON(doc); err != nil {
			return nil, fmt.Errorf("marshal %s: %w", name, err)
		}
	}

	// Both MCP manifests keep the transport their spec requires; only the
	// endpoint is deployment-specific.
	for _, name := range []string{"mcp.json", ".mcp.json"} {
		doc, err := readManifest(dir, name)
		if err != nil {
			return nil, err
		}
		server, err := nestedObject(doc, "mcpServers", "codebridge")
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		server["url"] = mcpURL
		if files[name], err = marshalJSON(doc); err != nil {
			return nil, fmt.Errorf("marshal %s: %w", name, err)
		}
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

func readManifest(dir, name string) (map[string]any, error) {
	raw, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(name)))
	if err != nil {
		return nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("%s: %w", name, err)
	}
	return doc, nil
}

func nestedObject(doc map[string]any, keys ...string) (map[string]any, error) {
	current := doc
	for i, key := range keys {
		value, ok := current[key]
		if !ok {
			nested := map[string]any{}
			current[key] = nested
			current = nested
			continue
		}
		next, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("%s must be an object", strings.Join(keys[:i+1], "."))
		}
		current = next
	}
	return current, nil
}

func validateMCPURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("--mcp-url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return "", errors.New("--mcp-url must be an absolute HTTPS URL without credentials, query, or fragment")
	}
	if u.Scheme != "https" {
		return "", errors.New("--mcp-url must use https")
	}
	if !strings.HasSuffix(strings.TrimRight(u.Path, "/"), "/mcp") {
		return "", errors.New("--mcp-url must point to the CodeBridge /mcp endpoint")
	}
	u.Path = strings.TrimRight(u.Path, "/")
	return u.String(), nil
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
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	// Deterministic archives: the tracked plugin_*.json manifests lead, then
	// everything else in lexical order.
	sort.Slice(names, func(i, j int) bool {
		return zipOrder(names[i]) < zipOrder(names[j])
	})
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, name := range names {
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		if _, err := io.Copy(w, bytes.NewReader(files[name])); err != nil {
			return err
		}
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// zipOrder keeps the manifest files first so the archive lists the same way the
// package directory does.
func zipOrder(name string) string {
	if manifests[name] {
		return "0" + name
	}
	return "1" + name
}
