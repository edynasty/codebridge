package clientconfig

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type Config struct {
	ManagerURL          string            `json:"manager_url,omitempty"`
	DeviceID            string            `json:"device_id,omitempty"`
	DeviceName          string            `json:"device_name,omitempty"`
	Workspaces          map[string]string `json:"workspaces,omitempty"`
	WritableWorkspaces  []string          `json:"writable_workspaces,omitempty"`
	CredentialFile      string            `json:"credential_file,omitempty"`
	AllowSensitiveFiles bool              `json:"allow_sensitive_files,omitempty"`
	EnableLSP           bool              `json:"enable_lsp,omitempty"`
	CheckpointDir       string            `json:"checkpoint_dir,omitempty"`
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".codebridge-client.json"
	}
	return filepath.Join(home, ".config", "codebridge", "client.json")
}

// ResolvePath finds the config path before flag.Parse so config values can
// become flag defaults. --config supports both "--config path" and
// "--config=path". CODEBRIDGE_CONFIG overrides the default when the flag is
// absent.
func ResolvePath(args []string) (string, bool, error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" {
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", true, errors.New("--config requires a path")
			}
			return expandHome(strings.TrimSpace(args[i+1])), true, nil
		}
		if strings.HasPrefix(arg, "--config=") {
			value := strings.TrimSpace(strings.TrimPrefix(arg, "--config="))
			if value == "" {
				return "", true, errors.New("--config requires a path")
			}
			return expandHome(value), true, nil
		}
	}
	if value := strings.TrimSpace(os.Getenv("CODEBRIDGE_CONFIG")); value != "" {
		return expandHome(value), true, nil
	}
	return DefaultPath(), false, nil
}

func Load(path string, required bool) (Config, error) {
	var cfg Config
	path = expandHome(strings.TrimSpace(path))
	if path == "" {
		return cfg, errors.New("config path is empty")
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && !required {
		return cfg, nil
	}
	if err != nil {
		return cfg, fmt.Errorf("read client config: %w", err)
	}
	if err := json.Unmarshal(b, &cfg); err != nil {
		return cfg, fmt.Errorf("decode client config: %w", err)
	}
	cfg.ManagerURL = strings.TrimSpace(cfg.ManagerURL)
	cfg.DeviceID = strings.TrimSpace(cfg.DeviceID)
	cfg.DeviceName = strings.TrimSpace(cfg.DeviceName)
	cfg.CredentialFile = expandHome(strings.TrimSpace(cfg.CredentialFile))
	if cfg.Workspaces == nil {
		cfg.Workspaces = map[string]string{}
	}
	for name, path := range cfg.Workspaces {
		trimmedName := strings.TrimSpace(name)
		trimmedPath := expandHome(strings.TrimSpace(path))
		if trimmedName != name {
			delete(cfg.Workspaces, name)
		}
		if trimmedName != "" {
			cfg.Workspaces[trimmedName] = trimmedPath
		}
	}
	return cfg, nil
}

func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err == nil && home != "" {
			if path == "~" {
				return home
			}
			return filepath.Join(home, strings.TrimPrefix(path, "~/"))
		}
	}
	return path
}
