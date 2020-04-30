package vfsutil

import (
	"context"
	"fmt"
	"regexp"

	"github.com/prometheus/client_golang/prometheus"
)

// NewGitHubRepoVFS creates a new VFS backed by a GitHub downloadable
// repository archive.
func NewGitHubRepoVFS(ctx context.Context, repo, rev string) (*ArchiveFS, error) {
	if !githubRepoRx.MatchString(repo) {
		return nil, fmt.Errorf(`invalid GitHub repo %q: must be "github.com/user/repo"`, repo)
	}

	url := fmt.Sprintf("https://codeload.%s/zip/%s", repo, rev)
	return NewZipVFS(ctx, url, ghFetch.Inc, ghFetchFailed.Inc, false)
}

var githubRepoRx = regexp.MustCompile(`^github\.com/[\w.-]{1,100}/[\w.-]{1,100}$`)

var ghFetch = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "golangserver_vfs_github_fetch_total",
	Help: "Total number of fetches by GitHubRepoVFS.",
})
var ghFetchFailed = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "golangserver_vfs_github_fetch_failed_total",
	Help: "Total number of fetches by GitHubRepoVFS that failed.",
})

func init() {
	prometheus.MustRegister(ghFetch)
	prometheus.MustRegister(ghFetchFailed)
}
