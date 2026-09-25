package agentops

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	maxPatchFiles       = 20
	maxPatchFileBytes   = 512 * 1024
	maxPatchTotalBytes  = 2 * 1024 * 1024
	maxCheckpointsKept  = 20
	checkpointStoreFile = "checkpoints.json"
	diffContextLines    = 3
)

// FileEdit is one all-or-nothing change in an apply_patch call.
//
//	Create:  OldText == "", NewText != ""  -> file must not exist
//	delete:  OldText != "", NewText == ""  -> full content must match OldText
//	replace: both set                      -> exactly one occurrence of OldText
type FileEdit struct {
	Path    string `json:"path"`
	OldText string `json:"old_text,omitempty"`
	NewText string `json:"new_text,omitempty"`
}

type PatchSummary struct {
	Path   string `json:"path"`
	Action string `json:"action"`
}

type ApplyPatchResult struct {
	Applied      bool           `json:"applied"`
	Preview      bool           `json:"preview,omitempty"`
	Diff         string         `json:"diff,omitempty"`
	Files        []PatchSummary `json:"files"`
	CheckpointID string         `json:"checkpoint_id,omitempty"`
	Message      string         `json:"message,omitempty"`
}

type RollbackResult struct {
	RolledBack    bool     `json:"rolled_back"`
	CheckpointID  string   `json:"checkpoint_id"`
	RestoredFiles []string `json:"restored_files"`
	RemovedFiles  []string `json:"removed_files,omitempty"`
}

type checkpointFile struct {
	Path string `json:"path"`
	// Blob names the pre-patch content in the local blob store
	// (<state dir>/checkpoints/blobs/<sha256>); empty when the file was absent.
	Blob    string `json:"blob,omitempty"`
	Absent  bool   `json:"absent,omitempty"`
	Mode    uint32 `json:"mode,omitempty"`
	Existed bool   `json:"existed"`
}

type checkpoint struct {
	ID        string           `json:"id"`
	Workspace string           `json:"workspace"`
	CreatedAt time.Time        `json:"created_at"`
	Files     []checkpointFile `json:"files"`
}

type checkpointStore struct {
	Checkpoints []checkpoint `json:"checkpoints"`
}

var writeMu sync.Mutex

