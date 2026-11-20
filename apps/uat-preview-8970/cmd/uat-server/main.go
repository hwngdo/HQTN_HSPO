package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Run from an isolated QA folder. Preview mode does not access production.
func buildHandler(root, upstream, confirm string) (http.Handler, error) {
	preview := filepath.Join(root, "preview.html")
	if _, e := os.Stat(preview); e != nil {
		return nil, fmt.Errorf("missing preview.html in %s: %w", root, e)
	}
	mux := http.NewServeMux()
	if upstream == "" {
		mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/__uat/health" {
				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "preview-only", "version": "1.9.7-RC2.1", "production_data": false})
				return
			}
			if r.URL.Path != "/" && r.URL.Path != "/preview.html" {
				http.NotFound(w, r)
				return
			}
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, preview)
		})
		return mux, nil
	}
	u, err := url.Parse(upstream)
	if err != nil || u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.Path != "" || u.RawQuery != "" {
		return nil, fmt.Errorf("-test-backend must be an http(s) origin without path/query")
	}
	if confirm != "I_HAVE_A_SEPARATE_TEST_CORE_AND_DATA" {
		return nil, fmt.Errorf("refusing to proxy: confirmation must assert distinct test Core, users and data")
	}
	host := u.Hostname()
	if strings.EqualFold(host, "localhost") || host == "127.0.0.1" || host == "::1" {
		return nil, fmt.Errorf("loopback upstream prohibited: may reuse production Core")
	}
	p := u.Port()
	if p == "8970" {
		return nil, fmt.Errorf("upstream must not point to staging port 8970")
	}
	proxy := httputil.NewSingleHostReverseProxy(u)
	proxy.Transport = &http.Transport{Proxy: http.ProxyFromEnvironment, MaxIdleConns: 20, IdleConnTimeout: 30 * time.Second}
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		http.Error(w, "Test backend unreachable", http.StatusBadGateway)
	}
	mux.HandleFunc("/__uat/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true, "mode": "external-test-core", "version": "1.9.7-RC2.1", "upstream": u.String(), "warning": "separate Core/data must be verified by administrator"})
	})
	// The RC UI override is local; backend APIs, other files are handled by test Core.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" || r.URL.Path == "/static/index.html" {
			w.Header().Set("Cache-Control", "no-store")
			http.ServeFile(w, r, filepath.Join(root, "static", "index.html"))
			return
		}
		proxy.ServeHTTP(w, r)
	})
	return mux, nil
}
func main() {
	addr := flag.String("listen", "127.0.0.1:8970", "UAT listen address (local by default)")
	root := flag.String("root", ".", "isolated UAT files folder")
	upstream := flag.String("test-backend", "", "optional DIFFERENT test machine origin, NOT the production Core")
	confirm := flag.String("confirm-isolation", "", "required for external test backend")
	flag.Parse()
	_, p, e := net.SplitHostPort(*addr)
	if e != nil || p != "8970" {
		log.Fatal("Only staging port 8970 allowed")
	}
	abs, e := filepath.Abs(*root)
	if e != nil {
		log.Fatal(e)
	}
	h, e := buildHandler(abs, *upstream, *confirm)
	if e != nil {
		log.Fatal(e)
	}
	srv := &http.Server{Addr: *addr, Handler: h, ReadHeaderTimeout: 8 * time.Second, MaxHeaderBytes: 1 << 20}
	fmt.Printf("HQTN_HSPO UAT 8970 | backend=%v | http://%s/\n", *upstream != "", *addr)
	log.Fatal(srv.ListenAndServe())
}
