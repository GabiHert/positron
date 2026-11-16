package run

import (
	"log"
	"net"
	"net/http"
	"net/http/pprof"
	"os"
	"strings"
)

// startPprof serves Go's profiler when POSITRON_PPROF names a local address, such as
// POSITRON_PPROF=127.0.0.1:6060. It is for measuring memory and is off unless set.
func startPprof() {
	addr := strings.TrimSpace(os.Getenv("POSITRON_PPROF"))
	if addr == "" {
		return
	}
	host, _, err := net.SplitHostPort(addr)
	if err != nil || (host != "127.0.0.1" && host != "localhost" && host != "::1") {
		log.Printf("pprof: %q is not a local address; not started", addr)
		return
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)
	go func() {
		log.Printf("pprof: serving on http://%s/debug/pprof/", addr)
		if err := http.ListenAndServe(addr, mux); err != nil {
			log.Printf("pprof: %v", err)
		}
	}()
}
