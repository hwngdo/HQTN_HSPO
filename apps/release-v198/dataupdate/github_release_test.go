package dataupdate

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type githubTestTransport func(*http.Request) (*http.Response, error)

func (transport githubTestTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	return transport(r)
}

func TestPreviewGitHubReleaseReusesApprovalFlow(t *testing.T) {
	g, closeServer := makeG(t)
	defer closeServer()
	cleanZIP := packageBytes(t, true, true)
	releaseHash := sha(cleanZIP)
	var calls int
	g.github = newGitHubReleaseSource("fixture/hspo", "fixture-token")
	g.github.client.Transport = githubTestTransport(func(r *http.Request) (*http.Response, error) {
		calls++
		if r.URL.Host != "api.github.com" || r.Header.Get("Authorization") != "Bearer fixture-token" {
			t.Fatalf("unexpected GitHub request: %s", r.URL)
		}
		var body []byte
		switch r.URL.Path {
		case "/repos/fixture/hspo/releases":
			body = []byte(fmt.Sprintf(`[
				{"tag_name":"app-v2","published_at":"2026-11-22T07:00:00Z","assets":[]},
				{"tag_name":"data-qa","published_at":"2026-11-23T07:00:00Z","prerelease":true,"assets":[{"id":2,"name":"DATA_UPDATE.zip"}]},
				{"tag_name":"data-v1","published_at":"2026-11-20T07:00:00Z","assets":[{"id":7,"name":"DATA_UPDATE.zip","state":"uploaded","digest":"sha256:%s","size":%d}]}
			]`, releaseHash, len(cleanZIP)))
		case "/repos/fixture/hspo/releases/assets/7":
			body = cleanZIP
		default:
			return nil, fmt.Errorf("unexpected path: %s", r.URL.Path)
		}
		return &http.Response{StatusCode: 200, ContentLength: int64(len(body)), Body: io.NopCloser(bytes.NewReader(body))}, nil
	})
	_, state := request(t, g, "GET", routePrefix+"status", "user=admin", "", "", nil)
	token := state["csrf"].(string)
	endpoint := routePrefix + "github-preview"
	denied, _ := request(t, g, "POST", endpoint, "user=staff", token, "", nil)
	badCSRF, _ := request(t, g, "POST", endpoint, "user=admin", "", "", nil)
	if denied != 403 || badCSRF != 403 || calls != 0 {
		t.Fatalf("GitHub was contacted without administrator authorization")
	}
	target := filepath.Join(g.root, "policy_data", "chemical_rules.json")
	before, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	status, preview := request(t, g, "POST", endpoint, "user=admin", token, "", nil)
	if status != 200 || preview["release_tag"] != "data-v1" || preview["can_apply"] != true {
		t.Fatalf("preview failed: %d %v", status, preview)
	}
	after, _ := os.ReadFile(target)
	if !bytes.Equal(before, after) {
		t.Fatal("preview modified operational data")
	}
	stage := g.stages[preview["id"].(string)]
	if err := os.WriteFile(stage.Path, packageBytes(t, true, false), 0600); err != nil {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"id": preview["id"], "confirm": "AP_DUNG"})
	status, _ = request(t, g, "POST", routePrefix+"apply", "user=admin", token, "application/json", body)
	if status != 400 {
		t.Fatal("changed staged ZIP was accepted")
	}
	releaseHash = strings.Repeat("0", 64)
	status, _ = request(t, g, "POST", endpoint, "user=admin", token, "", nil)
	if status != 400 || len(g.stages) != 1 {
		t.Fatal("digest mismatch left a new approved preview")
	}
}

func TestGitHubRedirectDropsCredentialsAndRejectsOtherHosts(t *testing.T) {
	dest, _ := url.Parse("https://release-assets.githubusercontent.com/path")
	r := &http.Request{URL: dest, Header: http.Header{"Authorization": {"Bearer fixture"}}}
	if err := githubAssetRedirect(r, nil); err != nil || r.Header.Get("Authorization") != "" {
		t.Fatal("valid storage redirect failed to remove authorization")
	}
	r.URL, _ = url.Parse("https://release-assets.githubusercontent.com.example.test/path")
	if githubAssetRedirect(r, nil) == nil {
		t.Fatal("allowed redirect outside GitHub")
	}
}
