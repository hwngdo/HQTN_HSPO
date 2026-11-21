package main

import (
	"crypto/rand"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"
	"unsafe"
)

const (
	createSuspended          = 0x00000004
	createUnicodeEnvironment = 0x00000400
	createNoWindow           = 0x08000000
	jobKillOnClose           = 0x00002000
	jobAccountingClass       = 1
	jobExtendedLimitsClass   = 9
)

var (
	createQAJob     = modKernel32.NewProc("CreateJobObjectW")
	setQAJobInfo    = modKernel32.NewProc("SetInformationJobObject")
	assignQAJob     = modKernel32.NewProc("AssignProcessToJobObject")
	queryQAJob      = modKernel32.NewProc("QueryInformationJobObject")
	terminateQAJob  = modKernel32.NewProc("TerminateJobObject")
	resumeQAThread  = modKernel32.NewProc("ResumeThread")
	createStopEvent = modKernel32.NewProc("CreateEventW")
	signalStopEvent = modKernel32.NewProc("SetEvent")
)

type qaJobLimits struct {
	Basic struct {
		ProcessTime, JobTime         int64
		Flags                        uint32
		MinWorkingSet, MaxWorkingSet uintptr
		ActiveProcessLimit           uint32
		Affinity                     uintptr
		PriorityClass, Scheduling    uint32
	}
	IOCounters      [6]uint64
	MemoryLimits    [2]uintptr
	PeakMemoryUsage [2]uintptr
}

type qaJobAccounting struct {
	Times                     [4]int64
	PageFaults, Total, Active uint32
	Terminated                uint32
}

// Only the QA action worker accesses these handles while an action is pending.
type qaServer struct {
	dir                string
	port               int
	job, process, stop syscall.Handle
	pid                uint32
}

func (s *qaServer) address() string {
	return "127.0.0.1:" + strconv.Itoa(s.port)
}

func (s *qaServer) url() string {
	return "http://" + s.address()
}

func (s *qaServer) running() bool {
	if s.process == 0 {
		return false
	}
	result, err := syscall.WaitForSingleObject(s.process, 0)
	return err == nil && result == syscall.WAIT_TIMEOUT
}

func (s *qaServer) ready() bool {
	if !s.running() {
		return false
	}
	transport := &http.Transport{DisableKeepAlives: true}
	client := http.Client{Transport: transport, Timeout: 350 * time.Millisecond}
	response, err := client.Get(s.url() + "/health")
	if err != nil {
		return false
	}
	response.Body.Close()
	return response.StatusCode == http.StatusOK && response.Header.Get("X-HSPO-QA-PID") == strconv.FormatUint(uint64(s.pid), 10)
}

