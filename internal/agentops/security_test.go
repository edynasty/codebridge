package agentops

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveUnderRoot(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveUnderRoot(root, "src"); err != nil {
		t.Fatalf("valid path: %v", err)
	}
	for _, bad := range []string{"../secret", "../../etc/passwd", filepath.Join("..", "x")} {
		if _, err := ResolveUnderRoot(root, bad); err == nil {
			t.Fatalf("expected rejection for %q", bad)
		}
	}
}

func TestResolveUnderRootRejectsSymlinkEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}
	root := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveUnderRoot(root, filepath.Join("escape", "secret.txt")); err == nil {
		t.Fatal("expected symlink escape to be rejected")
	}
}

func TestWorkspaceRootHandleRejectsSymlinkSwapOutsideRoot(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink permissions vary on Windows")
	}

	root := t.TempDir()
	outside := t.TempDir()
	safe := filepath.Join(root, "safe.txt")
	secret := filepath.Join(outside, "secret.txt")
	if err := os.WriteFile(safe, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secret, []byte("secret"), 0o600); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "alias")
	if err := os.Symlink("safe.txt", alias); err != nil {
		t.Fatal(err)
	}

	// Simulate a successful pre-open policy/path validation.
	if _, err := ResolveUnderRoot(root, "alias"); err != nil {
		t.Fatalf("initial in-root symlink should validate: %v", err)
	}

	handle, err := openWorkspaceRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	defer handle.Close()

	if err := os.Remove(alias); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(secret, alias); err != nil {
		t.Fatal(err)
	}

	if f, err := handle.Open("alias"); err == nil {
		_ = f.Close()
		t.Fatal("os.Root followed swapped symlink outside workspace")
	}
}
