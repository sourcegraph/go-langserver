package main // import "github.com/sourcegraph/go-langserver"

import (
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/sourcegraph/go-langserver/buildserver"

	"github.com/gorilla/websocket"
	"github.com/sourcegraph/go-langserver/langserver"
	"github.com/sourcegraph/jsonrpc2"

	_ "net/http/pprof"
)

var (
	mode           = flag.String("mode", "stdio", "communication mode (stdio|tcp|websocket)")
	addr           = flag.String("addr", ":4389", "server listen address (tcp or websocket)")
	trace          = flag.Bool("trace", false, "print all requests and responses")
	logfile        = flag.String("logfile", "", "also log to this file (in addition to stderr)")
	printVersion   = flag.Bool("version", false, "print version and exit")
	pprof          = flag.String("pprof", "", "start a pprof http server (https://golang.org/pkg/net/http/pprof/)")
	freeosmemory   = flag.Bool("freeosmemory", true, "aggressively free memory back to the OS")
	useBuildServer = flag.Bool("usebuildserver", false, "use a build server to fetch dependencies, fetch files via Zip URL, etc.")
	// TODO remove blacklistGoGet from sourcegraph/sourcegraph https://sourcegraph.sgdev.org/search?q=repo:%5Egithub%5C.com/sourcegraph/enterprise%24+blacklistgoget
	noGoGetDomains = flag.String("nogogetdomains", "", "List of domains in import paths to NOT perform `go get` on, but instead treat as standard Git repositories. Separated by ','. For example, if your code imports non-go-gettable packages like `\"mygitolite.aws.me.org/mux.git/subpkg\"` you may set this option to `\"mygitolite.aws.me.org\"` and the build server will effectively run `git clone mygitolite.aws.me.org/mux.git` instead of performing the usual `go get` dependency resolution behavior.")
	blacklistGoGet = flag.String("blacklistgoget", "", "List of domains to blacklist dependency fetching from. Separated by ','. Unlike `noGoGetDomains` (which tries to use a hueristic to determine where to clone the dependencies from), this option outright prevents fetching of dependencies with the given domain name. This will prevent code intelligence from working on these dependencies, so most users should not use this option.")

	// Default Config, can be overridden by InitializationOptions
	usebinarypkgcache  = flag.Bool("usebinarypkgcache", true, "use $GOPATH/pkg binary .a files (improves performance). Can be overridden by InitializationOptions.")
	maxparallelism     = flag.Int("maxparallelism", 0, "use at max N parallel goroutines to fulfill requests. Can be overridden by InitializationOptions.")
	gocodecompletion   = flag.Bool("gocodecompletion", false, "enable completion (extra memory burden). Can be overridden by InitializationOptions.")
	diagnostics        = flag.Bool("diagnostics", false, "enable diagnostics (extra memory burden). Can be overridden by InitializationOptions.")
	funcSnippetEnabled = flag.Bool("func-snippet-enabled", true, "enable argument snippets on func completion. Can be overridden by InitializationOptions.")
	formatTool         = flag.String("format-tool", "goimports", "which tool is used to format documents. Supported: goimports and gofmt. Can be overridden by InitializationOptions.")
	lintTool           = flag.String("lint-tool", "none", "which tool is used to linting. Supported: none and golint. Can be overridden by InitializationOptions.")
)

// version is the version field we report back. If you are releasing a new version:
// 1. Create commit without -dev suffix.
// 2. Create commit with version incremented and -dev suffix
// 3. Push to master
// 4. Tag the commit created in (1) with the value of the version string
const version = "v3-dev"

