//go:build windows

package main

import (
	"fmt"
 "net/http"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	WM_DESTROY       = 0x0002
	WM_TIMER         = 0x0113
	WM_RBUTTONUP     = 0x0205
	WM_LBUTTONDBLCLK = 0x0203
	WM_APP           = 0x8000
	WM_TRAYICON      = WM_APP + 1

	NIM_ADD    = 0x00000000
	NIM_MODIFY = 0x00000001
	NIM_DELETE = 0x00000002

	NIF_MESSAGE = 0x00000001
	NIF_ICON    = 0x00000002
	NIF_TIP     = 0x00000004

	MF_STRING    = 0x00000000
	MF_SEPARATOR = 0x00000800

	TPM_RETURNCMD   = 0x0100
	TPM_NONOTIFY    = 0x0080
	TPM_RIGHTBUTTON = 0x0002

	IMAGE_ICON      = 1
	LR_LOADFROMFILE = 0x0010
	LR_DEFAULTSIZE  = 0x0040

	MB_OK              = 0x00000000
	MB_ICONINFORMATION = 0x00000040
 MB_YESNO = 0x00000004
 IDYES=6

	SW_SHOWNORMAL = 1

	CMD_OPEN  = 1001
	CMD_ABOUT = 1002
	CMD_HIDE  = 1003
	CMD_STOP  = 1004
 CMD_SERVER_START = 1005
 CMD_SERVER_STOP = 1006
 CMD_SERVER_RESTART = 1007
 CMD_SERVER_STATUS = 1008

	trayID = 1
)

type point struct {
	X int32
	Y int32
}

type msg struct {
	HWnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type wndClassEx struct {
	CbSize        uint32
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
	HIconSm       uintptr
}

type notifyIconData struct {
	CbSize            uint32
	HWnd              uintptr
	UID               uint32
	UFlags            uint32
	UCallbackMessage  uint32
	HIcon             uintptr
	SzTip             [128]uint16
	DwState           uint32
	DwStateMask       uint32
	SzInfo            [256]uint16
	UTimeoutOrVersion uint32
	SzInfoTitle       [64]uint16
	DwInfoFlags       uint32
	GuidItem          [16]byte
	HBalloonIcon      uintptr
}

var (
	modUser32   = syscall.NewLazyDLL("user32.dll")
	modShell32  = syscall.NewLazyDLL("shell32.dll")
	modKernel32 = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassExW    = modUser32.NewProc("RegisterClassExW")
	procCreateWindowExW     = modUser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = modUser32.NewProc("DefWindowProcW")
	procDestroyWindow       = modUser32.NewProc("DestroyWindow")
	procGetMessageW         = modUser32.NewProc("GetMessageW")
	procTranslateMessage    = modUser32.NewProc("TranslateMessage")
	procDispatchMessageW    = modUser32.NewProc("DispatchMessageW")
	procPostQuitMessage     = modUser32.NewProc("PostQuitMessage")
	procCreatePopupMenu     = modUser32.NewProc("CreatePopupMenu")
	procAppendMenuW         = modUser32.NewProc("AppendMenuW")
	procTrackPopupMenu      = modUser32.NewProc("TrackPopupMenu")
	procDestroyMenu         = modUser32.NewProc("DestroyMenu")
	procGetCursorPos        = modUser32.NewProc("GetCursorPos")
	procSetForegroundWindow = modUser32.NewProc("SetForegroundWindow")
	procMessageBoxW         = modUser32.NewProc("MessageBoxW")
	procLoadImageW          = modUser32.NewProc("LoadImageW")
	procLoadIconW           = modUser32.NewProc("LoadIconW")
	procSetTimer            = modUser32.NewProc("SetTimer")
	procKillTimer           = modUser32.NewProc("KillTimer")

	procShellNotifyIconW = modShell32.NewProc("Shell_NotifyIconW")
	procShellExecuteW    = modShell32.NewProc("ShellExecuteW")

	procGetModuleHandleW = modKernel32.NewProc("GetModuleHandleW")
	procCreateMutexW     = modKernel32.NewProc("CreateMutexW")
	procGetLastError     = modKernel32.NewProc("GetLastError")
)

var (
	mainHwnd       uintptr
	trayData       notifyIconData
	loadedIcon     uintptr
	currentVersion string
	mutexHandle    uintptr
)

func u16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

func copyUTF16(dst []uint16, s string) {
	v := syscall.StringToUTF16(s)
	if len(v) > len(dst) {
		v = v[:len(dst)]
		v[len(v)-1] = 0
	}
	copy(dst, v)
}

func appDir() string {
	exe, err := os.Executable()
	if err == nil {
		return filepath.Dir(exe)
	}
	wd, _ := os.Getwd()
	return wd
}

func readVersion() string {
	candidates := []string{
		filepath.Join(appDir(), "VERSION.txt"),
		"VERSION.txt",
	}
	for _, path := range candidates {
		raw, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		text := strings.TrimSpace(strings.TrimPrefix(string(raw), "\ufeff"))
		if text == "" {
			continue
		}
		// VERSION.txt is expected to contain one version token. If it has
		// additional lines, use the first non-empty line only.
		for _, line := range strings.Split(text, "\n") {
			line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
			if line != "" {
				return line
			}
		}
	}
	return "không xác định"
}

func portStatus(addr string) string {
	c, err := net.DialTimeout("tcp", addr, 350*time.Millisecond)
	if err != nil {
		return "Chưa phản hồi"
	}
	_ = c.Close()
	return "Đang hoạt động"
}

func webStatus() string {
	if portStatus("127.0.0.1:8867") == "Đang hoạt động" {
		return "Đang hoạt động"
	}
	return "Chưa phản hồi"
}

func aboutText() string {
	version := readVersion()
	return fmt.Sprintf(
		"HQTN_HSPO v%s\r\n"+
			"Hệ thống hỗ trợ tra cứu nghiệp vụ Hải quan\r\n\r\n"+
			"Chức năng chính:\r\n"+
			"- Tra cứu mã HS, biểu thuế và VAT\r\n"+
			"- Chính sách mặt hàng và căn cứ pháp lý\r\n"+
			"- Hóa chất / CAS và đọc MSDS-PDF offline\r\n"+
			"- Hỗ trợ đối chiếu kiểm dịch\r\n\r\n"+
			"Đơn vị: Hải quan Thái Nguyên\r\n"+
			"Web server: %s (cập nhật tích hợp HTTP 8867 (Core: 8865))\r\n"+
			"OCR Tesseract: %s (cổng 8872; LAN theo cấu hình máy chủ)\r\n\r\n"+
			"OCR chạy nền; có thể tiếp tục tra HS trong khi đang quét PDF.",
		version, webStatus(), portStatus("127.0.0.1:8872"),
	)
}

func showAbout() {
	procMessageBoxW.Call(
		mainHwnd,
		uintptr(unsafe.Pointer(u16(aboutText()))),
		uintptr(unsafe.Pointer(u16("Giới thiệu / Trạng thái HQTN_HSPO"))),
		MB_OK|MB_ICONINFORMATION,
	)
}

func openApp() {
	procShellExecuteW.Call(
		mainHwnd,
		uintptr(unsafe.Pointer(u16("open"))),
		uintptr(unsafe.Pointer(u16("http://localhost:8867"))),
		0, 0,
		SW_SHOWNORMAL,
	)
}

func runHidden(name string, args ...string) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	_ = cmd.Run()
}

