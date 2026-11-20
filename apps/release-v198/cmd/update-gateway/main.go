package main

import (
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/hmac"
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
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const maxFiles = 250
const maxFileSize = int64(100 << 20)
const maxPackBytes = int64(350 << 20)

type FileSpec struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}
type Manifest struct {
	Format              string     `json:"format"`
	PackID              string     `json:"pack_id"`
	DataVersion         string     `json:"data_version"`
	RuleVersion         string     `json:"rule_version"`
	MinApp              string     `json:"min_app_version"`
	Description         string     `json:"description"`
	LegalReviewed       bool       `json:"legal_reviewed"`
	LegalReviewDate     string     `json:"legal_review_date,omitempty"`
	LegalReviewEvidence string     `json:"legal_review_evidence,omitempty"`
	Files               []FileSpec `json:"files"`
}
type Prior struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	SHA256 string `json:"sha256,omitempty"`
}
type BackupManifest struct {
	Format  string  `json:"format"`
	Created string  `json:"created"`
	Label   string  `json:"label"`
	Prior   []Prior `json:"prior"`
}

func allowed(p string) bool {
	if !validPath(p) {
		return false
	}
	low := strings.ToLower(p)
	if strings.HasPrefix(low, "policy_data/") && strings.Count(low, "/") == 1 && strings.HasSuffix(low, ".json") {
		return true
	}
	if strings.HasPrefix(low, "static/documents/pdf/") && strings.HasSuffix(low, ".pdf") {
		return true
	}
	if strings.HasPrefix(low, "static/documents/") && strings.Count(low, "/") == 2 && (strings.HasSuffix(low, ".docx") || strings.HasSuffix(low, ".doc") || strings.HasSuffix(low, ".pdf")) {
		return true
	}
	if strings.HasPrefix(low, "static/documents/") && strings.Count(low, "/") == 2 && strings.HasSuffix(low, ".json") {
		return true
	}
	if strings.HasPrefix(low, "static/") && strings.Count(low, "/") == 1 {
		return strings.HasSuffix(low, ".json") || strings.HasSuffix(low, ".json.gz")
	}
	return false
}

var windowsName = regexp.MustCompile(`^[a-zA-Z]:`)

