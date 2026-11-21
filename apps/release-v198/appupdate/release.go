package appupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"hspo/release-v198/dataupdate"
)

const (
	Repository   = "hwngdo/HQTN_HSPO"
	SetupName    = "HQTN_HSPO_QA_Setup_Win64.exe"
	ManifestName = "HQTN_HSPO_QA_Release.json"
	versionFile  = "APP_RELEASE.txt"
	maxSetup     = 500 << 20
	maxManifest  = 64 << 10
)

// Set by the installer build. The signing key stays on the release machine.
var releasePublicKey string

type Manifest struct {
	Format  string `json:"format"`
	Version string `json:"version"`
	Notes   string `json:"notes"`
	File    string `json:"file"`
	Size    int64  `json:"size"`
	SHA256  string `json:"sha256"`
}

type envelope struct {
	Payload   []byte `json:"payload"`
	Signature []byte `json:"signature"`
}

type Candidate struct {
	Manifest Manifest
	Signed   []byte
	Setup    dataupdate.GitHubAsset
}

type Source interface {
	List(context.Context) ([]dataupdate.GitHubRelease, error)
	Download(context.Context, dataupdate.GitHubAsset) (io.ReadCloser, error)
}

func parseVersion(value string) ([3]uint32, error) {
	var version [3]uint32
	fields := strings.Split(value, ".")
	if len(fields) != len(version) {
		return version, fmt.Errorf("phiên bản không hợp lệ: %q", value)
	}
	for i, field := range fields {
		number, err := strconv.ParseUint(field, 10, 16)
		if err != nil || strconv.FormatUint(number, 10) != field {
			return version, fmt.Errorf("phiên bản không hợp lệ: %q", value)
		}
		version[i] = uint32(number)
	}
	return version, nil
}

func ValidVersion(value string) bool {
	_, err := parseVersion(value)
	return err == nil
}

func newer(value, current string) bool {
	next, nextErr := parseVersion(value)
	prev, prevErr := parseVersion(current)
	if nextErr != nil || prevErr != nil {
		return false
	}
	for i := range next {
		if next[i] != prev[i] {
			return next[i] > prev[i]
		}
	}
	return false
}

func InstalledVersion(root string) (string, error) {
	marker := filepath.Join(root, "HQTN_HSPO_QA_INSTALL.txt")
	if f, err := os.Lstat(marker); err != nil || !f.Mode().IsRegular() {
		return "", errors.New("cần bản được cài bằng HQTN_HSPO QA Setup để cập nhật chương trình")
	}
	b, err := os.ReadFile(filepath.Join(root, versionFile))
	if errors.Is(err, os.ErrNotExist) {
		return "1.9.8", nil
	}
	value := strings.TrimSpace(string(b))
	if err != nil || !ValidVersion(value) {
		return "", errors.New("không xác định được phiên bản chương trình đã cài")
	}
	return value, nil
}

func publicKey() (ed25519.PublicKey, error) {
	key, err := hex.DecodeString(releasePublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return nil, errors.New("bộ cài chưa có khóa xác minh bản phát hành")
	}
	return key, nil
}

func verify(key ed25519.PublicKey, b []byte) (Manifest, error) {
	var m Manifest
	var signed envelope
	if len(key) != ed25519.PublicKeySize || len(b) > maxManifest ||
		json.Unmarshal(b, &signed) != nil || !ed25519.Verify(key, signed.Payload, signed.Signature) {
		return m, errors.New("chữ ký bản phát hành không hợp lệ")
	}
	if err := json.Unmarshal(signed.Payload, &m); err != nil {
		return m, err
	}
	hash, err := hex.DecodeString(m.SHA256)
	if err != nil || len(hash) != sha256.Size || m.Format != "HQTN_HSPO_QA_RELEASE_V1" ||
		m.File != SetupName || m.Size <= 0 || m.Size > maxSetup || !ValidVersion(m.Version) || len(m.Notes) > 12000 {
		return m, errors.New("thông tin bản phát hành không hợp lệ")
	}
	return m, nil
}

func Sign(key ed25519.PrivateKey, version, notes, setup string) ([]byte, error) {
	file, err := os.Open(setup)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	hash := sha256.New()
	size, err := io.Copy(hash, io.LimitReader(file, maxSetup+1))
	if err != nil {
		return nil, err
	}
	m := Manifest{"HQTN_HSPO_QA_RELEASE_V1", version, notes, SetupName, size, hex.EncodeToString(hash.Sum(nil))}
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	signed, err := json.Marshal(envelope{payload, ed25519.Sign(key, payload)})
	if err != nil {
		return nil, err
	}
	if _, err := verify(key.Public().(ed25519.PublicKey), signed); err != nil {
		return nil, err
	}
	return signed, nil
}

