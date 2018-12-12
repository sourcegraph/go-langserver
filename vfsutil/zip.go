package vfsutil

import (
	"context"
	"encoding/base64"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"golang.org/x/net/context/ctxhttp"

	"github.com/fhs/go-netrc/netrc"
	"github.com/pkg/errors"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/opentracing/opentracing-go/ext"
)

// NewZipVFS downloads a zip archive from a URL (or fetches from the local cache
// on disk) and returns a new VFS backed by that zip archive.
func NewZipVFS(ctx context.Context, url string, onFetchStart, onFetchFailed func(), evictOnClose bool) (*ArchiveFS, error) {
	request, err := http.NewRequest("HEAD", url, nil)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to construct a new request with URL %s", url)
	}
	err = setAuthFromNetrc(request)
	response, err := ctxhttp.Do(ctx, nil, request)
	if err != nil {
		log.Printf("Unable to set auth from netrc: %s", err)
	}
	if err != nil {
		return nil, err
	}
	if response.StatusCode != http.StatusOK {
		return nil, nil
	}

	fetch := func(ctx context.Context) (ar *archiveReader, err error) {
		span, ctx := opentracing.StartSpanFromContext(ctx, "zip Fetch")
		ext.Component.Set(span, "zipvfs")
		span.SetTag("url", url)
		defer func() {
			if err != nil {
				ext.Error.Set(span, true)
				span.SetTag("err", err)
			}
			span.Finish()
		}()

		ff, err := cachedFetch(ctx, "zipvfs", url, func(ctx context.Context) (io.ReadCloser, error) {
			onFetchStart()
			request, err := http.NewRequest("GET", url, nil)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to construct a new request with URL %s", url)
			}
			request.Header.Add("Accept", "application/zip")
			err = setAuthFromNetrc(request)
			if err != nil {
				log.Printf("Unable to set auth from netrc: %s", err)
			}
			resp, err := ctxhttp.Do(ctx, nil, request)
			if err != nil {
				return nil, errors.Wrapf(err, "failed to fetch zip archive from %s", url)
			}
			if resp.StatusCode != http.StatusOK {
				resp.Body.Close()
				return nil, errors.Errorf("zip URL %s returned HTTP %d", url, resp.StatusCode)
			}
			return resp.Body, nil
		})
		if err != nil {
			onFetchFailed()
			return nil, errors.Wrapf(err, "failed to fetch/write/open zip archive from %s", url)
		}
		f := ff.File

		zr, err := zipNewFileReader(f)
		if err != nil {
			f.Close()
			return nil, errors.Wrapf(err, "failed to read zip archive from %s", url)
		}

		if len(zr.File) == 0 {
			f.Close()
			return nil, errors.Errorf("zip archive from %s is empty", url)
		}

		return &archiveReader{
			Reader:           zr,
			Closer:           f,
			StripTopLevelDir: true,
		}, nil
	}

	return &ArchiveFS{fetch: fetch, EvictOnClose: evictOnClose}, nil
}

func setAuthFromNetrc(req *http.Request) error {
	host := req.URL.Host
	if i := strings.Index(host, ":"); i != -1 {
		host = host[:i]
	}
	netrcFile := os.ExpandEnv("$HOME/.netrc")
	if _, err := os.Stat(netrcFile); os.IsNotExist(err) {
		return nil
	}
	machine, err := netrc.FindMachine(netrcFile, host)
	if err != nil {
		return err
	}
	if machine == nil {
		return nil
	}
	req.Header.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(fmt.Sprintf("%s:%s", machine.Login, machine.Password))))
	return nil
}
