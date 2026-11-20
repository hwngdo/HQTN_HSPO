package accounts

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureStore(t *testing.T) *Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "users.json")
	users := `[ {"username":"admin","displayName":"Quản trị","role":"admin","active":true,"password":"secret"}, {"username":"boss","display_name":"Quản trị 2","role":"admin","active":true,"password":"other"}, {"username":"staff","displayName":"Nhân viên","role":"staff","active":true,"password":"pass"} ]`
	if e := os.WriteFile(path, []byte(users), 0600); e != nil {
		t.Fatal(e)
	}
	s := NewStore(path)
	if _, e := s.Snapshot(); e != nil {
		t.Fatal(e)
	}
	return s
}
func TestSessionChecksLiveAccountState(t *testing.T) {
	s := fixtureStore(t)
	fakeValidatedSession := func(r *http.Request) (SessionIdentity, error) {
		// TEST ONLY; production must supply a real authenticated Core session.
		return SessionIdentity{Username: r.Header.Get("X-Test-Session")}, nil
	}
	api := API{Store: s, IsAdmin: s.AdminFromSession(fakeValidatedSession)}
	query := func(user string, path string, method string, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, "/"+path, strings.NewReader(body))
		req.Header.Set("X-Test-Session", user)
		if method == "PATCH" {
			req.Header.Set("Content-Type", "application/json")
		}
		r := httptest.NewRecorder()
		api.ServeHTTP(r, req)
		return r
	}
	if r := query("staff", "", "GET", ""); r.Code != 403 {
		t.Fatalf("staff can admin GET: %d", r.Code)
	}
	if r := query("admin", "", "GET", ""); r.Code != 200 || strings.Contains(r.Body.String(), "secret") {
		t.Fatalf("admin GET insecure: %d %s", r.Code, r.Body.String())
	}
	if r := query("admin", "staff/active", "PATCH", `{"value":false}`); r.Code != 200 {
		t.Fatalf("lock failed %d %s", r.Code, r.Body.String())
	}
	if _, err := s.ValidateLoginUser("staff"); err != ErrInactive {
		t.Fatalf("locked account can login: %v", err)
	}
	if r := query("admin", "staff/active", "PATCH", `{"value":true}`); r.Code != 200 {
		t.Fatalf("unlock %d", r.Code)
	}
	if _, err := s.ValidateLoginUser("staff"); err != nil {
		t.Fatal("unlocked account denied")
	}
	if r := query("admin", "boss/role", "PATCH", `{"value":"staff"}`); r.Code != 200 {
		t.Fatalf("role change: %d %s", r.Code, r.Body.String())
	}
	if r := query("boss", "staff/active", "PATCH", `{"value":false}`); r.Code != 403 {
		t.Fatalf("demoted admin retains privilege: %d", r.Code)
	}
	if r := query("admin", "admin/active", "PATCH", `{"value":false}`); r.Code != 400 {
		t.Fatalf("last admin lock permitted %d", r.Code)
	}
	if r := query("admin", "boss/role", "PATCH", `{"value":"admin"}`); r.Code != 200 {
		t.Fatalf("restore admin %d", r.Code)
	}
	if r := query("admin", "admin/active", "PATCH", `{"value":false}`); r.Code != 200 {
		t.Fatalf("lock admin (two admins) failed %d", r.Code)
	}
	if r := query("admin", "staff/name", "PATCH", `{"value":"Tên mới"}`); r.Code != 403 {
		t.Fatalf("locked admin retained session: %d", r.Code)
	}
	if r := query("boss", "staff/name", "PATCH", `{"value":"Tên mới"}`); r.Code != 200 {
		t.Fatalf("current admin failed: %d", r.Code)
	}
	b, err := os.ReadFile(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	var users []map[string]interface{}
	if err = json.Unmarshal(b, &users); err != nil {
		t.Fatal(err)
	}
	for _, x := range users {
		if x["username"] == "staff" {
			if x["password"] != "pass" || x["role"] != "staff" || x["active"] != true || x["display_name"] != "Tên mới" || x["displayName"] != "Tên mới" {
				t.Fatalf("unexpected side effects: %#v", x)
			}
		}
	}
}

func TestMalformedAndUntrustedRequestsFailClosed(t *testing.T) {
	s := fixtureStore(t)
	api := API{Store: s, IsAdmin: func(_ *http.Request) bool { return true }}
	tests := []struct {
		path, body, content, fetch string
		code                       int
	}{
		{"staff/active", `{"value":null}`, "application/json", "same-origin", 400},
		{"staff/active", `{"value":"false"}`, "application/json", "same-origin", 400},
		{"staff/active", `{"value":false}`, "text/plain", "same-origin", 415},
		{"staff/active", `{"value":false}`, "application/json", "cross-site", 403},
		{"staff/nope", `{"value":"any"}`, "application/json", "same-origin", 404},
	}
	for _, tc := range tests {
		q := httptest.NewRequest("PATCH", "/"+tc.path, strings.NewReader(tc.body))
		q.Header.Set("Content-Type", tc.content)
		q.Header.Set("Sec-Fetch-Site", tc.fetch)
		rr := httptest.NewRecorder()
		api.ServeHTTP(rr, q)
		if rr.Code != tc.code {
			t.Fatalf("%s expected %d got %d", tc.path, tc.code, rr.Code)
		}
	}
}
