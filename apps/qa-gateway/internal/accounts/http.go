package accounts

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
)

// Authorize is supplied by the actual Core session handler; never trust the
// frontend to assert admin role. Return true only for an active admin session.
type Authorize func(*http.Request) bool

type API struct {
	Store   *Store
	IsAdmin Authorize
}

func (a API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if a.Store == nil || a.IsAdmin == nil || !a.IsAdmin(r) {
		http.Error(w, `{"error":"forbidden"}`, http.StatusForbidden)
		return
	}
	if r.Method != "GET" && r.Method != "PATCH" {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if r.Method == "GET" {
		x, e := a.Store.Snapshot()
		if e != nil {
			fail(w, e)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"users": x})
		return
	}
	if !strings.HasPrefix(r.Header.Get("Content-Type"), "application/json") {
		w.WriteHeader(415)
		return
	}
	if fs := r.Header.Get("Sec-Fetch-Site"); fs != "" && fs != "same-origin" && fs != "none" {
		w.WriteHeader(403)
		return
	}
	// URLs are relative to this mounted handler; no paths or usernames outside a
	// controlled name and one of three explicitly allowed actions.
	path := strings.Trim(r.URL.Path, "/")
	parts := strings.Split(path, "/")
	if len(parts) != 2 || parts[0] == "" || strings.ContainsAny(parts[0], "\\./") {
		w.WriteHeader(404)
		return
	}
	username, action := parts[0], parts[1]
	decoder := json.NewDecoder(io.LimitReader(r.Body, 8192))
	decoder.DisallowUnknownFields()
	var e error
	switch action {
	case "name":
		var v struct {
			Value string `json:"value"`
		}
		if decoder.Decode(&v) != nil {
			w.WriteHeader(400)
			return
		}
		e = a.Store.SetDisplayName(username, v.Value)
	case "active":
		var v struct {
			Value *bool `json:"value"`
		}
		if decoder.Decode(&v) != nil || v.Value == nil {
			w.WriteHeader(400)
			return
		}
		e = a.Store.SetActive(username, *v.Value)
	case "role":
		var v struct {
			Value string `json:"value"`
		}
		if decoder.Decode(&v) != nil {
			w.WriteHeader(400)
			return
		}
		e = a.Store.SetRole(username, v.Value)
	default:
		w.WriteHeader(404)
		return
	}
	if e != nil {
		fail(w, e)
		return
	}
	// Snapshot forces a disk reread and returns only non-sensitive fields.
	x, e := a.Store.Snapshot()
	if e != nil {
		fail(w, e)
		return
	}
	for _, u := range x {
		if strings.EqualFold(u.Username, username) {
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": u})
			return
		}
	}
	fail(w, ErrMissingUser)
}
func fail(w http.ResponseWriter, e error) {
	status := http.StatusInternalServerError
	switch {
	case errors.Is(e, ErrMissingUser):
		status = 404
	case errors.Is(e, ErrConflict):
		status = 409
	case errors.Is(e, ErrLastAdmin):
		status = 400
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": e.Error()})
}
