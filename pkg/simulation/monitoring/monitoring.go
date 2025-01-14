package monitoring

import (
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"

	"github.com/felixge/fgprof"
	"istio.io/istio/pkg/monitoring"
)

func StartMonitoring(port int) *http.Server {
	exporter, err := monitoring.RegisterPrometheusExporter(nil, nil)
	if err != nil {
		panic(err.Error())
	}
	mux := http.NewServeMux()
	mux.Handle("/metrics", exporter)
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	mux.Handle("/debug/pprof/fgprof", fgprof.Handler())
	server := &http.Server{
		Handler: mux,
		Addr:    fmt.Sprintf(":%d", port),
	}
	go func() {
		log.Println("monitoring server", server.ListenAndServe())
	}()
	return server
}
