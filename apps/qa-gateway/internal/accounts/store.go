// Package accounts implements lossless, transactional updates to the HQTN_HSPO
// user JSON file. It is intended for integration into a rebuilt Core; it does
// not monkey-patch or replace the currently deployed Core executable.
package accounts

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

var (
	ErrMissingUser = errors.New("khong tim thay tai khoan")
	ErrLastAdmin   = errors.New("khong the khoa/ha quyen admin hoat dong cuoi cung")
	ErrConflict    = errors.New("du lieu da thay doi tu ben ngoai; can tai lai")
)

type UserMap map[string]json.RawMessage

type PublicUser struct {
	Username    string `json:"username"`
	DisplayName string `json:"display_name"`
	Role        string `json:"role"`
	Active      bool   `json:"active"`
}

type Store struct {
	mu       sync.Mutex
	Path     string
	lastHash string
}

func NewStore(path string) *Store { return &Store{Path: path} }

func decode(b []byte) ([]UserMap, error) {
	var users []UserMap
	if len(b) == 0 || json.Unmarshal(b, &users) != nil || users == nil {
		return nil, errors.New("users.json phai la mang JSON hop le")
	}
	seen := map[string]bool{}
	for i, u := range users {
		n := strings.ToLower(strings.TrimSpace(getString(u, "username")))
		if n == "" {
			return nil, fmt.Errorf("user[%d] thieu username", i)
		}
		if seen[n] {
			return nil, fmt.Errorf("username trung lap: %s", n)
		}
		seen[n] = true
		if _, ok := u["active"]; ok {
			if err := json.Unmarshal(u["active"], new(bool)); err != nil {
				return nil, fmt.Errorf("active khong hop le: %s", n)
			}
		}
	}
	return users, nil
}
func getString(u UserMap, k string) string { var v string; _ = json.Unmarshal(u[k], &v); return v }
func getBool(u UserMap, k string) bool     { var v bool; _ = json.Unmarshal(u[k], &v); return v }
func put(u UserMap, k string, v any)       { b, _ := json.Marshal(v); u[k] = b }
func clone(in []UserMap) ([]UserMap, error) {
	b, e := json.Marshal(in)
	if e != nil {
		return nil, e
	}
	var out []UserMap
	e = json.Unmarshal(b, &out)
	return out, e
}
func canonicalName(u UserMap) string {
	if n := strings.TrimSpace(getString(u, "display_name")); n != "" {
		return n
	}
	if n := strings.TrimSpace(getString(u, "displayName")); n != "" {
		return n
	}
	return getString(u, "username")
}
func normalize(u UserMap) bool {
	current := canonicalName(u)
	// Transitional dual-write: old pre-1.9.x clients may still read displayName.
	// The new Core contract is display_name. Do not drop the legacy key yet.
	changed := getString(u, "display_name") != current || getString(u, "displayName") != current
	if changed {
		put(u, "display_name", current)
		put(u, "displayName", current)
	}
	return changed
}
func hash(b []byte) string { s := sha256.Sum256(b); return hex.EncodeToString(s[:]) }
func (s *Store) readLocked() ([]UserMap, []byte, error) {
	b, e := os.ReadFile(s.Path)
	if e != nil {
		return nil, nil, e
	}
	users, e := decode(b)
	return users, b, e
}
func (s *Store) Snapshot() ([]PublicUser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, b, e := s.readLocked()
	if e != nil {
		return nil, e
	}
	s.lastHash = hash(b)
	out := make([]PublicUser, len(users))
	for i, u := range users {
		out[i] = PublicUser{Username: getString(u, "username"), DisplayName: canonicalName(u), Role: getString(u, "role"), Active: getBool(u, "active")}
	}
	return out, nil
}
func verifyUsers(path string, expected []byte) error {
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	if hash(b) != hash(expected) {
		return errors.New("verify sau ghi that bai")
	}
	_, e = decode(b)
	return e
}

