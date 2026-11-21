package hspo

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"hspo/release-v198/appupdate"
	"hspo/release-v198/dataupdate"
)

const sessionCookie = "hspo_session"

type App struct {
	Root      string
	DataDir   string
	StaticDir string
	PolicyDir string
	HTTPPort  int
	HTTPSPort int

	mu            sync.RWMutex
	ocrMu         sync.Mutex
	Users         []User
	Verify        []VerifyItem
	History       []HistoryItem
	Sessions      map[string]string // session token -> username
	Presence      map[string]Presence
	Shutdown      func()
	dataUpdate    http.Handler
	ManagedByTray bool
}

func New(root string) (*App, error) {
	if root == "" {
		root = "."
	}
	if _, err := os.Stat(filepath.Join(root, "userdata")); err == nil {
		return nil, errors.New("v1.9.8 userdata requires a compatible Core; use an isolated QA directory")
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("check legacy userdata: %w", err)
	}
	a := &App{
		Root:      root,
		DataDir:   filepath.Join(root, "data"),
		StaticDir: filepath.Join(root, "static"),
		PolicyDir: filepath.Join(root, "policy_data"),
		HTTPPort:  8870,
		HTTPSPort: 8871,
		Sessions:  map[string]string{},
		Presence:  map[string]Presence{},
	}
	if err := os.MkdirAll(a.DataDir, 0755); err != nil {
		return nil, err
	}
	if err := a.load(); err != nil {
		return nil, err
	}
	program := appupdate.WebHandler(root, func() bool { return a.ManagedByTray })
	update, err := dataupdate.NewHandler(root, a.authorizeDataUpdate, program, func() *dataupdate.GitHubSource {
		return dataupdate.NewGitHubSource(appupdate.Repository, appupdate.Token())
	})
	if err != nil {
		return nil, err
	}
	a.dataUpdate = update
	return a, nil
}

func (a *App) authorizeDataUpdate(r *http.Request) (string, error) {
	user, err := a.userFromReq(r)
	if err != nil {
		return "", errors.New("chưa đăng nhập hoặc phiên đã hết hạn")
	}
	if user.Role != "admin" {
		return "", errors.New("chỉ Admin được cập nhật dữ liệu")
	}
	return user.Username, nil
}

func (a *App) load() error {
	usersErr := readJSON(filepath.Join(a.DataDir, "users.json"), &a.Users)
	if usersErr != nil && !errors.Is(usersErr, os.ErrNotExist) {
		return fmt.Errorf("load users.json: %w", usersErr)
	}
	if err := readJSON(filepath.Join(a.DataDir, "verification.json"), &a.Verify); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load verification.json: %w", err)
	}
	if err := readJSON(filepath.Join(a.DataDir, "search_history.json"), &a.History); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load search_history.json: %w", err)
	}
	if usersErr == nil {
		if len(a.Users) == 0 {
			return errors.New("users.json contains no accounts")
		}
		for _, user := range a.Users {
			if user.Active && (user.Password == "" || user.Password == hashPassword("")) {
				return errors.New("users.json contains an active account with an empty password")
			}
		}
		return nil
	}
	if len(a.Verify) > 0 || len(a.History) > 0 {
		return errors.New("users.json is missing from a data directory with existing records")
	}
	password := os.Getenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD")
	if len(strings.TrimSpace(password)) < 12 {
		return errors.New("set HSPO_RECOVERY_QA_ADMIN_PASSWORD to initialize an empty QA runtime")
	}
	a.Users = []User{{Username: "admin", DisplayName: "Administrator", Password: hashPassword(password), Role: "admin", Active: true, CreatedAt: now()}}
	return a.writeUsers(a.Users)
}

func hashPassword(p string) string {
	h := sha256.Sum256([]byte(p))
	return hex.EncodeToString(h[:])
}
func newToken() string { b := make([]byte, 32); _, _ = rand.Read(b); return hex.EncodeToString(b) }
func now() string      { return time.Now().Format("2006-01-02 15:04:05") }

