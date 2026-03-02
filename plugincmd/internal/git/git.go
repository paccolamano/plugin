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

func NewClient(provider, token, serverURL string) (GitClient, error) {
	opts := []util.HTTPClientOption{util.WithToken(token)}
	if serverURL != "" {
		opts = append(opts, util.WithBaseURL(serverURL))
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
