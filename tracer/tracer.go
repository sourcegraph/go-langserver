package tracer

import (
	"log"
	"os"
	"path/filepath"
	"strconv"

	lightstep "github.com/lightstep/lightstep-tracer-go"
	opentracing "github.com/opentracing/opentracing-go"
	jaeger "github.com/uber/jaeger-client-go"
	jaegercfg "github.com/uber/jaeger-client-go/config"
	jaegerlog "github.com/uber/jaeger-client-go/log"
	jaegermetrics "github.com/uber/jaeger-lib/metrics"
)

var (
	lightstepAccessToken         = os.Getenv("LIGHTSTEP_ACCESS_TOKEN")
	lightstepProject             = os.Getenv("LIGHTSTEP_PROJECT")
	lightstepIncludeSensitive, _ = strconv.ParseBool(os.Getenv("LIGHTSTEP_INCLUDE_SENSITIVE"))
	useJaeger, _                 = strconv.ParseBool(os.Getenv("USE_JAEGER"))
)

func Init() {
	serviceName := filepath.Base(os.Args[0])
	if useJaeger {
		log.Println("Distributed tracing enabled", "tracer", "jaeger")
		cfg := jaegercfg.Configuration{
			Sampler: &jaegercfg.SamplerConfig{
				Type:  jaeger.SamplerTypeConst,
				Param: 1,
			},
		}
		_, err := cfg.InitGlobalTracer(
			serviceName,
			jaegercfg.Logger(jaegerlog.StdLogger),
			jaegercfg.Metrics(jaegermetrics.NullFactory),
		)
		if err != nil {
			log.Printf("Could not initialize jaeger tracer: %s", err.Error())
			return
		}
		return
	}

	if lightstepAccessToken != "" {
		log.Println("Distributed tracing enabled", "tracer", "Lightstep")
		opentracing.InitGlobalTracer(lightstep.NewTracer(lightstep.Options{
			AccessToken: lightstepAccessToken,
			UseGRPC:     true,
			Tags: opentracing.Tags{
				lightstep.ComponentNameKey: serviceName,
			},
			DropSpanLogs: !lightstepIncludeSensitive,
		}))

		// Ignore warnings from the tracer about SetTag calls with unrecognized value types. The
		// github.com/lightstep/lightstep-tracer-go package calls fmt.Sprintf("%#v", ...) on them, which is fine.
		defaultHandler := lightstep.NewEventLogOneError()
		lightstep.SetGlobalEventHandler(func(e lightstep.Event) {
			if _, ok := e.(lightstep.EventUnsupportedValue); ok {
				// ignore
			} else {
				defaultHandler(e)
			}
		})
	}
}
