package monitoring

import (
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"

	"github.com/felixge/fgprof"
)

func StartMonitoring(port int) *http.Server {
	// TODO add metrics
	mux := http.NewServeMux()
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
