package gateway

import (
	"bytes"
	"encoding/json"
	"fmt"
	"hqtn.local/hspo-sprint4-qa/internal/accounts"
	"hqtn.local/hspo-sprint4-qa/internal/diagnostics"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func setup(t *testing.T) (*Gateway, *httptest.Server, string) {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "users.json")
	b := `[{"username":"root","displayName":"Admin Root","role":"admin","active":true,"password":"HASH"},{"username":"backup","display_name":"Backup Admin","role":"admin","active":true,"password":"HASH2"},{"username":"staff","display_name":"Staff","role":"staff","active":true,"password":"HASH3"}]`
	if e := os.WriteFile(p, []byte(b), 0600); e != nil {
		t.Fatal(e)
	}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/login" {
			var v struct {
				Username string `json:"username"`
			}
			_ = json.NewDecoder(r.Body).Decode(&v)
			w.Header().Set("Set-Cookie", "core_session="+v.Username+"; Path=/; HttpOnly")
			_, _ = fmt.Fprintf(w, `{"ok":true,"user":{"username":%q,"role":"admin"}}`, v.Username)
			return
		}
		if r.URL.Path == "/api/me" {
			c, e := r.Cookie("core_session")
			if e != nil || c.Value == "invalid" {
				http.Error(w, "unauthorized", 401)
				return
			}
			_, _ = fmt.Fprintf(w, `{"ok":true,"user":{"username":%q,"role":"admin"}}`, c.Value)
			return
		}
		if r.URL.Path == "/api/chem/analyze" {
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		if r.URL.Path == "/api/admin/backup" {
			_, _ = io.WriteString(w, `{"ok":true}`)
			return
		}
		if r.URL.Path == "/" {
			_, _ = io.WriteString(w, "REAL CORE FRONTEND")
			return
		}
		http.NotFound(w, r)
	}))
	g, e := New(upstream.URL, accounts.NewStore(p), diagnostics.NewStore(filepath.Join(dir, "diagnostics")))
	if e != nil {
		t.Fatal(e)
	}
	return g, upstream, p
}
func req(method, url, user string, body []byte) *http.Request {
	r := httptest.NewRequest(method, url, bytes.NewReader(body))
	if user != "" {
		r.AddCookie(&http.Cookie{Name: "core_session", Value: user})
	}
	return r
}
func TestNameRoleActiveAndPersistentDisk(t *testing.T) {
	g, u, p := setup(t)
	defer u.Close()
	for _, tc := range []struct{ path, body string }{
		{"/api/admin/account-v2/staff/name", `{"value":"Tên mới"}`},
		{"/api/admin/account-v2/staff/role", `{"value":"admin"}`},
		{"/api/admin/account-v2/staff/active", `{"value":false}`},
	} {
		r := req("PATCH", tc.path, "root", []byte(tc.body))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("patch %s: %d %s", tc.path, w.Code, w.Body.String())
		}
	}
	// Even if the original Core still recognizes an old cookie, inactive user is refused.
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req("GET", "/api/chem/analyze", "staff", nil))
	if w.Code != 401 {
		t.Fatalf("locked staff API access: %d", w.Code)
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, req("GET", "/api/admin/account-v2", "root", nil))
	if w.Code != 200 {
		t.Fatalf("list status: %d", w.Code)
	}
	b, e := os.ReadFile(p)
	if e != nil {
		t.Fatal(e)
	}
	var users []map[string]any
	if e = json.Unmarshal(b, &users); e != nil {
		t.Fatal(e)
	}
	for _, v := range users {
		if v["username"] == "staff" {
			if v["display_name"] != "Tên mới" || v["displayName"] != "Tên mới" || v["active"] != false || v["role"] != "admin" || v["password"] != "HASH3" {
				t.Fatalf("persisted mutation mismatch: %+v", v)
			}
		}
	}
}
func TestDeniesForgedHeaderRoleAndRevokedAdmin(t *testing.T) {
	g, u, _ := setup(t)
	defer u.Close()
	for _, who := range []string{"staff", "", "invalid"} {
		r := req("GET", "/api/admin/account-v2", who, nil)
		r.Header.Set("X-QA-Session", "root")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code == 200 {
			t.Fatalf("forged admin accepted: %q", who)
		}
	}
	r := req("PATCH", "/api/admin/account-v2/root/role", "backup", []byte(`{"value":"staff"}`))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 200 {
		t.Fatalf("demote: %d %s", w.Code, w.Body.String())
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, req("GET", "/api/admin/account-v2", "root", nil))
	if w.Code != 403 {
		t.Fatalf("revoked admin accepted: %d", w.Code)
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, req("GET", "/api/admin/backup", "root", nil))
	if w.Code != 403 {
		t.Fatalf("old admin API bypass: %d", w.Code)
	}
}
func TestDiagnosticsWithRealPDFAndAdminDownload(t *testing.T) {
	g, u, _ := setup(t)
	defer u.Close()
	body := &bytes.Buffer{}
	bw := multipart.NewWriter(body)
	bw.WriteField("stage", "OCR_FAILED")
	bw.WriteField("message", "offline error")
	fw, e := bw.CreateFormFile("pdf", "sample.pdf")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = fw.Write([]byte("%PDF-1.4\nhello"))
	_ = bw.Close()
	r := req("POST", "/api/diagnostics/msds/", "staff", body.Bytes())
	r.Header.Set("Content-Type", bw.FormDataContentType())
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 201 {
		t.Fatalf("capture: %d %s", w.Code, w.Body.String())
	}
	var event struct {
		ErrorID string `json:"error_id"`
	}
	if e = json.Unmarshal(w.Body.Bytes(), &event); e != nil || event.ErrorID == "" {
		t.Fatal(w.Body.String())
	}
	for _, who := range []string{"staff", "root"} {
		w = httptest.NewRecorder()
		g.ServeHTTP(w, req("GET", "/api/diagnostics/msds/"+event.ErrorID+"/zip", who, nil))
		if who == "staff" && w.Code != 403 {
			t.Fatalf("staff export got %d", w.Code)
		}
		if who == "root" && (w.Code != 200 || w.Body.Len() < 70) {
			t.Fatalf("admin zip %d bytes %d", w.Code, w.Body.Len())
		}
	}
}
func TestLoopbackAndNoRecursiveProxy(t *testing.T) {
	if e := LoopbackListen("0.0.0.0:8788", "http://127.0.0.1:8765"); e == nil {
		t.Fatal("accepted LAN QA bind")
	}
	if e := LoopbackListen("127.0.0.1:8788", "http://127.0.0.1:8788"); e == nil {
		t.Fatal("accepted loop")
	}
	if _, e := New("http://example.com:8765", nil, nil); e == nil {
		t.Fatal("accepted external upstream")
	}
}
func TestCrossSiteWriteBlocked(t *testing.T) {
	g, u, _ := setup(t)
	defer u.Close()
	r := req("PATCH", "/api/admin/account-v2/staff/active", "root", []byte(`{"value":false}`))
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("Sec-Fetch-Site", "cross-site")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross-site %d", w.Code)
	}
}
func TestCoreFrontendProxy(t *testing.T) {
	g, u, _ := setup(t)
	defer u.Close()
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req("GET", "/", "", nil))
	if !strings.Contains(w.Body.String(), "REAL CORE FRONTEND") {
		t.Fatalf("frontend proxy failed %s", w.Body.String())
	}
}

