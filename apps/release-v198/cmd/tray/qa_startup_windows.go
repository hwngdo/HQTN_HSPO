package main

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"
)

const windowsRunKey = "Software\\Microsoft\\Windows\\CurrentVersion\\Run"

var (
	registryDLL    = syscall.NewLazyDLL("advapi32.dll")
	regCreateKey   = registryDLL.NewProc("RegCreateKeyExW")
	regSetValue    = registryDLL.NewProc("RegSetValueExW")
	regDeleteValue = registryDLL.NewProc("RegDeleteValueW")
)

type startupEntry struct {
	key, name, command string
}

func qaInstanceID(dir string) string {
	if resolved, err := filepath.EvalSymlinks(dir); err == nil {
		dir = resolved
	}
	digest := sha256.Sum256([]byte(strings.ToLower(filepath.Clean(dir))))
	return fmt.Sprintf("%x", digest[:12])
}

func qaStartupEntry(exe, id string, port int) startupEntry {
	return startupEntry{
		key:     windowsRunKey,
		name:    "HQTN_HSPO_QA_" + id,
		command: "\"" + exe + "\" -qa -background -qa-port=" + strconv.Itoa(port),
	}
}

func (e startupEntry) read() (bool, error) {
	var key syscall.Handle
	err := syscall.RegOpenKeyEx(syscall.HKEY_CURRENT_USER, u16(e.key), 0, syscall.KEY_QUERY_VALUE, &key)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	defer syscall.RegCloseKey(key)
	return e.readValue(key)
}

func (e startupEntry) readValue(key syscall.Handle) (bool, error) {
	// Run commands are limited to 260 UTF-16 characters.
	var value [261]uint16
	length := uint32(unsafe.Sizeof(value))
	var kind uint32
	err := syscall.RegQueryValueEx(key, u16(e.name), nil, &kind, (*byte)(unsafe.Pointer(&value[0])), &length)
	if errors.Is(err, syscall.ERROR_FILE_NOT_FOUND) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if kind != syscall.REG_SZ || syscall.UTF16ToString(value[:length/2]) != e.command {
		return false, errors.New("mục khởi động QA đang có lệnh khác")
	}
	return true, nil
}

func (e startupEntry) toggle() error {
	command, err := syscall.UTF16FromString(e.command)
	if err != nil {
		return err
	}
	if len(command)-1 > 260 {
		return errors.New("đường dẫn chương trình quá dài để khởi động cùng Windows")
	}
	var key syscall.Handle
	status, _, _ := regCreateKey.Call(syscall.HKEY_CURRENT_USER, uintptr(unsafe.Pointer(u16(e.key))), 0, 0, 0,
		syscall.KEY_QUERY_VALUE|syscall.KEY_SET_VALUE, 0, uintptr(unsafe.Pointer(&key)), 0)
	if status != 0 {
		return syscall.Errno(status)
	}
	defer syscall.RegCloseKey(key)
	enabled, err := e.readValue(key)
	if err != nil {
		return err
	}
	if enabled {
		status, _, _ = regDeleteValue.Call(uintptr(key), uintptr(unsafe.Pointer(u16(e.name))))
	} else {
		status, _, _ = regSetValue.Call(uintptr(key), uintptr(unsafe.Pointer(u16(e.name))), 0, syscall.REG_SZ,
			uintptr(unsafe.Pointer(&command[0])), uintptr(len(command)*2))
	}
	if status != 0 {
		return syscall.Errno(status)
	}
	return nil
}
