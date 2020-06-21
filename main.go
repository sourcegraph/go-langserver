package main // import "github.com/sourcegraph/go-langserver"

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"syscall/js"
	"time"

	"github.com/sourcegraph/go-langserver/debugserver"
	"github.com/sourcegraph/go-langserver/tracer"
	"github.com/sourcegraph/go-langserver/vfsutil"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/sourcegraph/go-langserver/buildserver"
	"github.com/sourcegraph/go-langserver/langserver"
	"github.com/sourcegraph/jsonrpc2"

	_ "net/http/pprof"
)

var (
	mode              = flag.String("mode", "stdio", "communication mode (stdio|tcp|websocket)")
	addr              = flag.String("addr", ":4389", "server listen address (tcp or websocket)")
	trace             = flag.Bool("trace", false, "print all requests and responses")
	logfile           = flag.String("logfile", "", "also log to this file (in addition to stderr)")
	printVersion      = flag.Bool("version", false, "print version and exit")
	pprof             = flag.String("pprof", "", "start a pprof http server (https://golang.org/pkg/net/http/pprof/)")
	freeosmemory      = flag.Bool("freeosmemory", true, "aggressively free memory back to the OS")
	useBuildServer    = flag.Bool("usebuildserver", false, "use a build server to fetch dependencies, fetch files via Zip URL, etc.")
	cacheDir          = flag.String("cachedir", "/tmp", "directory to store cached archives")
	maxCacheSizeBytes = flag.Int64("maxCacheSizeBytes", 50*1024*1024*1024, "the maximum size of the cache directory after evicting entries")

	// Default Config, can be overridden by InitializationOptions
	usebinarypkgcache  = flag.Bool("usebinarypkgcache", true, "use $GOPATH/pkg binary .a files (improves performance). Can be overridden by InitializationOptions.")
	maxparallelism     = flag.Int("maxparallelism", 0, "use at max N parallel goroutines to fulfill requests. Can be overridden by InitializationOptions.")
	gocodecompletion   = flag.Bool("gocodecompletion", false, "enable completion (extra memory burden). Can be overridden by InitializationOptions.")
	diagnostics        = flag.Bool("diagnostics", false, "enable diagnostics (extra memory burden). Can be overridden by InitializationOptions.")
	funcSnippetEnabled = flag.Bool("func-snippet-enabled", true, "enable argument snippets on func completion. Can be overridden by InitializationOptions.")
	formatTool         = flag.String("format-tool", "goimports", "which tool is used to format documents. Supported: goimports and gofmt. Can be overridden by InitializationOptions.")
	lintTool           = flag.String("lint-tool", "none", "which tool is used to linting. Supported: none and golint. Can be overridden by InitializationOptions.")

	openGauge = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "golangserver_build_open_connections",
		Help: "Number of open connections to the language server.",
	})
)

func init() {
	prometheus.MustRegister(openGauge)
}

// version is the version field we report back. If you are releasing a new version:
// 1. Create commit without -dev suffix.
// 2. Create commit with version incremented and -dev suffix
// 3. Push to master
// 4. Tag the commit created in (1) with the value of the version string
const version = "v3-dev"

func main() {
	flag.Parse()
	log.SetFlags(0)

	vfsutil.ArchiveCacheDir = filepath.Join(*cacheDir, "lang-go-archive-cache")
	vfsutil.MaxCacheSizeBytes = *maxCacheSizeBytes

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

	js.Global().Set("wasmGoProcess", js.FuncOf(process));

	run(cfg)

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(cfg langserver.Config) error {
	tracer.Init()

	go debugserver.Start()

	if *useBuildServer {
		// If go-langserver crashes, all the archives it has cached are not
		// evicted. Over time this leads to us filling up the disk. This is a
		// simple fix were we do a best-effort purge of the cache.
		// https://github.com/sourcegraph/sourcegraph/issues/6090
		_ = os.RemoveAll(vfsutil.ArchiveCacheDir)

		// PERF: Hide latency of fetching golang/go from the first typecheck
		go buildserver.FetchCommonDeps()
	}

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

	newHandler := func() (jsonrpc2.Handler, io.Closer) {
		if *useBuildServer {
			handler := buildserver.NewHandler(cfg)
			return jsonrpc2.AsyncHandler(jsonrpc2.HandlerWithError(handler.Handle)), handler
		}
		return langserver.NewHandler(cfg), ioutil.NopCloser(strings.NewReader(""))
	}

	switch *mode {
	case "stdio":
		log.Println("langserver-go: reading on stdin, writing on stdout")
		handler, closer := newHandler()
		<-jsonrpc2.NewConn(context.Background(), jsonrpc2.NewBufferedStream(wasmRWC{}, jsonrpc2.VSCodeObjectCodec{}), handler, connOpt...).DisconnectNotify()
		err := closer.Close()
		if err != nil {
			log.Println(err)
		}
		log.Println("connection closed")
		return nil

	default:
		return fmt.Errorf("invalid mode %q", *mode)
	}
}

var jsonRPCWriteBuffer bytes.Buffer

var readChannel chan string =  make(chan string);

func process(this js.Value, args []js.Value) interface {} {
	readChannel <- args[0].String();
	return nil
}

type wasmRWC struct{}


func (wasmRWC) Read(p []byte) (int, error) {
	jsonRPCData:= <- readChannel

	var jsonRPCReadBuffer bytes.Buffer

	jsonRPCReadBuffer.WriteString(jsonRPCData);
	log.Println("readBuffer", jsonRPCData)

	readChannel = make(chan string)
	return jsonRPCReadBuffer.Read(p)
}

func (wasmRWC) Write(p []byte) (int, error) {
	log.Println("writeBuffer", string (p))

	js.Global().Call("wasmGoHandler", string (p));
	return jsonRPCWriteBuffer.Write(p)
}

func (wasmRWC) Close() error {
	// if err := os.Stdin.Close(); err != nil {
	// 	return err
	// }
	// return os.Stdout.Close()
	return nil
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