func stopAll() {
	// Keep the shutdown scope aligned with the legacy tray's known processes.
	// Ignore "not found" errors so shutdown remains idempotent.
	targets := []string{
		"HQTN_HSPO_Core.exe",
		"HQTN_HSPO_UpdateGateway.exe",
		"HQTN_HSPO_OCR_Bridge.exe",
		"HQTN_HSPO_Launcher.exe",
		"HQTN_HSPO_TEST_Launcher.exe",
		"HQTN_HSPO_TEST_Runtime_Server.exe",
	}
	for _, target := range targets {
		runHidden("taskkill.exe", "/F", "/IM", target)
	}
}


func serverReady(port string) bool {
 cli:=&http.Client{Timeout:700*time.Millisecond}
 resp,err:=cli.Get("http://127.0.0.1:"+port+"/")
 if err!=nil{return false}; defer resp.Body.Close();return resp.StatusCode<500
}
func waitServer(port string,d time.Duration) bool {until:=time.Now().Add(d);for time.Now().Before(until){if serverReady(port){return true};time.Sleep(350*time.Millisecond)};return false}
func ask(title,body string) bool {
 v,_,_:=procMessageBoxW.Call(mainHwnd,uintptr(unsafe.Pointer(u16(body))),uintptr(unsafe.Pointer(u16(title))),MB_YESNO|MB_ICONINFORMATION)
 return v==IDYES
}
func showTrayError(t string){procMessageBoxW.Call(mainHwnd,uintptr(unsafe.Pointer(u16(t))),uintptr(unsafe.Pointer(u16("HQTN_HSPO - Server"))),MB_OK|MB_ICONINFORMATION)}
func trayLaunch(name string) error {
 full:=filepath.Join(appDir(),name);if _,err:=os.Stat(full);err!=nil{return fmt.Errorf("Thiếu %s: %w",name,err)}
 c:=exec.Command(full);c.Dir=appDir();c.SysProcAttr=&syscall.SysProcAttr{HideWindow:true};return c.Start()
}
func startServer() error {
 // Do not create a duplicate server when the original is already responsive.
 if serverReady("8867") {return nil}
 if !serverReady("8872") {if err:=trayLaunch("HQTN_HSPO_OCR_Bridge.exe");err!=nil{return err}}
 if portStatus("127.0.0.1:8865")!="Đang hoạt động" {
   if err:=trayLaunch("HQTN_HSPO_Launcher.exe");err!=nil{return err}
 }
 if !waitServer("8865",15*time.Second){return fmt.Errorf("Core 8865 chưa phản hồi")}
 if err:=trayLaunch("HQTN_HSPO_UpdateGateway.exe");err!=nil{return err}
 if !waitServer("8867",12*time.Second){return fmt.Errorf("Gateway 8867 chưa phản hồi")}
 return nil
}
func stopServer() {
 // Server components only; tray survives. These names follow the original launcher.
 for _,n:=range []string{"HQTN_HSPO_UpdateGateway.exe","HQTN_HSPO_Launcher.exe","HQTN_HSPO_Core.exe","HQTN_HSPO_OCR_Bridge.exe"}{runHidden("taskkill.exe","/F","/IM",n)}
}
func serverStatusText() string {
 return fmt.Sprintf("Gateway 8867: %s\r\nCore 8865: %s\r\nOCR 8872: %s",portStatus("127.0.0.1:8867"),portStatus("127.0.0.1:8865"),portStatus("127.0.0.1:8872"))
}
func loadTrayIcon() uintptr {
	iconPath := filepath.Join(appDir(), "HQTN_HSPO.ico")
	h, _, _ := procLoadImageW.Call(
		0,
		uintptr(unsafe.Pointer(u16(iconPath))),
		IMAGE_ICON,
		0, 0,
		LR_LOADFROMFILE|LR_DEFAULTSIZE,
	)
	if h != 0 {
		return h
	}
	// IDI_APPLICATION fallback.
	h, _, _ = procLoadIconW.Call(0, 32512)
	return h
}

