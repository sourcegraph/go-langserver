package vfsutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	"github.com/fhs/go-netrc/netrc"
	"github.com/pkg/errors"
	"github.com/sourcegraph/go-langserver/diskcache"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// NewZipVFS downloads a zip archive from a URL (or fetches from the local cache
// on disk) and returns a new VFS backed by that zip archive.
func NewZipVFS(ctx context.Context, urlString string, onFetchStart, onFetchFailed func(), evictOnClose bool) (*ArchiveFS, error) {
	request, err := http.NewRequest("HEAD", urlString, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct a new request with URL %s", urlString)
	}
	setAuthFromNetrc(request)
	response, err := ctxhttp.Do(ctx, nil, request)
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unable to fetch zip from %s (expected HTTP response code 200, but got %d)", urlString, response.StatusCode)
	}

	fetch := func(ctx context.Context) (ar *archiveReader, err error) {
		span, ctx := opentracing.StartSpanFromContext(ctx, "zip Fetch")
		ext.Component.Set(span, "zipvfs")
		span.SetTag("url", urlString)
		defer func() {
			if err != nil {
				ext.Error.Set(span, true)
				span.SetTag("err", err)
			}
			span.Finish()
		}()

		store := &diskcache.Store{
			Dir:               filepath.Join(ArchiveCacheDir, "zipvfs"),
			Component:         "zipvfs",
			MaxCacheSizeBytes: MaxCacheSizeBytes,
		}

		// Create a new URL that doesn't include the user:password (the access
		// token) so that the same repository at a revision for a different user
		// results in a cache hit.
		urlStruct, err := url.Parse(urlString)
		if err != nil {
			return nil, err
		}
		urlStruct.User = nil
		urlWithoutUser := urlStruct.String()

		ff, err := cachedFetch(ctx, urlWithoutUser, store, func(ctx context.Context) (io.ReadCloser, error) {
			onFetchStart()
			request, err := http.NewRequest("GET", urlString, nil)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to construct a new request with URL %s", urlString)
			}
			request.Header.Add("Accept", "application/zip")
			setAuthFromNetrc(request)
			fmt.Println("**** REQ ", urlString)
			resp, err := ctxhttp.Do(ctx, nil, request)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to fetch zip archive from %s", urlString)
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, errors.Errorf("zip URL %s returned HTTP %d", urlString, resp.StatusCode)
			}
			return resp.Body, nil
		})
		if err != nil {
			onFetchFailed()
			return nil, errors.Wrapf(err, "failed to fetch/write/open zip archive from %s", urlString)
		}
		f := ff.File

		zr, err := zipNewFileReader(f)
		if err != nil {
			f.Close()
			return nil, errors.Wrapf(err, "failed to read zip archive from %s", urlString)
		}

		if len(zr.File) == 0 {
			f.Close()
			return nil, errors.Errorf("zip archive from %s is empty", urlString)
		}

		return &archiveReader{
			Reader:           zr,
			Closer:           f,
			StripTopLevelDir: true,
			Evicter:          store,
		}, nil
	}

	return &ArchiveFS{fetch: fetch, EvictOnClose: evictOnClose}, nil
}

func setAuthFromNetrc(req *http.Request) {
	host := req.URL.Host
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}
	netrcFile := os.ExpandEnv("$HOME/.netrc")
	if _, err := os.Stat(netrcFile); os.IsNotExist(err) {
		return
	}
	machine, err := netrc.FindMachine(netrcFile, host)
	if err != nil || machine == nil {
		return
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", machine.Login, machine.Password))))
}