func validPath(p string) bool {
	if p == "" || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || windowsName.MatchString(p) || strings.Contains(p, ":") || strings.Contains(p, "\x00") {
		return false
	}
	for _, s := range strings.Split(p, "/") {
		if s == "" || s == "." || s == ".." || strings.HasSuffix(s, ".") || strings.HasSuffix(s, " ") {
			return false
		}
	}
	return true
}
func sha(b []byte) string                          { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }
func readZip(path string) (*zip.ReadCloser, error) { return zip.OpenReader(path) }
func readZipEntry(f *zip.File) ([]byte, error) {
	if f.UncompressedSize64 > uint64(maxFileSize) {
		return nil, fmt.Errorf("tệp quá lớn: %s", f.Name)
	}
	r, e := f.Open()
	if e != nil {
		return nil, e
	}
	defer r.Close()
	return io.ReadAll(io.LimitReader(r, maxFileSize+1))
}
func getPackage(path string) (Manifest, map[string][]byte, error) {
	var m Manifest
	z, e := readZip(path)
	if e != nil {
		return m, nil, e
	}
	defer z.Close()
	if len(z.File) > maxFiles+2 {
		return m, nil, errors.New("gói có quá nhiều tệp")
	}
	raw := map[string][]byte{}
	var n int64
	seen := map[string]bool{}
	for _, f := range z.File {
		if f.FileInfo().IsDir() {
			continue
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return m, nil, fmt.Errorf("không nhận symlink: %s", f.Name)
		}
		if f.Name != "manifest.json" && !allowed(f.Name) {
			return m, nil, fmt.Errorf("đường dẫn không được phép: %s", f.Name)
		}
		lower := strings.ToLower(f.Name)
		if seen[lower] {
			return m, nil, fmt.Errorf("trùng tên tệp: %s", f.Name)
		}
		seen[lower] = true
		n += int64(f.UncompressedSize64)
		if n > maxPackBytes {
			return m, nil, errors.New("gói vượt kích thước dữ liệu cho phép")
		}
		b, er := readZipEntry(f)
		if er != nil {
			return m, nil, er
		}
		raw[f.Name] = b
	}
	mb, ok := raw["manifest.json"]
	if !ok {
		return m, nil, errors.New("thiếu manifest.json")
	}
	if e = json.Unmarshal(mb, &m); e != nil {
		return m, nil, fmt.Errorf("manifest không hợp lệ: %w", e)
	}
	if m.Format != "HQTN_HSPO_DATA_V1" || m.PackID == "" || m.DataVersion == "" || m.RuleVersion == "" {
		return m, nil, errors.New("thiếu format, mã gói hoặc phiên bản dữ liệu/quy tắc")
	}
	if len(m.PackID) > 96 || len(m.DataVersion) > 64 || len(m.RuleVersion) > 64 || len(m.Description) > 1000 || len(m.LegalReviewEvidence) > 3000 {
		return m, nil, errors.New("thông tin manifest quá dài")
	}
	if len(m.Files) == 0 {
		return m, nil, errors.New("gói không có tệp dữ liệu")
	}
	if len(m.Files) != len(raw)-1 {
		return m, nil, errors.New("tệp thực tế không khớp danh sách manifest")
	}
	referenced := map[string]bool{}
	for _, f := range m.Files {
		if !allowed(f.Path) {
			return m, nil, fmt.Errorf("không được phép cập nhật: %s", f.Path)
		}
		if referenced[strings.ToLower(f.Path)] {
			return m, nil, fmt.Errorf("trùng tệp trong manifest: %s", f.Path)
		}
		referenced[strings.ToLower(f.Path)] = true
		b, ok := raw[f.Path]
		if !ok {
			return m, nil, fmt.Errorf("thiếu tệp: %s", f.Path)
		}
		if int64(len(b)) != f.Size || sha(b) != strings.ToLower(f.SHA256) {
			return m, nil, fmt.Errorf("SHA-256 hoặc dung lượng sai: %s", f.Path)
		}
		if er := checkPayload(f.Path, b); er != nil {
			return m, nil, er
		}
	}
	return m, raw, nil
}
func checkPayload(p string, b []byte) error {
	l := strings.ToLower(p)
	if strings.HasSuffix(l, ".json") || strings.HasSuffix(l, ".json.gz") {
		if strings.HasSuffix(l, ".gz") {
			r, e := gzip.NewReader(bytes.NewReader(b))
			if e != nil {
				return fmt.Errorf("gzip lỗi %s: %w", p, e)
			}
			defer r.Close()
			d, e := io.ReadAll(io.LimitReader(r, maxFileSize+1))
			if e != nil {
				return e
			}
			b = d
		}
		if len(b) > int(maxFileSize) || !json.Valid(b) {
			return fmt.Errorf("JSON không hợp lệ: %s", p)
		}
		if l == "static/tariff_2026.json.gz" {
			return checkTariffDataset(b)
		}
	} else if strings.HasSuffix(l, ".pdf") {
		if !bytes.HasPrefix(b, []byte("%PDF-")) {
			return fmt.Errorf("PDF không hợp lệ: %s", p)
		}
	} else if strings.HasSuffix(l, ".docx") {
		if !bytes.HasPrefix(b, []byte("PK")) {
			return fmt.Errorf("DOCX không hợp lệ: %s", p)
		}
	} else if strings.HasSuffix(l, ".doc") {
		if len(b) < 8 || !bytes.Equal(b[:8], []byte{0xd0, 0xcf, 0x11, 0xe0, 0xa1, 0xb1, 0x1a, 0xe1}) {
			return fmt.Errorf("DOC không hợp lệ: %s", p)
		}
	}
	return nil
}
func checkTariffDataset(data []byte) error {
	var dataset struct {
		Records []*struct {
			HS    string                     `json:"hs"`
			Rates map[string]json.RawMessage `json:"rates"`
		} `json:"records"`
	}
	if err := json.Unmarshal(data, &dataset); err != nil || len(dataset.Records) == 0 {
		return errors.New("Dữ liệu biểu thuế phải có danh sách records chứa các dòng HS")
	}
	for i, record := range dataset.Records {
		if record == nil || strings.TrimSpace(record.HS) == "" || record.Rates == nil {
			return fmt.Errorf("Dòng biểu thuế %d thiếu mã HS hoặc bảng thuế rates", i+1)
		}
	}
	return nil
}

