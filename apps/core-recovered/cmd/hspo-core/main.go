package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"hqtn-hspo-recovered/internal/hspo"
)

func main() {
	// The recovered reference has incomplete authorization and is for offline QA only.
	// Explicit opt-in is REQUIRED; never expose this binary to LAN or real user data.
	if os.Getenv("HSPO_ALLOW_INSECURE_RECOVERY_QA") != "I_ACKNOWLEDGE_INSECURE_QA_ONLY" {
		log.Fatal("Recovered QA reference only. Refusing to run without explicit insecure QA opt-in.")
	}
	root := flag.String("root", ".", "HQTN_HSPO runtime directory")
	httpPort := flag.Int("http", 8970, "QA HTTP port")
	flag.Parse()
	app, err := hspo.New(*root)
	if err != nil {
		log.Fatal(err)
	}
	app.HTTPPort = *httpPort
	app.LogSummary()
	srv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", *httpPort), Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second}
	app.Shutdown = func() {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		_ = srv.Shutdown(ctx)
	}
	go func() {
		log.Printf("HQTN_HSPO recovered core listening on http://127.0.0.1:%d", *httpPort)
		if e := srv.ListenAndServe(); e != nil && e != http.ErrServerClosed {
			log.Fatal(e)
		}
	}()
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt, syscall.SIGTERM)
	<-ch
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	_ = srv.Shutdown(ctx)
}
