package dataupdate

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/rand"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTariffDatasetPayload(t *testing.T) {
	cases := []struct {
		name string
		body string
		ok   bool
	}{
		{"legacy shape", `{"records":[{"hs":"01013010","rates":{"NKUD":["0","source","2026-01-01"]}},{"hs":"0101","rates":{}}]}`, true},
		{"missing records", `{"version":"2026"}`, false},
		{"empty records", `{"records":[]}`, false},
		{"null record", `{"records":[null]}`, false},
		{"missing HS code", `{"records":[{"rates":{}}]}`, false},
		{"missing rates", `{"records":[{"hs":"01013010"}]}`, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var compressed bytes.Buffer
			writer := gzip.NewWriter(&compressed)
			if _, err := writer.Write([]byte(tc.body)); err != nil {
				t.Fatal(err)
			}
			if err := writer.Close(); err != nil {
				t.Fatal(err)
			}
			if err := checkPayload("static/tariff_2026.json.gz", compressed.Bytes()); (err == nil) != tc.ok {
				t.Fatalf("checkPayload error = %v, want success = %t", err, tc.ok)
			}
		})
	}
}

func TestLegalReviewDate(t *testing.T) {
	manifest := Manifest{LegalReviewed: true, LegalReviewEvidence: "Fixture review evidence", LegalReviewDate: "2026-02-30"}
	if manifest.canApply() {
		t.Fatal("accepted a nonexistent calendar date")
	}
	manifest.LegalReviewDate = "2026-02-28"
	if !manifest.canApply() {
		t.Fatal("rejected legacy review metadata with a valid date")
	}
}

