package main

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
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	resp, err := s.request(ctx, "?per_page=30", "application/vnd.github+json")
	if err != nil {
		return "", githubReleaseAsset{}, errors.New("Không kết nối được GitHub để đọc bản phát hành")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", githubReleaseAsset{}, fmt.Errorf("Không đọc được bản phát hành GitHub (HTTP %d)", resp.StatusCode)
	}
	var releases []struct {
		Tag        string               `json:"tag_name"`
		Draft      bool                 `json:"draft"`
		Prerelease bool                 `json:"prerelease"`
		Published  time.Time            `json:"published_at"`
		Assets     []githubReleaseAsset `json:"assets"`
	}
	if json.NewDecoder(io.LimitReader(resp.Body, 2<<20)).Decode(&releases) != nil {
		return "", githubReleaseAsset{}, errors.New("Không đọc được danh sách bản phát hành GitHub")
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

func (g *Gateway) previewGitHubPackage(w http.ResponseWriter, r *http.Request, username string) {
	if err := g.postCheck(r); err != nil {
		jerr(w, http.StatusForbidden, err.Error())
		return
	}
	if g.github == nil {
		jerr(w, http.StatusServiceUnavailable, "Máy chủ chưa cấu hình nguồn GitHub")
		return
	}
	tag, asset, err := g.github.latestDataAsset(r.Context())
	if err != nil {
		jerr(w, http.StatusBadGateway, err.Error())
		return
	}
	response, err := g.github.request(r.Context(), fmt.Sprintf("/assets/%d", asset.ID), "application/octet-stream")
	if err != nil {
		jerr(w, http.StatusBadGateway, "Không tải được gói dữ liệu từ GitHub")
		return
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK || (response.ContentLength >= 0 && response.ContentLength != asset.Size) {
		jerr(w, http.StatusBadGateway, "Tệp tải xuống không khớp thông tin phát hành")
		return
	}
	g.previewDataPackage(w, response.Body, username, asset.Digest, tag)
}
