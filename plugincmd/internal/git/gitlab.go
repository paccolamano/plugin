package git

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/paccolamano/plugin/plugincmd/internal/httputil"
)

type glRelease struct {
	TagName string   `json:"tag_name"`
	Assets  glAssets `json:"assets"`
}

type glAssets struct {
	Sources []glSource `json:"sources"`
}

type glSource struct {
	Format string `json:"format"`
	URL    string `json:"url"`
}

type glClient struct {
	client *httputil.Client
}

func newGLClient(opts ...httputil.ClientOption) *glClient {
	c := httputil.NewClient(
		httputil.WithBaseURL("https://gitlab.com/api/v4"),
		httputil.WithHeader("Content-Type", "application/json"),
		httputil.WithHeader("User-Agent", "pocketbase-plugin-manager"),
	)
	for _, opt := range opts {
		opt(c)
	}
	return &glClient{client: c}
}

func (glc *glClient) GetRelease(ctx context.Context, repo, version string) (*Release, error) {
	projectID := url.PathEscape(repo)
	latest := version == "" || version == "latest"

	var path string
	var q url.Values
	if latest {
		path = fmt.Sprintf("/projects/%s/releases", projectID)
		q = url.Values{"per_page": {"1"}, "order_by": {"released_at"}, "sort": {"desc"}}
	} else {
		path = fmt.Sprintf("/projects/%s/releases/%s", projectID, url.PathEscape(version))
	}

	resp, err := glc.client.DoRequest(ctx, http.MethodGet, path, q, -1, nil, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		if latest {
			return nil, fmt.Errorf("repository %q not found", repo)
		}
		return nil, fmt.Errorf("repository %q: release %q not found", repo, version)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("repository %q: authentication required or access denied (HTTP %d)", repo, resp.StatusCode)
	default:
		return nil, fmt.Errorf("repository %q: GitLab API error: %s", repo, resp.Status)
	}

	var r glRelease
	if latest {
		var releases []glRelease
		if err := json.NewDecoder(resp.Body).Decode(&releases); err != nil {
			return nil, fmt.Errorf("failed to parse releases: %w", err)
		}
		if len(releases) == 0 {
			return nil, fmt.Errorf("repository %q has no releases", repo)
		}
		r = releases[0]
	} else {
		if err := json.NewDecoder(resp.Body).Decode(&r); err != nil {
			return nil, fmt.Errorf("failed to parse release: %w", err)
		}
	}

	return toRelease(r, repo)
}

func (glc *glClient) DownloadRelease(ctx context.Context, rawURL string) (io.ReadCloser, error) {
	resp, err := glc.client.DoAbsolute(ctx, http.MethodGet, rawURL, nil, -1, nil, nil)
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

func toRelease(r glRelease, repo string) (*Release, error) {
	for _, s := range r.Assets.Sources {
		if s.Format == "tar.gz" {
			return &Release{TagName: r.TagName, TarballURL: s.URL}, nil
		}
	}
	return nil, fmt.Errorf("repository %q: release %q has no tar.gz source", repo, r.TagName)
}
