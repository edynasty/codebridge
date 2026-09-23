package clientcred

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

type fileData struct {
	Credentials map[string]string `json:"credentials"`
}

func DefaultPath() string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ".codebridge-credentials.json"
	}
	return filepath.Join(home, ".config", "codebridge", "credentials.json")
}

func Key(managerURL, deviceID string) string { return managerURL + "|" + deviceID }

func Load(path, key string) (string, error) {
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	var d fileData
	if err := json.Unmarshal(b, &d); err != nil {
		return "", err
	}
	return d.Credentials[key], nil
}

func Save(path, key, credential string) error {
	if credential == "" {
		return nil
	}
	d := fileData{Credentials: map[string]string{}}
	if b, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(b, &d)
	}
	if d.Credentials == nil {
		d.Credentials = map[string]string{}
	}
	d.Credentials[key] = credential
	b, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
