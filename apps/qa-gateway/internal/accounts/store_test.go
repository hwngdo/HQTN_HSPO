package accounts

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixture = `[
 {"username":"admin","display_name":"Admin New","displayName":"Old","role":"admin","active":true,"password":"original hash","custom":"preserve"},
 {"username":"hungdq","displayName":"Do Quang Hung","role":"admin","active":true,"password":"hash-hungdq"},
 {"username":"huelt","display_name":"Le Thi Hue","role":"user","active":true,"password":"hash-huelt"}
]`

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	p := filepath.Join(t.TempDir(), "users.json")
	if e := os.WriteFile(p, []byte(fixture), 0600); e != nil {
		t.Fatal(e)
	}
	s := NewStore(p)
	_, e := s.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	return s, p
}
func parsed(t *testing.T, p string) []UserMap {
	t.Helper()
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	u, e := decode(b)
	if e != nil {
		t.Fatal(e)
	}
	return u
}
func TestMigrationPreservesUnknownAndPasswords(t *testing.T) {
	s, p := newTestStore(t)
	n, e := s.MigrateNames()
	if e != nil || n != 3 {
		t.Fatalf("migrate %d: %v", n, e)
	}
	u := parsed(t, p)
	if getString(u[0], "displayName") != "Admin New" {
		t.Fatal("canonical priority")
	}
	if getString(u[1], "display_name") != "Do Quang Hung" {
		t.Fatal("legacy migration")
	}
	if getString(u[0], "custom") != "preserve" || getString(u[0], "password") != "original hash" {
		t.Fatal("lost keys")
	}
	b, _ := filepath.Glob(p + ".*.bak")
	if len(b) != 1 {
		t.Fatal("backup missing")
	}
	_, e = s.MigrateNames()
	if e != nil {
		t.Fatal(e)
	}
	b, _ = filepath.Glob(p + ".*.bak")
	if len(b) != 1 {
		t.Fatal("idempotent migration created backup")
	}
}
func TestUpdateIndependentAndRestart(t *testing.T) {
	s, p := newTestStore(t)
	if e := s.SetDisplayName("huelt", "New Name"); e != nil {
		t.Fatal(e)
	}
	if e := s.SetActive("huelt", false); e != nil {
		t.Fatal(e)
	}
	if e := s.SetRole("huelt", "admin"); e != nil {
		t.Fatal(e)
	}
	u := parsed(t, p)
	if getString(u[2], "password") != "hash-huelt" || getBool(u[2], "active") != false || getString(u[2], "display_name") != "New Name" || getString(u[2], "role") != "admin" {
		t.Fatal("cross-field mutation")
	}
	fresh := NewStore(p)
	snap, e := fresh.Snapshot()
	if e != nil || snap[2].DisplayName != "New Name" || snap[2].Active {
		t.Fatal("restart reload incorrect")
	}
	if e = fresh.SetActive("huelt", true); e != nil {
		t.Fatal(e)
	}
	if !getBool(parsed(t, p)[2], "active") {
		t.Fatal("unlock")
	}
}
func TestLastAdminProtected(t *testing.T) {
	s, p := newTestStore(t)
	if e := s.SetActive("hungdq", false); e != nil {
		t.Fatal(e)
	}
	if e := s.SetActive("admin", false); !errors.Is(e, ErrLastAdmin) {
		t.Fatalf("last admin disable: %v", e)
	}
	if e := s.SetRole("admin", "user"); !errors.Is(e, ErrLastAdmin) {
		t.Fatalf("last admin demote: %v", e)
	}
	if !getBool(parsed(t, p)[0], "active") {
		t.Fatal("last admin changed")
	}
}
func TestConflictIfExternalEdit(t *testing.T) {
	s, p := newTestStore(t)
	f, e := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0600)
	if e != nil {
		t.Fatal(e)
	}
	_, _ = f.WriteString(" \n")
	_ = f.Close()
	if e = s.SetDisplayName("huelt", "Name"); !errors.Is(e, ErrConflict) {
		t.Fatalf("conflict: %v", e)
	}
}
func TestNoCredentialsInExport(t *testing.T) {
	s, _ := newTestStore(t)
	var b bytes.Buffer
	if e := s.AuditSafeExport(&b); e != nil {
		t.Fatal(e)
	}
	if strings.Contains(b.String(), "password") || strings.Contains(b.String(), "hash-") {
		t.Fatal("secrets in export")
	}
}
func TestInvalidAndDuplicateData(t *testing.T) {
	for _, tc := range []string{`{}`, `[{"username":"x"},{"username":"X"}]`, `[{"username":"a","active":"yes"}]`} {
		if _, e := decode([]byte(tc)); e == nil {
			t.Fatalf("accepted: %s", tc)
		}
	}
}
func TestInvalidNameNeverWritten(t *testing.T) {
	s, p := newTestStore(t)
	before, _ := os.ReadFile(p)
	if e := s.SetDisplayName("huelt", "   "); e == nil {
		t.Fatal("accepted blank")
	}
	after, _ := os.ReadFile(p)
	if !bytes.Equal(before, after) {
		t.Fatal("mutated on rejected update")
	}
}
func TestSnapshotNotExposePassword(t *testing.T) {
	s, _ := newTestStore(t)
	v, e := s.Snapshot()
	if e != nil {
		t.Fatal(e)
	}
	b, _ := json.Marshal(v)
	if strings.Contains(string(b), "password") {
		t.Fatal("credential leak")
	}
}