func (m Manifest) canApply() bool {
	_, err := time.Parse("2006-01-02", m.LegalReviewDate)
	return m.LegalReviewed && len(strings.TrimSpace(m.LegalReviewEvidence)) >= 12 && err == nil
}

func safeTarget(root, p string) (string, error) {
	if !allowed(p) {
		return "", errors.New("tệp không nằm trong thư mục dữ liệu")
	}
	abs, e := filepath.Abs(root)
	if e != nil {
		return "", e
	}
	t := filepath.Join(abs, filepath.FromSlash(p))
	if !strings.HasPrefix(strings.ToLower(t), strings.ToLower(abs)+string(os.PathSeparator)) {
		return "", errors.New("đường dẫn ra ngoài chương trình")
	}
	return t, nil
}
func runCheck(root string) error {
	if _, e := os.Stat(filepath.Join(root, "HQTN_HSPO_Core.exe")); e != nil {
		return errors.New("không thấy HQTN_HSPO_Core.exe; hãy chạy trên thư mục chương trình máy chủ")
	}
	if _, e := os.Stat(filepath.Join(root, "static", "index.html")); e != nil {
		return errors.New("không thấy static/index.html")
	}
	return nil
}
func checkApp(root, needed string) error {
	b, e := os.ReadFile(filepath.Join(root, "VERSION.txt"))
	if e != nil {
		return e
	}
	installed := strings.TrimSpace(string(b))
	if needed != "" && installed != needed {
		return fmt.Errorf("gói dành cho %s, chương trình hiện là %s", needed, installed)
	}
	return nil
}
func ensureSafeParents(root string, paths []string) error {
	for _, p := range paths {
		t, e := safeTarget(root, p)
		if e != nil {
			return e
		}
		for dir := filepath.Dir(t); strings.HasPrefix(strings.ToLower(dir), strings.ToLower(root)); dir = filepath.Dir(dir) {
			fi, e := os.Lstat(dir)
			if e == nil && fi.Mode()&os.ModeSymlink != 0 {
				return fmt.Errorf("thư mục symlink không được chấp nhận: %s", dir)
			}
			if dir == root {
				break
			}
		}
	}
	return nil
}
func backup(root, label string, paths []string) (string, error) {
	d := filepath.Join(root, "data_updates", "backups")
	if e := os.MkdirAll(d, 0750); e != nil {
		return "", e
	}
	fn := filepath.Join(d, "backup_"+time.Now().Format("20060102_150405.000000000")+".zip")
	f, e := os.Create(fn)
	if e != nil {
		return "", e
	}
	z := zip.NewWriter(f)
	m := BackupManifest{Format: "HQTN_HSPO_BACKUP_V1", Created: time.Now().Format(time.RFC3339), Label: label}
	for _, p := range paths {
		t, _ := safeTarget(root, p)
		b, e := os.ReadFile(t)
		if os.IsNotExist(e) {
			m.Prior = append(m.Prior, Prior{Path: p, Exists: false})
			continue
		}
		if e != nil {
			z.Close()
			f.Close()
			return "", e
		}
		if int64(len(b)) > maxFileSize {
			z.Close()
			f.Close()
			return "", fmt.Errorf("backup tệp quá lớn: %s", p)
		}
		m.Prior = append(m.Prior, Prior{Path: p, Exists: true, SHA256: sha(b)})
		w, e := z.Create("before/" + p)
		if e != nil {
			z.Close()
			f.Close()
			return "", e
		}
		if _, e = w.Write(b); e != nil {
			z.Close()
			f.Close()
			return "", e
		}
	}
	mb, _ := json.MarshalIndent(m, "", "  ")
	w, e := z.Create("before.json")
	if e != nil {
		z.Close()
		f.Close()
		return "", e
	}
	_, e = w.Write(mb)
	if e != nil {
		z.Close()
		f.Close()
		return "", e
	}
	if e = z.Close(); e != nil {
		f.Close()
		return "", e
	}
	if e = f.Close(); e != nil {
		return "", e
	}
	return fn, nil
}
func loadBackup(path string) (BackupManifest, map[string][]byte, error) {
	var m BackupManifest
	z, e := zip.OpenReader(path)
	if e != nil {
		return m, nil, e
	}
	defer z.Close()
	data := map[string][]byte{}
	for _, f := range z.File {
		if f.Name == "before.json" {
			b, er := readZipEntry(f)
			if er != nil {
				return m, nil, er
			}
			if er = json.Unmarshal(b, &m); er != nil {
				return m, nil, er
			}
			continue
		}
		if !strings.HasPrefix(f.Name, "before/") || !allowed(strings.TrimPrefix(f.Name, "before/")) {
			return m, nil, errors.New("backup chứa đường dẫn không hợp lệ")
		}
		b, er := readZipEntry(f)
		if er != nil {
			return m, nil, er
		}
		data[strings.TrimPrefix(f.Name, "before/")] = b
	}
	if m.Format != "HQTN_HSPO_BACKUP_V1" {
		return m, nil, errors.New("không đúng định dạng backup của HSPO")
	}
	for _, p := range m.Prior {
		if !allowed(p.Path) {
			return m, nil, errors.New("đường dẫn backup không hợp lệ")
		}
		if p.Exists && sha(data[p.Path]) != p.SHA256 {
			return m, nil, fmt.Errorf("backup sai SHA: %s", p.Path)
		}
	}
	return m, data, nil
}
func replace(root string, desired map[string][]byte, absent []string) error {
	stage, e := os.MkdirTemp(root, ".hspo_data_stage_")
	if e != nil {
		return e
	}
	defer os.RemoveAll(stage)
	for p, b := range desired {
		t := filepath.Join(stage, filepath.FromSlash(p))
		if e = os.MkdirAll(filepath.Dir(t), 0750); e != nil {
			return e
		}
		if e = os.WriteFile(t, b, 0644); e != nil {
			return e
		}
	}
	keys := make([]string, 0, len(desired))
	for k := range desired {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, p := range keys {
		t, _ := safeTarget(root, p)
		if e = os.MkdirAll(filepath.Dir(t), 0750); e != nil {
			return e
		}
		if e = os.Rename(filepath.Join(stage, filepath.FromSlash(p)), t); e != nil {
			return fmt.Errorf("không thay được %s (hãy đóng HSPO): %w", p, e)
		}
	}
	for _, p := range absent {
		t, _ := safeTarget(root, p)
		if e = os.Remove(t); e != nil && !os.IsNotExist(e) {
			return e
		}
	}
	return nil
}
func audit(root, operation, detail string) {
	d := filepath.Join(root, "data_updates")
	os.MkdirAll(d, 0750)
	f, e := os.OpenFile(filepath.Join(d, "history.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if e != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s | %s | %s\n", time.Now().Format(time.RFC3339), operation, detail)
}
func versions(root string, m Manifest) {
	path := filepath.Join(root, "static", "hspo_data_version.json")
	data := map[string]any{"data_version": m.DataVersion, "rule_version": m.RuleVersion, "pack_id": m.PackID, "updated_at": time.Now().Format(time.RFC3339), "legal_reviewed": m.LegalReviewed, "description": m.Description, "legal_review_date": m.LegalReviewDate, "legal_review_evidence": m.LegalReviewEvidence}
	b, _ := json.MarshalIndent(data, "", "  ")
	_ = os.WriteFile(path, b, 0644)
}
func printPreview(root string, m Manifest, raw map[string][]byte) {
	fmt.Printf("GÓI: %s\nMô tả: %s\nDữ liệu: %s  |  Quy tắc: %s\n", m.PackID, m.Description, m.DataVersion, m.RuleVersion)
	if !m.LegalReviewed {
		fmt.Println("CẢNH BÁO: chưa xác nhận rà soát hiệu lực đầy đủ; không coi là gói pháp lý đã nghiệm thu")
	}
	fmt.Printf("Số tệp: %d\n", len(m.Files))
	for _, f := range m.Files {
		t, _ := safeTarget(root, f.Path)
		b, e := os.ReadFile(t)
		kind := "THÊM"
		if e == nil {
			kind = "THAY ĐỔI"
			if sha(b) == sha(raw[f.Path]) {
				kind = "KHÔNG ĐỔI"
			}
		}
		fmt.Printf("  %-11s %s (%d byte)\n", kind, f.Path, f.Size)
	}
}

// Gateway quản trị: xác thực quyền Admin bằng phiên đăng nhập của Core;
// các thao tác ghi tệp chỉ thực hiện sau khi qua kiểm tra gói, preview và backup.
const routePrefix = "/__hspo_admin/"
const gatewayListen = ":8867"
const coreURL = "http://127.0.0.1:8865"

type Stage struct {
	Path   string
	At     time.Time
	PackID string
	By     string
	Digest string
}
type Gateway struct {
	root      string
	github    *githubReleaseSource
	core      *url.URL
	client    *http.Client
	key       []byte
	stages    map[string]Stage
	mu        sync.Mutex
	operation sync.Mutex
	started   time.Time
}

func jwrite(w http.ResponseWriter, code int, data any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(data)
}
func jerr(w http.ResponseWriter, code int, s string) {
	jwrite(w, code, map[string]any{"ok": false, "error": s})
}
func (g *Gateway) admin(r *http.Request) (string, error) {
	// Do not trust frontend role flags. Ask Core over loopback using the SAME session cookie.
	req, e := http.NewRequestWithContext(r.Context(), "GET", g.core.String()+"/api/me", nil)
	if e != nil {
		return "", e
	}
	req.Header.Set("Cookie", r.Header.Get("Cookie"))
	if a := r.Header.Get("Authorization"); a != "" {
		req.Header.Set("Authorization", a)
	}
	resp, e := g.client.Do(req)
	if e != nil {
		return "", errors.New("không liên lạc được Core để xác thực")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return "", errors.New("chưa đăng nhập hoặc phiên đã hết hạn")
	}
	var obj struct {
		OK   bool `json:"ok"`
		User struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"user"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 16384)).Decode(&obj) != nil || !strings.EqualFold(obj.User.Role, "admin") {
		return "", errors.New("chỉ Admin của HSPO được thực hiện")
	}
	if strings.TrimSpace(obj.User.Username) == "" {
		return "", errors.New("không xác định được tài khoản Admin")
	}
	return obj.User.Username, nil
}
func (g *Gateway) csrf(r *http.Request) string {
	h := hmac.New(sha256.New, g.key)
	h.Write([]byte(r.Header.Get("Cookie")))
	h.Write([]byte("/HSPO/"))
	return hex.EncodeToString(h.Sum(nil))
}
func (g *Gateway) postCheck(r *http.Request) error {
	if r.Method != "POST" {
		return errors.New("chỉ nhận POST")
	}
	origin := r.Header.Get("Origin")
	if origin != "" && origin != "http://"+r.Host && origin != "https://"+r.Host {
		return errors.New("Origin không hợp lệ")
	}
	want, got := g.csrf(r), r.Header.Get("X-HSPO-CSRF")
	if len(got) == 0 || !hmac.Equal([]byte(want), []byte(got)) {
		return errors.New("Phiên kiểm tra bảo mật hết hạn, hãy tải lại trang")
	}
	return nil
}
func (g *Gateway) cleanup() {
	g.mu.Lock()
	defer g.mu.Unlock()
	for k, st := range g.stages {
		if time.Since(st.At) > 45*time.Minute {
			_ = os.Remove(st.Path)
			delete(g.stages, k)
		}
	}
}
func (g *Gateway) previewDataPackage(w http.ResponseWriter, src io.Reader, username, expectedHash, releaseTag string) {
	dir := filepath.Join(g.root, "data_updates", "staging")
	if e := os.MkdirAll(dir, 0700); e != nil {
		jerr(w, 500, "Không tạo được thư mục staging: "+e.Error())
		return
	}
	idBytes := make([]byte, 16)
	if _, e := rand.Read(idBytes); e != nil {
		jerr(w, 500, "Lỗi tạo mã gói")
		return
	}
	id := hex.EncodeToString(idBytes)
	filename := filepath.Join(dir, id+".zip")
	f, e := os.OpenFile(filename, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		jerr(w, 500, "Không tạo được tệp staging")
		return
	}
	digest := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(f, digest), io.LimitReader(src, maxPackBytes+1))
	closeErr := f.Close()
	if copyErr != nil || closeErr != nil || n > maxPackBytes {
		_ = os.Remove(filename)
		jerr(w, 400, "Gói ZIP quá lớn hoặc lỗi ghi tạm")
		return
	}
	if expectedHash != "" && hex.EncodeToString(digest.Sum(nil)) != expectedHash {
		_ = os.Remove(filename)
		jerr(w, 400, "SHA-256 của gói GitHub không khớp")
		return
	}
	m, raw, e := getPackage(filename)
	if e != nil {
		_ = os.Remove(filename)
		jerr(w, 400, "Gói không hợp lệ: "+e.Error())
		return
	}
	if e = checkApp(g.root, m.MinApp); e != nil {
		_ = os.Remove(filename)
		jerr(w, 400, e.Error())
		return
	}
	stats := []map[string]any{}
	changed := 0
	for _, sp := range m.Files {
		target, _ := safeTarget(g.root, sp.Path)
		prev, err := os.ReadFile(target)
		action := "Thêm"
		if err == nil {
			action = "Thay đổi"
			if sha(prev) == sha(raw[sp.Path]) {
				action = "Không đổi"
			}
		}
		if action != "Không đổi" {
			changed++
		}
		stats = append(stats, map[string]any{"path": sp.Path, "action": action, "bytes": sp.Size})
	}
	g.mu.Lock()
	g.stages[id] = Stage{Path: filename, At: time.Now(), PackID: m.PackID, By: username, Digest: hex.EncodeToString(digest.Sum(nil))}
	g.mu.Unlock()
	jwrite(w, 200, map[string]any{"ok": true, "id": id, "pack_id": m.PackID, "data_version": m.DataVersion, "rule_version": m.RuleVersion, "description": m.Description, "files": stats, "changed": changed, "legal_reviewed": m.LegalReviewed, "legal_review_date": m.LegalReviewDate, "legal_review_evidence": m.LegalReviewEvidence, "can_apply": m.canApply(), "release_tag": releaseTag})
}