func (s *qaServer) start() error {
	if s.ready() {
		return nil
	}
	if err := s.shutdown(); err != nil {
		return err
	}
	conn, err := net.DialTimeout("tcp4", s.address(), 200*time.Millisecond)
	if err == nil {
		conn.Close()
		return fmt.Errorf("cổng %d đang được tiến trình khác sử dụng", s.port)
	}
	if err := s.launch(); err != nil {
		return err
	}
	deadline := time.Now().Add(15 * time.Second)
	for s.running() && time.Now().Before(deadline) {
		if s.ready() {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	var exitCode uint32
	_ = syscall.GetExitCodeProcess(s.process, &exitCode)
	if err := s.shutdown(); err != nil {
		return err
	}
	return fmt.Errorf("Core QA chưa phản hồi tại %s (mã thoát %d). Kiểm tra tệp chương trình và thư mục dữ liệu QA", s.address(), exitCode)
}

func (s *qaServer) launch() error {
	exe := filepath.Join(s.dir, "HQTN_HSPO_Core_QA.exe")
	if _, err := os.Stat(exe); err != nil {
		return fmt.Errorf("mở Core QA: %w", err)
	}

	rawJob, _, err := createQAJob.Call(0, 0)
	if rawJob == 0 {
		return fmt.Errorf("tạo nhóm tiến trình QA: %w", err)
	}
	job := syscall.Handle(rawJob)
	attached := false
	defer func() {
		if !attached {
			syscall.CloseHandle(job)
		}
	}()

	var limits qaJobLimits
	limits.Basic.Flags = jobKillOnClose
	if ok, _, err := setQAJobInfo.Call(rawJob, jobExtendedLimitsClass, uintptr(unsafe.Pointer(&limits)), unsafe.Sizeof(limits)); ok == 0 {
		return fmt.Errorf("đặt chế độ dừng nhóm QA: %w", err)
	}

	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return fmt.Errorf("tạo mã phiên Core QA: %w", err)
	}
	eventName := fmt.Sprintf("Local\\HQTN_HSPO_QA_STOP_%x", nonce)
	rawEvent, _, err := createStopEvent.Call(0, 1, 0, uintptr(unsafe.Pointer(u16(eventName))))
	if rawEvent == 0 {
		return fmt.Errorf("tạo tín hiệu dừng QA: %w", err)
	}
	event := syscall.Handle(rawEvent)
	defer func() {
		if !attached {
			syscall.CloseHandle(event)
		}
	}()

	const optIn = "HSPO_ALLOW_INSECURE_RECOVERY_QA="
	env := []string{optIn + "I_ACKNOWLEDGE_INSECURE_QA_ONLY"}
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(strings.ToUpper(entry), optIn) {
			env = append(env, entry)
		}
	}
	sort.Slice(env, func(i, j int) bool { return strings.ToUpper(env[i]) < strings.ToUpper(env[j]) })
	envBlock := utf16.Encode([]rune(strings.Join(env, "\x00") + "\x00\x00"))
	args := []string{exe, "-root", s.dir, "-http", strconv.Itoa(s.port), "-tray-stop-event", eventName}
	for i := range args {
		args[i] = syscall.EscapeArg(args[i])
	}
	info := syscall.StartupInfo{Cb: uint32(unsafe.Sizeof(syscall.StartupInfo{}))}
	var child syscall.ProcessInformation
	err = syscall.CreateProcess(u16(exe), u16(strings.Join(args, " ")), nil, nil, false,
		createSuspended|createUnicodeEnvironment|createNoWindow, &envBlock[0], u16(s.dir), &info, &child)
	if err != nil {
		return fmt.Errorf("khởi động Core QA: %w", err)
	}
	defer syscall.CloseHandle(child.Thread)
	defer func() {
		if !attached {
			syscall.TerminateProcess(child.Process, 1)
			syscall.CloseHandle(child.Process)
		}
	}()
	if ok, _, err := assignQAJob.Call(rawJob, uintptr(child.Process)); ok == 0 {
		return fmt.Errorf("cô lập tiến trình Core QA: %w", err)
	}
	if result, _, err := resumeQAThread.Call(uintptr(child.Thread)); uint32(result) == ^uint32(0) {
		return fmt.Errorf("chạy Core QA: %w", err)
	}
	s.job, s.stop, s.process, s.pid = job, event, child.Process, child.ProcessId
	attached = true
	return nil
}

func (s *qaServer) activeProcesses() (uint32, error) {
	var counts qaJobAccounting
	ok, _, err := queryQAJob.Call(uintptr(s.job), jobAccountingClass, uintptr(unsafe.Pointer(&counts)), unsafe.Sizeof(counts), 0)
	if ok == 0 {
		return 0, err
	}
	return counts.Active, nil
}

func (s *qaServer) shutdown() error {
	if s.job == 0 {
		return nil
	}
	if s.running() {
		if ok, _, _ := signalStopEvent.Call(uintptr(s.stop)); ok != 0 {
			syscall.WaitForSingleObject(s.process, 5000)
		}
	}
	active, err := s.activeProcesses()
	if err != nil {
		return fmt.Errorf("kiểm tra nhóm QA: %w", err)
	}
	if active != 0 {
		if ok, _, err := terminateQAJob.Call(uintptr(s.job), 1); ok == 0 {
			return fmt.Errorf("dừng nhóm QA: %w", err)
		}
		deadline := time.Now().Add(2 * time.Second)
		for active != 0 && time.Now().Before(deadline) {
			time.Sleep(40 * time.Millisecond)
			active, err = s.activeProcesses()
			if err != nil {
				return fmt.Errorf("kiểm tra nhóm QA: %w", err)
			}
		}
		if active != 0 {
			return fmt.Errorf("nhóm tiến trình QA chưa dừng hẳn")
		}
	}
	s.release()
	return nil
}

func (s *qaServer) release() {
	for _, handle := range []syscall.Handle{s.job, s.process, s.stop} {
		if handle != 0 {
			syscall.CloseHandle(handle)
		}
	}
	s.job, s.process, s.stop, s.pid = 0, 0, 0, 0
}
