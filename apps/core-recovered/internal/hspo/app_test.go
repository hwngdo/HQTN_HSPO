package hspo

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRecoveredCorePreservesUnreadableAccounts(t *testing.T) {
	root := t.TempDir()
	data := filepath.Join(root, "data")
	if err := os.Mkdir(data, 0700); err != nil {
		t.Fatal(err)
	}
	accounts := filepath.Join(data, "users.json")
	original := []byte(`{"unexpected_legacy_shape":[]}`)
	if err := os.WriteFile(accounts, original, 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "synthetic-test-password")
	if _, err := New(root); err == nil {
		t.Fatal("expected unsupported account file to prevent startup")
	}
	after, err := os.ReadFile(accounts)
	if err != nil || string(after) != string(original) {
		t.Fatalf("account file changed: %q, %v", after, err)
	}
}

func TestRecoveredCoreBootstrapAndReload(t *testing.T) {
	root := t.TempDir()
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "")
	if _, err := New(root); err == nil {
		t.Fatal("expected bootstrap without password to fail")
	}
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "synthetic-test-password")
	fresh, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "")
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(reloaded.Users) != 1 || reloaded.Users[0].Password != fresh.Users[0].Password {
		t.Fatal("bootstrap account did not persist")
	}
}

func TestRecoveredCoreRefusesLegacyRuntime(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "userdata"), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "synthetic-test-password")
	if _, err := New(root); err == nil {
		t.Fatal("expected runtime with legacy userdata to prevent startup")
	}
	if _, err := os.Stat(filepath.Join(root, "data")); !os.IsNotExist(err) {
		t.Fatalf("unexpected QA data directory: %v", err)
	}
}
