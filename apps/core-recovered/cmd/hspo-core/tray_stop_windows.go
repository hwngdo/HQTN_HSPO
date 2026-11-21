package main

import (
	"fmt"
	"syscall"
	"unsafe"
)

func watchTrayStop(name string) (<-chan struct{}, func(), error) {
	if name == "" {
		return nil, func() {}, nil
	}
	namePtr, err := syscall.UTF16PtrFromString(name)
	if err != nil {
		return nil, nil, err
	}
	kernel := syscall.NewLazyDLL("kernel32.dll")
	const eventModifyState = 0x0002
	event, _, err := kernel.NewProc("OpenEventW").Call(syscall.SYNCHRONIZE|eventModifyState, 0, uintptr(unsafe.Pointer(namePtr)))
	if event == 0 {
		return nil, nil, fmt.Errorf("open Tray shutdown event: %w", err)
	}
	handle := syscall.Handle(event)
	stopped := make(chan struct{})
	go func() {
		syscall.WaitForSingleObject(handle, syscall.INFINITE)
		close(stopped)
	}()
	return stopped, func() {
		kernel.NewProc("SetEvent").Call(event)
		<-stopped
		syscall.CloseHandle(handle)
	}, nil
}
