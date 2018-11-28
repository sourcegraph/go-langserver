package gosrc

import (
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strings"
	"sync"

	"github.com/sourcegraph/sourcegraph/pkg/conf"
)

// Adapted from github.com/golang/gddo/gosrc.

// RuntimeVersion is the version of go stdlib to use. We allow it to be
// different to runtime.Version for test data.
var RuntimeVersion = runtime.Version()

type noGoGetDomainsT struct {
	mu      sync.RWMutex
	domains []string
}

func (n *noGoGetDomainsT) get() []string {
	n.mu.RLock()
	defer n.mu.RUnlock()
	return n.domains
}

func (n *noGoGetDomainsT) reconfigure() {
	n.mu.Lock()
	defer n.mu.Unlock()

	// Parse noGoGetDomains to avoid needing to validate them when
	// resolving static import paths
	n.domains = parseCommaSeparatedList(conf.Get().NoGoGetDomains)
}

func parseCommaSeparatedList(list string) []string {
	split := strings.Split(list, ",")
	i := 0
	for _, s := range split {
		s = strings.TrimSpace(s)
		if s != "" {
			split[i] = s
			i++
		}
	}
	return split[:i]
}

// noGoGetDomains is a list of domains we do not attempt standard go vanity
// import resolution. Instead we take an educated guess based on the URL how
// to create the directory struct.
var noGoGetDomains = &noGoGetDomainsT{}

func init() {
	conf.Watch(noGoGetDomains.reconfigure)
}

type Directory struct {
	ImportPath  string // the Go import path for this package
	ProjectRoot string // import path prefix for all packages in the project
	CloneURL    string // the VCS clone URL
	RepoPrefix  string // the path to this directory inside the repo, if set
	VCS         string // one of "git", "hg", "svn", etc.
	Rev         string // the VCS revision specifier, if any
}

var errNoMatch = errors.New("no match")

func ResolveImportPath(client *http.Client, importPath string) (*Directory, error) {
	if d, err := resolveStaticImportPath(importPath); err == nil {
		return d, nil
	} else if err != nil && err != errNoMatch {
		return nil, err
	}
	return resolveDynamicImportPath(client, importPath)
}

func resolveStaticImportPath(importPath string) (*Directory, error) {
	if IsStdlibPkg(importPath) {
		return &Directory{
			ImportPath:  importPath,
			ProjectRoot: "",
			CloneURL:    "https://github.com/golang/go",
			RepoPrefix:  "src",
			VCS:         "git",
			Rev:         RuntimeVersion,
		}, nil
	}

	// This allows users to set a list of domains that we should NEVER perform
	// go get or git clone against. This is useful when e.g. a user has not
	// correctly configured a monorepo and we are constantly hitting their
	// production website to resolve import paths like "facebook.com/pkg/util"
	// and skewing their own 404 metrics. This DOES mean these imports will be
	// broken until they do correctly configure their monorepo (so we can
	// identify its GOPATH), but it gives them a quick escape hatch that is
	// better than "turn off the Sourcegraph server".
	for _, domain := range conf.Get().BlacklistGoGet {
		if strings.HasPrefix(importPath, domain) {
			return nil, errors.New("import path in blacklistGoGet configuration")
		}
	}

	// This allows a user to set a list of domains that are considered to be
	// non-go-gettable, i.e. standard git repositories. Some on-prem customers
	// use setups like this, where they directly import non-go-gettable git
	// repository URLs like "mygitolite.aws.me.org/mux.git/subpkg"
	for _, domain := range noGoGetDomains.get() {
		if !strings.HasPrefix(importPath, domain) {
			continue
		}
		d, err := guessImportPath(importPath)
		if d != nil || err != nil {
			return d, err
		}
	}

	switch {
	case strings.HasPrefix(importPath, "github.com/"):
		parts := strings.SplitN(importPath, "/", 4)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid github.com/golang.org import path: %q", importPath)
		}
		repo := parts[0] + "/" + parts[1] + "/" + parts[2]
		return &Directory{
			ImportPath:  importPath,
			ProjectRoot: repo,
			CloneURL:    "https://" + repo,
			VCS:         "git",
		}, nil

	case strings.HasPrefix(importPath, "golang.org/x/"):
		d, err := resolveStaticImportPath(strings.Replace(importPath, "golang.org/x/", "github.com/golang/", 1))
		if err != nil {
			return nil, err
		}
		d.ProjectRoot = strings.Replace(d.ProjectRoot, "github.com/golang/", "golang.org/x/", 1)
		return d, nil
	}
	return nil, errNoMatch
}

