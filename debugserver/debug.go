package debugserver

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/net/trace"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func getenv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}

var (
	addr = getenv("PROF_HTTP", ":6060")
)

func init() {
	err := json.Unmarshal([]byte(getenv("PROF_SERVICES", "[]")), &Services)
	if err != nil {
		panic("failed to JSON unmarshal PROF_SERVICES: " + err.Error())
	}

	if addr == "" {
		// Look for our binname in the services list
		name := filepath.Base(os.Args[0])
		for _, svc := range Services {
			if svc.Name == name {
				addr = svc.Host
				break
			}
		}
	}
}

// Endpoint is a handler for the debug server. It will be displayed on the
// debug index page.
type Endpoint struct {
	// Name is the name shown on the index page for the endpoint
	Name string
	// Path is passed to http.Mux.Handle as the pattern.
	Path string
	// Handler is the debug handler
	Handler http.Handler
}

// Services is the list of registered services' debug addresses. Populated
// from SRC_PROF_MAP.
var Services []Service

// Service is a service's debug addr (host:port).
type Service struct {
	// Name of the service. Always the binary name. example: "gitserver"
	Name string

	// Host is the host:port for the services SRC_PROF_HTTP. example:
	// "127.0.0.1:6060"
	Host string
}

// Start runs a debug server (pprof, prometheus, etc) if it is configured (via
// SRC_PROF_HTTP environment variable). It is blocking.
func Start(extra ...Endpoint) {
	if addr == "" {
		return
	}

	pp := http.NewServeMux()
	index := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`
				<a href="vars">Vars</a><br>
				<a href="debug/pprof/">PProf</a><br>
				<a href="metrics">Metrics</a><br>
				<a href="debug/requests">Requests</a><br>
				<a href="debug/events">Events</a><br>
			`))
		for _, e := range extra {
			fmt.Fprintf(w, `<a href="%s">%s</a><br>`, strings.TrimPrefix(e.Path, "/"), e.Name)
		}
		w.Write([]byte(`
				<br>
				<form method="post" action="gc" style="display: inline;"><input type="submit" value="GC"></form>
				<form method="post" action="freeosmemory" style="display: inline;"><input type="submit" value="Free OS Memory"></form>
			`))
	})
	pp.Handle("/", index)
	pp.Handle("/debug", index)
	pp.Handle("/vars", http.HandlerFunc(expvarHandler))
	pp.Handle("/gc", http.HandlerFunc(gcHandler))
	pp.Handle("/freeosmemory", http.HandlerFunc(freeOSMemoryHandler))
	pp.Handle("/debug/pprof/", http.HandlerFunc(pprof.Index))
	pp.Handle("/debug/pprof/cmdline", http.HandlerFunc(pprof.Cmdline))
	pp.Handle("/debug/pprof/profile", http.HandlerFunc(pprof.Profile))
	pp.Handle("/debug/pprof/symbol", http.HandlerFunc(pprof.Symbol))
	pp.Handle("/debug/pprof/trace", http.HandlerFunc(pprof.Trace))
	pp.Handle("/debug/requests", http.HandlerFunc(trace.Traces))
	pp.Handle("/debug/events", http.HandlerFunc(trace.Events))
	pp.Handle("/metrics", promhttp.Handler())
	for _, e := range extra {
		pp.Handle(e.Path, e.Handler)
	}
	log.Println("warning: could not start debug HTTP server:", http.ListenAndServe(addr, pp))
}
