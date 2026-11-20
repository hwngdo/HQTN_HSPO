package diagnostics

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCaptureFailedPDFAndSafeExport(t *testing.T) {
	s := NewStore(t.TempDir())
	pdf := []byte("%PDF-1.5\nmalformed test bytes\n")
	ev, e := s.Capture(Event{Username: "qa-user", Filename: `C:\\private\\msds-5.pdf`, Stage: "ocr", Message: "OCR failed token=abc123"}, bytes.NewReader(pdf), int64(len(pdf)), "Authorization: secret\nBearer abc123\nCookie: session-secret")
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(ev.Filename, "\\") || strings.Contains(ev.Filename, "/") {
		t.Fatal("path not sanitized")
	}
	p := filepath.Join(s.Root, time.Now().UTC().Format("2006-01-02"), ev.ID)
	raw, e := os.ReadFile(filepath.Join(p, "error.json"))
	if e != nil {
		t.Fatal(e)
	}
	if strings.Contains(string(raw), "abc123") {
		t.Fatal("secrets in metadata")
	}
	logb, _ := os.ReadFile(filepath.Join(p, "ocr.log"))
	if strings.Contains(string(logb), "abc123") || strings.Contains(string(logb), "secret") {
		t.Fatal("secrets in logs")
	}
	var out bytes.Buffer
	if e = s.Export(ev.ID, &out, false); e == nil {
		t.Fatal("staff exported sensitive PDF")
	}
	if e = s.Export(ev.ID, &out, true); e != nil {
		t.Fatal(e)
	}
	zr, e := zip.NewReader(bytes.NewReader(out.Bytes()), int64(out.Len()))
	if e != nil || len(zr.File) != 3 {
		t.Fatal("export invalid")
	}
	var back Event
	if e = json.Unmarshal(raw, &back); e != nil || back.ID != ev.ID || !back.PDFSaved {
		t.Fatal("metadata invalid")
	}
}
func TestLimitsAndNoPDFOnError(t *testing.T) {
	s := NewStore(t.TempDir())
	_, e := s.Capture(Event{Stage: "ocr", Message: "x", Filename: "m.pdf"}, bytes.NewReader([]byte("NO")), 2, "")
	if e == nil {
		t.Fatal("accepted non-PDF")
	}
	if d, _ := os.ReadDir(s.Root); len(d) > 1 {
		t.Fatal("leaked failed folder")
	}
	_, e = s.Capture(Event{Stage: "ocr", Message: "x", Filename: "m.pdf"}, bytes.NewReader([]byte("%PDF-")), MaxPDFBytes+1, "")
	if e == nil {
		t.Fatal("accepted oversized")
	}
}
func TestNoPDFForCaptureIfDisabled(t *testing.T) {
	s := NewStore(t.TempDir())
	s.SaveFailedPDF = false
	ev, e := s.Capture(Event{Stage: "parse", Message: "x", Filename: "scan.pdf"}, nil, 0, "")
	if e != nil || ev.PDFSaved {
		t.Fatal(e)
	}
}
func TestCleanupSafe(t *testing.T) {
	s := NewStore(t.TempDir())
	old := time.Now().UTC().Add(-35 * 24 * time.Hour)
	date := old.Format("2006-01-02")
	base := filepath.Join(s.Root, date)
	e := os.MkdirAll(filepath.Join(base, "20260801T000000_aaaaaaaaaaaaaaaaaaaaaaaa"), 0700)
	if e != nil {
		t.Fatal(e)
	}
	_ = os.MkdirAll(filepath.Join(base, "do-not-delete"), 0700)
	n, e := s.Cleanup(time.Now())
	if e != nil || n != 1 {
		t.Fatalf("cleanup %d: %v", n, e)
	}
	if _, e = os.Stat(filepath.Join(base, "do-not-delete")); e != nil {
		t.Fatal("deleted non-diagnostic")
	}
}

func TestQuotaRetainsMetadataWithoutCopyingLargePDF(t *testing.T) {
	s := NewStore(t.TempDir())
	s.MaxTotalBytes = 1024 // far smaller than PDF; should store error record only
	pdf := []byte("%PDF-1.4\nfixture\n")
	ev, e := s.Capture(Event{Username: "staff", Filename: "test.pdf", Stage: "ocr", Message: "timeout"}, bytes.NewReader(pdf), int64(len(pdf)), "network timeout")
	if e != nil {
		t.Fatal(e)
	}
	if ev.PDFSaved {
		t.Fatal("quota should prevent saving PDF")
	}
	day := ev.ID[:4] + "-" + ev.ID[4:6] + "-" + ev.ID[6:8]
	folder := filepath.Join(s.Root, day, ev.ID)
	if _, e := os.Stat(filepath.Join(folder, "error.json")); e != nil {
		t.Fatal("lost error metadata", e)
	}
	if _, e := os.Stat(filepath.Join(folder, "source.pdf")); !os.IsNotExist(e) {
		t.Fatal("saved sensitive PDF over quota")
	}
}
