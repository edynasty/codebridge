package runtime

import (
	"fmt"
	"os"
	"path/filepath"
)

const (
	// appDirName is the per-user durable directory for the daemon's state.
	appDirName = "CodeBridge"
	// dbFileName is the runtime database inside appDirName.
	dbFileName = "runtime.db"
)

// DefaultDir returns the durable store directory. On macOS os.UserConfigDir is
// ~/Library/Application Support, so this resolves to
// ~/Library/Application Support/CodeBridge — deliberately not os.UserCacheDir,
// which the OS may purge (architecture §10, §18). Other platforms follow
// os.UserConfigDir until the daemon ships there.
func DefaultDir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("runtime: locate user config directory: %w", err)
	}
	return filepath.Join(base, appDirName), nil
}

// DefaultPath returns the default runtime database path: DefaultDir()/runtime.db.
func DefaultPath() (string, error) {
	dir, err := DefaultDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, dbFileName), nil
}
