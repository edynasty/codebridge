package agentops

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
)

const symbolIndexMaxBytes = 16 * 1024 * 1024

func errSymbolRequired(field string) error {
	return fmt.Errorf("%s is required", field)
}

func errUnsupportedSymbolFile(rel string) error {
	return fmt.Errorf("symbol tools support Go, Java, TypeScript and JavaScript files, not %q", logicalPath(rel))
}

func errSymbolFileTooLarge(rel string) error {
	return fmt.Errorf("file %q exceeds the symbol scan limit of %d bytes", logicalPath(rel), maxSymbolFileBytes)
}

func errSymbolNotFound(name, rel string) error {
	return fmt.Errorf("symbol %q was not found in %q", name, logicalPath(rel))
}

// symbolIndex is a per-workspace cache of parsed symbols keyed by
// (modtime, size). It lives only on the local agent machine and is never
// uploaded to the Manager.
type symbolIndex struct {
	mu    sync.Mutex
	files map[string]symbolFileEntry
}

type symbolFileEntry struct {
	ModTimeNs int64    `json:"mtime_ns"`
	Size      int64    `json:"size"`
	Symbols   []Symbol `json:"symbols"`
}

type symbolIndexFile struct {
	Version int                        `json:"version"`
	Files   map[string]symbolFileEntry `json:"files"`
}

func (s *Service) loadSymbolIndex(workspace, root string) *symbolIndex {
	idx := &symbolIndex{files: map[string]symbolFileEntry{}}
	if s.IndexDir == "" {
		return idx
	}
	path := s.symbolIndexPath(workspace)
	b, err := os.ReadFile(path)
	if err != nil {
		return idx
	}
	var file symbolIndexFile
	if err := json.Unmarshal(b, &file); err != nil || file.Version != 1 {
		return idx
	}
	idx.files = file.Files
	return idx
}

func (s *Service) symbolIndexPath(workspace string) string {
	sum := sha256.Sum256([]byte(workspace))
	name := hex.EncodeToString(sum[:8]) + ".json"
	return filepath.Join(s.IndexDir, name)
}

func (i *symbolIndex) lookup(rel, abs string, info fs.FileInfo) ([]Symbol, bool) {
	i.mu.Lock()
	defer i.mu.Unlock()
	entry, ok := i.files[rel]
	if !ok || entry.Size != info.Size() || entry.ModTimeNs != info.ModTime().UnixNano() {
		return nil, false
	}
	return entry.Symbols, true
}

func (i *symbolIndex) store(rel string, info fs.FileInfo, symbols []Symbol) {
	i.mu.Lock()
	defer i.mu.Unlock()
	i.files[rel] = symbolFileEntry{ModTimeNs: info.ModTime().UnixNano(), Size: info.Size(), Symbols: symbols}
}

func (i *symbolIndex) save(path string) error {
	if path == "" {
		return nil
	}
	i.mu.Lock()
	file := symbolIndexFile{Version: 1, Files: i.files}
	i.mu.Unlock()
	if len(file.Files) == 0 {
		return nil
	}
	b, err := json.Marshal(file)
	if err != nil {
		return err
	}
	if len(b) > symbolIndexMaxBytes {
		return errors.New("symbol index exceeds local cache limit")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// DefaultClientStateDir returns the local cache directory for indexes and
// checkpoints; it stays on the agent machine and is never uploaded.
func DefaultClientStateDir() string {
	if dir, err := os.UserCacheDir(); err == nil && dir != "" {
		return filepath.Join(dir, "codebridge")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".codebridge"
	}
	return filepath.Join(home, ".cache", "codebridge")
}
