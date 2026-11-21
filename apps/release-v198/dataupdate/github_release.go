package dataupdate

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

const githubDataAsset = "DATA_UPDATE.zip"

var githubRepositoryName = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9-]{0,38}/[a-zA-Z0-9_.-]{1,100}$`)

type githubReleaseSource struct {
	repository string
	token      string
	client     *http.Client
}

type githubReleaseAsset struct {
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
	State  string `json:"state"`
}

type GitHubSource = githubReleaseSource
type GitHubAsset = githubReleaseAsset

type GitHubRelease struct {
	Tag        string        `json:"tag_name"`
	Draft      bool          `json:"draft"`
	Prerelease bool          `json:"prerelease"`
	Published  time.Time     `json:"published_at"`
	Assets     []GitHubAsset `json:"assets"`
}

func NewGitHubSource(repository, token string) *GitHubSource {
	return newGitHubReleaseSource(repository, token)
}

func (s *githubReleaseSource) List(ctx context.Context) ([]GitHubRelease, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := s.request(ctx, "?per_page=30", "application/vnd.github+json")
	if err != nil {
		return nil, errors.New("Không kết nối được GitHub để đọc bản phát hành")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Không đọc được bản phát hành GitHub (HTTP %d). Kiểm tra quyền đọc kho", resp.StatusCode)
	}
	var releases []GitHubRelease
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&releases) != nil {
		return nil, errors.New("Không đọc được danh sách bản phát hành GitHub")
	}
	return releases, nil
}

func (s *githubReleaseSource) Download(ctx context.Context, asset GitHubAsset) (io.ReadCloser, error) {
	if asset.ID <= 0 || asset.State != "uploaded" || asset.Size <= 0 {
		return nil, errors.New("Thông tin tệp GitHub không hợp lệ")
	}
	response, err := s.request(ctx, fmt.Sprintf("/assets/%d", asset.ID), "application/octet-stream")
	if err != nil {
		return nil, errors.New("Không tải được tệp từ GitHub")
	}
	if response.StatusCode != http.StatusOK || (response.ContentLength >= 0 && response.ContentLength != asset.Size) {
		response.Body.Close()
		return nil, errors.New("Tệp tải xuống không khớp thông tin phát hành")
	}
	return response.Body, nil
}

func newGitHubReleaseSource(repository, token string) *githubReleaseSource {
	repository, token = strings.TrimSpace(repository), strings.TrimSpace(token)
	if !githubRepositoryName.MatchString(repository) || strings.HasSuffix(repository, "/.") || strings.HasSuffix(repository, "/..") || token == "" {
		return nil
	}
	return &githubReleaseSource{
		repository: repository,
		token:      token,
		client:     &http.Client{Timeout: 8 * time.Minute, CheckRedirect: githubAssetRedirect},
	}
}

func githubAssetRedirect(req *http.Request, via []*http.Request) error {
	host := strings.ToLower(req.URL.Hostname())
	if len(via) > 3 || req.URL.Scheme != "https" || req.URL.User != nil || req.URL.Port() != "" ||
		!(host == "api.github.com" || strings.HasSuffix(host, ".githubusercontent.com")) {
		return errors.New("GitHub chuyển hướng tải gói đến địa chỉ không được phép")
	}
	req.Header.Del("Authorization")
	return nil
}

func (s *githubReleaseSource) request(ctx context.Context, suffix, accept string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		"https://api.github.com/repos/"+s.repository+"/releases"+suffix, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", accept)
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	return s.client.Do(req)
}

func (s *githubReleaseSource) latestDataAsset(ctx context.Context) (string, githubReleaseAsset, error) {
	releases, err := s.List(ctx)
	if err != nil {
		return "", githubReleaseAsset{}, err
	}
	var selected githubReleaseAsset
	var tag string
	var published time.Time
	for _, release := range releases {
		if release.Draft || release.Prerelease || release.Tag == "" || release.Published.IsZero() ||
			!release.Published.After(published) {
			continue
		}
		for _, asset := range release.Assets {
			if asset.Name == githubDataAsset {
				tag, published, selected = release.Tag, release.Published, asset
				break
			}
		}
	}
	if tag == "" {
		return "", selected, errors.New("Chưa có DATA_UPDATE.zip trong các bản phát hành gần đây")
	}
	if selected.ID <= 0 || selected.State != "uploaded" || selected.Size <= 0 || selected.Size > maxPackBytes {
		return "", githubReleaseAsset{}, errors.New("Thông tin tệp DATA_UPDATE.zip không hợp lệ")
	}
	digest, ok := strings.CutPrefix(selected.Digest, "sha256:")
	hash, err := hex.DecodeString(digest)
	if !ok || err != nil || len(hash) != 32 {
		return "", githubReleaseAsset{}, errors.New("GitHub chưa cung cấp SHA-256 cho gói dữ liệu")
	}
	selected.Digest = hex.EncodeToString(hash)
	return tag, selected, nil
}

func (g *Gateway) githubSource() *GitHubSource {
	if g.github != nil || g.privateSource == nil {
		return g.github
	}
	return g.privateSource()
}

func (g *Gateway) previewGitHubPackage(w http.ResponseWriter, r *http.Request, username string) {
	if err := g.postCheck(r); err != nil {
		jerr(w, http.StatusForbidden, err.Error())
		return
	}
	source := g.githubSource()
	if source == nil {
		jerr(w, http.StatusServiceUnavailable, "Máy chủ chưa cấu hình nguồn GitHub")
		return
	}
	tag, asset, err := source.latestDataAsset(r.Context())
	if err != nil {
		jerr(w, http.StatusBadGateway, err.Error())
		return
	}
	body, err := source.Download(r.Context(), asset)
	if err != nil {
		jerr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer body.Close()
	g.previewDataPackage(w, body, username, asset.Digest, tag)
}