func TestImportedLegacyTariffDataset(t *testing.T) {
	path := os.Getenv("HSPO_QA_LEGACY_TARIFF")
	if path == "" {
		t.Skip("no imported v1.9.8 tariff supplied")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkPayload("static/tariff_2026.json.gz", data); err != nil {
		t.Fatal(err)
	}
}

func makeG(t *testing.T) (*Gateway, func()) {
	t.Helper()
	root := t.TempDir()
	if e := os.MkdirAll(filepath.Join(root, "static"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.MkdirAll(filepath.Join(root, "policy_data"), 0755); e != nil {
		t.Fatal(e)
	}
	os.WriteFile(filepath.Join(root, "HQTN_HSPO_Core.exe"), []byte("fixture"), 0644)
	os.WriteFile(filepath.Join(root, "static", "index.html"), []byte("hspo fixture"), 0644)
	os.WriteFile(filepath.Join(root, "VERSION.txt"), []byte("1.9.8\n"), 0644)
	os.WriteFile(filepath.Join(root, "policy_data", "chemical_rules.json"), []byte(`{"old":1}`), 0644)
	auth := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/me" {
			http.NotFound(w, r)
			return
		}
		switch r.Header.Get("Cookie") {
		case "user=admin":
			w.Write([]byte(`{"ok":true,"user":{"username":"tester","role":"admin"}}`))
		case "user=staff":
			w.Write([]byte(`{"ok":true,"user":{"username":"normal","role":"user"}}`))
		default:
			http.Error(w, "unauthorized", 401)
		}
	}))
	u, _ := urlParse(auth.URL)
	key := make([]byte, 32)
	rand.Read(key)
	return &Gateway{root: root, core: u, key: key, started: time.Now(), stages: map[string]Stage{}, client: &http.Client{Timeout: 5 * time.Second}}, auth.Close
}
func urlParse(s string) (*url.URL, error) { return url.Parse(s) }
func packageBytes(t *testing.T, valid bool, reviewed bool) []byte {
	t.Helper()
	var buf bytes.Buffer
	z := zip.NewWriter(&buf)
	raw := []byte(`{"updated":2}`)
	manifest := Manifest{Format: "HQTN_HSPO_DATA_V1", PackID: "TEST-001", DataVersion: "2026.11.02.01", RuleVersion: "2026.11.02.01", MinApp: "1.9.8", Description: "Testing, not for production", LegalReviewed: reviewed, LegalReviewDate: "2026-11-02", LegalReviewEvidence: "Evidence signed by legal reviewer - fixture", Files: []FileSpec{{Path: "policy_data/chemical_rules.json", SHA256: sha(raw), Size: int64(len(raw))}}}
	if !valid {
		manifest.Files[0].SHA256 = strings.Repeat("0", 64)
	}
	b, _ := json.Marshal(manifest)
	w, _ := z.Create("manifest.json")
	w.Write(b)
	w, _ = z.Create("policy_data/chemical_rules.json")
	w.Write(raw)
	z.Close()
	return buf.Bytes()
}
func request(t *testing.T, g *Gateway, method, path, cookie, csrf, typ string, body []byte) (int, map[string]any) {
	t.Helper()
	r := httptest.NewRequest(method, path, bytes.NewReader(body))
	r.Header.Set("Cookie", cookie)
	if csrf != "" {
		r.Header.Set("X-HSPO-CSRF", csrf)
	}
	if typ != "" {
		r.Header.Set("Content-Type", typ)
	}
	w := httptest.NewRecorder()
	g.serve(w, r)
	result := map[string]any{}
	json.Unmarshal(w.Body.Bytes(), &result)
	return w.Code, result
}
func TestRolesAndCSRF(t *testing.T) {
	g, cleanup := makeG(t)
	defer cleanup()
	code, _ := request(t, g, "GET", routePrefix+"status", "user=staff", "", "", nil)
	if code != 403 {
		t.Fatalf("user should be forbidden got %d", code)
	}
	code, _ = request(t, g, "GET", routePrefix+"status", "user=admin", "", "", nil)
	if code != 200 {
		t.Fatalf("admin status %d", code)
	}
	code, _ = request(t, g, "POST", routePrefix+"apply", "user=admin", "", "application/json", []byte(`{}`))
	if code != 403 {
		t.Fatalf("no csrf should be forbidden got %d", code)
	}
}
func TestPreviewApplyRollback(t *testing.T) {
	g, cleanup := makeG(t)
	defer cleanup()
	_, st := request(t, g, "GET", routePrefix+"status", "user=admin", "", "", nil)
	token := st["csrf"].(string)
	body := &bytes.Buffer{}
	mw := multipart.NewWriter(body)
	part, _ := mw.CreateFormFile("file", "dataset.zip")
	part.Write(packageBytes(t, true, true))
	mw.Close()
	code, prev := request(t, g, "POST", routePrefix+"preview", "user=admin", token, mw.FormDataContentType(), body.Bytes())
	if code != 200 {
		t.Fatalf("preview: %d %v", code, prev)
	}
	id := prev["id"].(string)
	payload, _ := json.Marshal(map[string]any{"id": id, "confirm": "AP_DUNG"})
	code, applied := request(t, g, "POST", routePrefix+"apply", "user=admin", token, "application/json", payload)
	if code != 200 {
		t.Fatalf("apply %d %v", code, applied)
	}
	b, _ := os.ReadFile(filepath.Join(g.root, "policy_data", "chemical_rules.json"))
	if !bytes.Contains(b, []byte("updated")) {
		t.Fatalf("not applied %s", b)
	}
	_, list := request(t, g, "GET", routePrefix+"backups", "user=admin", "", "", nil)
	items := list["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("expected 1 backup got %v", items)
	}
	name := items[0].(map[string]any)["name"].(string)
	rb, _ := json.Marshal(map[string]any{"file": name, "confirm": "KHOI_PHUC"})
	code, result := request(t, g, "POST", routePrefix+"rollback", "user=admin", token, "application/json", rb)
	if code != 200 {
		t.Fatalf("rollback %d %v", code, result)
	}
	b, _ = os.ReadFile(filepath.Join(g.root, "policy_data", "chemical_rules.json"))
	if string(b) != `{"old":1}` {
		t.Fatalf("not restored: %s", b)
	}
}
func TestBadChecksumAndUnreviewed(t *testing.T) {
	g, cleanup := makeG(t)
	defer cleanup()
	_, st := request(t, g, "GET", routePrefix+"status", "user=admin", "", "", nil)
	token := st["csrf"].(string)
	for _, tc := range []struct {
		valid, reviewed bool
		want            int
	}{{false, true, 400}, {true, false, 200}} {
		var body bytes.Buffer
		mw := multipart.NewWriter(&body)
		part, _ := mw.CreateFormFile("file", "dataset.zip")
		part.Write(packageBytes(t, tc.valid, tc.reviewed))
		mw.Close()
		code, output := request(t, g, "POST", routePrefix+"preview", "user=admin", token, mw.FormDataContentType(), body.Bytes())
		if code != tc.want {
			t.Fatalf("code %d want %d output %v", code, tc.want, output)
		}
		if !tc.reviewed && code == 200 && output["can_apply"] != false {
			t.Fatalf("unreviewed may not be applied")
		}
	}
}
func TestPathTraversal(t *testing.T) {
	if allowed("../secrets.txt") || allowed("static/index.html") || allowed("policy_data/evil.exe") || !allowed("static/documents/pdf/legal.pdf") {
		t.Fatal("allow list failed")
	}
}
