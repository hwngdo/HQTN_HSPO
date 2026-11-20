package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"
)

func hiddenCmd(name string, args ...string) *exec.Cmd { c := exec.Command(name, args...); return c }
func waitWeb(url string, timeout time.Duration) bool {
	end := time.Now().Add(timeout)
	for time.Now().Before(end) {
		r, e := http.Get(url + "/health")
		if e == nil {
			r.Body.Close()
			if r.StatusCode < 500 {
				return true
			}
		}
		time.Sleep(300 * time.Millisecond)
	}
	return false
}
func openBrowser(url string) {
	if runtime.GOOS == "windows" {
		_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	}
}

func main() {
	// The recovered reference has incomplete authorization and is for offline QA only.
	// Explicit opt-in is REQUIRED; never expose this binary to LAN or real user data.
	if os.Getenv("HSPO_ALLOW_INSECURE_RECOVERY_QA") != "I_ACKNOWLEDGE_INSECURE_QA_ONLY" {
		log.Fatal("Recovered QA reference only. Refusing to run without explicit insecure QA opt-in.")
	}
	root := flag.String("root", ".", "runtime directory")
	port := flag.Int("http", 8970, "QA HTTP port")
	flag.Parse()
	runtimeDir, err := filepath.Abs(*root)
	if err != nil {
		log.Fatal(err)
	}
	launcher, err := os.Executable()
	if err != nil {
		log.Fatal(err)
	}
	exe := filepath.Join(filepath.Dir(launcher), "HQTN_HSPO_Recovered_Core_REFERENCE_NOT_PRODUCTION.exe")
	if _, e := os.Stat(exe); e != nil {
		fmt.Println("Không thấy", exe)
		return
	}
	cmd := hiddenCmd(exe, "-root", runtimeDir, "-http", fmt.Sprint(*port))
	cmd.Dir = runtimeDir
	if e := cmd.Start(); e != nil {
		fmt.Println(e)
		return
	}
	url := fmt.Sprintf("http://127.0.0.1:%d", *port)
	if waitWeb(url, 15*time.Second) {
		openBrowser(url)
	} else {
		fmt.Println("Core chưa sẵn sàng:", url)
	}
}
