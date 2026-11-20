package main

import (
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func root(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	if e := os.WriteFile(filepath.Join(p, "preview.html"), []byte("PREVIEW_ONLY"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(p, "static"), 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(p, "static", "index.html"), []byte("UI_OVERRIDE"), 0600); e != nil {
		t.Fatal(e)
	}
	return p
}
func TestPreviewIsolation(t *testing.T) {
	h, e := buildHandler(root(t), "", "")
	if e != nil {
		t.Fatal(e)
	}
	for _, test := range []struct {
		path, method string
		status       int
	}{{"/", "GET", 200}, {"/__uat/health", "GET", 200}, {"/api/users", "GET", 404}, {"/api/login", "POST", 404}} {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(test.method, test.path, nil))
		if w.Code != test.status {
			t.Fatalf("%s %s expected %d got %d", test.method, test.path, test.status, w.Code)
		}
	}
}
func TestBackendRequiresIsolationConsent(t *testing.T) {
	r := root(t)
	for _, u := range []string{"http://127.0.0.1:8870", "http://localhost:8870", "http://testserver:8970", "http://testserver:8870"} {
		if _, e := buildHandler(r, u, ""); e == nil {
			t.Fatal("accepted without consent: " + u)
		}
	}
}
func TestSameMachineLoopbackDenied(t *testing.T) {
	if _, e := buildHandler(root(t), "http://127.0.0.1:8870", "I_HAVE_A_SEPARATE_TEST_CORE_AND_DATA"); e == nil {
		t.Fatal("accepted production loopback")
	}
}
func TestExternalTestBackend(t *testing.T) {
	h, err := buildHandler(root(t), "http://uat-test-machine.example:8870", "I_HAVE_A_SEPARATE_TEST_CORE_AND_DATA")
	if err != nil {
		t.Fatal(err)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != 200 || !strings.Contains(w.Body.String(), "UI_OVERRIDE") {
		t.Fatalf("got %d %s", w.Code, w.Body.String())
	}
}