func Check(ctx context.Context, current, token string) (*Candidate, error) {
	key, err := publicKey()
	if err != nil {
		return nil, err
	}
	source := dataupdate.NewGitHubSource(Repository, token)
	if source == nil {
		return nil, errors.New("cần cấp quyền đọc GitHub một lần trong menu Tray")
	}
	return check(ctx, source, key, current)
}

func check(ctx context.Context, source Source, key ed25519.PublicKey, current string) (*Candidate, error) {
	releases, err := source.List(ctx)
	if err != nil {
		return nil, err
	}
	var best string
	var setup, manifest dataupdate.GitHubAsset
	for _, release := range releases {
		version, ok := strings.CutPrefix(release.Tag, "qa-v")
		if !ok || release.Draft || release.Prerelease || release.Published.IsZero() ||
			!newer(version, current) || (best != "" && !newer(version, best)) {
			continue
		}
		var nextSetup, nextManifest dataupdate.GitHubAsset
		for _, asset := range release.Assets {
			switch asset.Name {
			case SetupName:
				nextSetup = asset
			case ManifestName:
				nextManifest = asset
			}
		}
		if nextSetup.ID > 0 && nextManifest.ID > 0 {
			best, setup, manifest = version, nextSetup, nextManifest
		}
	}
	if best == "" {
		return nil, nil
	}
	if manifest.Size <= 0 || manifest.Size > maxManifest {
		return nil, errors.New("manifest bản phát hành quá lớn")
	}
	body, err := source.Download(ctx, manifest)
	if err != nil {
		return nil, err
	}
	defer body.Close()
	b, err := io.ReadAll(io.LimitReader(body, maxManifest+1))
	if err != nil {
		return nil, err
	}
	m, err := verify(key, b)
	if err != nil {
		return nil, err
	}
	if m.Version != best || m.Size != setup.Size || setup.State != "uploaded" {
		return nil, errors.New("gói cài không khớp phiên bản đã ký")
	}
	return &Candidate{m, b, setup}, nil
}

func PendingDir(root string) string {
	return filepath.Join(root, "program_updates", "pending")
}

func Queue(ctx context.Context, root, token string, c *Candidate) error {
	source := dataupdate.NewGitHubSource(Repository, token)
	if source == nil {
		return errors.New("cần cấp quyền đọc GitHub trong menu Tray")
	}
	return queue(ctx, root, source, c)
}

func queue(ctx context.Context, root string, source Source, c *Candidate) error {
	parent := filepath.Dir(PendingDir(root))
	if err := os.MkdirAll(parent, 0700); err != nil {
		return err
	}
	stage, err := os.MkdirTemp(parent, "download-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(stage)
	ctx, cancel := context.WithTimeout(ctx, 8*time.Minute)
	defer cancel()
	body, err := source.Download(ctx, c.Setup)
	if err != nil {
		return err
	}
	defer body.Close()
	setup, err := os.OpenFile(filepath.Join(stage, SetupName), os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	hash := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(setup, hash), io.LimitReader(body, c.Manifest.Size+1))
	if closeErr := setup.Close(); copyErr != nil || closeErr != nil || n != c.Manifest.Size ||
		hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(c.Manifest.SHA256) {
		return errors.New("SHA-256 hoặc dung lượng bộ cài không khớp")
	}
	if err := os.WriteFile(filepath.Join(stage, ManifestName), c.Signed, 0600); err != nil {
		return err
	}
	if err := os.Rename(stage, PendingDir(root)); err != nil {
		return errors.New("đã có cập nhật chờ cài hoặc không thể bàn giao gói cài")
	}
	return nil
}

func VerifyPending(root, dir string) (Manifest, error) {
	key, err := publicKey()
	if err != nil {
		return Manifest{}, err
	}
	return verifyPending(root, dir, key)
}

func verifyPending(root, dir string, key ed25519.PublicKey) (Manifest, error) {
	current, err := InstalledVersion(root)
	if err != nil {
		return Manifest{}, err
	}
	file, err := os.Open(filepath.Join(dir, ManifestName))
	if err != nil {
		return Manifest{}, err
	}
	b, err := io.ReadAll(io.LimitReader(file, maxManifest+1))
	file.Close()
	if err != nil {
		return Manifest{}, err
	}
	m, err := verify(key, b)
	if err != nil {
		return Manifest{}, err
	}
	if !newer(m.Version, current) {
		return Manifest{}, errors.New("chỉ cài bản có phiên bản mới hơn")
	}
	setup, err := os.Open(filepath.Join(dir, SetupName))
	if err != nil {
		return Manifest{}, err
	}
	defer setup.Close()
	hash := sha256.New()
	n, err := io.Copy(hash, io.LimitReader(setup, m.Size+1))
	if err != nil || n != m.Size || hex.EncodeToString(hash.Sum(nil)) != strings.ToLower(m.SHA256) {
		return Manifest{}, errors.New("gói chờ cài không khớp SHA-256 đã ký")
	}
	return m, nil
}
