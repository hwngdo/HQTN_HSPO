package main

import (
	"flag"
	"fmt"
	"hqtn.local/hspo-sprint4-qa/internal/accounts"
	"hqtn.local/hspo-sprint4-qa/internal/diagnostics"
	"hqtn.local/hspo-sprint4-qa/internal/gateway"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func main() {
	qa := flag.Bool("qa", false, "explicitly enable isolated QA only; production use is unsupported")
	listen := flag.String("listen", "127.0.0.1:8788", "loopback QA listen address")
	upstream := flag.String("upstream", "http://127.0.0.1:8765", "existing Core local origin")
	data := flag.String("users", "", "COPY of userdata/users.json; never use a production path")
	diagPath := flag.String("diagnostics", "", "QA diagnostics directory")
	htmlPath := flag.String("html", "", "Sprint 3 QA HTML (optional)")
	flag.Parse()
	if !*qa || *data == "" || *diagPath == "" {
		log.Fatal("QA only: -qa -users <COPY of users.json> -diagnostics <QA folder> required")
	}
	if e := gateway.LoopbackListen(*listen, *upstream); e != nil {
		log.Fatal(e)
	}
	if !strings.EqualFold(filepath.Base(*data), "users.qa.json") {
		log.Fatal("QA only: -users must point to an isolated copy named users.qa.json")
	}
	if _, e := os.Stat(*data); e != nil {
		log.Fatal(e)
	}
	s := accounts.NewStore(*data)
	if _, e := s.Snapshot(); e != nil {
		log.Fatal(e)
	}
	ds := diagnostics.NewStore(*diagPath)
	if _, e := ds.Cleanup(time.Now()); e != nil {
		log.Fatal(e)
	}
	g, e := gateway.New(*upstream, s, ds)
	if e != nil {
		log.Fatal(e)
	}
	if *htmlPath != "" {
		b, e := os.ReadFile(*htmlPath)
		if e != nil {
			log.Fatal(e)
		}
		g.HTML = b
	}
	fmt.Printf("QA gateway only: http://%s/ -> %s (production unchanged)\n", *listen, *upstream)
	server := &http.Server{Addr: *listen, Handler: g, ReadHeaderTimeout: 5 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Fatal(server.ListenAndServe())
}
