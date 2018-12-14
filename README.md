# Go Language Server [![Build Status](https://travis-ci.org/sourcegraph/go-langserver.svg)](https://travis-ci.org/sourcegraph/go-langserver)

> *Note:* We have deprioritized work on this language server for use in
> editors in favor of Google's upcoming Go language server. It is in the best
> interests of the community to only have a single language server. If you
> want to use a Go language server with your editor in the meantime, try
> https://github.com/saibing/bingo, which is a partial fork of this repository
> with fixes for Go modules and other editor bugs.

go-langserver is a [Go](https://golang.org) language server that
speaks
[Language Server Protocol](https://github.com/Microsoft/language-server-protocol). It
supports editor features such as go-to-definition, hover, and find-references
for Go projects.

[**Open in Sourcegraph**](https://sourcegraph.com/github.com/sourcegraph/go-langserver/-/tree/langserver)

To build and install the standalone `go-langserver` run

```
go get -u github.com/sourcegraph/go-langserver
```

# Support

|    | Hover | Jump to def | Find references | Workspace symbols | VFS extension | Isolated | Parallel |
|----|-------|-------------|-----------------|-------------------|---------------|----------|----------|
| Go |   ✔   |      ✔      |        ✔        |         ✔         |       ✔       |     ✔    |     ✔    |

## InitializationOptions

If you are a client wanting to integrate go-langserver, you can use the following as `initializationOptions` in your [initialize](https://microsoft.github.io/language-server-protocol/specification#initialize) request to adjust the behaviour:

```typescript
interface GoInitializationOptions {
  /**
   * funcSnippetEnabled enables the returning of argument snippets
   * on `func` completions, eg. func(foo string, arg2 bar).
   * Requires code completion to be enabled.
   *
   * Defaults to true if not specified.
   */
  funcSnippetEnabled?: boolean;

  /**
   * gocodeCompletionEnabled enables code completion feature (using gocode).
   *
   * Defaults to false if not specified.
   */
  gocodeCompletionEnabled?: boolean;

  /**
   * formatTool decides which tool is used to format documents. Supported: goimports and gofmt.
   *
   * Defaults to goimports if not specified.
   */
  formatTool?: "goimports" | "gofmt";


  /**
   * lintTool decides which tool is used for linting documents. Supported: none and golint
   *
   * Diagnostics must be enabled for linting to work.
   *
   * Defaults to none if not specified.
   */
  lintTool?: "none" | "golint";

  /**
   * goimportsLocalPrefix sets the local prefix (comma-separated string) that goimports will use.
   *
   * Defaults to empty string if not specified.
   */
  goimportsLocalPrefix?: string;

  /**
   * MaxParallelism controls the maximum number of goroutines that should be used
   * to fulfill requests. This is useful in editor environments where users do
   * not want results ASAP, but rather just semi quickly without eating all of
   * their CPU.
   *
   * Defaults to half of your CPU cores if not specified.
   */
  maxParallelism?: number;

  /**
   * useBinaryPkgCache controls whether or not $GOPATH/pkg binary .a files should
   * be used.
   *
   * Defaults to true if not specified.
   */
  useBinaryPkgCache?: boolean;

  /**
   * DiagnosticsEnabled enables handling of diagnostics.
   *
   * Defaults to false if not specified.
   */
  diagnosticsEnabled?: boolean;
}
```

## Profiling

If you run into performance issues while using the language server, it can be very helpful to attach a CPU or memory profile with the issue report. To capture one, first [install Go](https://golang.org/doc/install), start `go-langserver` with the pprof flag (e.g. `$GOPATH/bin/go-langserver -pprof :6060`) and then:

Capture a heap (memory) profile:

```bash
go tool pprof -svg $GOPATH/bin/go-langserver http://localhost:6060/debug/pprof/heap > heap.svg
```

Capture a CPU profile:

```bash
go tool pprof -svg $GOPATH/bin/go-langserver http://localhost:6060/debug/pprof/profile > cpu.svg
```

Since these capture the active resource usage, it's best to run these commands while the issue is occurring (i.e. while memory or CPU is high).
