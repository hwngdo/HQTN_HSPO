package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHTTPRequiresRealAdminAndNoPasswordLeak(t *testing.T) {
	s, _ := newTestStore(t)
	handler := API{Store: s, IsAdmin: func(r *http.Request) bool { return r.Header.Get("X-QA-Admin") == "yes" }}
	r := httptest.NewRequest("GET", "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatal("no auth")
	}
	r.Header.Set("X-QA-Admin", "yes")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatal(rec.Code)
	}
	if strings.Contains(rec.Body.String(), "password") || strings.Contains(rec.Body.String(), "hash-") {
		t.Fatal("credential exposed")
	}
}
func TestHTTPPatchIsolatedFields(t *testing.T) {
	s, p := newTestStore(t)
	handler := API{Store: s, IsAdmin: func(r *http.Request) bool { return true }}
	r := httptest.NewRequest("PATCH", "/huelt/active", strings.NewReader(`{"value":false}`))
	r.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 200 {
		t.Fatal(rec.Code, rec.Body.String())
	}
	var v struct {
		User PublicUser `json:"user"`
	}
	if json.Unmarshal(rec.Body.Bytes(), &v) != nil || v.User.Active {
		t.Fatal("active not updated")
	}
	u := parsed(t, p)
	if getString(u[2], "password") != "hash-huelt" || getString(u[2], "display_name") != "Le Thi Hue" {
		t.Fatal("changed unrelated data")
	}
}
func TestHTTPRejectsForeignOriginAndUnknownAction(t *testing.T) {
	s, _ := newTestStore(t)
	handler := API{Store: s, IsAdmin: func(r *http.Request) bool { return true }}
	r := httptest.NewRequest("PATCH", "/huelt/name", strings.NewReader(`{"value":"X"}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 403 {
		t.Fatal(rec.Code)
	}
	r = httptest.NewRequest("PATCH", "/huelt/delete", strings.NewReader(`{"value":"X"}`))
	r.Header.Set("Content-Type", "application/json")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, r)
	if rec.Code != 404 {
		t.Fatal(rec.Code)
	}
}
