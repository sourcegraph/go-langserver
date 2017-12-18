# Go Language Server [![Build Status](https://travis-ci.org/sourcegraph/go-langserver.svg)](https://travis-ci.org/sourcegraph/go-langserver)

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
| Go |   x   |      x      |        x        |         x         |       x       |     x    |     x    |

## Profiling

If you run into performance issues while using the language server, it can be very helpful to attach a CPU or memory profile with the issue report. To capture one, first [install Go](https://golang.org/doc/install) and then:

Capture a heap (memory) profile:

```bash
go tool pprof -svg $GOPATH/bin/go-langserver http://localhost:6060/debug/pprof/heap > heap.svg
```

Capture a CPU profile:

```bash
go tool pprof -svg $GOPATH/bin/go-langserver http://localhost:6060/debug/pprof/profile > cpu.svg
```

Since these capture the active resource usage, it's best to run these commands while the issue is occuring (i.e. while memory or CPU is high).
