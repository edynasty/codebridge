package agentops

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/edynasty/codebridge/internal/protocol"
)

func TestToolErrorsDoNotExposePhysicalWorkspaceRoot(t *testing.T) {
	root := t.TempDir()
	service := &Service{Roots: map[string]string{"demo": root}}

	tests := []protocol.AgentRequest{
		{
			Tool:      "read_file",
			Workspace: "demo",
			Args:      map[string]any{"path": "missing.txt"},
		},
		{
			Tool:      "list_directory",
			Workspace: "demo",
			Args:      map[string]any{"path": "missing-dir"},
		},
	}

	for _, req := range tests {
		_, err := service.Execute(context.Background(), req)
		if err == nil {
			t.Fatalf("%s unexpectedly succeeded", req.Tool)
		}
		if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(root)) {
			t.Fatalf("%s leaked workspace root in error: %v", req.Tool, err)
		}
	}
}

func TestListDirectoryUsesLogicalPathForSymlinkWorkspaceRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	realRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(realRoot, "hello.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": link}}
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "list_directory",
		Workspace: "demo",
		Args:      map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries, ok := result.([]DirEntry)
	if !ok || len(entries) != 1 {
		t.Fatalf("unexpected directory result: %#v", result)
	}
	if entries[0].Path != "hello.txt" {
		t.Fatalf("physical path leaked through directory result: %#v", entries[0])
	}
	if strings.Contains(entries[0].Path, realRoot) || strings.Contains(entries[0].Path, link) {
		t.Fatalf("directory path leaked physical root: %#v", entries[0])
	}
}

func TestResolveBrokenSymlinkErrorDoesNotExposePhysicalRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	root := t.TempDir()
	if err := os.Symlink(filepath.Join(root, "missing-target"), filepath.Join(root, "broken")); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveUnderRoot(root, "broken")
	if err == nil {
		t.Fatal("broken symlink unexpectedly resolved")
	}
	if strings.Contains(strings.ToLower(err.Error()), strings.ToLower(root)) {
		t.Fatalf("broken symlink error leaked workspace root: %v", err)
	}
}

func TestFindFilesWithSymlinkWorkspaceRootUsesLogicalPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	realRoot := t.TempDir()
	src := filepath.Join(realRoot, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "needle.go"), []byte("package demo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": link}}
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "find_files",
		Workspace: "demo",
		Args:      map[string]any{"pattern": "*.go"},
	})
	if err != nil {
		t.Fatal(err)
	}
	paths, ok := result.([]string)
	if !ok || len(paths) != 1 {
		t.Fatalf("unexpected find_files result: %#v", result)
	}
	if paths[0] != "src/needle.go" {
		t.Fatalf("find_files returned non-logical path %q", paths[0])
	}
	if strings.Contains(paths[0], realRoot) || strings.Contains(paths[0], link) || strings.HasPrefix(paths[0], "..") {
		t.Fatalf("find_files leaked physical workspace path: %q", paths[0])
	}
}

func TestSearchCodeWithSymlinkWorkspaceRootUsesLogicalPaths(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	realRoot := t.TempDir()
	src := filepath.Join(realRoot, "src")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	const needle = "CODEBRIDGE_PRIVACY_NEEDLE"
	if err := os.WriteFile(filepath.Join(src, "needle.txt"), []byte("prefix "+needle+" suffix\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "workspace-link")
	if err := os.Symlink(realRoot, link); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": link}}
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "search_code",
		Workspace: "demo",
		Args:      map[string]any{"query": needle, "path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	matches, ok := result.([]SearchMatch)
	if !ok || len(matches) == 0 {
		t.Fatalf("unexpected search_code result: %#v", result)
	}
	for _, match := range matches {
		if match.Path != "src/needle.txt" {
			t.Fatalf("search_code returned non-logical path %q", match.Path)
		}
		if strings.Contains(match.Path, realRoot) || strings.Contains(match.Path, link) || strings.HasPrefix(match.Path, "..") {
			t.Fatalf("search_code leaked physical workspace path: %q", match.Path)
		}
	}
}
