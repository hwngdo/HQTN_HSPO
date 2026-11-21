package main

import (
	"context"
	"fmt"
	"time"
	"unsafe"

	"hspo/release-v198/appupdate"
)

const (
	wmQAResult = WM_APP + 2
	wmQAOpen   = WM_APP + 3
	cmdStartup = 1009
	mfGrayed   = 0x00000001
	mfChecked  = 0x00000008
)

type qaActionResult struct {
	command uintptr
	err     error
	current string
	offer   *appupdate.Candidate
}

var (
	qa        *qaServer
	qaStartup startupEntry
	qaBusy    bool
	qaCommand uintptr
	qaResult  = make(chan qaActionResult, 1)
)

func qaStatus() string {
	if qaBusy {
		switch qaCommand {
		case cmdCheckRelease:
			return "Đang kiểm tra cập nhật"
		case cmdStageRelease:
			return "Đang tải bộ cài"
		case cmdApplyRelease:
			return "Đang bàn giao bộ cài"
		}
		return "Đang xử lý Server"
	}
	if qa.running() {
		return "Server đang chạy"
	}
	return "Server đã tắt"
}

func showQAAbout() {
	version := currentVersion
	if installed, err := appupdate.InstalledVersion(qa.dir); err == nil {
		version = installed
	}
	body := fmt.Sprintf(
		"HQTN_HSPO QA · Phiên bản %s\r\nHải quan Thái Nguyên\r\n\r\n"+
			"Hỗ trợ tra cứu mã HS, biểu thuế, VAT, chính sách mặt hàng và căn cứ pháp lý. "+
			"Tra cứu hóa chất theo mã CAS hoặc tên và đọc mã CAS từ tệp MSDS/SDS bằng OCR trên máy.\r\n\r\n"+
			"%s\r\n%s", version, qaStatus(), qa.url(),
	)
	procMessageBoxW.Call(mainHwnd, uintptr(unsafe.Pointer(u16(body))),
		uintptr(unsafe.Pointer(u16("Giới thiệu HQTN_HSPO QA"))), MB_OK|MB_ICONINFORMATION)
}

func showQAMenu(menu uintptr) {
	var actionFlags uintptr
	if qaBusy {
		actionFlags = mfGrayed
	}
	appendMenu(menu, mfGrayed, 0, qaStatus())
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, actionFlags, CMD_OPEN, "Mở ứng dụng")
	appendMenu(menu, MF_STRING, CMD_ABOUT, "Giới thiệu")
	appendMenu(menu, actionFlags, cmdCheckRelease, "Kiểm tra cập nhật")
	appendMenu(menu, actionFlags, cmdReleaseWeb, "Xem cập nhật trên web")
	appendMenu(menu, actionFlags, cmdGitHubAccess, "Cấu hình quyền đọc GitHub")
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, actionFlags, CMD_SERVER_START, "Bật Server")
	appendMenu(menu, actionFlags, CMD_SERVER_STOP, "Tắt Server (giữ Tray)")
	appendMenu(menu, actionFlags, CMD_SERVER_RESTART, "Khởi động lại Server")
	appendMenu(menu, MF_SEPARATOR, 0, "")
	var startupFlags uintptr
	if enabled, _ := qaStartup.read(); enabled {
		startupFlags = mfChecked
	}
	appendMenu(menu, startupFlags, cmdStartup, "Khởi động cùng Windows")
	appendMenu(menu, actionFlags, CMD_STOP, "Tắt HQTN_HSPO QA hoàn toàn")

	switch command := trackTrayMenu(menu); command {
	case cmdCheckRelease:
		checkProgramRelease()
	case cmdGitHubAccess:
		if acquireGitHubAccess() {
			updateNotice("Đã lưu quyền đọc GitHub cho tài khoản Windows hiện tại.")
		}
	case CMD_ABOUT:
		showQAAbout()
	case cmdStartup:
		if err := qaStartup.toggle(); err != nil {
			showTrayError("Không đổi được chế độ khởi động cùng Windows: " + err.Error())
		}
	case CMD_OPEN, CMD_SERVER_START, cmdReleaseWeb:
		runQAAction(command)
	case CMD_SERVER_STOP:
		if ask("Tắt Server QA?", "Các phiên tra cứu QA sẽ bị ngắt. Biểu tượng Tray vẫn ở khay hệ thống.") {
			runQAAction(command)
		}
	case CMD_SERVER_RESTART:
		if ask("Khởi động lại Server QA?", "Các phiên tra cứu QA sẽ bị gián đoạn trong lúc Server khởi động lại.") {
			runQAAction(command)
		}
	case CMD_STOP:
		if ask("Tắt hoàn toàn HQTN_HSPO QA?", "Dừng Server QA và thoát Tray.") {
			runQAAction(command)
		}
	}
}

func runQAAction(command uintptr) {
	runQAReleaseAction(command, nil)
}

func runQAReleaseAction(command uintptr, offer *appupdate.Candidate) {
	if qaBusy {
		return
	}
	qaBusy = true
	qaCommand = command
	refreshTrayVersion()
	go func() {
		result := qaActionResult{command: command}
		switch command {
		case CMD_OPEN, CMD_SERVER_START, cmdReleaseWeb:
			result.err = qa.start()
		case CMD_SERVER_STOP, CMD_STOP:
			result.err = qa.shutdown()
		case CMD_SERVER_RESTART:
			if result.err = qa.shutdown(); result.err == nil {
				result.err = qa.start()
			}
		case cmdCheckRelease:
			if result.current, result.err = appupdate.InstalledVersion(qa.dir); result.err == nil {
				ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
				result.offer, result.err = appupdate.Check(ctx, result.current, appupdate.Token())
				cancel()
			}
		case cmdStageRelease:
			result.err = appupdate.Queue(context.Background(), qa.dir, appupdate.Token(), offer)
		case cmdApplyRelease:
			result.err = launchProgramUpdater()
		}
		qaResult <- result
		procPostMessageW.Call(mainHwnd, wmQAResult, 0, 0)
	}()
}

func finishQAAction() {
	select {
	case result := <-qaResult:
		qaBusy = false
		refreshTrayVersion()
		if result.err != nil {
			showTrayError(result.err.Error())
			return
		}
		switch result.command {
		case CMD_OPEN:
			openURL(qa.url())
		case cmdReleaseWeb:
			openURL(qa.url() + "/#cap-nhat")
		case cmdCheckRelease:
			if result.offer == nil {
				updateNotice("Anh đang dùng phiên bản mới nhất trên kênh QA (" + result.current + ").")
			} else if ask("Có phiên bản mới", "Phiên bản đang chạy "+result.current+
				"\r\nPhiên bản mới "+result.offer.Manifest.Version+
				"\r\n\r\nTải và cài đặt bản cập nhật? Server sẽ khởi động lại sau khi tải xong.") {
				runQAReleaseAction(cmdStageRelease, result.offer)
			}
		case cmdStageRelease:
			pollProgramUpdate()
		case CMD_STOP, cmdApplyRelease:
			procDestroyWindow.Call(mainHwnd)
		}
	default:
	}
}
