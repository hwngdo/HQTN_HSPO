package appupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"hspo/release-v198/dataupdate"
)

type fixtureSource struct {
	releases []dataupdate.GitHubRelease
	payloads map[int64][]byte
}

type fixtureTransport func(*http.Request) (*http.Response, error)

func (transport fixtureTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return transport(r)
}

func (f fixtureSource) List(context.Context) ([]dataupdate.GitHubRelease, error) {
	return f.releases, nil
}

func (f fixtureSource) Download(_ context.Context, asset dataupdate.GitHubAsset) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.payloads[asset.ID])), nil
}

func TestSignedReleaseDiscoveryAndQueue(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"HQTN_HSPO_QA_INSTALL.txt", "APP_RELEASE.txt"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("1.9.9"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	setup := []byte("synthetic setup")
	path := filepath.Join(root, SetupName)
	if err := os.WriteFile(path, setup, 0600); err != nil {
		t.Fatal(err)
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	notes := "Sửa giao diện và thêm ghi chú phát hành."
	envelope, err := Sign(private, "1.9.10", notes, path)
	if err != nil {
		t.Fatal(err)
	}
	assets := []dataupdate.GitHubAsset{
		{ID: 1, Name: SetupName, Size: int64(len(setup)), State: "uploaded"},
		{ID: 2, Name: ManifestName, Size: int64(len(envelope)), State: "uploaded"},
	}
	now := time.Now()
	source := fixtureSource{
		releases: []dataupdate.GitHubRelease{
			{Tag: "qa-v1.9.11", Draft: true, Published: now, Assets: assets},
			{Tag: "qa-v1.9.12", Prerelease: true, Published: now, Assets: assets},
			{Tag: "qa-v1.9.10", Published: now, Assets: assets},
			{Tag: "qa-v1.9.9", Published: now, Assets: assets},
		},
		payloads: map[int64][]byte{1: setup, 2: envelope},
	}
	ctx := context.Background()
	candidate, err := check(ctx, source, public, "1.9.9")
	if err != nil || candidate == nil || candidate.Manifest.Version != "1.9.10" || candidate.Manifest.Notes != notes {
		t.Fatalf("check release: %#v, %v", candidate, err)
	}
	if same, err := check(ctx, source, public, "1.9.10"); same != nil || err != nil {
		t.Fatalf("should be up to date: %#v, %v", same, err)
	}
	if _, err := os.Stat(PendingDir(root)); !os.IsNotExist(err) {
		t.Fatalf("checking must not stage or install anything: %v", err)
	}

	source.payloads[1] = bytes.Repeat([]byte{0}, len(setup))
	if err := queue(ctx, root, source, candidate); err == nil {
		t.Fatal("accepted a download with a wrong SHA-256")
	}
	source.payloads[1] = setup
	if err := queue(ctx, root, source, candidate); err != nil {
		t.Fatal(err)
	}
	if err := queue(ctx, root, source, candidate); err == nil {
		t.Fatal("allowed a second pending update")
	}
	if _, err := verifyPending(root, PendingDir(root), public); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(PendingDir(root), SetupName), []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := verifyPending(root, PendingDir(root), public); err == nil {
		t.Fatal("accepted a modified staged installer")
	}
	other, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := check(ctx, source, other, "1.9.9"); err == nil {
		t.Fatal("accepted a manifest signed by an untrusted key")
	}
}

func TestWebOffersSignedNotesAndRequiresExactApproval(t *testing.T) {
	root := t.TempDir()
	setup := []byte("setup fixture")
	for name, content := range map[string][]byte{
		"HQTN_HSPO_QA_INSTALL.txt": []byte("QA"),
		"APP_RELEASE.txt":          []byte("1.9.9"),
		SetupName:                  setup,
	} {
		if err := os.WriteFile(filepath.Join(root, name), content, 0600); err != nil {
			t.Fatal(err)
		}
	}
	public, private, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	const notes = "Cập nhật biểu mẫu và giao diện."
	signed, err := Sign(private, "1.9.10", notes, filepath.Join(root, SetupName))
	if err != nil {
		t.Fatal(err)
	}
	releases, err := json.Marshal([]dataupdate.GitHubRelease{
		{Tag: "qa-v1.9.10", Published: time.Now(), Assets: []dataupdate.GitHubAsset{
			{Name: SetupName, ID: 1, Size: int64(len(setup)), State: "uploaded"},
			{Name: ManifestName, ID: 2, Size: int64(len(signed)), State: "uploaded"},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	original, originalKey := http.DefaultTransport, releasePublicKey
	t.Cleanup(func() { http.DefaultTransport, releasePublicKey = original, originalKey })
	t.Setenv("HSPO_GITHUB_REPOSITORY", Repository)
	t.Setenv("HSPO_GITHUB_READ_TOKEN", "fixture-test-token")
	releasePublicKey = hex.EncodeToString(public)
	prefix := "/repos/" + Repository + "/releases"
	downloads := 0
	http.DefaultTransport = fixtureTransport(func(r *http.Request) (*http.Response, error) {
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer fixture-test-token" {
			t.Fatalf("unexpected release request: %s", r.URL)
		}
		var body []byte
		switch r.URL.Path {
		case prefix:
			body = releases
		case prefix + "/assets/1":
			downloads++
			body = setup
		case prefix + "/assets/2":
			body = signed
		default:
			t.Fatalf("unexpected release endpoint %s", r.URL)
		}
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(body)), ContentLength: int64(len(body))}, nil
	})
	handler := WebHandler(root, func() bool { return true })
	post := func(action, body string) *httptest.ResponseRecorder {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest("POST", dataupdate.RoutePrefix+action, strings.NewReader(body)))
		return response
	}
	check := post("program-check", "")
	var offer struct {
		Current string
		Release Manifest
	}
	if err := json.Unmarshal(check.Body.Bytes(), &offer); err != nil || check.Code != 200 ||
		offer.Current != "1.9.9" || offer.Release.Version != "1.9.10" || offer.Release.Notes != notes {
		t.Fatalf("check result: %d %s (%v)", check.Code, check.Body.String(), err)
	}
	if response := post("program-install", `{"version":"1.9.11"}`); response.Code != 409 || downloads != 0 {
		t.Fatalf("wrong version downloaded: %d (%d downloads)", response.Code, downloads)
	}
	if response := post("program-install", `{"version":"1.9.10"}`); response.Code != 200 || downloads != 1 {
		t.Fatalf("install response: %d %s (%d downloads)", response.Code, response.Body.String(), downloads)
	}
	if _, err := VerifyPending(root, PendingDir(root)); err != nil {
		t.Fatal(err)
	}
}
