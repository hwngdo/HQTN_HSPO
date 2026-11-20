//go:build windows

package main

import (
    "fmt"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "syscall"
    "time"
    "unsafe"
)

var (
    user32 = syscall.NewLazyDLL("user32.dll")
    shell32 = syscall.NewLazyDLL("shell32.dll")
    messageBoxW = user32.NewProc("MessageBoxW")
    shellExecuteW = shell32.NewProc("ShellExecuteW")
)

func u16(s string) *uint16 { p, _ := syscall.UTF16PtrFromString(s); return p }

func appDir() string {
    exe, err := os.Executable()
    if err == nil { return filepath.Dir(exe) }
    wd, _ := os.Getwd()
    return wd
}

func readVersion() string {
    raw, err := os.ReadFile(filepath.Join(appDir(), "VERSION.txt"))
    if err != nil { return "không xác định" }
    for _, line := range strings.Split(strings.TrimPrefix(string(raw), "\ufeff"), "\n") {
        line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
        if line != "" { return line }
    }
    return "không xác định"
}

func syncStaticVersion() error {
    v := readVersion()
    if v == "không xác định" { return fmt.Errorf("không đọc được VERSION.txt") }
    dir := filepath.Join(appDir(), "static")
    if err := os.MkdirAll(dir, 0755); err != nil { return err }
    dst := filepath.Join(dir, "VERSION.txt")
    tmp := dst + ".tmp"
    if err := os.WriteFile(tmp, []byte(v+"\r\n"), 0644); err != nil { return err }
    _ = os.Remove(dst)
    return os.Rename(tmp, dst)
}

func msg(title, body string) {
    messageBoxW.Call(0, uintptr(unsafe.Pointer(u16(body))), uintptr(unsafe.Pointer(u16(title))), 0x10)
}

func hiddenCmd(name string, args ...string) *exec.Cmd {
    c := exec.Command(name, args...)
    c.Dir = appDir()
    c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
    return c
}

func killName(name string) {
    c := hiddenCmd("taskkill.exe", "/F", "/IM", name)
    _ = c.Run()
}

func runPS(file string, args ...string) error {
    a := []string{"-NoProfile", "-ExecutionPolicy", "Bypass", "-File", filepath.Join(appDir(), file)}
    a = append(a, args...)
    return hiddenCmd("powershell.exe", a...).Run()
}

func waitURL(url string, timeout time.Duration) bool {
    deadline := time.Now().Add(timeout)
    cli := &http.Client{Timeout: 900 * time.Millisecond}
    for time.Now().Before(deadline) {
        r, err := cli.Get(url)
        if err == nil {
            _ = r.Body.Close()
            if r.StatusCode >= 200 && r.StatusCode < 500 { return true }
        }
        time.Sleep(350 * time.Millisecond)
    }
    return false
}

func startHidden(path string) error {
    c := hiddenCmd(path)
    return c.Start()
}

func openApp() {
    shellExecuteW.Call(0, uintptr(unsafe.Pointer(u16("open"))), uintptr(unsafe.Pointer(u16("http://localhost:8867"))), 0, 0, 1)
}

func required(name string) bool {
    _, err := os.Stat(filepath.Join(appDir(), name))
    return err == nil
}

func main() {
    _ = os.Chdir(appDir())
    version := readVersion()
    title := "HQTN_HSPO v" + version

    if err := syncStaticVersion(); err != nil {
        msg(title, "Không đồng bộ được VERSION.txt cho giao diện web:\r\n"+err.Error())
        return
    }

    requiredFiles := []string{
        "HQTN_HSPO_Launcher.exe",
        "HQTN_HSPO_UpdateGateway.exe",
        "HQTN_HSPO_OCR_Bridge.exe",
        "HQTN_HSPO_Tray.exe",
        "HQTN_HSPO_Tesseract_Bootstrap.ps1",
        "HQTN_HSPO_Tesseract_Check.ps1",
        "HQTN_HSPO_OCR_HealthCheck.ps1",
    }
    for _, f := range requiredFiles {
        if !required(f) {
            msg(title, "Thiếu thành phần bắt buộc: "+f)
            return
        }
    }

    // Keep legacy startup semantics: clear stale service processes before a clean launch.
    for _, n := range []string{
        "HQTN_HSPO_Tray.exe",
        "HQTN_HSPO_OCR_Bridge.exe",
        "HQTN_HSPO_Launcher.exe",
        "HQTN_HSPO_TEST_Runtime_Server.exe",
        "HQTN_HSPO_Core.exe",
        "HQTN_HSPO_UpdateGateway.exe",
    } { killName(n) }
    time.Sleep(600 * time.Millisecond)

    tess := filepath.Join(appDir(), "tools", "tesseract", "tesseract.exe")
    if _, err := os.Stat(tess); err != nil {
        if err := runPS("HQTN_HSPO_Tesseract_Bootstrap.ps1"); err != nil {
            msg(title, "Không triển khai được Tesseract nội bộ.")
            return
        }
    }
    if err := runPS("HQTN_HSPO_Tesseract_Check.ps1"); err != nil {
        msg(title, "Tesseract nội bộ chưa sẵn sàng.")
        return
    }

    if err := startHidden(filepath.Join(appDir(), "HQTN_HSPO_OCR_Bridge.exe")); err != nil {
        msg(title, "Không khởi chạy được OCR Bridge: "+err.Error())
        return
    }
    if err := runPS("HQTN_HSPO_OCR_HealthCheck.ps1", "-Wait"); err != nil {
        msg(title, "OCR Bridge chưa sẵn sàng. Hãy kiểm tra cổng 8872 và Tesseract.")
        return
    }

    if err := startHidden(filepath.Join(appDir(), "HQTN_HSPO_Launcher.exe")); err != nil {
        msg(title, "Không khởi chạy được Web Server/Core: "+err.Error())
        return
    }
    if !waitURL("http://127.0.0.1:8865/", 20*time.Second) {
        msg(title, "Web Server chưa sẵn sàng tại http://127.0.0.1:8865")
        return
    }

    if err := startHidden(filepath.Join(appDir(), "HQTN_HSPO_UpdateGateway.exe")); err != nil {
        msg(title, "Không khởi chạy được cổng cập nhật dữ liệu: " + err.Error())
        return
    }
    if !waitURL("http://127.0.0.1:8867/__hspo_admin/ping", 12*time.Second) {
        msg(title, "Cổng quản trị dữ liệu 8867 chưa sẵn sàng. Kiểm tra quyền firewall/cổng và thử lại.")
        return
    }

    if err := startHidden(filepath.Join(appDir(), "HQTN_HSPO_Tray.exe")); err != nil {
        msg(title, "Hệ thống đã chạy nhưng không khởi động được Tray: "+err.Error())
        openApp()
        return
    }
    openApp()
}