func addTray() bool {
	loadedIcon = loadTrayIcon()
	trayData = notifyIconData{}
	trayData.CbSize = uint32(unsafe.Sizeof(trayData))
	trayData.HWnd = mainHwnd
	trayData.UID = trayID
	trayData.UFlags = NIF_MESSAGE | NIF_ICON | NIF_TIP
	trayData.UCallbackMessage = WM_TRAYICON
	trayData.HIcon = loadedIcon
	currentVersion = readVersion()
	copyUTF16(trayData.SzTip[:], "HQTN_HSPO v"+currentVersion+" - đang chạy")
	r, _, _ := procShellNotifyIconW.Call(NIM_ADD, uintptr(unsafe.Pointer(&trayData)))
	return r != 0
}

func refreshTrayVersion() {
	version := readVersion()
	if version == currentVersion {
		return
	}
	currentVersion = version
	copyUTF16(trayData.SzTip[:], "HQTN_HSPO v"+currentVersion+" - đang chạy")
	procShellNotifyIconW.Call(NIM_MODIFY, uintptr(unsafe.Pointer(&trayData)))
}

func removeTray() {
	if trayData.HWnd != 0 {
		procShellNotifyIconW.Call(NIM_DELETE, uintptr(unsafe.Pointer(&trayData)))
		trayData.HWnd = 0
	}
}

func appendMenu(menu uintptr, flags uintptr, id uintptr, text string) {
	var p uintptr
	if text != "" {
		p = uintptr(unsafe.Pointer(u16(text)))
	}
	procAppendMenuW.Call(menu, flags, id, p)
}

