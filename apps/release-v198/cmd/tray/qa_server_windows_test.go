package main

import (
	"io"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"unsafe"
)

func TestQAStorageAndProcessIsolation(t *testing.T) {
	core := os.Getenv("HSPO_TRAY_TEST_CORE")
	if core == "" {
		t.Skip("set HSPO_TRAY_TEST_CORE to the built recovered Core")
	}

	dir := t.TempDir()
	build, err := os.Open(core)
	if err != nil {
		t.Fatal(err)
	}
	defer build.Close()
	copyFile, err := os.Create(filepath.Join(dir, "HQTN_HSPO_Core_QA.exe"))
	if err != nil {
		t.Fatal(err)
	}
	_, copyErr := io.Copy(copyFile, build)
	closeErr := copyFile.Close()
	if copyErr != nil || closeErr != nil {
		t.Fatalf("copy Core: %v, %v", copyErr, closeErr)
	}
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "synthetic-tray-password")

	oldServer, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer oldServer.Close()
	server := qaServer{dir: dir, port: oldServer.Addr().(*net.TCPAddr).Port}
	defer server.release()
	if err := server.start(); err == nil || server.process != 0 {
		t.Fatal("Tray accepted a port already in use")
	}
	assertListener(t, server.address())

	qaPort, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	server.port = qaPort.Addr().(*net.TCPAddr).Port
	qaPort.Close()
	if err := server.start(); err != nil {
		t.Fatal(err)
	}
	firstPID := server.pid
	if err := server.start(); err != nil || server.pid != firstPID {
		t.Fatalf("starting a ready Core spawned another process: %v", err)
	}
	if err := server.shutdown(); err != nil {
		t.Fatal(err)
	}
	if server.running() || server.job != 0 {
		t.Fatal("Tray retained the stopped Core")
	}
	t.Setenv("HSPO_RECOVERY_QA_ADMIN_PASSWORD", "")
	if err := server.start(); err != nil {
		t.Fatalf("restart with previously initialized data: %v", err)
	}

	// Closing the job also stops Core if the tray process exits unexpectedly.
	address := server.address()
	done := server.process
	server.process = 0
	defer syscall.CloseHandle(done)
	server.release()
	if result, err := syscall.WaitForSingleObject(done, 5000); err != nil || result != syscall.WAIT_OBJECT_0 {
		t.Fatalf("Core survived closing the QA job: result %d, %v", result, err)
	}
	listener, err := net.Listen("tcp4", address)
	if err != nil {
		t.Fatal("the QA port was not released:", err)
	}
	listener.Close()
	assertListener(t, oldServer.Addr().String())
}

func assertListener(t *testing.T, address string) {
	t.Helper()
	conn, err := net.Dial("tcp4", address)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close()
}

func TestQAStartupToggle(t *testing.T) {
	if os.Getenv("HSPO_TRAY_TEST_CORE") == "" {
		t.Skip("enable Windows Tray integration tests")
	}
	dir := t.TempDir()
	entry := qaStartupEntry(filepath.Join(dir, "Path With Spaces", "HQTN_HSPO_Tray_QA.exe"), qaInstanceID(dir), 18971)
	entry.key = "Software\\" + entry.name + "_Test"
	regDeleteKey := registryDLL.NewProc("RegDeleteKeyW")
	t.Cleanup(func() { regDeleteKey.Call(syscall.HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(entry.key)))) })

	assertStartup(t, entry, false)
	if err := entry.toggle(); err != nil {
		t.Fatal(err)
	}
	assertStartup(t, entry, true)
	conflicting := entry
	conflicting.command = "different.exe"
	if err := conflicting.toggle(); err == nil {
		t.Fatal("Tray overwrote an existing command")
	}
	assertStartup(t, entry, true)
	if err := entry.toggle(); err != nil {
		t.Fatal(err)
	}
	assertStartup(t, entry, false)
}

func assertStartup(t *testing.T, entry startupEntry, want bool) {
	t.Helper()
	enabled, err := entry.read()
	if err != nil || enabled != want {
		t.Fatalf("startup enabled = %t, want %t: %v", enabled, want, err)
	}
}
