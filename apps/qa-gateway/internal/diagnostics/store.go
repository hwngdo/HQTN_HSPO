package diagnostics

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"
)

const MaxPDFBytes int64 = 20 << 20
const MaxLogBytes = 128 << 10

var (
	bearer     = regexp.MustCompile(`(?i)Bearer\s+[a-z0-9._~+/-]+=*`)
	cookie     = regexp.MustCompile(`(?i)((?:cookie|set-cookie|authorization|password|token|api[_-]?key)\s*[:=]\s*)[^\s;]+`)
	urlSecrets = regexp.MustCompile(`(?i)([?&](?:token|key|secret|password|session)=)[^&\s]+`)
	fileSafe   = regexp.MustCompile(`[^a-zA-Z0-9._-]`)
)

type Event struct {
	ID         string `json:"error_id"`
	Time       string `json:"timestamp"`
	Username   string `json:"username"`
	ClientHost string `json:"client_host,omitempty"`
	ClientIP   string `json:"client_ip,omitempty"`
	Filename   string `json:"filename"`
	Stage      string `json:"stage"`
	Message    string `json:"message"`
	AppVersion string `json:"app_version,omitempty"`
	OCRVersion string `json:"ocr_version,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	PDFSaved   bool   `json:"pdf_saved"`
}

type Store struct {
	mu            sync.Mutex
	Root          string
	MaxTotalBytes int64
	Retention     time.Duration
	SaveFailedPDF bool
}

func NewStore(root string) *Store {
	return &Store{Root: root, MaxTotalBytes: 1 << 30, Retention: 30 * 24 * time.Hour, SaveFailedPDF: true}
}
func redacted(s string) string {
	s = bearer.ReplaceAllString(s, "Bearer [REDACTED]")
	s = cookie.ReplaceAllString(s, "$1[REDACTED]")
	s = urlSecrets.ReplaceAllString(s, "$1[REDACTED]")
	return s
}
func safeName(name string) string {
	// Drop client-supplied path (both POSIX and Windows separators).
	name = strings.ReplaceAll(name, "\\", "/")
	name = filepath.Base(name)
	name = fileSafe.ReplaceAllString(name, "_")
	if name == "" || name == "." || name == ".." {
		return "document.pdf"
	}
	if len(name) > 120 {
		name = name[len(name)-120:]
	}
	return name
}
func newID() (string, error) {
	b := make([]byte, 12)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return time.Now().UTC().Format("20060102T150405") + "_" + hex.EncodeToString(b), nil
}
func (s *Store) Capture(ev Event, pdf io.Reader, pdfSize int64, logtext string) (Event, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ev.Stage == "" || ev.Message == "" {
		return ev, errors.New("requires error stage and message")
	}
	if pdf != nil && (pdfSize < 0 || pdfSize > MaxPDFBytes) {
		return ev, errors.New("pdf exceeds limit")
	}
	id, e := newID()
	if e != nil {
		return ev, e
	}
	ev.ID = id
	ev.Time = time.Now().UTC().Format(time.RFC3339Nano)
	ev.Username = redacted(ev.Username)
	ev.ClientHost = redacted(ev.ClientHost)
	ev.ClientIP = redacted(ev.ClientIP)
	ev.Filename = safeName(ev.Filename)
	ev.Message = redacted(ev.Message)
	ev.RequestID = redacted(ev.RequestID)
	ev.Stage = redacted(ev.Stage)
	ev.PDFSaved = false
	day := time.Now().UTC().Format("2006-01-02")
	dir := filepath.Join(s.Root, day, id)
	if e = os.MkdirAll(dir, 0700); e != nil {
		return ev, e
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(dir)
		}
	}()
	// When the configured budget is nearly exhausted, keep the error record
	// rather than copying another sensitive PDF. Do not evict recent evidence.
	savePDF := pdf != nil && s.SaveFailedPDF
	if savePDF && s.MaxTotalBytes > 0 {
		used, qe := totalFileBytes(s.Root)
		if qe != nil {
			return ev, qe
		}
		if used+pdfSize+int64(len(logtext))+8192 > s.MaxTotalBytes {
			savePDF = false
		}
	}
	if savePDF {
		if !strings.EqualFold(filepath.Ext(ev.Filename), ".pdf") {
			return ev, errors.New("PDF extension required")
		}
		f, e := os.OpenFile(filepath.Join(dir, "source.pdf"), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
		if e != nil {
			return ev, e
		}
		n, e := io.Copy(f, io.LimitReader(pdf, MaxPDFBytes+1))
		closeErr := f.Close()
		if e != nil {
			return ev, e
		}
		if closeErr != nil {
			return ev, closeErr
		}
		if n > MaxPDFBytes || (pdfSize >= 0 && n != pdfSize) {
			return ev, errors.New("pdf byte length mismatch")
		}
		// PDF signature check avoids storing unrelated uploads in the diagnostics directory.
		fh, e := os.Open(filepath.Join(dir, "source.pdf"))
		if e != nil {
			return ev, e
		}
		header := make([]byte, 5)
		_, e = io.ReadFull(fh, header)
		_ = fh.Close()
		if e != nil || string(header) != "%PDF-" {
			return ev, errors.New("not a PDF")
		}
		ev.PDFSaved = true
	}
	if len(logtext) > MaxLogBytes {
		logtext = logtext[len(logtext)-MaxLogBytes:]
	}
	if e = os.WriteFile(filepath.Join(dir, "ocr.log"), []byte(redacted(logtext)), 0600); e != nil {
		return ev, e
	}
	b, e := json.MarshalIndent(ev, "", "  ")
	if e != nil {
		return ev, e
	}
	if e = os.WriteFile(filepath.Join(dir, "error.json"), append(b, '\n'), 0600); e != nil {
		return ev, e
	}
	cleanup = false
	return ev, nil
}

// Export is an admin-only building block. The HTTP layer must authorize admin
// before calling; no unauthenticated file-serving endpoints are provided here.
func (s *Store) Export(errorID string, dst io.Writer, admin bool) error {
	if !admin {
		return errors.New("admin permission required")
	}
	if !regexp.MustCompile(`^[0-9]{8}T[0-9]{6}_[0-9a-f]{24}$`).MatchString(errorID) {
		return errors.New("invalid error id")
	}
	day := errorID[:4] + "-" + errorID[4:6] + "-" + errorID[6:8]
	dir := filepath.Join(s.Root, day, errorID)
	zw := zip.NewWriter(dst)
	for _, name := range []string{"error.json", "ocr.log", "source.pdf"} {
		p := filepath.Join(dir, name)
		if _, e := os.Stat(p); errors.Is(e, os.ErrNotExist) {
			continue
		} else if e != nil {
			return e
		}
		f, e := os.Open(p)
		if e != nil {
			return e
		}
		entry, e := zw.Create(name)
		if e != nil {
			f.Close()
			return e
		}
		_, e = io.Copy(entry, f)
		f.Close()
		if e != nil {
			return e
		}
	}
	return zw.Close()
}
func (s *Store) Cleanup(now time.Time) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Retention <= 0 {
		return 0, fmt.Errorf("retention must be positive")
	}
	entries, e := os.ReadDir(s.Root)
	if errors.Is(e, os.ErrNotExist) {
		return 0, nil
	}
	if e != nil {
		return 0, e
	}
	deleted := 0
	for _, day := range entries {
		if !day.IsDir() {
			continue
		}
		d, e := time.Parse("2006-01-02", day.Name())
		if e != nil {
			continue
		}
		if now.UTC().Sub(d) < s.Retention+24*time.Hour {
			continue
		}
		// Remove only subdirs matching our generated ID pattern; never another file.
		base := filepath.Join(s.Root, day.Name())
		cases, e := os.ReadDir(base)
		if e != nil {
			return deleted, e
		}
		for _, c := range cases {
			if !c.IsDir() || !regexp.MustCompile(`^[0-9]{8}T[0-9]{6}_[0-9a-f]{24}$`).MatchString(c.Name()) {
				continue
			}
			if e = os.RemoveAll(filepath.Join(base, c.Name())); e != nil {
				return deleted, e
			}
			deleted++
		}
	}
	return deleted, nil
}

// totalFileBytes counts only regular files under diagnostics; used for a
// conservative quota check before saving another PDF. Symlinks are not followed.
func totalFileBytes(root string) (int64, error) {
	var size int64
	e := filepath.WalkDir(root, func(_ string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !d.Type().IsRegular() {
			return nil
		}
		st, e := d.Info()
		if e != nil {
			return e
		}
		size += st.Size()
		return nil
	})
	if errors.Is(e, os.ErrNotExist) {
		return 0, nil
	}
	return size, e
}

// Has verifies that the requested record exists before any ZIP response header
// is written. IDs are generated internally and cannot contain path separators.
func (s *Store) Has(errorID string) bool {
	if !regexp.MustCompile(`^[0-9]{8}T[0-9]{6}_[0-9a-f]{24}$`).MatchString(errorID) {
		return false
	}
	day := errorID[:4] + "-" + errorID[4:6] + "-" + errorID[6:8]
	f := filepath.Join(s.Root, day, errorID, "error.json")
	st, e := os.Stat(f)
	return e == nil && st.Mode().IsRegular()
}
