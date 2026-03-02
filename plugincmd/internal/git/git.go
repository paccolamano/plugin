package git

import (
	"context"
	"fmt"
	"io"

	"github.com/paccolamano/plugin/plugincmd/internal/httputil"
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
	opts := []httputil.ClientOption{httputil.WithToken(token)}
	if serverURL != "" {
		opts = append(opts, httputil.WithBaseURL(serverURL))
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
