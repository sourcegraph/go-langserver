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
	"runtime"
	"runtime/debug"
	"time"

	"github.com/sourcegraph/go-langserver/langserver"
	"github.com/sourcegraph/jsonrpc2"

	_ "net/http/pprof"
)

var (
	mode               = flag.String("mode", "stdio", "communication mode (stdio|tcp)")
	addr               = flag.String("addr", ":4389", "server listen address (tcp)")
	trace              = flag.Bool("trace", false, "print all requests and responses")
	logfile            = flag.String("logfile", "", "also log to this file (in addition to stderr)")
	printVersion       = flag.Bool("version", false, "print version and exit")
	pprof              = flag.String("pprof", ":6060", "start a pprof http server (https://golang.org/pkg/net/http/pprof/)")
	freeosmemory       = flag.Bool("freeosmemory", true, "aggressively free memory back to the OS")
	usebinarypkgcache  = flag.Bool("usebinarypkgcache", true, "use $GOPATH/pkg binary .a files (improves performance)")
	maxparallelism     = flag.Int("maxparallelism", -1, "use at max N parallel goroutines to fulfill requests")
	gocodecompletion   = flag.Bool("gocodecompletion", false, "enable completion (extra memory burden)")
	funcSnippetEnabled = flag.Bool("func-snippet-enabled", true, "enable argument snippets on func completion")
)

// version is the version field we report back. If you are releasing a new version:
// 1. Create commit without -dev suffix.
// 2. Create commit with version incremented and -dev suffix
// 3. Push to master
// 4. Tag the commit created in (1) with the value of the version string
const version = "v2-dev"

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
	langserver.UseBinaryPkgCache = *usebinarypkgcache

	// Default max parallelism to half the CPU cores, but at least always one.
	if *maxparallelism <= 0 {
		*maxparallelism = runtime.NumCPU() / 2
		if *maxparallelism <= 0 {
			*maxparallelism = 1
		}
	}
	langserver.MaxParallelism = *maxparallelism

	langserver.GocodeCompletionEnabled = *gocodecompletion

	langserver.FuncSnippetEnabled = *funcSnippetEnabled

	cfg := langserver.Config{}

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

	handler := langserver.NewHandler(cfg)

	switch *mode {
	case "tcp":
		lis, err := net.Listen("tcp", *addr)
		if err != nil {
			return err
		}
		defer lis.Close()

		log.Println("langserver-go: listening on", *addr)
		for {
			conn, err := lis.Accept()
			if err != nil {
				return err
			}
			jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(conn, jsonrpc2.VSCodeObjectCodec{}), handler, connOpt...)
		}

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
