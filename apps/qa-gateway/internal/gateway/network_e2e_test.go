package gateway

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"hqtn.local/hspo-sprint4-qa/internal/accounts"
	"hqtn.local/hspo-sprint4-qa/internal/diagnostics"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Network-level QA E2E: two ACTUAL local HTTP servers (a fake Core and the QA
// Gateway), cookie sessions, on-disk persistence, and an optional real MSDS.
// This is NOT an E2E test of the deployed Windows Core executable.
func TestNetworkEndToEndAccountsAndMSDS(t *testing.T) {
	dir := t.TempDir()
	userPath := filepath.Join(dir, "users.qa.json")
	fixture := `[ {"username":"admin","displayName":"Test Admin","active":true,"role":"admin","password":"untouched-hash-A"}, {"username":"otheradmin","display_name":"Backup Admin","active":true,"role":"admin","password":"untouched-hash-B"}, {"username":"staff","displayName":"Staff User","active":true,"role":"staff","password":"untouched-hash-C"} ]`
	if err := os.WriteFile(userPath, []byte(fixture), 0600); err != nil {
		t.Fatal(err)
	}
	core := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/login":
			var v struct {
				Username string `json:"username"`
			}
			_ = json.NewDecoder(r.Body).Decode(&v)
			if v.Username != "admin" && v.Username != "otheradmin" && v.Username != "staff" {
				http.Error(w, "invalid", 401)
				return
			}
			http.SetCookie(w, &http.Cookie{Name: "core_session", Value: v.Username, Path: "/", HttpOnly: true})
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": map[string]any{"username": v.Username, "role": "admin"}})
		case "/api/me":
			c, e := r.Cookie("core_session")
			if e != nil {
				http.Error(w, "unauthorized", 401)
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": map[string]any{"username": c.Value}})
		case "/api/chem/analyze":
			_, _ = io.WriteString(w, `{"ok":true}`)
		default:
			http.NotFound(w, r)
		}
	}))
	defer core.Close()
	diagRoot := filepath.Join(dir, "diagnostics")
	gate, e := New(core.URL, accounts.NewStore(userPath), diagnostics.NewStore(diagRoot))
	if e != nil {
		t.Fatal(e)
	}
	front := httptest.NewServer(gate)
	defer front.Close()
	newClient := func() *http.Client {
		jar, e := cookiejar.New(nil)
		if e != nil {
			t.Fatal(e)
		}
		return &http.Client{Jar: jar}
	}
	admin := newClient()
	staff := newClient()
	call := func(c *http.Client, method, path, contentType string, body []byte, code int) []byte {
		t.Helper()
		req, e := http.NewRequest(method, front.URL+path, bytes.NewReader(body))
		if e != nil {
			t.Fatal(e)
		}
		if contentType != "" {
			req.Header.Set("Content-Type", contentType)
		}
		res, e := c.Do(req)
		if e != nil {
			t.Fatal(e)
		}
		defer res.Body.Close()
		b, _ := io.ReadAll(res.Body)
		if res.StatusCode != code {
			t.Fatalf("%s %s => %d, wanted %d: %s", method, path, res.StatusCode, code, string(b))
		}
		return b
	}
	login := func(c *http.Client, username string, code int) {
		call(c, "POST", "/api/login", "application/json", []byte(fmt.Sprintf(`{"username":%q,"password":"test"}`, username)), code)
	}
	patch := func(user, action, value string) {
		call(admin, "PATCH", "/api/admin/account-v2/"+user+"/"+action, "application/json", []byte(`{"value":`+value+`}`), 200)
	}
	login(admin, "admin", 200)
	login(staff, "staff", 200)
	call(staff, "GET", "/api/admin/account-v2", "", nil, 403)
	patch("staff", "active", "false")
	call(staff, "GET", "/api/me", "", nil, 401)
	call(staff, "GET", "/api/chem/analyze", "", nil, 401)
	login(newClient(), "staff", 401)
	patch("staff", "active", "true")
	login(newClient(), "staff", 200)
	patch("staff", "name", `"Nhân viên kiểm thử"`)
	patch("staff", "role", `"user"`)
	raw, e := os.ReadFile(userPath)
	if e != nil {
		t.Fatal(e)
	}
	if !bytes.Contains(raw, []byte(`"password": "untouched-hash-C"`)) {
		t.Fatal("password changed in on-disk JSON")
	}
	// Mimic process restart: a new account store and gateway must reread the disk.
	renewed, e := New(core.URL, accounts.NewStore(userPath), diagnostics.NewStore(diagRoot))
	if e != nil {
		t.Fatal(e)
	}
	gate.Accounts = renewed.Accounts
	var people struct {
		Users []accounts.PublicUser `json:"users"`
	}
	if e = json.Unmarshal(call(admin, "GET", "/api/admin/account-v2", "", nil, 200), &people); e != nil {
		t.Fatal(e)
	}
	found := false
	for _, u := range people.Users {
		if u.Username == "staff" {
			found = true
			if u.DisplayName != "Nhân viên kiểm thử" || u.Role != "user" || !u.Active {
				t.Fatalf("persisted user mismatch: %#v", u)
			}
		}
	}
	if !found {
		t.Fatal("missing user")
	}
	sample := []byte("%PDF-1.4\nQA synthetic PDF\n")
	if p := os.Getenv("HSPO_TEST_MSDS"); p != "" {
		sample, e = os.ReadFile(p)
		if e != nil {
			t.Fatal(e)
		}
	}
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for k, v := range map[string]string{"stage": "OCR_FAILED", "message": "OCR could not read PDF", "log": "Cookie: ses=123\nAuthorization: Bearer abcde"} {
		if e := writer.WriteField(k, v); e != nil {
			t.Fatal(e)
		}
	}
	f, e := writer.CreateFormFile("pdf", "MSDS-5.pdf")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = f.Write(sample); e != nil {
		t.Fatal(e)
	}
	_ = writer.Close()
	b := call(staff, "POST", "/api/diagnostics/msds/", writer.FormDataContentType(), buf.Bytes(), 201)
	var saved struct {
		ErrorID string `json:"error_id"`
		Saved   bool   `json:"pdf_saved"`
	}
	if e = json.Unmarshal(b, &saved); e != nil || !saved.Saved || saved.ErrorID == "" {
		t.Fatalf("bad diag result: %v %s", e, b)
	}
	call(staff, "GET", "/api/diagnostics/msds/", "", nil, 403)
	listing := call(admin, "GET", "/api/diagnostics/msds/", "", nil, 200)
	if !bytes.Contains(listing, []byte(saved.ErrorID)) || bytes.Contains(listing, sample) {
		t.Fatal("diagnostic listing missing metadata or leaked raw PDF")
	}
	call(staff, "GET", "/api/diagnostics/msds/"+saved.ErrorID+"/zip", "", nil, 403)
	archive := call(admin, "GET", "/api/diagnostics/msds/"+saved.ErrorID+"/zip", "", nil, 200)
	z, e := zip.NewReader(bytes.NewReader(archive), int64(len(archive)))
	if e != nil {
		t.Fatal(e)
	}
	actual := []byte(nil)
	redacted := false
	for _, f := range z.File {
		rc, e := f.Open()
		if e != nil {
			t.Fatal(e)
		}
		contents, _ := io.ReadAll(rc)
		_ = rc.Close()
		if f.Name == "source.pdf" {
			actual = contents
		}
		if f.Name == "ocr.log" {
			redacted = !strings.Contains(string(contents), "Bearer abcde")
		}
	}
	if !bytes.Equal(actual, sample) || !redacted {
		t.Fatal("MSDS ZIP integrity or log redaction mismatch")
	}
}