func main() {
	flag.Parse()
	log.SetFlags(0)

	// Start pprof server, if desired.
	if *pprof != "" {
		go func() {
			log.Println(http.ListenAndServe(*pprof, nil))
		}()
	}

	if *freeosmemory {
		go freeOSMemory()
	}

	cfg := langserver.NewDefaultConfig()
	cfg.FuncSnippetEnabled = *funcSnippetEnabled
	cfg.GocodeCompletionEnabled = *gocodecompletion
	cfg.DiagnosticsEnabled = *diagnostics
	cfg.UseBinaryPkgCache = *usebinarypkgcache
	cfg.FormatTool = *formatTool
	cfg.LintTool = *lintTool

	if *maxparallelism > 0 {
		cfg.MaxParallelism = *maxparallelism
	}

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg langserver.Config) error {
	if *printVersion {
		fmt.Println(version)
		return nil
	}

	var logW io.Writer
	if *logfile == "" {
		logW = os.Stderr
	} else {
		f, err := os.Create(*logfile)
		if err != nil {
			return err
		}
		defer f.Close()
		logW = io.MultiWriter(os.Stderr, f)
	}
	log.SetOutput(logW)

	var connOpt []jsonrpc2.ConnOpt
	if *trace {
		connOpt = append(connOpt, jsonrpc2.LogMessages(log.New(logW, "", 0)))
	}

	noGoGetDomainsSlice := parseCommaSeparatedList(*noGoGetDomains)
	blacklistGoGetSlice := parseCommaSeparatedList(*blacklistGoGet)

	var handler jsonrpc2.Handler
	if useBuildServer != nil && *useBuildServer {
		handler = buildserver.NewHandler(cfg, noGoGetDomainsSlice, blacklistGoGetSlice)
		buildserver.FetchCommonDeps(noGoGetDomainsSlice, blacklistGoGetSlice)
	} else {
		handler = langserver.NewHandler(cfg)
	}

	switch *mode {
	case "tcp":
		lis, err := net.Listen("tcp", *addr)
		if err != nil {
			return err
		}
		defer lis.Close()

		log.Println("langserver-go: listening for TCP connections on", *addr)

		newConnectionCount := 0

		for {
			conn, err := lis.Accept()
			if err != nil {
				return err
			}
			newConnectionCount = newConnectionCount + 1
			connectionID := newConnectionCount
			log.Printf("langserver-go: received incoming WebSocket connection #%d\n", connectionID)
			jsonrpc2Connection := jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{}), handler, connOpt...)
			go func() {
				<-jsonrpc2Connection.DisconnectNotify()
				log.Printf("langserver-go: disconnected WebSocket connection #%d\n", connectionID)
			}()
		}

	case "websocket":
		mux := http.NewServeMux()

		newConnectionCount := 0

		mux.HandleFunc("/", func(responseWriter http.ResponseWriter, request *http.Request) {
			var upgrader = websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return true }}
			connection, err := upgrader.Upgrade(responseWriter, request, nil)
			if err != nil {
				log.Println("error upgrading HTTP to WebSocket:", err)
			}
			defer connection.Close()
			newConnectionCount = newConnectionCount + 1
			connectionID := newConnectionCount
			log.Printf("langserver-go: received incoming WebSocket connection #%d\n", connectionID)

			// TODO figure out if it's possible to share the handler across
			// connections. I had to create a new handler on every new connection,
			// otherwise it would throw an "already initialized" error.
			var handler jsonrpc2.Handler
			if useBuildServer != nil && *useBuildServer {
				handler = buildserver.NewHandler(cfg, noGoGetDomainsSlice, blacklistGoGetSlice)
				buildserver.FetchCommonDeps(noGoGetDomainsSlice, blacklistGoGetSlice)
			} else {
				handler = langserver.NewHandler(cfg)
			}
			<-jsonrpc2.NewConn(context.Background(), NewObjectStream(connection), handler, connOpt...).DisconnectNotify()
			log.Printf("langserver-go: disconnected WebSocket connection #%d\n", connectionID)
		})

		log.Println("langserver-go: listening for WebSocket connections on", *addr)
		http.ListenAndServe(*addr, mux)
		return nil

	case "stdio":
		log.Println("langserver-go: reading on stdin, writing on stdout")
		<-jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(stdrwc{}, jsonrpc2.VSCodeObjectCodec{}), handler, connOpt...).DisconnectNotify()
		log.Println("connection closed")
		return nil

	default:
		return fmt.Errorf("invalid mode %q", *mode)
	}
}

type stdrwc struct{}

func (stdrwc) Read(p []byte) (int, error) {
	return os.Stdin.Read(p)
}

func (stdrwc) Write(p []byte) (int, error) {
	return os.Stdout.Write(p)
}

func (stdrwc) Close() error {
	if err := os.Stdin.Close(); err != nil {
		return err
	}
	return os.Stdout.Close()
}

// freeOSMemory should be called in a goroutine, it invokes
// runtime/debug.FreeOSMemory() more aggressively than the runtime default of
// 5 minutes after GC.
//
// There is a long-standing known issue with Go in which memory is not returned
// to the OS aggressively enough[1], which coincidently harms our application
// quite a lot because we perform so many short-burst heap allocations during
// the type-checking phase.
//
// This function should only be invoked in editor mode, not in sourcegraph.com
// mode, because users running the language server as part of their editor
// generally expect much lower memory usage. In contrast, on sourcegraph.com we
// can give our servers plenty of RAM and allow Go to consume as much as it
// wants. Go does reuse the memory not free'd to the OS, and as such enabling
// this does _technically_ make our application perform less optimally -- but
// in practice this has no observable effect in editor mode.
//
// The end effect of performing this is that repeating "hover over code" -> "make an edit"
// 10 times inside a large package like github.com/docker/docker/cmd/dockerd:
//
//
// 	| Real Before | Real After | Real Change | Go Before | Go After | Go Change |
// 	|-------------|------------|-------------|-----------|----------|-----------|
// 	| 7.61GB      | 4.12GB     | -45.86%     | 3.92GB    | 3.33GB   | -15.05%   |
//
// Where `Real` means real memory reported by OS X Activity Monitor, and `Go`
// means memory reported by Go as being in use.
//
// TL;DR: 46% less memory consumption for users running with the vscode-go extension.
//
// [1] https://github.com/golang/go/issues/14735#issuecomment-194470114
func freeOSMemory() {
	for {
		time.Sleep(1 * time.Second)
		debug.FreeOSMemory()
	}
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
