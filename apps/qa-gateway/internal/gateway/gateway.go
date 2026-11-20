// Package gateway integrates the Sprint 2 account and MSDS diagnostics APIs
// behind the original Core's /api/me session. QA ONLY: this bridge does not
// replace or patch the original production Core.
package gateway

import (
	"encoding/json"
	"errors"
	"fmt"
	"hqtn.local/hspo-sprint4-qa/internal/accounts"
	"hqtn.local/hspo-sprint4-qa/internal/diagnostics"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"
)

const AccountPrefix = "/api/admin/account-v2"
const DiagPrefix = "/api/diagnostics/msds/"

type Gateway struct {
	Accounts    *accounts.Store
	Diagnostics *diagnostics.Store
	Upstream    *url.URL
	Proxy       *httputil.ReverseProxy
	Client      *http.Client
	HTML        []byte // Optional isolated QA UI; original Core UI is unchanged
}

func New(upstream string, acc *accounts.Store, diag *diagnostics.Store) (*Gateway, error) {
	u, e := url.Parse(upstream)
	if e != nil || u.Scheme != "http" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Path != "" {
		return nil, errors.New("upstream must be an origin-only local http URL")
	}
	host := u.Hostname()
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return nil, errors.New("QA upstream must be loopback only")
	}
	if u.Port() == "" {
		return nil, errors.New("upstream port required")
	}
	p := httputil.NewSingleHostReverseProxy(u)
	p.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "Core upstream unavailable", http.StatusBadGateway)
	}
	c := &http.Client{Timeout: 3 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	return &Gateway{Accounts: acc, Diagnostics: diag, Upstream: u, Proxy: p, Client: c}, nil
}

// Identity always comes from a real successful upstream /api/me, never from
// username or role supplied by a browser, custom header, or localStorage.
func (g *Gateway) resolve(r *http.Request) (accounts.SessionIdentity, error) {
	req, e := http.NewRequestWithContext(r.Context(), "GET", g.Upstream.String()+"/api/me", nil)
	if e != nil {
		return accounts.SessionIdentity{}, e
	}
	req.Header.Set("Accept", "application/json")
	for _, c := range r.Cookies() {
		req.AddCookie(c)
	}
	res, e := g.Client.Do(req)
	if e != nil {
		return accounts.SessionIdentity{}, e
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return accounts.SessionIdentity{}, errors.New("upstream rejected session")
	}
	var v struct {
		User struct {
			Username string `json:"username"`
		} `json:"user"`
	}
	if e = json.NewDecoder(io.LimitReader(res.Body, 4096)).Decode(&v); e != nil {
		return accounts.SessionIdentity{}, e
	}
	if strings.TrimSpace(v.User.Username) == "" {
		return accounts.SessionIdentity{}, errors.New("upstream identity missing")
	}
	return accounts.SessionIdentity{Username: v.User.Username}, nil
}

func (g *Gateway) current(r *http.Request) (accounts.PublicUser, error) {
	id, e := g.resolve(r)
	if e != nil {
		return accounts.PublicUser{}, e
	}
	return g.Accounts.CurrentUser(id.Username)
}

func withJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(fmt.Sprintf(`{"error":%q}`, msg)))
}
func (g *Gateway) serveLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		withJSONError(w, 405, "method not allowed")
		return
	}
	req, err := http.NewRequestWithContext(r.Context(), "POST", g.Upstream.String()+"/api/login", io.LimitReader(r.Body, 65536))
	if err != nil {
		withJSONError(w, 500, "request failed")
		return
	}
	req.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	req.Header.Set("Accept-Encoding", "identity")
	res, err := g.Client.Do(req)
	if err != nil {
		withJSONError(w, 502, "Core login unavailable")
		return
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		withJSONError(w, 502, "upstream response error")
		return
	}
	var data map[string]json.RawMessage
	if json.Unmarshal(body, &data) != nil {
		withJSONError(w, 502, "Core login returned invalid JSON")
		return
	}
	var ok bool
	_ = json.Unmarshal(data["ok"], &ok)
	if res.StatusCode == 200 && ok {
		var user struct {
			Username string `json:"username"`
		}
		if json.Unmarshal(data["user"], &user) != nil || user.Username == "" {
			withJSONError(w, 502, "Core login identity missing")
			return
		}
		current, err := g.Accounts.ValidateLoginUser(user.Username)
		if err != nil {
			withJSONError(w, 401, "invalid credentials or inactive account")
			return
		}
		b, e := json.Marshal(current)
		if e != nil {
			withJSONError(w, 500, "user encoding error")
			return
		}
		data["user"] = b
		body, _ = json.Marshal(data)
	}
	for key, values := range res.Header {
		switch http.CanonicalHeaderKey(key) {
		case "Content-Length", "Content-Encoding", "Transfer-Encoding", "Connection":
			continue
		}
		for _, v := range values {
			w.Header().Add(key, v)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(res.StatusCode)
	_, _ = w.Write(body)
}

func (g *Gateway) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if g.Accounts == nil || g.Diagnostics == nil {
		withJSONError(w, 503, "QA gateway not configured")
		return
	}
	if len(g.HTML) > 0 && r.Method == "GET" && (r.URL.Path == "/" || r.URL.Path == "/static/index.html") {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write(g.HTML)
		return
	}
	if r.URL.Path == "/healthz" {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"build":"sprint4-qa","base_core":"1.9.6.9","production":false}`))
		return
	}
	// Do not accept cross-site browser writes even for proxied APIs.
	if r.Method != "GET" && r.Method != "HEAD" {
		if origin := r.Header.Get("Origin"); origin != "" {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Host != r.Host ||
				(parsed.Scheme != "http" && parsed.Scheme != "https") {
				withJSONError(w, http.StatusForbidden, "cross-origin request blocked")
				return
			}
		}
		site := r.Header.Get("Sec-Fetch-Site")
		if site != "" && site != "same-origin" && site != "none" {
			withJSONError(w, 403, "cross-site request blocked")
			return
		}
	}
	if r.URL.Path == "/api/login" {
		g.serveLogin(w, r)
		return
	}
	if r.URL.Path == "/api/me" {
		user, e := g.current(r)
		if e != nil {
			withJSONError(w, 401, "not authenticated or inactive")
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "user": user})
		return
	}
	if strings.HasPrefix(r.URL.Path, DiagPrefix) {
		ident, e := g.current(r)
		if e != nil {
			withJSONError(w, 401, "not authenticated or inactive")
			return
		}
		a := diagnostics.API{Store: g.Diagnostics, AppVersion: "1.9.6.9-qa", OCRVersion: "1.9.6.9-lan", Resolve: func(*http.Request) (diagnostics.Caller, error) {
			return diagnostics.Caller{Username: ident.Username, Admin: strings.EqualFold(ident.Role, "admin")}, nil
		}}
		http.StripPrefix(DiagPrefix, a).ServeHTTP(w, r)
		return
	}
	if r.URL.Path == AccountPrefix || strings.HasPrefix(r.URL.Path, AccountPrefix+"/") {
		id, e := g.resolve(r)
		if e != nil {
			withJSONError(w, 401, "not authenticated")
			return
		}
		me, e := g.Accounts.CurrentUser(id.Username)
		if e != nil || !strings.EqualFold(me.Role, "admin") {
			withJSONError(w, 403, "forbidden")
			return
		}
		api := accounts.API{Store: g.Accounts, IsAdmin: func(*http.Request) bool { return true }}
		http.StripPrefix(AccountPrefix, api).ServeHTTP(w, r)
		return
	}
	// The old admin account writers use an ambiguous, multi-field contract.
	// Do not proxy mutations of these paths in QA: they would bypass the
	// canonical one-field API and potentially write to the original Core.
	if r.Method != http.MethodGet && r.Method != http.MethodHead &&
		(r.URL.Path == "/api/users" || r.URL.Path == "/api/admin/users" ||
			strings.HasPrefix(r.URL.Path, "/api/admin/users/")) {
		withJSONError(w, http.StatusConflict, "legacy account mutation disabled in QA; use account-v2")
		return
	}
	if strings.HasPrefix(r.URL.Path, "/api/") {
		// Original Core creates the credential session. All other API traffic must
		// have an active current account, including users locked after login.
		switch r.URL.Path {
		case "/api/login", "/api/logout":
		default:
			u, e := g.current(r)
			if e != nil {
				withJSONError(w, 401, "not authenticated or inactive")
				return
			}
			if (strings.HasPrefix(r.URL.Path, "/api/admin/") || r.URL.Path == "/api/users") && !strings.EqualFold(u.Role, "admin") {
				withJSONError(w, 403, "forbidden")
				return
			}
		}
	}
	// QA host serves original Core/static unchanged. HTTP cookies are forwarded;
	// no session tokens, roles or claims are synthesized by the gateway.
	g.Proxy.ServeHTTP(w, r)
}

func LoopbackListen(addr, upstream string) error {
	host, port, e := net.SplitHostPort(addr)
	if e != nil || port == "" {
		return errors.New("invalid listen address")
	}
	if host != "127.0.0.1" && host != "localhost" && host != "::1" {
		return errors.New("QA must listen on loopback")
	}
	lPort := port
	u, e := url.Parse(upstream)
	if e != nil {
		return e
	}
	if u.Port() == lPort && (u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1") {
		return errors.New("gateway must not point to itself")
	}
	return nil
}
