package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	"time"

	"hqtn-hspo-recovered/internal/hspo"
	"hspo/release-v198/appupdate"
)

func main() {
	// The recovered reference has incomplete authorization and is for offline QA only.
	// Explicit opt-in is REQUIRED; never expose this binary to LAN or real user data.
	if os.Getenv("HSPO_ALLOW_INSECURE_RECOVERY_QA") != "I_ACKNOWLEDGE_INSECURE_QA_ONLY" {
		log.Fatal("Recovered QA reference only. Refusing to run without explicit insecure QA opt-in.")
	}
	root := flag.String("root", ".", "HQTN_HSPO runtime directory")
	httpPort := flag.Int("http", 8970, "QA HTTP port")
	stopEvent := flag.String("tray-stop-event", "", "Windows Tray shutdown event")
	initializeOnly := flag.Bool("initialize-only", false, "initialize the isolated QA data directory and exit")
	flag.Parse()
	trayStop, closeTrayStop, err := watchTrayStop(*stopEvent)
	if err != nil {
		log.Fatal(err)
	}
	defer closeTrayStop()
	app, err := hspo.New(*root)
	if err != nil {
		log.Fatal(err)
	}
	if *initializeOnly {
		return
	}
	app.HTTPPort = *httpPort
	app.ManagedByTray = *stopEvent != ""
	app.LogSummary()
	addr := fmt.Sprintf("127.0.0.1:%d", *httpPort)
	listener, err := net.Listen("tcp4", addr)
	if err != nil {
		log.Fatal(err)
	}
	handler := app.Handler()
	if *stopEvent != "" {
		appRelease, _ := appupdate.InstalledVersion(*root)
		next := handler
		handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/health" {
				w.Header().Set("X-HSPO-QA-PID", strconv.Itoa(os.Getpid()))
				if appRelease != "" {
					w.Header().Set("X-HSPO-App-Release", appRelease)
				}
			}
			next.ServeHTTP(w, r)
		})
	}
	srv := &http.Server{Handler: handler, ReadHeaderTimeout: 10 * time.Second}
	shutdown := make(chan struct{})
	var requestOnce sync.Once
	app.Shutdown = func() {
		requestOnce.Do(func() { close(shutdown) })
	}
	serveErr := make(chan error, 1)
	go func() {
		log.Printf("HQTN_HSPO recovered core listening on http://%s", addr)
		serveErr <- srv.Serve(listener)
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(ch)
	select {
	case <-ch:
	case <-trayStop:
	case <-shutdown:
	case err := <-serveErr:
		if err != http.ErrServerClosed {
			log.Fatal(err)
		}
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Print(err)
		_ = srv.Close()
	}
}
