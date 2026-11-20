package diagnostics

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
)

// Caller is derived from a validated Core server session, never from client
// JSON, X-User, query string, or a hidden form field.
type Caller struct {
	Username string
	Admin    bool
}
type ResolveCaller func(*http.Request) (Caller, error)

type API struct {
	Store      *Store
	Resolve    ResolveCaller
	AppVersion string
	OCRVersion string
}

// GET {id}/zip is exclusively admin. POST captures a *failed* PDF/OCR request.
// Mount at /api/diagnostics/msds/ after the real Core's authenticated router.
func (a API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if a.Store == nil || a.Resolve == nil {
		http.Error(w, "not configured", 503)
		return
	}
	caller, e := a.Resolve(r)
	if e != nil || strings.TrimSpace(caller.Username) == "" {
		http.Error(w, "not authenticated", 401)
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		http.Error(w, "forbidden", 403)
		return
	}
	path := strings.Trim(r.URL.Path, "/")
	if r.Method == "GET" {
		if !caller.Admin {
			http.Error(w, "forbidden", 403)
			return
		}
		if path == "" {
			events, err := a.Store.List(50)
			if err != nil {
				http.Error(w, "diagnostics unavailable", 500)
				return
			}
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "cases": events, "count": len(events)})
			return
		}
		parts := strings.Split(path, "/")
		if len(parts) != 2 || parts[1] != "zip" {
			http.NotFound(w, r)
			return
		}
		id := parts[0]
		if !a.Store.Has(id) {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=HQTN_MSDS_ERROR_%s.zip", id))
		if e := a.Store.Export(id, w, true); e != nil { // error should ideally be checked before writing body
			// Store.Export performs strict ID checks and opens only known files.
			return
		}
		return
	}
	if r.Method != "POST" || path != "" {
		http.Error(w, "method not allowed", 405)
		return
	}
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "multipart/form-data;") {
		http.Error(w, "multipart required", 415)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, MaxPDFBytes+MaxLogBytes+64<<10)
	if e := r.ParseMultipartForm(64 << 10); e != nil {
		http.Error(w, "bad multipart or too large", 400)
		return
	}
	if r.MultipartForm != nil {
		defer r.MultipartForm.RemoveAll()
	}
	stage := r.FormValue("stage")
	msg := r.FormValue("message")
	logtext := r.FormValue("log")
	if len(stage) > 128 || len(msg) > 4000 || len(logtext) > int(MaxLogBytes) {
		http.Error(w, "oversized metadata", 400)
		return
	}
	var pdf io.Reader
	var n int64
	var filename string
	var f io.ReadCloser
	f, fh, err := r.FormFile("pdf")
	if err == nil { // field is optional when there is no readable file available
		filename = fh.Filename
		n = fh.Size
		if n > MaxPDFBytes {
			http.Error(w, "pdf too large", 413)
			return
		}
		defer f.Close()
		pdf = f
	} else if !errors.Is(err, http.ErrMissingFile) {
		http.Error(w, "invalid pdf field", 400)
		return
	}
	if filename == "" {
		filename = r.FormValue("filename")
	}
	clientIP := r.RemoteAddr
	if ip, _, e := net.SplitHostPort(clientIP); e == nil {
		clientIP = ip
	}
	event, err := a.Store.Capture(Event{Username: caller.Username, ClientIP: clientIP,
		Filename: filename, Stage: stage, Message: msg, AppVersion: a.AppVersion,
		OCRVersion: a.OCRVersion}, pdf, n, logtext)
	if err != nil {
		http.Error(w, "could not store diagnostic", 500)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(201)
	_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "error_id": event.ID, "pdf_saved": event.PDFSaved})
}
