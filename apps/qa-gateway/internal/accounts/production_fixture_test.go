package accounts

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// Optional QA-only test using a real users.json, copied into a temp directory.
// No credential data is logged or checked into the source archive.
func TestActualUsersFixtureMigrationIsLossless(t *testing.T) {
	input := os.Getenv("HSPO_TEST_USERS")
	if input == "" {
		t.Skip("Set HSPO_TEST_USERS only for isolated QA on a local copy")
	}
	original, err := os.ReadFile(input)
	if err != nil {
		t.Fatal(err)
	}
	var before []map[string]json.RawMessage
	if err := json.Unmarshal(original, &before); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "users.qa.json")
	if err := os.WriteFile(path, original, 0600); err != nil {
		t.Fatal(err)
	}
	store := NewStore(path)
	_, err = store.MigrateNames()
	if err != nil {
		t.Fatal(err)
	}
	updated, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var after []map[string]json.RawMessage
	if err := json.Unmarshal(updated, &after); err != nil {
		t.Fatal(err)
	}
	if len(before) != len(after) {
		t.Fatalf("user count changed %d -> %d", len(before), len(after))
	}
	for i := range before {
		if !bytes.Equal(before[i]["username"], after[i]["username"]) {
			t.Fatalf("username changed at index %d", i)
		}
		for key, old := range before[i] {
			if key == "display_name" || key == "displayName" {
				continue
			}
			if !bytes.Equal(old, after[i][key]) {
				t.Fatalf("unexpected mutation of key %q for index %d", key, i)
			}
		}
		if !reflect.DeepEqual(after[i]["display_name"], after[i]["displayName"]) {
			t.Fatalf("name aliases disagree for index %d", i)
		}
	}
	// Re-read from disk with a new store; migration must not be lost on restart.
	restarted := NewStore(path)
	saved, err := restarted.Snapshot()
	if err != nil || len(saved) != len(before) {
		t.Fatal("restart read failed", err)
	}
}