func readJSON(path string, dst any) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(b, dst)
}
func writeJSON(path string, v any) error {
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err = os.WriteFile(tmp, b, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
func jsonOut(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func (a *App) writeUsers(users []User) error {
	path := filepath.Join(a.DataDir, "users.json")
	tmp, err := os.CreateTemp(a.DataDir, ".users-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	encoder := json.NewEncoder(tmp)
	encoder.SetIndent("", "  ")
	err = encoder.Encode(users)
	closeErr := tmp.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(tmp.Name(), path)
}
func (a *App) saveVerify() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return writeJSON(filepath.Join(a.DataDir, "verification.json"), a.Verify)
}
func (a *App) saveHistory() error {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return writeJSON(filepath.Join(a.DataDir, "search_history.json"), a.History)
}

func (a *App) publicUser(u User) User { u.Password = ""; return u }
func (a *App) findUser(name string) *User {
	for i := range a.Users {
		if strings.EqualFold(a.Users[i].Username, name) {
			return &a.Users[i]
		}
	}
	return nil
}

func (a *App) userFromReq(r *http.Request) (*User, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.sessionUser(r)
}

// sessionUser returns a copy of the current account. The caller holds a.mu.
func (a *App) sessionUser(r *http.Request) (*User, error) {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return nil, err
	}
	un := a.Sessions[c.Value]
	if un == "" {
		return nil, errors.New("invalid session")
	}
	for i := range a.Users {
		if a.Users[i].Username == un && a.Users[i].Active {
			u := a.Users[i]
			return &u, nil
		}
	}
	return nil, errors.New("user not found")
}

func (a *App) auth(admin bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		u, err := a.userFromReq(r)
		if err != nil {
			jsonOut(w, 401, map[string]any{"ok": false, "error": "Chưa đăng nhập"})
			return
		}
		if admin && u.Role != "admin" {
			jsonOut(w, 403, map[string]any{"ok": false, "error": "Không có quyền thực hiện"})
			return
		}
		next(w, r)
	}
}

func clientIP(r *http.Request) string {
	if v := strings.TrimSpace(strings.Split(r.Header.Get("X-Forwarded-For"), ",")[0]); v != "" {
		return v
	}
	h, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return h
	}
	return r.RemoteAddr
}
func isLocal(r *http.Request) bool {
	ip := clientIP(r)
	return ip == "127.0.0.1" || ip == "::1" || ip == "localhost"
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "SAMEORIGIN")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

func (a *App) Handler() http.Handler { return securityHeaders(a.routes()) }

func (a *App) routes() http.Handler {
	mux := http.NewServeMux()
	mux.Handle(dataupdate.RoutePrefix, a.dataUpdate)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		jsonOut(w, 200, map[string]any{"ok": true, "version": "recovered-v1.9.4.1", "ocr_mode": "local_qa"})
	})
	mux.HandleFunc("/api/login", a.login)
	mux.HandleFunc("/api/logout", a.logout)
	mux.HandleFunc("/api/me", a.auth(false, a.me))
	mux.HandleFunc("/api/users", a.auth(false, a.usersAPI))
	mux.HandleFunc("/api/admin/users", a.auth(true, a.adminUsersAPI))
	mux.HandleFunc("/api/account/password", a.auth(false, a.accountPassword))
	mux.HandleFunc("/api/verify", a.auth(false, a.verifyAPI))
	mux.HandleFunc("/api/chem/analyze", a.auth(false, a.analyzeChem))
	mux.HandleFunc("/api/sources", a.auth(false, a.sourcesAPI))
	mux.HandleFunc("/api/system", a.auth(false, a.systemAPI))
	mux.HandleFunc("/api/presence", a.auth(false, a.presenceAPI))
	mux.HandleFunc("/api/search-history", a.auth(false, a.searchHistoryAPI))
	mux.HandleFunc("/api/ocr", a.auth(false, a.ocrAPI))
	mux.HandleFunc("/api/shutdown", a.auth(true, a.shutdownAPI))
	// Recovery placeholders for endpoints visible in the shipped UI but not reconstructed yet.
	for _, p := range []string{"/api/announcements", "/api/admin/backups", "/api/admin/backup", "/api/admin/update-preview", "/api/admin/update-apply", "/api/admin/rollback", "/api/history.xlsx", "/api/search-history/export.csv"} {
		path := p
		adminOnly := path == "/api/history.xlsx" || path == "/api/search-history/export.csv"
		mux.HandleFunc(path, a.auth(adminOnly, func(w http.ResponseWriter, r *http.Request) {
			jsonOut(w, 501, map[string]any{"ok": false, "error": "Recovered source placeholder: " + path})
		}))
	}
	fs := http.FileServer(http.Dir(a.StaticDir))
	mux.Handle("/static/", http.StripPrefix("/static/", fs))
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		http.ServeFile(w, r, filepath.Join(a.StaticDir, "index.html"))
	})
	return mux
}

