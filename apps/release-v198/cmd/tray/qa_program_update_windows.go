package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"hspo/release-v198/appupdate"
)

const (
	cmdCheckRelease = 1010
	cmdStageRelease = 1011
	cmdApplyRelease = 1012
	cmdGitHubAccess = 1013
	cmdReleaseWeb   = 1014
)

func updateNotice(message string) {
	procMessageBoxW.Call(mainHwnd, uintptr(unsafe.Pointer(u16(message))),
		uintptr(unsafe.Pointer(u16("Cập nhật HQTN_HSPO QA"))), MB_OK|MB_ICONINFORMATION)
}

func acquireGitHubAccess() bool {
	saved, err := appupdate.PromptToken(mainHwnd)
	if err != nil {
		updateNotice("Không lưu được quyền đọc GitHub: " + err.Error())
	}
	return saved
}

func checkProgramRelease() {
	if _, err := appupdate.InstalledVersion(qa.dir); err != nil {
		updateNotice(err.Error())
		return
	}
	if appupdate.Token() == "" && !acquireGitHubAccess() {
		return
	}
	runQAAction(cmdCheckRelease)
}

func pollProgramUpdate() {
	if qaBusy {
		return
	}
	if _, err := os.Stat(appupdate.PendingDir(qa.dir)); err == nil {
		runQAAction(cmdApplyRelease)
	}
}

func launchProgramUpdater() error {
	pending := appupdate.PendingDir(qa.dir)
	job, err := os.MkdirTemp(filepath.Dir(pending), "install-")
	if err != nil {
		return err
	}
	payload := filepath.Join(job, "release")
	if err := os.Rename(pending, payload); err != nil {
		os.Remove(job)
		return err
	}
	// Moving the download out of the queue prevents a failed package retrying
	// on every timer tick. Keep the installer log with the failed update.
	if _, err := appupdate.VerifyPending(qa.dir, payload); err != nil {
		return err
	}
	runner := filepath.Join(filepath.Dir(pending), "HQTN_HSPO_Updater_QA.exe")
	if err := copyProgramFile(filepath.Join(qa.dir, "HQTN_HSPO_Tray_QA.exe"), runner); err != nil {
		return err
	}
	if err := qa.shutdown(); err != nil {
		return err
	}
	command := exec.Command(runner, "-apply-program-update", payload, "-parent-pid", strconv.Itoa(os.Getpid()), "-qa-port", strconv.Itoa(qa.port))
	command.Dir = qa.dir
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		return errors.Join(err, qa.start())
	}
	_ = command.Process.Release()
	return nil
}

func waitForTrayExit(pid int) error {
	if pid <= 0 {
		return errors.New("thiếu mã tiến trình Tray")
	}
	h, err := syscall.OpenProcess(syscall.SYNCHRONIZE, false, uint32(pid))
	if errors.Is(err, syscall.Errno(87)) {
		return nil
	}
	if err != nil {
		return err
	}
	defer syscall.CloseHandle(h)
	status, err := syscall.WaitForSingleObject(h, 20000)
	if err != nil {
		return err
	}
	if status != syscall.WAIT_OBJECT_0 {
		return errors.New("Tray chưa thoát để cài phiên bản mới")
	}
	return nil
}

func programUpdateMutex(root string) *uint16 {
	return u16("Local\\HQTN_HSPO_QA_UPGRADE_" + qaInstanceID(root))
}

func upgradeInProgress(root string) bool {
	open := modKernel32.NewProc("OpenMutexW")
	handle, _, _ := open.Call(syscall.SYNCHRONIZE, 0, uintptr(unsafe.Pointer(programUpdateMutex(root))))
	if handle == 0 {
		return false
	}
	syscall.CloseHandle(syscall.Handle(handle))
	return true
}

func applyProgramRelease(payload string, parent, port int) error {
	runner, err := os.Executable()
	if err != nil {
		return err
	}
	updateRoot := filepath.Dir(runner)
	root := filepath.Dir(updateRoot)
	if !strings.EqualFold(filepath.Base(updateRoot), "program_updates") ||
		!strings.EqualFold(filepath.Base(runner), "HQTN_HSPO_Updater_QA.exe") ||
		!strings.EqualFold(filepath.Clean(filepath.Dir(filepath.Dir(payload))), updateRoot) ||
		filepath.Base(payload) != "release" || !strings.HasPrefix(filepath.Base(filepath.Dir(payload)), "install-") ||
		port < 1024 || port > 65535 || port == 8865 || port == 8867 || port == 8872 {
		return errors.New("thư mục hoặc cổng của bộ cập nhật không hợp lệ")
	}
	oldVersion, err := appupdate.InstalledVersion(root)
	if err != nil {
		return err
	}
	lock, _, lockErr := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(programUpdateMutex(root))))
	if lock == 0 {
		return lockErr
	}
	defer func() {
		if lock != 0 {
			syscall.CloseHandle(syscall.Handle(lock))
		}
	}()
	if lockErr == syscall.Errno(183) {
		return errors.New("đang cài một bản cập nhật khác")
	}
	if err := waitForTrayExit(parent); err != nil {
		return err
	}
	resume := func(version string) error {
		if lock != 0 {
			syscall.CloseHandle(syscall.Handle(lock))
			lock = 0
		}
		return restartUpdatedTray(root, port, version)
	}
	m, err := appupdate.VerifyPending(root, payload)
	if err != nil {
		return errors.Join(err, resume(oldVersion))
	}
	job := filepath.Dir(payload)
	backup := filepath.Join(job, "previous")
	if err := saveProgram(root, backup); err != nil {
		return errors.Join(fmt.Errorf("không sao lưu được chương trình: %w", err), resume(oldVersion))
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	setup := exec.CommandContext(ctx, filepath.Join(payload, appupdate.SetupName), "/VERYSILENT",
		"/SUPPRESSMSGBOXES", "/NORESTART", "/SP-", "/NOCLOSEAPPLICATIONS", "/DIR="+root, "/LOG="+filepath.Join(job, "setup.log"))
	setup.Dir = root
	setup.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	err = setup.Run()
	if err == nil {
		var actual string
		actual, err = appupdate.InstalledVersion(root)
		if err == nil && actual != m.Version {
			err = fmt.Errorf("bộ cài trả về phiên bản %s, cần %s", actual, m.Version)
		}
	}
	if err == nil {
		err = resume(m.Version)
	}
	if err != nil {
		if restoreErr := restoreProgram(root, backup); restoreErr != nil {
			return errors.Join(err, fmt.Errorf("khôi phục thất bại: %w", restoreErr))
		}
		return errors.Join(fmt.Errorf("đã khôi phục bản %s sau khi cập nhật lỗi: %w", oldVersion, err), resume(oldVersion))
	}
	os.RemoveAll(backup)
	os.RemoveAll(payload)
	return nil
}