func (s *Service) applyPatch(workspace, root string, edits []FileEdit, preview, confirm bool) (ApplyPatchResult, error) {
	if !s.Writable[workspace] {
		return ApplyPatchResult{}, errors.New("workspace is not write-enabled; writing requires explicit local opt-in on the client")
	}
	if len(edits) == 0 {
		return ApplyPatchResult{}, errors.New("edits are required")
	}
	if len(edits) > maxPatchFiles {
		return ApplyPatchResult{}, fmt.Errorf("patch exceeds %d files", maxPatchFiles)
	}
	if !preview && !confirm {
		return ApplyPatchResult{}, errors.New("confirmation required: call with preview=true first, then repeat with confirm=true")
	}

	rootReal, err := s.resolveWorkspaceRoot(root)
	if err != nil {
		return ApplyPatchResult{}, err
	}

	planned := make([]plannedEdit, 0, len(edits))
	total := 0
	seen := map[string]bool{}

	for _, edit := range edits {
		rel := logicalPath(strings.TrimSpace(edit.Path))
		if rel == "" || rel == "." {
			return ApplyPatchResult{}, errors.New("edit path is required")
		}
		if filepath.IsAbs(edit.Path) {
			return ApplyPatchResult{}, fmt.Errorf("absolute paths are not allowed")
		}
		if strings.HasPrefix(rel, "../") {
			return ApplyPatchResult{}, fmt.Errorf("path escapes workspace")
		}
		// Sensitive paths are never writable, even with the local read opt-in.
		if isSensitivePath(rel) {
			return ApplyPatchResult{}, fmt.Errorf("refusing to write sensitive path %q", rel)
		}
		if seen[rel] {
			return ApplyPatchResult{}, fmt.Errorf("duplicate edit path %q", rel)
		}
		seen[rel] = true

		action := "replace"
		switch {
		case edit.OldText == "" && edit.NewText != "":
			action = "create"
		case edit.OldText != "" && edit.NewText == "":
			action = "delete"
		case edit.OldText == "" && edit.NewText == "":
			return ApplyPatchResult{}, fmt.Errorf("edit for %q must set old_text or new_text", rel)
		}
		if len(edit.NewText) > maxPatchFileBytes {
			return ApplyPatchResult{}, fmt.Errorf("edit for %q exceeds %d bytes", rel, maxPatchFileBytes)
		}
		total += len(edit.NewText)
		if total > maxPatchTotalBytes {
			return ApplyPatchResult{}, fmt.Errorf("patch exceeds total %d bytes", maxPatchTotalBytes)
		}
		if !utf8.ValidString(edit.NewText) || strings.ContainsRune(edit.NewText, 0) {
			return ApplyPatchResult{}, fmt.Errorf("edit for %q contains binary content; binary writes are not supported", rel)
		}

		abs, err := ResolveUnderRoot(root, rel)
		if err != nil {
			return ApplyPatchResult{}, err
		}
		info, statErr := os.Lstat(abs)

		switch action {
		case "create":
			if statErr == nil {
				return ApplyPatchResult{}, fmt.Errorf("cannot create %q: file already exists", rel)
			}
			if !errors.Is(statErr, fs.ErrNotExist) {
				return ApplyPatchResult{}, safePathError("stat", rel, statErr)
			}
			if err := ensureParentDir(rootReal, rel); err != nil {
				return ApplyPatchResult{}, err
			}
			planned = append(planned, plannedEdit{rel: rel, abs: abs, action: action, newContent: []byte(edit.NewText)})
		case "delete":
			if statErr != nil {
				return ApplyPatchResult{}, safePathError("delete", rel, statErr)
			}
			current, readErr := os.ReadFile(abs)
			if readErr != nil {
				return ApplyPatchResult{}, safePathError("read", rel, readErr)
			}
			if string(current) != edit.OldText {
				return ApplyPatchResult{}, fmt.Errorf("delete %q failed: old_text does not match the full current content", rel)
			}
			planned = append(planned, plannedEdit{rel: rel, abs: abs, action: action, oldContent: current, mode: info.Mode().Perm()})
		case "replace":
			if statErr != nil {
				return ApplyPatchResult{}, safePathError("read", rel, statErr)
			}
			current, readErr := os.ReadFile(abs)
			if readErr != nil {
				return ApplyPatchResult{}, safePathError("read", rel, readErr)
			}
			count := strings.Count(string(current), edit.OldText)
			if count == 0 {
				return ApplyPatchResult{}, fmt.Errorf("replace %q failed: old_text not found", rel)
			}
			if count > 1 {
				return ApplyPatchResult{}, fmt.Errorf("replace %q failed: old_text matches %d locations; make it unique", rel, count)
			}
			planned = append(planned, plannedEdit{
				rel: rel, abs: abs, action: action,
				oldContent: current,
				newContent: []byte(strings.Replace(string(current), edit.OldText, edit.NewText, 1)),
				mode:       info.Mode().Perm(),
			})
		}
	}

	summaries := make([]PatchSummary, 0, len(planned))
	for _, p := range planned {
		summaries = append(summaries, PatchSummary{Path: p.rel, Action: p.action})
	}

	if preview {
		var diff strings.Builder
		for _, p := range planned {
			writeEditDiff(&diff, p.rel, p.action, p.oldContent, p.newContent)
		}
		return ApplyPatchResult{
			Preview: true,
			Diff:    diff.String(),
			Files:   summaries,
			Message: "Preview only; nothing was written. Re-issue the same patch with confirm=true to apply.",
		}, nil
	}

	writeMu.Lock()
	defer writeMu.Unlock()

	cp, err := s.createCheckpoint(workspace, planned)
	if err != nil {
		return ApplyPatchResult{}, err
	}

	rootHandle, err := openWorkspaceRoot(root)
	if err != nil {
		return ApplyPatchResult{}, err
	}
	defer rootHandle.Close()

	applied := make([]plannedEdit, 0, len(planned))
	for _, p := range planned {
		writeErr := func() error {
			switch p.action {
			case "delete":
				return rootHandle.Remove(p.rel)
			case "create":
				f, err := rootHandle.OpenFile(p.rel, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Write(p.newContent)
				return err
			default:
				f, err := rootHandle.OpenFile(p.rel, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, p.mode)
				if err != nil {
					return err
				}
				defer f.Close()
				_, err = f.Write(p.newContent)
				return err
			}
		}()
		if writeErr != nil {
			s.restorePartial(root, rootHandle, applied, cp)
			return ApplyPatchResult{}, fmt.Errorf("patch failed while writing %q; rolled back to checkpoint %s", p.rel, cp.ID)
		}
		applied = append(applied, p)
	}

	return ApplyPatchResult{
		Applied:      true,
		Files:        summaries,
		CheckpointID: cp.ID,
		Message:      fmt.Sprintf("Applied %d file change(s). Roll back with rollback_patch and checkpoint_id %s.", len(applied), cp.ID),
	}, nil
}

// ensureParentDir creates missing parent directories for a create edit,
// resolving the parent under the workspace root first.
func ensureParentDir(rootReal, rel string) error {
	parent := logicalPath(filepath.Dir(strings.TrimPrefix(rel, "./")))
	if parent == "." || parent == "" {
		return nil
	}
	if _, err := ResolveUnderRoot(rootReal, parent+"/"); err != nil {
		return fmt.Errorf("parent directory of %q is not allowed", rel)
	}
	if _, err := os.Stat(filepath.Join(rootReal, filepath.FromSlash(parent))); err == nil {
		return nil
	}
	return os.MkdirAll(filepath.Join(rootReal, filepath.FromSlash(parent)), 0o755)
}

func (s *Service) rollbackPatch(workspace, root, checkpointID string) (RollbackResult, error) {
	if !s.Writable[workspace] {
		return RollbackResult{}, errors.New("workspace is not write-enabled; writing requires explicit local opt-in on the client")
	}
	store, err := s.loadCheckpointStore()
	if err != nil {
		return RollbackResult{}, err
	}
	var cp *checkpoint
	for i := range store.Checkpoints {
		if store.Checkpoints[i].ID == checkpointID && store.Checkpoints[i].Workspace == workspace {
			cp = &store.Checkpoints[i]
			break
		}
	}
	if cp == nil {
		return RollbackResult{}, fmt.Errorf("checkpoint %q not found for this workspace", checkpointID)
	}

	writeMu.Lock()
	defer writeMu.Unlock()

	rootHandle, err := openWorkspaceRoot(root)
	if err != nil {
		return RollbackResult{}, err
	}
	defer rootHandle.Close()

	result := RollbackResult{CheckpointID: checkpointID}
	for _, f := range cp.Files {
		if f.Absent {
			if err := rootHandle.Remove(f.Path); err != nil && !errors.Is(err, fs.ErrNotExist) {
				return result, safePathError("rollback remove", f.Path, err)
			}
			result.RemovedFiles = append(result.RemovedFiles, f.Path)
			continue
		}
		content, err := s.loadBlob(f.Blob)
		if err != nil {
			return result, fmt.Errorf("rollback %q failed: checkpoint blob is unavailable", f.Path)
		}
		mode := fs.FileMode(f.Mode)
		if mode == 0 {
			mode = 0o644
		}
		if err := rootHandle.MkdirAll(filepath.Dir(filepath.FromSlash(f.Path)), 0o755); err != nil {
			return result, safePathError("rollback mkdir", f.Path, err)
		}
		if err := rootHandle.WriteFile(f.Path, content, mode); err != nil {
			return result, safePathError("rollback write", f.Path, err)
		}
		result.RestoredFiles = append(result.RestoredFiles, f.Path)
	}
	result.RolledBack = true
	return result, nil
}

// plannedEdit is one fully validated change ready to be written.
type plannedEdit struct {
	rel        string
	abs        string
	action     string
	oldContent []byte
	newContent []byte
	mode       fs.FileMode
}

// createCheckpoint snapshots every affected file into the local
// content-addressed blob store before the patch is applied, so rollback
// restores the exact pre-patch content. Any directory works; write mode no
// longer requires a git repository.
func (s *Service) createCheckpoint(workspace string, planned []plannedEdit) (checkpoint, error) {
	cp := checkpoint{ID: newCheckpointID(), Workspace: workspace, CreatedAt: time.Now().UTC()}
	for _, p := range planned {
		entry := checkpointFile{Path: p.rel}
		info, err := os.Lstat(p.abs)
		switch {
		case err == nil:
			entry.Existed = true
			entry.Mode = uint32(info.Mode().Perm())
			content, readErr := os.ReadFile(p.abs)
			if readErr != nil {
				return cp, safePathError("checkpoint read", p.rel, readErr)
			}
			blob, blobErr := s.storeBlob(content)
			if blobErr != nil {
				return cp, fmt.Errorf("checkpoint %q failed: %w", p.rel, blobErr)
			}
			entry.Blob = blob
		case errors.Is(err, fs.ErrNotExist):
			entry.Absent = true
		default:
			return cp, safePathError("checkpoint stat", p.rel, err)
		}
		cp.Files = append(cp.Files, entry)
	}
	if err := s.appendCheckpoint(cp); err != nil {
		return cp, fmt.Errorf("persist checkpoint: %w", err)
	}
	return cp, nil
}

func (s *Service) restorePartial(root string, rootHandle *os.Root, applied []plannedEdit, cp checkpoint) {
	for _, p := range applied {
		for _, f := range cp.Files {
			if f.Path != p.rel {
				continue
			}
			if f.Absent {
				_ = rootHandle.Remove(p.rel)
				continue
			}
			if content, err := s.loadBlob(f.Blob); err == nil {
				mode := fs.FileMode(f.Mode)
				if mode == 0 {
					mode = 0o644
				}
				_ = rootHandle.WriteFile(p.rel, content, mode)
			}
		}
	}
}

func (s *Service) checkpointDir() string {
	if s.CheckpointDir != "" {
		return s.CheckpointDir
	}
	return DefaultClientStateDir()
}

func (s *Service) checkpointStorePath() string {
	return filepath.Join(s.checkpointDir(), checkpointStoreFile)
}

// blobDir holds content-addressed pre-write snapshots. Blobs are pruned when
// the checkpoint referencing them falls out of the retention window.
func (s *Service) blobDir() string {
	return filepath.Join(s.checkpointDir(), "blobs")
}

// storeBlob writes content under its SHA-256 and returns the blob name.
func (s *Service) storeBlob(content []byte) (string, error) {
	sum := sha256.Sum256(content)
	name := hex.EncodeToString(sum[:])
	dir := s.blobDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if _, err := os.Stat(path); err == nil {
		return name, nil
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, 0o600); err != nil {
		return "", err
	}
	if err := os.Rename(tmp, path); err != nil {
		return "", err
	}
	return name, nil
}

func (s *Service) loadBlob(name string) ([]byte, error) {
	if !isBlobName(name) {
		return nil, errors.New("invalid checkpoint blob name")
	}
	return os.ReadFile(filepath.Join(s.blobDir(), name))
}

func isBlobName(name string) bool {
	if len(name) != sha256.Size*2 {
		return false
	}
	for i := range name {
		if c := name[i]; (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}

// pruneBlobs deletes snapshots no longer referenced by a retained checkpoint.
func (s *Service) pruneBlobs(store checkpointStore) {
	keep := map[string]bool{}
	for _, cp := range store.Checkpoints {
		for _, f := range cp.Files {
			if f.Blob != "" {
				keep[f.Blob] = true
			}
		}
	}
	entries, err := os.ReadDir(s.blobDir())
	if err != nil {
		return
	}
	for _, entry := range entries {
		if keep[entry.Name()] {
			continue
		}
		_ = os.Remove(filepath.Join(s.blobDir(), entry.Name()))
	}
}

func (s *Service) loadCheckpointStore() (checkpointStore, error) {
	var store checkpointStore
	b, err := os.ReadFile(s.checkpointStorePath())
	if errors.Is(err, os.ErrNotExist) {
		return store, nil
	}
	if err != nil {
		return store, fmt.Errorf("read checkpoint store: %w", err)
	}
	if err := json.Unmarshal(b, &store); err != nil {
		return store, fmt.Errorf("decode checkpoint store: %w", err)
	}
	return store, nil
}

func (s *Service) appendCheckpoint(cp checkpoint) error {
	store, err := s.loadCheckpointStore()
	if err != nil {
		return err
	}
	store.Checkpoints = append(store.Checkpoints, cp)
	if len(store.Checkpoints) > maxCheckpointsKept {
		store.Checkpoints = store.Checkpoints[len(store.Checkpoints)-maxCheckpointsKept:]
	}
	s.pruneBlobs(store)
	b, err := json.MarshalIndent(store, "", "  ")
	if err != nil {
		return err
	}
	path := s.checkpointStorePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func newCheckpointID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return fmt.Sprintf("cbk_%d", time.Now().UnixNano())
	}
	return "cbk_" + hex.EncodeToString(b[:])
}

// writeEditDiff renders one unified-diff-style section for a planned edit.
// The changed region is known exactly, so no general diff algorithm is needed.
func writeEditDiff(out *strings.Builder, rel, action string, oldContent, newContent []byte) {
	out.WriteString("diff --git a/" + rel + " b/" + rel + "\n")
	oldLines := splitLines(string(oldContent))
	newLines := splitLines(string(newContent))
	switch action {
	case "create":
		writeHunk(out, 0, 0, 1, len(newLines), nil, newLines)
	case "delete":
		writeHunk(out, 1, len(oldLines), 0, 0, oldLines, nil)
	default:
		start, removed, added := locateChangedLines(oldLines, newLines)
		from := start - diffContextLines
		if from < 0 {
			from = 0
		}
		to := start + removed + diffContextLines
		if to > len(oldLines) {
			to = len(oldLines)
		}
		before := oldLines[from:start]
		after := oldLines[start+removed : to]
		changed := newLines[start : start+added]
		out.WriteString(fmt.Sprintf("--- a/%s\n+++ b/%s\n", rel, rel))
		writeHunkRaw(out, from+1, to-from, from+1, from+len(changed), before, after, removed2lines(oldLines, start, removed), changed)
	}
}

// locateChangedLines returns the replaced line span: the first changed line,
// how many old lines it covers, and how many new lines replace it. It is only
// called for exact-match replacements where newLines is oldLines after a
// single old_text -> new_text substitution.
func locateChangedLines(oldLines, newLines []string) (start, removed, added int) {
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	suffix := 0
	for suffix < len(oldLines)-prefix && suffix < len(newLines)-prefix &&
		oldLines[len(oldLines)-1-suffix] == newLines[len(newLines)-1-suffix] {
		suffix++
	}
	start = prefix
	removed = len(oldLines) - prefix - suffix
	added = len(newLines) - prefix - suffix
	if removed < 0 {
		removed = 0
	}
	if added < 0 {
		added = 0
	}
	return start, removed, added
}

func removed2lines(lines []string, start, count int) []string {
	if count <= 0 || start+count > len(lines) {
		return nil
	}
	return lines[start : start+count]
}

func writeHunk(out *strings.Builder, oldStart, oldCount, newStart, newCount int, removed, added []string) {
	out.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
	for _, line := range removed {
		out.WriteString("-" + line + "\n")
	}
	for _, line := range added {
		out.WriteString("+" + line + "\n")
	}
}

func writeHunkRaw(out *strings.Builder, oldStart, oldCount, newStart, newCount int, contextBefore, contextAfter, removed, added []string) {
	out.WriteString(fmt.Sprintf("@@ -%d,%d +%d,%d @@\n", oldStart, oldCount, newStart, newCount))
	for _, line := range contextBefore {
		out.WriteString(" " + line + "\n")
	}
	for _, line := range removed {
		out.WriteString("-" + line + "\n")
	}
	for _, line := range added {
		out.WriteString("+" + line + "\n")
	}
	for _, line := range contextAfter {
		out.WriteString(" " + line + "\n")
	}
}

func splitLines(content string) []string {
	if content == "" {
		return nil
	}
	lines := strings.Split(strings.TrimSuffix(content, "\n"), "\n")
	return lines
}