func showTrayMenu() {
	refreshTrayVersion()
	menu, _, _ := procCreatePopupMenu.Call()
	if menu == 0 {
		return
	}
	defer procDestroyMenu.Call(menu)

	appendMenu(menu, MF_STRING, CMD_OPEN, "Mở ứng dụng")
	appendMenu(menu, MF_STRING, CMD_ABOUT, "Giới thiệu")
 appendMenu(menu, MF_STRING, CMD_SERVER_STATUS, "Kiểm tra trạng thái Server")
 appendMenu(menu, MF_SEPARATOR, 0, "")
 appendMenu(menu, MF_STRING, CMD_SERVER_START, "Bật Server")
 appendMenu(menu, MF_STRING, CMD_SERVER_STOP, "Tắt Server (giữ Tray)")
 appendMenu(menu, MF_STRING, CMD_SERVER_RESTART, "Khởi động lại Server")
	appendMenu(menu, MF_SEPARATOR, 0, "")
	appendMenu(menu, MF_STRING, CMD_HIDE, "Ẩn biểu tượng (Server vẫn chạy)")
	appendMenu(menu, MF_STRING, CMD_STOP, "Tắt HQTN_HSPO hoàn toàn")

	var pt point
	procGetCursorPos.Call(uintptr(unsafe.Pointer(&pt)))
	procSetForegroundWindow.Call(mainHwnd)
	cmd, _, _ := procTrackPopupMenu.Call(
		menu,
		TPM_RETURNCMD|TPM_NONOTIFY|TPM_RIGHTBUTTON,
		uintptr(pt.X), uintptr(pt.Y),
		0, mainHwnd, 0,
	)

	switch cmd {
	case CMD_OPEN:
		openApp()
	case CMD_ABOUT:
		showAbout()
	case CMD_SERVER_STATUS:
  showTrayError(serverStatusText())
 case CMD_SERVER_START:
  if err:=startServer();err!=nil{showTrayError("Không bật được Server: "+err.Error())}else{showTrayError("Server: "+serverStatusText())}
 case CMD_SERVER_STOP:
  if ask("Tắt Server?","Dừng các dịch vụ trên máy này? Những người đang tra cứu sẽ bị ngắt kết nối. Tray vẫn hoạt động."){stopServer()}
 case CMD_SERVER_RESTART:
  if ask("Khởi động lại Server?","Kết nối đang sử dụng sẽ bị gián đoạn. Tiếp tục?"){stopServer();time.Sleep(800*time.Millisecond);if err:=startServer();err!=nil{showTrayError("Không khởi động lại được: "+err.Error())}else{showTrayError("Đã yêu cầu khởi động lại. "+serverStatusText())}}
 case CMD_HIDE:
		removeTray()
		procDestroyWindow.Call(mainHwnd)
	case CMD_STOP:
		removeTray()
		stopAll()
		procDestroyWindow.Call(mainHwnd)
	}
}

func wndProc(hwnd uintptr, message uint32, wParam, lParam uintptr) uintptr {
	switch message {
	case WM_TRAYICON:
		switch uint32(lParam) {
		case WM_RBUTTONUP:
			showTrayMenu()
			return 0
		case WM_LBUTTONDBLCLK:
			openApp()
			return 0
		}
	case WM_TIMER:
		refreshTrayVersion()
		return 0
	case WM_DESTROY:
		procKillTimer.Call(hwnd, 1)
		removeTray()
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wParam, lParam)
	return r
}

func singleInstance() bool {
	const ERROR_ALREADY_EXISTS = 183
	name := u16("Local\\HQTN_HSPO_TRAY_DYNAMIC_SINGLE_INSTANCE")
	h, _, _ := procCreateMutexW.Call(0, 0, uintptr(unsafe.Pointer(name)))
	if h == 0 {
		return true // fail open rather than blocking the tray entirely
	}
	mutexHandle = h
	e, _, _ := procGetLastError.Call()
	return e != ERROR_ALREADY_EXISTS
}

func main() {
	if !singleInstance() {
		return
	}

	hInstance, _, _ := procGetModuleHandleW.Call(0)
	className := u16("HQTN_HSPO_TRAY_CLASS_DYNAMIC")
	wc := wndClassEx{
		CbSize:        uint32(unsafe.Sizeof(wndClassEx{})),
		LpfnWndProc:   syscall.NewCallback(wndProc),
		HInstance:     hInstance,
		LpszClassName: className,
		HIcon:         loadTrayIcon(),
		HIconSm:       loadTrayIcon(),
	}
	atom, _, _ := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc)))
	if atom == 0 {
		return
	}

	hwnd, _, _ := procCreateWindowExW.Call(
		0,
		uintptr(unsafe.Pointer(className)),
		uintptr(unsafe.Pointer(u16("HQTN_HSPO Tray"))),
		0,
		0, 0, 0, 0,
		0, 0, hInstance, 0,
	)
	if hwnd == 0 {
		return
	}
	mainHwnd = hwnd

	if !addTray() {
		procDestroyWindow.Call(mainHwnd)
		return
	}

	// Refresh VERSION.txt regularly, so even a package update that changes
	// VERSION.txt while the tray stays alive will update the tooltip.
	procSetTimer.Call(mainHwnd, 1, 30000, 0)

	var m msg
	for {
		r, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}