func restartUpdatedTray(root string, port int, version string) error {
	command := exec.Command(filepath.Join(root, "HQTN_HSPO_Tray_QA.exe"), "-qa", "-background", "-qa-port", strconv.Itoa(port))
	command.Dir = root
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := command.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- command.Wait() }()
	client := http.Client{Transport: &http.Transport{DisableKeepAlives: true}, Timeout: 400 * time.Millisecond}
	defer client.CloseIdleConnections()
	until := time.Now().Add(24 * time.Second)
	for time.Now().Before(until) {
		select {
		case <-done:
			return errors.New("Tray đã thoát trong khi khởi động")
		default:
		}
		response, err := client.Get("http://127.0.0.1:" + strconv.Itoa(port) + "/health")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK && response.Header.Get("X-HSPO-App-Release") == version &&
				response.Header.Get("X-HSPO-QA-PID") != "" {
				return nil
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	_ = command.Process.Kill()
	<-done
	return errors.New("Server chưa sẵn sàng sau khi cài")
}

func programFiles(root string) ([]string, error) {
	rootFiles, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, item := range rootFiles {
		name := item.Name()
		switch {
		case name == "static" || name == "tools":
			err = filepath.WalkDir(filepath.Join(root, name), func(path string, entry fs.DirEntry, err error) error {
				if err != nil {
					return err
				}
				rel, err := filepath.Rel(root, path)
				if err != nil {
					return err
				}
				rel = filepath.ToSlash(rel)
				name := strings.ToLower(rel)
				if entry.IsDir() {
					if name == "static/documents" {
						return filepath.SkipDir
					}
					return nil
				}
				if name == "static" || name == "tools" {
					return fmt.Errorf("thư mục tài nguyên không hợp lệ: %s", rel)
				}
				if strings.HasPrefix(name, "static/") && (strings.HasSuffix(name, ".json") || strings.HasSuffix(name, ".json.gz")) {
					return nil
				}
				if !entry.Type().IsRegular() {
					return fmt.Errorf("tệp chương trình không hợp lệ: %s", rel)
				}
				names = append(names, rel)
				return nil
			})
			if err != nil {
				return nil, err
			}
		case name == "HQTN_HSPO_Core_QA.exe", name == "HQTN_HSPO_Tray_QA.exe", name == "HQTN_HSPO_OCR_Worker.ps1",
			name == "APP_RELEASE.txt", name == "VERSION.txt", strings.HasPrefix(name, "unins000"):
			if !item.Type().IsRegular() {
				return nil, fmt.Errorf("tệp chương trình không hợp lệ: %s", name)
			}
			names = append(names, name)
		}
	}
	return names, nil
}

func copyProgramFile(source, target string) error {
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return err
	}
	out, err := os.CreateTemp(filepath.Dir(target), ".hspo-copy-")
	if err != nil {
		return err
	}
	defer os.Remove(out.Name())
	_, err = io.Copy(out, in)
	if err = errors.Join(err, out.Close()); err != nil {
		return err
	}
	return os.Rename(out.Name(), target)
}

func saveProgram(root, backup string) error {
	names, err := programFiles(root)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(backup, 0700); err != nil {
		return err
	}
	for _, name := range names {
		if err := copyProgramFile(filepath.Join(root, name), filepath.Join(backup, name)); err != nil {
			return err
		}
	}
	return nil
}

func restoreProgram(root, backup string) error {
	if err := saveProgram(backup, root); err != nil {
		return err
	}
	names, err := programFiles(root)
	if err != nil {
		return err
	}
	for _, name := range names {
		if _, err := os.Stat(filepath.Join(backup, name)); errors.Is(err, os.ErrNotExist) {
			if err := os.Remove(filepath.Join(root, name)); err != nil {
				return err
			}
		} else if err != nil {
			return err
		}
	}
	return nil
}
