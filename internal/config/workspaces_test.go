package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseWorkspacesDoesNotAdvertisePhysicalPath(t *testing.T) {
	d := t.TempDir()
	roots, adv, err := ParseWorkspaces("demo=" + d)
	if err != nil {
		t.Fatal(err)
	}
	if roots["demo"] != filepath.Clean(d) {
		t.Fatalf("unexpected root %q", roots["demo"])
	}
	if len(adv) != 1 || adv[0].Name != "demo" || adv[0].Path != "" {
		t.Fatalf("unexpected advertised workspace: %#v", adv)
	}
	_ = os.RemoveAll(d)
}
