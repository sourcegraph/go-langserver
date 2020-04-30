package buildserver

import (
	"context"
	"fmt"
	"io"
	"io/ioutil"
	"net/url"
	"path"
	"strings"

	"github.com/pkg/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/sourcegraph/ctxvfs"
	"github.com/sourcegraph/go-langserver/vfsutil"
	"github.com/sourcegraph/go-lsp/lspext"
)

// RemoteFS fetches a zip archive from the URL specified in the zipURL field of
// the initializationOptions and returns a virtual file system interface for
// accessing the files in the specified repo at the given commit.
//
// SECURITY NOTE: This DOES NOT check that the user or context has permissions
// to read the repo. We assume permission checks happen before a request reaches
// a build server.
var RemoteFS = func(ctx context.Context, initializeParams lspext.InitializeParams) (ctxvfs.FileSystem, io.Closer, error) {
	zipURL := func() string {
		initializationOptions, ok := initializeParams.InitializationOptions.(map[string]interface{})
		if !ok {
			return ""
		}
		zipURL, _ := initializationOptions["zipURL"].(string)
		if zipURL != "" {
			return zipURL
		}
		zipURLTemplate, _ := initializationOptions["zipURLTemplate"].(string)
		if zipURLTemplate == "" {
			return ""
		}
		root, err := url.Parse(string(initializeParams.OriginalRootURI))
		if err != nil {
			return ""
		}
		return fmt.Sprintf(zipURLTemplate, path.Join(root.Host, root.Path), root.RawQuery)
	}()
	if zipURL != "" {
		vfs, err := vfsutil.NewZipVFS(ctx, zipURL, zipFetch.Inc, zipFetchFailed.Inc, true)
		return vfs, vfs, err
	}
	return nil, ioutil.NopCloser(strings.NewReader("")), errors.Errorf("no zipURL was provided in the initializationOptions")
}

var zipFetch = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "golangserver_vfs_zip_fetch_total",
	Help: "Total number of times a zip archive was fetched for the currently-viewed repo.",
})
var zipFetchFailed = prometheus.NewCounter(prometheus.CounterOpts{
	Name: "golangserver_vfs_zip_fetch_failed_total",
	Help: "Total number of times fetching a zip archive for the currently-viewed repo failed.",
})

func init() {
	prometheus.MustRegister(zipFetch)
	prometheus.MustRegister(zipFetchFailed)
}
