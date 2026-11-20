package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMSDSHTTPAuthenticatedCaptureAndAdminExport(t *testing.T) {
	s := NewStore(t.TempDir())
	a := API{Store: s, AppVersion: "1.9.7-qa", OCRVersion: "1.9.6.9-lan", Resolve: func(r *http.Request) (Caller, error) {
		// TEST ONLY: real Core must resolve session from signed server cookie.
		role := r.Header.Get("X-QA-Session")
		if role == "" {
			return Caller{}, nil
		}
		return Caller{Username: role, Admin: role == "admin"}, nil
	}}
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	for k, v := range map[string]string{"stage": "ocr_start", "message": "Failed to fetch", "log": "Authorization: Bearer abcdefg\nrequest failed", "username": "FAKE-OTHER-USER"} {
		if e := m.WriteField(k, v); e != nil {
			t.Fatal(e)
		}
	}
	var actual []byte
	path := os.Getenv("HSPO_TEST_MSDS")
	if path != "" {
		var e error
		actual, e = os.ReadFile(path)
		if e != nil {
			t.Fatal(e)
		}
	} else {
		actual = []byte("%PDF-1.4\nqa test only\n")
	}
	p, e := m.CreateFormFile("pdf", "MSDS-5.pdf")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = p.Write(actual); e != nil {
		t.Fatal(e)
	}
	_ = m.Close()
	req := httptest.NewRequest("POST", "/", &b)
	req.Header.Set("Content-Type", m.FormDataContentType())
	req.Header.Set("X-QA-Session", "staff")
	rr := httptest.NewRecorder()
	a.ServeHTTP(rr, req)
	if rr.Code != 201 {
		t.Fatalf("capture HTTP %d: %s", rr.Code, rr.Body.String())
	}
	var out struct {
		OK    bool   `json:"ok"`
		ID    string `json:"error_id"`
		Saved bool   `json:"pdf_saved"`
	}
	if e = json.Unmarshal(rr.Body.Bytes(), &out); e != nil || !out.OK || !out.Saved {
		t.Fatalf("bad response %#v, %v", out, e)
	}
	dir := filepath.Join(s.Root, out.ID[:4]+"-"+out.ID[4:6]+"-"+out.ID[6:8], out.ID)
	raw, e := os.ReadFile(filepath.Join(dir, "error.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "FAKE-OTHER-USER") || !strings.Contains(string(raw), `"username": "staff"`) {
		t.Fatal("client can forge identity")
	}
	log, e := os.ReadFile(filepath.Join(dir, "ocr.log"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(log), "abcdefg") {
		t.Fatal("secret leaked to log")
	}
	saved, e := os.ReadFile(filepath.Join(dir, "source.pdf"))
	if e != nil || !bytes.Equal(actual, saved) {
		t.Fatal("PDF round-trip corruption", e)
	}
	request := func(role string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", "/"+out.ID+"/zip", nil)
		r.Header.Set("X-QA-Session", role)
		response := httptest.NewRecorder()
		a.ServeHTTP(response, r)
		return response
	}
	if r := request("staff"); r.Code != 403 {
		t.Fatalf("staff export allowed %d", r.Code)
	}
	if r := request(""); r.Code != 401 {
		t.Fatalf("unauthenticated export allowed %d", r.Code)
	}
	result := request("admin")
	if result.Code != 200 {
		t.Fatalf("admin export failed %d", result.Code)
	}
	zr, e := zip.NewReader(bytes.NewReader(result.Body.Bytes()), int64(result.Body.Len()))
	if e != nil {
		t.Fatal(e)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
		if f.Name == "source.pdf" {
			rc, e := f.Open()
			if e != nil {
				t.Fatal(e)
			}
			payload, e := io.ReadAll(rc)
			rc.Close()
			if e != nil || !bytes.Equal(payload, actual) {
				t.Fatal("export damaged pdf")
			}
		}
	}
	for _, name := range []string{"error.json", "ocr.log", "source.pdf"} {
		if !names[name] {
			t.Fatalf("missing archive %s", name)
		}
	}
}

func TestMSDSHTTPRejectsForgedCrossSiteAndNonPDF(t *testing.T) {
	s := NewStore(t.TempDir())
	a := API{Store: s, Resolve: func(_ *http.Request) (Caller, error) { return Caller{Username: "u", Admin: true}, nil }}
	var b bytes.Buffer
	m := multipart.NewWriter(&b)
	_ = m.WriteField("stage", "ocr")
	_ = m.WriteField("message", "failed")
	p, e := m.CreateFormFile("pdf", "evil.pdf")
	if e != nil {
		t.Fatal(e)
	}
	_, _ = p.Write([]byte("not a PDF"))
	_ = m.Close()
	q := httptest.NewRequest("POST", "/", bytes.NewReader(b.Bytes()))
	q.Header.Set("Content-Type", m.FormDataContentType())
	q.Header.Set("Sec-Fetch-Site", "cross-site")
	rr := httptest.NewRecorder()
	a.ServeHTTP(rr, q)
	if rr.Code != 403 {
		t.Fatalf("CSRF not denied %d", rr.Code)
	}
	q = httptest.NewRequest("POST", "/", bytes.NewReader(b.Bytes()))
	q.Header.Set("Content-Type", m.FormDataContentType())
	rr = httptest.NewRecorder()
	a.ServeHTTP(rr, q)
	if rr.Code != 500 {
		t.Fatalf("non-PDF accepted %d", rr.Code)
	}
}

func TestExportUnknownIDIs404NotEmptyZIP(t *testing.T) {
	a := API{Store: NewStore(t.TempDir()), Resolve: func(_ *http.Request) (Caller, error) { return Caller{Username: "admin", Admin: true}, nil }}
	req := httptest.NewRequest("GET", "/20261022T000000_111111111111111111111111/zip", nil)
	rr := httptest.NewRecorder()
	a.ServeHTTP(rr, req)
	if rr.Code != 404 {
		t.Fatalf("missing error case should 404, got %d", rr.Code)
	}
}
