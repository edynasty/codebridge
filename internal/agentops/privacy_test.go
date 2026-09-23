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