// guessImportPath is used by noGoGetDomains since we can't do the usual
// go get resolution.
func guessImportPath(importPath string) (*Directory, error) {
	if !strings.Contains(importPath, ".git") {
		// Assume GitHub-like where two path elements is the project
		// root.
		parts := strings.SplitN(importPath, "/", 4)
		if len(parts) < 3 {
			return nil, fmt.Errorf("invalid GitHub-like import path: %q", importPath)
		}
		repo := parts[0] + "/" + parts[1] + "/" + parts[2]
		return &Directory{
			ImportPath:  importPath,
			ProjectRoot: repo,
			CloneURL:    "http://" + repo,
			VCS:         "git",
		}, nil
	}

	// TODO(slimsag): We assume that .git only shows up
	// once in the import path. Not always true, but generally
	// should be in 99% of cases.
	split := strings.Split(importPath, ".git")
	if len(split) != 2 {
		return nil, fmt.Errorf("expected one .git in %q", importPath)
	}

	return &Directory{
		ImportPath:  importPath,
		ProjectRoot: split[0] + ".git",
		CloneURL:    "http://" + split[0] + ".git",
		VCS:         "git",
	}, nil
}

// gopkgSrcTemplate matches the go-source dir templates specified by the
// popular gopkg.in
var gopkgSrcTemplate = regexp.MustCompile(`https://(github.com/[^/]*/[^/]*)/tree/([^/]*)\{/dir\}`)

func resolveDynamicImportPath(client *http.Client, importPath string) (*Directory, error) {
	metaProto, im, sm, err := fetchMeta(client, importPath)
	if err != nil {
		return nil, err
	}

	if im.prefix != importPath {
		var imRoot *importMeta
		metaProto, imRoot, _, err = fetchMeta(client, im.prefix)
		if err != nil {
			return nil, err
		}
		if *imRoot != *im {
			return nil, fmt.Errorf("project root mismatch: %q != %q", *imRoot, *im)
		}
	}

	// clonePath is the repo URL from import meta tag, with the "scheme://" prefix removed.
	// It should be used for cloning repositories.
	// repo is the repo URL from import meta tag, with the "scheme://" prefix removed, and
	// a possible ".vcs" suffix trimmed.
	i := strings.Index(im.repo, "://")
	if i < 0 {
		return nil, fmt.Errorf("bad repo URL: %s", im.repo)
	}
	clonePath := im.repo[i+len("://"):]
	repo := strings.TrimSuffix(clonePath, "."+im.vcs)
	dirName := importPath[len(im.prefix):]

	var dir *Directory
	if sm != nil {
		m := gopkgSrcTemplate.FindStringSubmatch(sm.dirTemplate)
		if len(m) > 0 {
			// We are doing best effort, so we ignore err
			dir, _ = resolveStaticImportPath(m[1] + dirName)
			if dir != nil {
				dir.Rev = m[2]
			}
		}
	}

	if dir == nil {
		// We are doing best effort, so we ignore err
		dir, _ = resolveStaticImportPath(repo + dirName)
	}

	if dir == nil {
		dir = &Directory{}
	}
	dir.ImportPath = importPath
	dir.ProjectRoot = im.prefix
	if dir.CloneURL == "" {
		dir.CloneURL = metaProto + "://" + repo + "." + im.vcs
	}
	dir.VCS = im.vcs
	return dir, nil
}

