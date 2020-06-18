package monitoring

import (
	"context"
	"fmt"
	"net/http"
	"net/http/pprof"
)

func StartMonitoring(ctx context.Context, port int) error {
	// TODO add metrics
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	server := &http.Server{
		Handler: mux,
		Addr:    fmt.Sprintf(":%d", port),
	}
	go server.ListenAndServe()
	<-ctx.Done()
	return server.Close()
}