func stagePackageMatches(stage Stage) bool {
	file, err := os.Open(stage.Path)
	if err != nil {
		return false
	}
	defer file.Close()
	digest := sha256.New()
	size, err := io.Copy(digest, io.LimitReader(file, maxPackBytes+1))
	return err == nil && size <= maxPackBytes && hex.EncodeToString(digest.Sum(nil)) == stage.Digest
}

func (g *Gateway) serve(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if r.URL.Path == routePrefix+"ping" {
		jwrite(w, 200, map[string]any{"ok": true, "service": "HQTN_HSPO_DataGateway", "version": "1.9.8"})
		return
	}
	username, e := g.admin(r)
	if e != nil {
		jerr(w, 403, e.Error())
		return
	}
	switch r.URL.Path {
	case routePrefix + "status":
		if r.Method != "GET" {
			jerr(w, 405, "GET only")
			return
		}
		g.cleanup()
		data := map[string]any{}
		b, e := os.ReadFile(filepath.Join(g.root, "static", "hspo_data_version.json"))
		if e == nil {
			_ = json.Unmarshal(b, &data)
		}
		var pending bool
		if stat, er := os.Stat(filepath.Join(g.root, "static", "hspo_data_version.json")); er == nil {
			pending = stat.ModTime().After(g.started.Add(2 * time.Second))
		}
		history := []string{}
		h, e := os.ReadFile(filepath.Join(g.root, "data_updates", "history.log"))
		if e == nil {
			lines := strings.Split(strings.TrimSpace(string(h)), "\n")
			if len(lines) > 12 {
				lines = lines[len(lines)-12:]
			}
			history = lines
		}
		jwrite(w, 200, map[string]any{"ok": true, "data": data, "csrf": g.csrf(r), "pending_restart": pending, "history": history, "username": username, "gateway": "1.9.8-data", "core_up": true, "github_enabled": g.github != nil})
	case routePrefix + "preview":
		if e := g.postCheck(r); e != nil {
			jerr(w, 403, e.Error())
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, maxPackBytes+(1<<20))
		if e := r.ParseMultipartForm(4 << 20); e != nil {
			jerr(w, 400, "Không đọc được gói ZIP hoặc vượt giới hạn 350 MB")
			return
		}
		src, header, e := r.FormFile("file")
		if e != nil {
			jerr(w, 400, "Chưa chọn tệp ZIP")
			return
		}
		defer src.Close()
		defer r.MultipartForm.RemoveAll()
		if !strings.HasSuffix(strings.ToLower(header.Filename), ".zip") {
			jerr(w, 400, "Chỉ cho phép tệp ZIP")
			return
		}
		g.previewDataPackage(w, src, username, "", "")
	case routePrefix + "github-preview":
		g.previewGitHubPackage(w, r, username)
	case routePrefix + "apply":
		if e := g.postCheck(r); e != nil {
			jerr(w, 403, e.Error())
			return
		}
		var req struct {
			ID      string `json:"id"`
			Confirm string `json:"confirm"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req) != nil || req.Confirm != "AP_DUNG" {
			jerr(w, 400, "Yêu cầu xác nhận AP_DUNG")
			return
		}
		g.operation.Lock()
		defer g.operation.Unlock()
		g.mu.Lock()
		stage, ok := g.stages[req.ID]
		g.mu.Unlock()
		if !ok || time.Since(stage.At) > 45*time.Minute || stage.By != username {
			jerr(w, 400, "Phiên xem trước đã hết hạn hoặc không thuộc người dùng này")
			return
		}
		if !stagePackageMatches(stage) {
			jerr(w, 400, "Gói đã thay đổi sau khi xem trước")
			return
		}
		m, raw, e := getPackage(stage.Path)
		if e != nil {
			jerr(w, 400, e.Error())
			return
		}
		if m.PackID != stage.PackID {
			jerr(w, 400, "Gói đã thay đổi sau khi xem trước")
			return
		}
		if e = checkApp(g.root, m.MinApp); e != nil {
			jerr(w, 400, e.Error())
			return
		}
		if !m.canApply() {
			jerr(w, 400, "Thiếu xác nhận và bằng chứng rà soát hiệu lực pháp luật. Không áp dụng.")
			return
		}
		paths := []string{"static/hspo_data_version.json"}
		desired := map[string][]byte{}
		for _, sp := range m.Files {
			paths = append(paths, sp.Path)
			desired[sp.Path] = raw[sp.Path]
		}
		if e = ensureSafeParents(g.root, paths); e != nil {
			jerr(w, 400, e.Error())
			return
		}
		bkp, e := backup(g.root, m.PackID, paths)
		if e != nil {
			jerr(w, 500, "Không tạo được sao lưu: "+e.Error())
			return
		}
		v := map[string]any{"data_version": m.DataVersion, "rule_version": m.RuleVersion, "pack_id": m.PackID, "updated_at": time.Now().Format(time.RFC3339), "legal_reviewed": m.LegalReviewed, "description": m.Description, "legal_review_date": m.LegalReviewDate, "legal_review_evidence": m.LegalReviewEvidence}
		desired["static/hspo_data_version.json"], _ = json.MarshalIndent(v, "", "  ")
		if e = replace(g.root, desired, nil); e != nil {
			// Best effort return to prior state and report both results
			bk, old, er := loadBackup(bkp)
			if er == nil {
				prev := map[string][]byte{}
				del := []string{}
				for _, x := range bk.Prior {
					if x.Exists {
						prev[x.Path] = old[x.Path]
					} else {
						del = append(del, x.Path)
					}
				}
				er = replace(g.root, prev, del)
			}
			audit(g.root, "FAILED_APPLY", username+" pack="+m.PackID+" backup="+filepath.Base(bkp))
			jerr(w, 500, "Áp dụng lỗi: "+e.Error()+"; khôi phục: "+fmt.Sprint(er))
			return
		}
		audit(g.root, "APPLY", username+" pack="+m.PackID+" backup="+filepath.Base(bkp))
		g.mu.Lock()
		delete(g.stages, req.ID)
		g.mu.Unlock()
		_ = os.Remove(stage.Path)
		jwrite(w, 200, map[string]any{"ok": true, "backup": filepath.Base(bkp), "data_version": m.DataVersion, "rule_version": m.RuleVersion, "restart_required": true})
	case routePrefix + "backups":
		if r.Method != "GET" {
			jerr(w, 405, "GET only")
			return
		}
		dir := filepath.Join(g.root, "data_updates", "backups")
		entries, _ := os.ReadDir(dir)
		out := []map[string]any{}
		for _, f := range entries {
			if f.IsDir() || !strings.HasPrefix(f.Name(), "backup_") || !strings.HasSuffix(f.Name(), ".zip") {
				continue
			}
			st, e := f.Info()
			if e != nil {
				continue
			}
			out = append(out, map[string]any{"name": f.Name(), "size": st.Size(), "date": st.ModTime().Format("02/01/2006 15:04")})
		}
		sort.Slice(out, func(i, j int) bool { return out[i]["name"].(string) > out[j]["name"].(string) })
		if len(out) > 30 {
			out = out[:30]
		}
		jwrite(w, 200, map[string]any{"ok": true, "items": out})
	case routePrefix + "rollback":
		if e := g.postCheck(r); e != nil {
			jerr(w, 403, e.Error())
			return
		}
		var req struct {
			File    string `json:"file"`
			Confirm string `json:"confirm"`
		}
		if json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&req) != nil || req.Confirm != "KHOI_PHUC" {
			jerr(w, 400, "Cần xác nhận KHOI_PHUC")
			return
		}
		if filepath.Base(req.File) != req.File || !strings.HasPrefix(req.File, "backup_") || !strings.HasSuffix(req.File, ".zip") {
			jerr(w, 400, "Tên sao lưu không hợp lệ")
			return
		}
		g.operation.Lock()
		defer g.operation.Unlock()
		b, old, e := loadBackup(filepath.Join(g.root, "data_updates", "backups", req.File))
		if e != nil {
			jerr(w, 400, "Sao lưu không hợp lệ: "+e.Error())
			return
		}
		paths := []string{}
		desired := map[string][]byte{}
		absent := []string{}
		for _, p := range b.Prior {
			paths = append(paths, p.Path)
			if p.Exists {
				desired[p.Path] = old[p.Path]
			} else {
				absent = append(absent, p.Path)
			}
		}
		if e = ensureSafeParents(g.root, paths); e != nil {
			jerr(w, 400, e.Error())
			return
		}
		guard, e := backup(g.root, "before_rollback_"+req.File, paths)
		if e != nil {
			jerr(w, 500, "Không tạo được backup bảo vệ: "+e.Error())
			return
		}
		if e = replace(g.root, desired, absent); e != nil {
			jerr(w, 500, "Khôi phục lỗi: "+e.Error()+"; backup bảo vệ "+filepath.Base(guard))
			return
		}
		audit(g.root, "ROLLBACK", username+" file="+req.File+" guard="+filepath.Base(guard))
		jwrite(w, 200, map[string]any{"ok": true, "guard": filepath.Base(guard), "restart_required": true})
	default:
		jerr(w, 404, "Không có chức năng này")
	}
}
func main() {
	root, er := os.Executable()
	if er != nil {
		log.Fatal(er)
	}
	root = filepath.Dir(root)
	if er = runCheck(root); er != nil {
		log.Fatal(er)
	}
	core, er := url.Parse(coreURL)
	if er != nil {
		log.Fatal(er)
	}
	key := make([]byte, 32)
	if _, er = rand.Read(key); er != nil {
		log.Fatal(er)
	}
	g := &Gateway{root: root, github: newGitHubReleaseSource(os.Getenv("HSPO_GITHUB_REPOSITORY"), os.Getenv("HSPO_GITHUB_READ_TOKEN")), core: core, key: key, started: time.Now(), stages: map[string]Stage{}, client: &http.Client{Timeout: 6 * time.Second}}
	proxy := httputil.NewSingleHostReverseProxy(core)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "Máy chủ HSPO chưa sẵn sàng, hãy khởi động HQTN_HSPO.", 502)
	}
	mux := http.NewServeMux()
	mux.HandleFunc(routePrefix, g.serve)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) { proxy.ServeHTTP(w, r) })
	ln, e := net.Listen("tcp", gatewayListen)
	if e != nil {
		log.Fatal(e)
	}
	log.Printf("HQTN_HSPO 1.9.8 Gateway trên cổng 8867 (Core 8865)")
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Fatal(server.Serve(ln))
}
