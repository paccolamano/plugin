package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/paccolamano/plugincmd/internal/httputil"
)

type ghRelease struct {
	TagName    string `json:"tag_name"`
	TarballURL string `json:"tarball_url"`
}

type ghClient struct {
	client *httputil.Client
}

func newGHClient(opts ...httputil.ClientOption) *ghClient {
	c := httputil.NewClient(
		httputil.WithBaseURL("https://api.github.com"),
		httputil.WithHeader("Accept", "application/vnd.github+json"),
		httputil.WithHeader("X-GitHub-Api-Version", "2022-11-28"),
		httputil.WithHeader("User-Agent", "pocketbase-plugin-manager"),
	)
	for _, opt := range opts {
		opt(c)
	}
	return &ghClient{client: c}
}

func (ghc *ghClient) GetRelease(ctx context.Context, repo, version string) (*Release, error) {
	var path string
	if version == "" || version == "latest" {
		path = fmt.Sprintf("/repos/%s/releases/latest", repo)
	} else {
		tag := version
		if !strings.HasPrefix(tag, "tags/") {
			tag = "tags/" + tag
		}
		path = fmt.Sprintf("/repos/%s/releases/%s", repo, tag)
	}

	resp, err := ghc.client.DoRequest(ctx, http.MethodGet, path, nil, -1, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return nil, fmt.Errorf("repository %q: release %q not found", repo, version)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("repository %q: authentication required or access denied (HTTP %d)", repo, resp.StatusCode)
	default:
		return nil, fmt.Errorf("repository %q: GitHub API error: %s", repo, resp.Status)
	}

	var r ghRelease
	if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
		return nil, fmt.Errorf("failed to parse release: %w", err)
	}
	return &Release{TagName: r.TagName, TarballURL: r.TarballURL}, nil
}

func (ghc *ghClient) DownloadRelease(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	resp, err := ghc.client.DoAbsolute(context.Background(), http.MethodGet, rawURL, nil, -1, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("download failed: %w", err)
	}
	switch resp.StatusCode {
	case http.StatusOK:
		return resp.Body, nil
	case http.StatusUnauthorized, http.StatusForbidden:
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: authentication required or access denied (HTTP %d)", resp.StatusCode)
	default:
		resp.Body.Close()
		return nil, fmt.Errorf("download failed: HTTP %s", resp.Status)
	}
}
