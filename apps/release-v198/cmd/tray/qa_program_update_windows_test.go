package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestProgramUpdateRollbackPreservesData(t *testing.T) {
	root := t.TempDir()
	backup := t.TempDir()
	put := func(name, contents string) {
		t.Helper()
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
			t.Fatal(err)
		}
	}
	put("APP_RELEASE.txt", "1.9.9")
	put("static/index.html", "old page")
	put("data/users.json", "fixture accounts")
	put("static/tariff_2026.json.gz", "fixture rates")
	put("static/documents/fixture.pdf", "fixture references")
	if err := saveProgram(root, backup); err != nil {
		t.Fatal(err)
	}
	put("APP_RELEASE.txt", "1.9.10")
	put("static/index.html", "new page")
	put("static/new.js", "new asset")
	put("static/tariff_2026.json.gz", "newly updated fixture rates")
	if err := restoreProgram(root, backup); err != nil {
		t.Fatal(err)
	}
	for name, want := range map[string]string{
		"APP_RELEASE.txt": "1.9.9", "static/index.html": "old page",
		"data/users.json": "fixture accounts", "static/tariff_2026.json.gz": "newly updated fixture rates",
		"static/documents/fixture.pdf": "fixture references",
	} {
		b, err := os.ReadFile(filepath.Join(root, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s = %q, want %q (%v)", name, b, want, err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, "static/new.js")); !os.IsNotExist(err) {
		t.Fatalf("new asset survived rollback: %v", err)
	}
}
