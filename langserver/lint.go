package langserver

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"go/build"
	"log"
	"os/exec"
	"path"
	"strconv"
	"strings"

	"github.com/sourcegraph/go-langserver/pkg/lsp"
	"github.com/sourcegraph/go-langserver/pkg/tools"
	"github.com/sourcegraph/jsonrpc2"
)

const (
	lintToolGolint = "golint"
	lintToolNone   = "none"
)

// Linter defines an interface for linting
type Linter interface {
	IsInstalled(ctx context.Context, bctx *build.Context) error
	Lint(ctx context.Context, bctx *build.Context, args ...string) (diagnostics, error)
}

// lint runs the configured lint command with the given arguments then published the
// results as diagnostics.
func (h *LangHandler) lint(ctx context.Context, bctx *build.Context, conn jsonrpc2.JSONRPC2, args []string, files []string) error {
	if h.linter == nil {
		return nil
	}

	diags, err := h.linter.Lint(ctx, bctx, args...)
	if err != nil {
		return err
	}

	return h.publishDiagnostics(ctx, conn, diags, h.config.LintTool, files)
}

// lintPackage runs LangHandler.lint for the package containing the uri.
func (h *LangHandler) lintPackage(ctx context.Context, bctx *build.Context, conn jsonrpc2.JSONRPC2, uri lsp.DocumentURI) error {
	filename := h.FilePath(uri)
	pkg, err := ContainingPackage(h.BuildContext(ctx), filename, h.RootFSPath)
	if err != nil {
		return err
	}

	files := make([]string, 0, len(pkg.GoFiles))
	for _, f := range pkg.GoFiles {
		files = append(files, path.Join(pkg.Dir, f))
	}
	return h.lint(ctx, bctx, conn, []string{path.Dir(filename)}, files)
}

// lintWorkspace runs LangHandler.lint for the entire workspace
func (h *LangHandler) lintWorkspace(ctx context.Context, bctx *build.Context, conn jsonrpc2.JSONRPC2) error {
	var files []string
	pkgs := tools.ListPkgsUnderDir(bctx, h.RootFSPath)
	find := h.getFindPackageFunc(h.RootFSPath)
	for _, pkg := range pkgs {
		p, err := find(ctx, bctx, pkg, h.RootFSPath, 0)
		if err != nil {
			if _, ok := err.(*build.NoGoError); ok {
				continue
			}
			if _, ok := err.(*build.MultiplePackageError); ok {
				continue
			}
			return err
		}

		for _, f := range p.GoFiles {
			files = append(files, path.Join(p.Dir, f))
		}
	}
	return h.lint(ctx, bctx, conn, []string{path.Join(h.RootFSPath, "/...")}, files)
}

// golint is a wrapper around the golint command that implements the
// linter interface.
type golint struct{}

func (l golint) IsInstalled(ctx context.Context, bctx *build.Context) error {
	_, err := exec.LookPath("golint")
	return err
}

func (l golint) Lint(ctx context.Context, bctx *build.Context, args ...string) (diagnostics, error) {
	cmd := exec.CommandContext(ctx, "golint", args...)
	cmd.Env = []string{
		"GOPATH=" + bctx.GOPATH,
		"GOROOT=" + bctx.GOROOT,
	}

	errBuff := new(bytes.Buffer)
	cmd.Stderr = errBuff

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, fmt.Errorf("lint command error: %s", err)
	}
	defer stdout.Close()

	err = cmd.Start()
	if err != nil {
		return nil, fmt.Errorf("lint command error: %s", err)
	}

	diags := diagnostics{}
	scanner := bufio.NewScanner(stdout)

	for scanner.Scan() {
		text := scanner.Text()
		file, line, _, message, err := parseLintResult(text)
		if err != nil {
			// If there is an error parsing a line still try to parse the remaining lines
			log.Printf("warning: error failed to parse lint result: %v", err)
			continue
		}

		diags[file] = append(diags[file], &lsp.Diagnostic{
			Message:  message,
			Severity: lsp.Warning,
			Source:   lintToolGolint,
			Range: lsp.Range{ // currently only supporting line level errors
				Start: lsp.Position{
					Line:      line,
					Character: 0,
				},
				End: lsp.Position{
					Line:      line,
					Character: 0,
				},
			},
		})
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("could not read lint command output: %s", err)
	}

	cmd.Wait()
	if err != nil {
		return nil, fmt.Errorf("lint command error: %s", err)
	}

	if errBuff.Len() > 0 {
		log.Panicf("warning: lint command stderr: %q", errBuff.String())
	}

	return diags, nil
}

func parseLintResult(l string) (file string, line, char int, message string, err error) {
	parts := strings.SplitN(l, " ", 2)
	if len(parts) != 2 {
		err = fmt.Errorf("invalid result %q", l)
		return
	}
	location := parts[0]
	message = parts[1]

	parts = strings.Split(location, ":")
	if l := len(parts); l < 3 || l > 4 {
		err = fmt.Errorf("invalid file location %q in %q", location, l)
		return
	}
	file = parts[0]
	line, err = strconv.Atoi(parts[1])
	if err != nil {
		err = fmt.Errorf("invalid line number in %q: %s", l, err)
		return
	}
	if parts[2] != "" {
		char, err = strconv.Atoi(parts[2])
		if err != nil {
			err = fmt.Errorf("invalid char number in %q: %s", l, err)
			return
		}
	}

	return file, line - 1, char - 1, message, nil // LSP is 0-indexed
}
