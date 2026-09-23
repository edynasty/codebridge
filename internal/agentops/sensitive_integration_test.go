package agentops

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/edynasty/codebridge/internal/protocol"
)

func TestSensitiveFilesBlockedAcrossReadDiscoveryAndSearch(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		".env":               "DB_PASSWORD=super-secret\n",
		".env.example":       "DB_PASSWORD=example\n",
		"private.key":        "PRIVATE KEY MATERIAL\n",
		"prod.tfvars":        "token = \"CODEBRIDGE_SECRET_NEEDLE\"\n",
		"src/safe.txt":       "CODEBRIDGE_SECRET_NEEDLE\n",
		"src/application.go": "package demo\n",
	}
	for name, body := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	service := &Service{Roots: map[string]string{"demo": root}}

	for _, path := range []string{".env", "private.key", "prod.tfvars"} {
		_, err := service.Execute(context.Background(), protocol.AgentRequest{
			Tool:      "read_file",
			Workspace: "demo",
			Args:      map[string]any{"path": path},
		})
		if err == nil || !strings.Contains(err.Error(), "blocked by local policy") {
			t.Fatalf("read_file %q was not blocked: %v", path, err)
		}
	}

	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "list_directory",
		Workspace: "demo",
		Args:      map[string]any{"path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	entries := result.([]DirEntry)
	for _, entry := range entries {
		if isSensitivePath(entry.Path) {
			t.Fatalf("list_directory exposed sensitive entry: %#v", entry)
		}
	}

	result, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "find_files",
		Workspace: "demo",
		Args:      map[string]any{"pattern": "*"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, path := range result.([]string) {
		if isSensitivePath(path) {
			t.Fatalf("find_files exposed sensitive path %q", path)
		}
	}

	result, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "search_code",
		Workspace: "demo",
		Args:      map[string]any{"query": "CODEBRIDGE_SECRET_NEEDLE", "path": "."},
	})
	if err != nil {
		t.Fatal(err)
	}
	matches := result.([]SearchMatch)
	if len(matches) != 1 || matches[0].Path != "src/safe.txt" {
		t.Fatalf("search_code did not filter sensitive matches: %#v", matches)
	}

	result, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "read_file",
		Workspace: "demo",
		Args:      map[string]any{"path": ".env.example"},
	})
	if err != nil {
		t.Fatalf(".env.example should remain readable: %v", err)
	}
	if result.(ReadFileResult).Content != "DB_PASSWORD=example\n" {
		t.Fatalf("unexpected .env.example result: %#v", result)
	}
}

func TestSensitiveFileOptInAllowsDirectRead(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=local\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := &Service{Roots: map[string]string{"demo": root}, AllowSensitiveFiles: true}
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "read_file",
		Workspace: "demo",
		Args:      map[string]any{"path": ".env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.(ReadFileResult).Content != "TOKEN=local\n" {
		t.Fatalf("unexpected sensitive read result: %#v", result)
	}
}

func TestGitDiffFiltersSensitiveFileContents(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGitForTest(t, root, "init", "-q")
	runGitForTest(t, root, "config", "user.email", "codebridge-test@example.invalid")
	runGitForTest(t, root, "config", "user.name", "CodeBridge Test")

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=old-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe-old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runGitForTest(t, root, "add", ".env", "safe.txt")
	runGitForTest(t, root, "commit", "-q", "-m", "initial")

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=new-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe-new\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": root}}
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "git_diff",
		Workspace: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := result.(map[string]any)["output"].(string)
	if !strings.Contains(output, "safe-new") {
		t.Fatalf("safe diff was lost: %s", output)
	}
	if strings.Contains(output, "new-secret") || strings.Contains(output, ".env") {
		t.Fatalf("sensitive diff leaked: %s", output)
	}

	_, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "git_diff",
		Workspace: "demo",
		Args:      map[string]any{"path": ".env"},
	})
	if err == nil || !strings.Contains(err.Error(), "blocked by local policy") {
		t.Fatalf("explicit sensitive git_diff was not blocked: %v", err)
	}

	service.AllowSensitiveFiles = true
	result, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "git_diff",
		Workspace: "demo",
		Args:      map[string]any{"path": ".env"},
	})
	if err != nil {
		t.Fatal(err)
	}
	output = result.(map[string]any)["output"].(string)
	if !strings.Contains(output, "new-secret") {
		t.Fatalf("sensitive opt-in did not restore explicit diff: %s", output)
	}
}

func runGitForTest(t *testing.T, root string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", root}, args...)
	cmd := exec.Command("git", cmdArgs...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v: %s", args, err, out)
	}
}

func TestGitStatusFiltersSensitiveFileNames(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	root := t.TempDir()
	runGitForTest(t, root, "init", "-q")

	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "safe.txt"), []byte("safe\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": root}}
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "git_status",
		Workspace: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	output := result.(map[string]any)["output"].(string)
	if strings.Contains(output, ".env") {
		t.Fatalf("git_status exposed sensitive file name: %s", output)
	}
	if !strings.Contains(output, "safe.txt") {
		t.Fatalf("git_status lost safe file name: %s", output)
	}

	service.AllowSensitiveFiles = true
	result, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "git_status",
		Workspace: "demo",
	})
	if err != nil {
		t.Fatal(err)
	}
	output = result.(map[string]any)["output"].(string)
	if !strings.Contains(output, ".env") {
		t.Fatalf("sensitive opt-in did not restore git_status entry: %s", output)
	}
}

func TestSensitiveSymlinkAliasIsBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".env"), []byte("TOKEN=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".git", "config"), []byte("credential=secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".env", filepath.Join(root, "safe-looking.txt")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(".git", filepath.Join(root, "metadata")); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": root}}

	_, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "read_file",
		Workspace: "demo",
		Args:      map[string]any{"path": "safe-looking.txt"},
	})
	if err == nil || !strings.Contains(err.Error(), "blocked by local policy") {
		t.Fatalf("symlink alias to .env was not blocked: %v", err)
	}

	_, err = service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "list_directory",
		Workspace: "demo",
		Args:      map[string]any{"path": "metadata"},
	})
	if err == nil || !strings.Contains(err.Error(), "blocked by local policy") {
		t.Fatalf("symlink alias to .git was not blocked: %v", err)
	}

	service.AllowSensitiveFiles = true
	result, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "read_file",
		Workspace: "demo",
		Args:      map[string]any{"path": "safe-looking.txt"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.(ReadFileResult).Content != "TOKEN=secret\n" {
		t.Fatalf("sensitive opt-in did not allow symlink target: %#v", result)
	}
}

func TestSensitiveWorkspaceRootBlockedByDefault(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, ".aws")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "credentials"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	service := &Service{Roots: map[string]string{"demo": root}}
	_, err := service.Execute(context.Background(), protocol.AgentRequest{
		Tool:      "list_directory",
		Workspace: "demo",
		Args:      map[string]any{"path": "."},
	})
	if err == nil || !strings.Contains(err.Error(), "workspace root is blocked") {
		t.Fatalf("sensitive workspace root was not blocked: %v", err)
	}
}