func TestLoginAndMeCanonicalAndRevocation(t *testing.T) {
	g, u, _ := setup(t)
	defer u.Close()
	for _, who := range []string{"staff", "root"} {
		r := req("POST", "/api/login", "", []byte(fmt.Sprintf(`{"username":%q,"password":"anything"}`, who)))
		r.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		g.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("initial login %s => %d", who, w.Code)
		}
		var data map[string]any
		_ = json.Unmarshal(w.Body.Bytes(), &data)
		user := data["user"].(map[string]any)
		expected := "staff"
		if who == "root" {
			expected = "admin"
		}
		if user["role"] != expected {
			t.Fatalf("stale upstream role accepted: %+v", user)
		}
	}
	// Active session gets invalidated immediately after an admin locks account.
	p := req("PATCH", "/api/admin/account-v2/staff/active", "root", []byte(`{"value":false}`))
	p.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, p)
	if w.Code != 200 {
		t.Fatalf("failed to lock: %d", w.Code)
	}
	r := req("POST", "/api/login", "", []byte(`{"username":"staff","password":"pass"}`))
	r.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 401 || len(w.Result().Cookies()) != 0 {
		t.Fatalf("locked login accepted or upstream cookie escaped: %d %+v", w.Code, w.Result().Cookies())
	}
	w = httptest.NewRecorder()
	g.ServeHTTP(w, req("GET", "/api/me", "staff", nil))
	if w.Code != 401 {
		t.Fatalf("locked existing session: %d", w.Code)
	}
}

func TestSprint4AdminDiagnosticsListingAndLegacyMutationBlock(t *testing.T) {
	g, up, _ := setup(t)
	defer up.Close()
	buf := &bytes.Buffer{}
	form := multipart.NewWriter(buf)
	_ = form.WriteField("stage", "OCR_FAILED")
	_ = form.WriteField("message", "failed to parse")
	_ = form.WriteField("filename", "test.pdf")
	_ = form.Close()
	req1 := req("POST", "/api/diagnostics/msds/", "staff", buf.Bytes())
	req1.Header.Set("Content-Type", form.FormDataContentType())
	rec := httptest.NewRecorder()
	g.ServeHTTP(rec, req1)
	if rec.Code != 201 {
		t.Fatalf("capture=%d %s", rec.Code, rec.Body.String())
	}
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, req("GET", "/api/diagnostics/msds/", "staff", nil))
	if rec.Code != 403 {
		t.Fatalf("nonadmin list %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	g.ServeHTTP(rec, req("GET", "/api/diagnostics/msds/", "root", nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "OCR_FAILED") || strings.Contains(rec.Body.String(), "source.pdf") {
		t.Fatalf("admin list %d %s", rec.Code, rec.Body.String())
	}
	for _, url := range []string{"/api/users", "/api/admin/users", "/api/admin/users/staff"} {
		rec = httptest.NewRecorder()
		g.ServeHTTP(rec, req("POST", url, "root", []byte(`{"active":false}`)))
		if rec.Code != 409 {
			t.Fatalf("legacy mutator %s %d", url, rec.Code)
		}
	}
}

func TestSprint4RejectsMaliciousOriginWithoutFetchMetadata(t *testing.T) {
	g, u, _ := setup(t)
	defer u.Close()
	r := req("PATCH", "/api/admin/account-v2/staff/active", "root", []byte(`{"value":false}`))
	r.Header.Set("Origin", "http://evil.example")
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	g.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatalf("cross origin accepted: %d", w.Code)
	}
}
