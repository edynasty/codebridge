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