// importMeta represents the values in a go-import meta tag.
//
// See https://golang.org/cmd/go/#hdr-Remote_import_paths.
type importMeta struct {
	prefix string // the import path corresponding to the repository root
	vcs    string // one of "git", "hg", "svn", etc.
	repo   string // root of the VCS repo containing a scheme and not containing a .vcs qualifier
}

// sourceMeta represents the values in a go-source meta tag.
type sourceMeta struct {
	prefix       string
	projectURL   string
	dirTemplate  string
	fileTemplate string
}

func fetchMeta(client *http.Client, importPath string) (scheme string, im *importMeta, sm *sourceMeta, err error) {
	uri := importPath
	if !strings.Contains(uri, "/") {
		// Add slash for root of domain.
		uri = uri + "/"
	}
	uri = uri + "?go-get=1"

	scheme = "https"
	resp, err := client.Get(scheme + "://" + uri)
	if err != nil || resp.StatusCode != 200 {
		if err == nil {
			resp.Body.Close()
		}
		scheme = "http"
		resp, err = client.Get(scheme + "://" + uri)
		if err != nil {
			return scheme, nil, nil, err
		}
	}
	defer resp.Body.Close()
	im, sm, err = parseMeta(scheme, importPath, resp.Body)
	return scheme, im, sm, err
}

func parseMeta(scheme, importPath string, r io.Reader) (im *importMeta, sm *sourceMeta, err error) {
	errorMessage := "go-import meta tag not found"

	d := xml.NewDecoder(r)
	d.Strict = false
metaScan:
	for {
		t, tokenErr := d.Token()
		if tokenErr != nil {
			break metaScan
		}
		switch t := t.(type) {
		case xml.EndElement:
			if strings.EqualFold(t.Name.Local, "head") {
				break metaScan
			}
		case xml.StartElement:
			if strings.EqualFold(t.Name.Local, "body") {
				break metaScan
			}
			if !strings.EqualFold(t.Name.Local, "meta") {
				continue metaScan
			}
			nameAttr := attrValue(t.Attr, "name")
			if nameAttr != "go-import" && nameAttr != "go-source" {
				continue metaScan
			}
			fields := strings.Fields(attrValue(t.Attr, "content"))
			if len(fields) < 1 {
				continue metaScan
			}
			prefix := fields[0]
			if !strings.HasPrefix(importPath, prefix) ||
				!(len(importPath) == len(prefix) || importPath[len(prefix)] == '/') {
				// Ignore if root is not a prefix of the  path. This allows a
				// site to use a single error page for multiple repositories.
				continue metaScan
			}
			switch nameAttr {
			case "go-import":
				if len(fields) != 3 {
					errorMessage = "go-import meta tag content attribute does not have three fields"
					continue metaScan
				}
				if im != nil {
					im = nil
					errorMessage = "more than one go-import meta tag found"
					break metaScan
				}
				im = &importMeta{
					prefix: prefix,
					vcs:    fields[1],
					repo:   fields[2],
				}
			case "go-source":
				if sm != nil {
					// Ignore extra go-source meta tags.
					continue metaScan
				}
				if len(fields) != 4 {
					continue metaScan
				}
				sm = &sourceMeta{
					prefix:       prefix,
					projectURL:   fields[1],
					dirTemplate:  fields[2],
					fileTemplate: fields[3],
				}
			}
		}
	}
	if im == nil {
		return nil, nil, fmt.Errorf("%s at %s://%s", errorMessage, scheme, importPath)
	}
	if sm != nil && sm.prefix != im.prefix {
		sm = nil
	}
	return im, sm, nil
}

func attrValue(attrs []xml.Attr, name string) string {
	for _, a := range attrs {
		if strings.EqualFold(a.Name.Local, name) {
			return a.Value
		}
	}
	return ""
}