// atomicReplace creates a backup of the previous file, writes a same-directory
// temporary file, verifies the new file, then atomically renames it.
// A successful backup remains beside the users file as a time-stamped .bak.
func atomicReplace(path string, old, newBytes []byte) error {
	dir := filepath.Dir(path)
	if e := os.MkdirAll(dir, 0700); e != nil {
		return e
	}
	stamp := time.Now().UTC().Format("20060102T150405.000000000")
	backup := path + "." + stamp + ".bak"
	if e := os.WriteFile(backup, old, 0600); e != nil {
		return fmt.Errorf("backup: %w", e)
	}
	tmp, e := os.CreateTemp(dir, ".hspo-users-*.tmp")
	if e != nil {
		return e
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if e = tmp.Chmod(0600); e != nil {
		tmp.Close()
		return e
	}
	if _, e = tmp.Write(newBytes); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Sync(); e != nil {
		tmp.Close()
		return e
	}
	if e = tmp.Close(); e != nil {
		return e
	}
	if e = verifyUsers(tmpName, newBytes); e != nil {
		return e
	}
	if e = os.Rename(tmpName, path); e != nil {
		return fmt.Errorf("atomic rename: %w", e)
	}
	if e = verifyUsers(path, newBytes); e != nil {
		return e
	}
	// Best effort directory sync for POSIX. Not available on all Windows filesystems.
	if d, e := os.Open(dir); e == nil {
		_ = d.Sync()
		_ = d.Close()
	}
	return nil
}
func (s *Store) commit(mutate func([]UserMap) (bool, error)) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	users, original, e := s.readLocked()
	if e != nil {
		return e
	}
	if s.lastHash != "" && s.lastHash != hash(original) {
		return ErrConflict
	}
	c, e := clone(users)
	if e != nil {
		return e
	}
	changed, e := mutate(c)
	if e != nil {
		return e
	}
	if !changed {
		s.lastHash = hash(original)
		return nil
	}
	if _, e = decodeMust(c); e != nil {
		return e
	}
	final, e := json.MarshalIndent(c, "", "  ")
	if e != nil {
		return e
	}
	final = append(final, '\n')
	if e = atomicReplace(s.Path, original, final); e != nil {
		return e
	}
	s.lastHash = hash(final)
	return nil
}
func decodeMust(v []UserMap) ([]UserMap, error) {
	b, e := json.Marshal(v)
	if e != nil {
		return nil, e
	}
	return decode(b)
}
func find(users []UserMap, username string) (int, error) {
	for i, u := range users {
		if strings.EqualFold(strings.TrimSpace(getString(u, "username")), strings.TrimSpace(username)) {
			return i, nil
		}
	}
	return -1, ErrMissingUser
}
func countActiveAdmins(users []UserMap) int {
	n := 0
	for _, u := range users {
		if strings.EqualFold(getString(u, "role"), "admin") && getBool(u, "active") {
			n++
		}
	}
	return n
}

// MigrateNames is reversible from the .bak file. Does not change passwords,
// username, role, active status or unrecognized/extension keys.
func (s *Store) MigrateNames() (int, error) {
	count := 0
	e := s.commit(func(users []UserMap) (bool, error) {
		for _, u := range users {
			if normalize(u) {
				count++
			}
		}
		return count > 0, nil
	})
	return count, e
}
func (s *Store) SetDisplayName(username, value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("ho ten khong duoc rong")
	}
	if len([]rune(value)) > 128 {
		return errors.New("ho ten qua dai")
	}
	return s.commit(func(users []UserMap) (bool, error) {
		i, e := find(users, username)
		if e != nil {
			return false, e
		}
		u := users[i]
		if canonicalName(u) == value && getString(u, "display_name") == value && getString(u, "displayName") == value {
			return false, nil
		}
		put(u, "display_name", value)
		put(u, "displayName", value)
		return true, nil
	})
}
func (s *Store) SetActive(username string, active bool) error {
	return s.commit(func(users []UserMap) (bool, error) {
		i, e := find(users, username)
		if e != nil {
			return false, e
		}
		u := users[i]
		if getBool(u, "active") == active {
			return false, nil
		}
		if !active && strings.EqualFold(getString(u, "role"), "admin") && getBool(u, "active") && countActiveAdmins(users) == 1 {
			return false, ErrLastAdmin
		}
		put(u, "active", active)
		return true, nil
	})
}
func (s *Store) SetRole(username, role string) error {
	if role != "admin" && role != "user" && role != "staff" {
		return errors.New("vai tro khong hop le")
	}
	return s.commit(func(users []UserMap) (bool, error) {
		i, e := find(users, username)
		if e != nil {
			return false, e
		}
		u := users[i]
		if getString(u, "role") == role {
			return false, nil
		}
		if getBool(u, "active") && strings.EqualFold(getString(u, "role"), "admin") && role != "admin" && countActiveAdmins(users) == 1 {
			return false, ErrLastAdmin
		}
		put(u, "role", role)
		return true, nil
	})
}

// Password changes deliberately remain in the existing Core password endpoint.
// Do not write a guessed hash into users.json: the deployed Core format has
// not been migrated and this store only manages identity, role and active state.
func (s *Store) AuditSafeExport(w io.Writer) error {
	users, e := s.Snapshot()
	if e != nil {
		return e
	}
	return json.NewEncoder(w).Encode(users) // never export password or token
}
