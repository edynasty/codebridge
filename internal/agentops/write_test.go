package agentops

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Write mode must work in a plain directory: no git repository is required, and
// checkpoints keep content-addressed snapshots locally so rollback still
// restores the pre-patch state.
func TestApplyPatchWithoutGitRepository(t *testing.T) {
	root := t.TempDir()
	svc := &Service{
		Roots:         map[string]string{"work": root},
		Writable:      map[string]bool{"work": true},
		CheckpointDir: t.TempDir(),
	}
	target := filepath.Join(root, "notes", "todo.txt")

	create := []FileEdit{{Path: "notes/todo.txt", NewText: "first\n"}}
	preview, err := svc.applyPatch("work", root, create, true, false)
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if !preview.Preview || preview.Diff == "" {
		t.Fatalf("preview did not return a diff: %+v", preview)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("preview wrote the file: %v", err)
	}

	applied, err := svc.applyPatch("work", root, create, false, true)
	if err != nil {
		t.Fatalf("apply: %v", err)
	}
	if !applied.Applied || applied.CheckpointID == "" {
		t.Fatalf("apply result: %+v", applied)
	}
	if got, err := os.ReadFile(target); err != nil || string(got) != "first\n" {
		t.Fatalf("content after create = %q (%v)", got, err)
	}

	replace := []FileEdit{{Path: "notes/todo.txt", OldText: "first", NewText: "second"}}
	if _, err := svc.applyPatch("work", root, replace, true, false); err != nil {
		t.Fatalf("replace preview: %v", err)
	}
	if _, err := svc.applyPatch("work", root, replace, false, true); err != nil {
		t.Fatalf("replace apply: %v", err)
	}
	if got, _ := os.ReadFile(target); string(got) != "second\n" {
		t.Fatalf("content after replace = %q", got)
	}

	rolled, err := svc.rollbackPatch("work", root, applied.CheckpointID)
	if err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if !rolled.RolledBack {
		t.Fatalf("rollback result: %+v", rolled)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("rollback did not remove the created file: %v", err)
	}
}

// A tampered checkpoint store must not turn a blob name into a path outside the
// local blob directory.
func TestLoadBlobRejectsNonBlobName(t *testing.T) {
	svc := &Service{CheckpointDir: t.TempDir()}
	for _, name := range []string{"", "../checkpoints.json", strings.Repeat("a", 63), strings.Repeat("g", 64)} {
		if _, err := svc.loadBlob(name); err == nil {
			t.Fatalf("expected blob name %q to be rejected", name)
		}
	}
}