func (a *App) login(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		jsonOut(w, 405, map[string]any{"ok": false})
		return
	}
	var q struct{ Username, Password string }
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q) != nil {
		jsonOut(w, 400, map[string]any{"ok": false, "error": "Dữ liệu không hợp lệ"})
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	var found *User
	for i := range a.Users {
		if strings.EqualFold(a.Users[i].Username, q.Username) {
			found = &a.Users[i]
			break
		}
	}
	if found == nil || !found.Active || found.Password != hashPassword(q.Password) {
		jsonOut(w, 401, map[string]any{"ok": false, "error": "Sai tài khoản hoặc mật khẩu"})
		return
	}
	tok := newToken()
	a.Sessions[tok] = found.Username
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: tok, Path: "/", HttpOnly: true, SameSite: http.SameSiteLaxMode})
	jsonOut(w, 200, map[string]any{"ok": true, "user": a.publicUser(*found)})
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if c, e := r.Cookie(sessionCookie); e == nil {
		a.mu.Lock()
		delete(a.Sessions, c.Value)
		a.mu.Unlock()
	}
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true})
	jsonOut(w, 200, map[string]any{"ok": true})
}
func (a *App) me(w http.ResponseWriter, r *http.Request) {
	u, err := a.userFromReq(r)
	if err != nil {
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	jsonOut(w, 200, map[string]any{"user": a.publicUser(*u)})
}

func (a *App) usersAPI(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	defer a.mu.RUnlock()
	u, err := a.sessionUser(r)
	if err != nil {
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	if r.Method != http.MethodGet {
		accountError(w, http.StatusMethodNotAllowed, "Phương thức không được hỗ trợ")
		return
	}
	if u.Role != "admin" {
		jsonOut(w, 200, []User{a.publicUser(*u)})
		return
	}
	out := make([]User, len(a.Users))
	for i, x := range a.Users {
		out[i] = a.publicUser(x)
	}
	sort.SliceStable(out, func(i, j int) bool {
		return out[i].Active && !out[j].Active
	})
	jsonOut(w, 200, out)
}
func (a *App) verifyAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "GET" {
		kind, key := r.URL.Query().Get("kind"), r.URL.Query().Get("key")
		a.mu.RLock()
		defer a.mu.RUnlock()
		if kind != "" && key != "" {
			for _, v := range a.Verify {
				if v.Kind == kind && v.Key == key {
					jsonOut(w, 200, v)
					return
				}
			}
			jsonOut(w, 200, map[string]any{})
			return
		}
		jsonOut(w, 200, a.Verify)
		return
	}
	if r.Method != "POST" {
		jsonOut(w, 405, map[string]any{"ok": false})
		return
	}
	var q struct{ Kind, Key, Action, Note, Label string }
	if json.NewDecoder(r.Body).Decode(&q) != nil {
		jsonOut(w, 400, map[string]any{"ok": false})
		return
	}
	a.mu.Lock()
	u, err := a.sessionUser(r)
	if err != nil {
		a.mu.Unlock()
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	idx := -1
	for i := range a.Verify {
		if a.Verify[i].Kind == q.Kind && a.Verify[i].Key == q.Key {
			idx = i
			break
		}
	}
	if idx < 0 {
		a.Verify = append(a.Verify, VerifyItem{Kind: q.Kind, Key: q.Key, Label: q.Label})
		idx = len(a.Verify) - 1
	}
	v := &a.Verify[idx]
	switch q.Action {
	case "need":
		v.NeedCheck = true
		v.Note = q.Note
	case "l1":
		v.L1User = u.Username
		v.L1Name = u.DisplayName
		v.L1At = now()
		v.NeedCheck = false
	case "l2":
		v.L2User = u.Username
		v.L2Name = u.DisplayName
		v.L2At = now()
		v.NeedCheck = false
	}
	a.mu.Unlock()
	_ = a.saveVerify()
	jsonOut(w, 200, map[string]any{"ok": true})
}

func countJSONArray(path string) int {
	b, e := os.ReadFile(path)
	if e != nil {
		return 0
	}
	var v any
	if json.Unmarshal(b, &v) != nil {
		return 0
	}
	switch x := v.(type) {
	case []any:
		return len(x)
	case map[string]any:
		for _, k := range []string{"records", "chemicals", "rules", "sources"} {
			if a, ok := x[k].([]any); ok {
				return len(a)
			}
		}
	}
	return 0
}
func (a *App) sourcesAPI(w http.ResponseWriter, r *http.Request) {
	jsonOut(w, 200, map[string]any{"hs_count": countJSONArray(filepath.Join(a.PolicyDir, "hs_policy.json")), "chemical_count": countJSONArray(filepath.Join(a.PolicyDir, "chemical_policy.json")), "rule_count": countJSONArray(filepath.Join(a.PolicyDir, "policy_engine_v2.json"))})
}
func lanIPs() []string {
	ifs, _ := net.InterfaceAddrs()
	var out []string
	for _, a := range ifs {
		ip, _, e := net.ParseCIDR(a.String())
		if e == nil && ip.To4() != nil && !ip.IsLoopback() {
			out = append(out, ip.String())
		}
	}
	sort.Strings(out)
	return out
}
func (a *App) systemAPI(w http.ResponseWriter, r *http.Request) {
	h, _ := os.Hostname()
	var urls []string
	for _, ip := range lanIPs() {
		urls = append(urls, fmt.Sprintf("http://%s:%d", ip, a.HTTPPort), fmt.Sprintf("https://%s:%d", ip, a.HTTPSPort))
	}
	jsonOut(w, 200, map[string]any{"mode": "LAN Server", "data_dir": filepath.Base(a.DataDir), "hostname": h, "http_port": a.HTTPPort, "port": a.HTTPSPort, "lan_urls": urls, "is_local": isLocal(r)})
}
func (a *App) presenceAPI(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	u, err := a.sessionUser(r)
	if err != nil {
		a.mu.Unlock()
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	if r.Method == "POST" {
		a.Presence[u.Username] = Presence{Username: u.Username, IP: clientIP(r), LastSeen: time.Now()}
	}
	cut := time.Now().Add(-90 * time.Second)
	var users []map[string]any
	for k, p := range a.Presence {
		if p.LastSeen.Before(cut) {
			delete(a.Presence, k)
			continue
		}
		users = append(users, map[string]any{"username": p.Username, "ip": p.IP, "last_seen": p.LastSeen.Format("2006-01-02 15:04:05")})
	}
	a.mu.Unlock()
	jsonOut(w, 200, map[string]any{"online_count": len(users), "users": users})
}
func (a *App) searchHistoryAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method == "POST" {
		var h HistoryItem
		if json.NewDecoder(r.Body).Decode(&h) != nil {
			jsonOut(w, 400, map[string]any{"ok": false})
			return
		}
		a.mu.Lock()
		u, err := a.sessionUser(r)
		if err != nil {
			a.mu.Unlock()
			accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
			return
		}
		h.Time = time.Now().UTC().Format(time.RFC3339)
		h.Username = u.Username
		h.DisplayName = u.DisplayName
		a.History = append(a.History, h)
		a.mu.Unlock()
		_ = a.saveHistory()
		jsonOut(w, 200, map[string]any{"ok": true})
		return
	}
	a.mu.RLock()
	u, err := a.sessionUser(r)
	if err != nil {
		a.mu.RUnlock()
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	arr := append([]HistoryItem(nil), a.History...)
	a.mu.RUnlock()
	total := len(arr)
	byM := map[string]int{}
	byU := map[string]int{}
	byUserSearch := map[string]int{}
	visible := make([]HistoryItem, 0)
	for _, h := range arr {
		byM[h.Module]++
		if u.Role != "admin" && !strings.EqualFold(h.Username, u.Username) {
			continue
		}
		visible = append(visible, h)
		key := h.Username
		if strings.EqualFold(key, u.Username) {
			key = u.Username
		}
		byU[key]++
		if h.Module != "msds_file" {
			byUserSearch[key]++
		}
	}
	limit := 250
	if requested, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && requested > 0 {
		limit = min(requested, 10000)
	}
	if len(visible) > limit {
		visible = visible[len(visible)-limit:]
	}
	for i, j := 0, len(visible)-1; i < j; i, j = i+1, j-1 {
		visible[i], visible[j] = visible[j], visible[i]
	}
	jsonOut(w, 200, map[string]any{"total": total, "by_module": byM, "by_user": byU,
		"by_user_search": byUserSearch, "recent": visible, "top_queries": []any{}})
}
func (a *App) shutdownAPI(w http.ResponseWriter, r *http.Request) {
	if !isLocal(r) {
		jsonOut(w, 403, map[string]any{"ok": false, "error": "Chỉ cho phép tắt từ máy chủ"})
		return
	}
	jsonOut(w, 200, map[string]any{"ok": true})
	if a.Shutdown != nil {
		go func() { time.Sleep(250 * time.Millisecond); a.Shutdown() }()
	}
}

func (a *App) LogSummary() {
	log.Printf("recovered core root=%s static=%s data=%s", a.Root, a.StaticDir, a.DataDir)
}
