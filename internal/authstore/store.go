package authstore

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

const stateVersion = 2

// DefaultAccount is the account devices fall back to when no explicit
// account was configured at enrollment time, and the account that MCP
// requests run under when OAuth is disabled for local development.
const DefaultAccount = "default"

const maxAccountIDBytes = 128

type Device struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	AccountID      string    `json:"account_id"`
	CredentialHash string    `json:"credential_hash"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type PublicDevice struct {
	ID        string    `json:"id"`
	Name      string    `json:"name"`
	AccountID string    `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type Enrollment struct {
	Hash      string    `json:"hash"`
	AccountID string    `json:"account_id"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type persistedState struct {
	Version     int                   `json:"version"`
	Devices     map[string]Device     `json:"devices"`
	Enrollments map[string]Enrollment `json:"enrollments"`
}

type Store struct {
	mu    sync.Mutex
	path  string
	state persistedState
}

func Open(path string) (*Store, error) {
	s := &Store{path: path, state: persistedState{Version: stateVersion, Devices: map[string]Device{}, Enrollments: map[string]Enrollment{}}}
	if path == "" {
		return s, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(b, &s.state); err != nil {
		return nil, fmt.Errorf("decode auth state: %w", err)
	}
	if s.state.Version != 1 && s.state.Version != stateVersion {
		return nil, fmt.Errorf("unsupported auth state version %d", s.state.Version)
	}
	if s.state.Version == 1 {
		// v1 had no accounts; single-user devices and pending enrollments
		// migrate to the default account so existing deployments keep working.
		for id, d := range s.state.Devices {
			d.AccountID = DefaultAccount
			s.state.Devices[id] = d
		}
		for h, e := range s.state.Enrollments {
			e.AccountID = DefaultAccount
			s.state.Enrollments[h] = e
		}
		s.state.Version = stateVersion
	}
	if s.state.Devices == nil {
		s.state.Devices = map[string]Device{}
	}
	if s.state.Enrollments == nil {
		s.state.Enrollments = map[string]Enrollment{}
	}
	return s, nil
}

func (s *Store) CreateEnrollment(ttl time.Duration, accountID string) (string, time.Time, error) {
	accountID = strings.TrimSpace(accountID)
	if err := validateAccountID(accountID); err != nil {
		return "", time.Time{}, err
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > 24*time.Hour {
		ttl = 24 * time.Hour
	}
	secret, err := randomSecret("enr_")
	if err != nil {
		return "", time.Time{}, err
	}
	now := time.Now().UTC()
	expires := now.Add(ttl)
	h := digest(secret)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	s.state.Enrollments[h] = Enrollment{Hash: h, AccountID: accountID, CreatedAt: now, ExpiresAt: expires}
	if err := s.persistLocked(); err != nil {
		delete(s.state.Enrollments, h)
		return "", time.Time{}, err
	}
	return secret, expires, nil
}

func validateAccountID(accountID string) error {
	if accountID == "" {
		return errors.New("account_id is required")
	}
	if len(accountID) > maxAccountIDBytes {
		return fmt.Errorf("account_id exceeds %d bytes", maxAccountIDBytes)
	}
	return nil
}

// EnrollDevice consumes an enrollment code exactly once and returns a new
// high-entropy device credential. Only the SHA-256 digest is persisted.
func (s *Store) EnrollDevice(code, deviceID, deviceName string) (string, error) {
	if code == "" {
		return "", errors.New("enrollment code is required")
	}
	if deviceID == "" || deviceName == "" {
		return "", errors.New("device id and name are required")
	}
	now := time.Now().UTC()
	codeHash := digest(code)

	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneExpiredLocked(now)
	entry, ok := s.state.Enrollments[codeHash]
	if !ok || now.After(entry.ExpiresAt) {
		return "", errors.New("invalid or expired enrollment code")
	}
	if _, exists := s.state.Devices[deviceID]; exists {
		return "", errors.New("device already enrolled; authenticate with its device credential or revoke it first")
	}
	credential, err := randomSecret("dev_")
	if err != nil {
		return "", err
	}
	delete(s.state.Enrollments, codeHash)
	s.state.Devices[deviceID] = Device{ID: deviceID, Name: deviceName, AccountID: entry.AccountID, CredentialHash: digest(credential), CreatedAt: now, UpdatedAt: now}
	if err := s.persistLocked(); err != nil {
		delete(s.state.Devices, deviceID)
		s.state.Enrollments[codeHash] = entry
		return "", err
	}
	return credential, nil
}

func (s *Store) VerifyDevice(deviceID, credential string) bool {
	if deviceID == "" || credential == "" {
		return false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.state.Devices[deviceID]
	if !ok {
		return false
	}
	got := digest(credential)
	return subtle.ConstantTimeCompare([]byte(got), []byte(d.CredentialHash)) == 1
}

func (s *Store) UpdateDeviceName(deviceID, name string) error {
	if name == "" {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.state.Devices[deviceID]
	if !ok {
		return errors.New("device not enrolled")
	}
	if d.Name == name {
		return nil
	}
	d.Name = name
	d.UpdatedAt = time.Now().UTC()
	s.state.Devices[deviceID] = d
	return s.persistLocked()
}

func (s *Store) RotateDevice(deviceID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.state.Devices[deviceID]
	if !ok {
		return "", errors.New("device not enrolled")
	}
	credential, err := randomSecret("dev_")
	if err != nil {
		return "", err
	}
	old := d
	d.CredentialHash = digest(credential)
	d.UpdatedAt = time.Now().UTC()
	s.state.Devices[deviceID] = d
	if err := s.persistLocked(); err != nil {
		s.state.Devices[deviceID] = old
		return "", err
	}
	return credential, nil
}

func (s *Store) RevokeDevice(deviceID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.state.Devices[deviceID]
	if !ok {
		return errors.New("device not enrolled")
	}
	delete(s.state.Devices, deviceID)
	if err := s.persistLocked(); err != nil {
		s.state.Devices[deviceID] = d
		return err
	}
	return nil
}

func (s *Store) ListDevices() []PublicDevice {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PublicDevice, 0, len(s.state.Devices))
	for _, d := range s.state.Devices {
		out = append(out, PublicDevice{ID: d.ID, Name: d.Name, AccountID: d.AccountID, CreatedAt: d.CreatedAt, UpdatedAt: d.UpdatedAt})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// DeviceAccount returns the account a device belongs to. The Manager uses it
// to bind the online registry entry to the persisted tenant and must reject
// registration when it is unknown.
func (s *Store) DeviceAccount(deviceID string) (string, bool) {
	if deviceID == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	d, ok := s.state.Devices[deviceID]
	if !ok || d.AccountID == "" {
		return "", false
	}
	return d.AccountID, true
}

func (s *Store) pruneExpiredLocked(now time.Time) {
	for h, e := range s.state.Enrollments {
		if now.After(e.ExpiresAt) {
			delete(s.state.Enrollments, h)
		}
	}
}

func (s *Store) persistLocked() error {
	if s.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s.state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func randomSecret(prefix string) (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return prefix + base64.RawURLEncoding.EncodeToString(b), nil
}

func digest(v string) string {
	sum := sha256.Sum256([]byte(v))
	return hex.EncodeToString(sum[:])
}
