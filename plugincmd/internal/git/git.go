package git

import (
	"context"
	"fmt"
	"io"
	"net/http"

	"github.com/paccolamano/plugin/plugincmd/internal/util"
)

type Release struct {
	TagName    string
	TarballURL string
}

type GitClient interface {
	GetRelease(ctx context.Context, repo, version string) (*Release, error)
	DownloadRelease(ctx context.Context, rawURL string) (io.ReadCloser, error)
}

// NewClient creates a Git provider client.
// apiBase is the API base URL (e.g. "https://api.github.com" for GitHub,
// "https://gitlab.com/api/v4" for GitLab). Use apiBaseURL to derive it from
// a web URL.
func NewClient(provider, apiBase, token string) (GitClient, error) {
	opts := []util.HTTPClientOption{
		util.WithToken(token),
		util.WithBaseURL(apiBase),
	}
	switch provider {
	case "github", "gitea", "forgejo":
		return newGHClient(opts...), nil
	case "gitlab":
		return newGLClient(opts...), nil
	default:
		return nil, fmt.Errorf("unknown provider %q (supported: github, gitea, forgejo, gitlab)", provider)
	}
}

// APIBaseURL derives the API base URL from a provider name and the web base
// URL (scheme + host, e.g. "https://github.com").
func APIBaseURL(provider, webBase string) string {
	switch provider {
	case "github":
		if webBase == "https://github.com" {
			return "https://api.github.com"
		}
		return webBase // GitHub Enterprise: same host, no path prefix
	case "gitlab":
		return webBase + "/api/v4"
	default: // gitea, forgejo
		return webBase
	}
}

func downloadRelease(ctx context.Context, client *util.HTTPClient, rawURL string) (io.ReadCloser, error) {
	resp, err := client.DoAbsoluteRequest(ctx, http.MethodGet, rawURL, nil, -1, nil, nil)
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
