package appupdate

import (
	"errors"
	"os"
	"strings"
	"syscall"
	"unsafe"
)

const credentialTarget = "HQTN_HSPO QA GitHub " + Repository
const genericCredential = 1

var credentialDLL = syscall.NewLazyDLL("advapi32.dll")

type credential struct {
	Flags, Type             uint32
	Target, Comment         *uint16
	Written                 syscall.Filetime
	Size                    uint32
	Data                    *byte
	Persist, AttributeCount uint32
	Attributes              uintptr
	Alias, Username         *uint16
}

func Token() string {
	if env := strings.TrimSpace(os.Getenv("HSPO_GITHUB_READ_TOKEN")); env != "" &&
		strings.EqualFold(strings.TrimSpace(os.Getenv("HSPO_GITHUB_REPOSITORY")), Repository) {
		return env
	}
	raw, _ := syscall.UTF16PtrFromString(credentialTarget)
	var saved *credential
	ok, _, _ := credentialDLL.NewProc("CredReadW").Call(uintptr(unsafe.Pointer(raw)), genericCredential, 0, uintptr(unsafe.Pointer(&saved)))
	if ok == 0 {
		return ""
	}
	defer credentialDLL.NewProc("CredFree").Call(uintptr(unsafe.Pointer(saved)))
	if saved.Size == 0 || saved.Size > 512 {
		return ""
	}
	return strings.TrimSpace(string(unsafe.Slice(saved.Data, saved.Size)))
}

func saveToken(token string) error {
	if !strings.HasPrefix(token, "github_pat_") || len(token) < 20 || len(token) > 256 || strings.ContainsAny(token, " \t\r\n") {
		return errors.New("hãy nhập fine-grained token GitHub có tiền tố github_pat_")
	}
	target, _ := syscall.UTF16PtrFromString(credentialTarget)
	user, _ := syscall.UTF16PtrFromString("GitHub")
	secret := []byte(token)
	defer clear(secret)
	item := credential{Type: genericCredential, Target: target, Size: uint32(len(secret)), Data: &secret[0], Persist: 2, Username: user}
	ok, _, err := credentialDLL.NewProc("CredWriteW").Call(uintptr(unsafe.Pointer(&item)), 0)
	if ok == 0 {
		return err
	}
	return nil
}

func PromptToken(window uintptr) (bool, error) {
	type info struct {
		Size           uint32
		Parent         uintptr
		Message, Title *uint16
		Banner         uintptr
	}
	target, _ := syscall.UTF16PtrFromString(credentialTarget)
	message, _ := syscall.UTF16PtrFromString("Nhập fine-grained token GitHub của kho HQTN_HSPO vào ô mật khẩu. Chỉ cấp Contents: Read. Quyền đọc được lưu cho tài khoản Windows hiện tại.")
	title, _ := syscall.UTF16PtrFromString("Quyền đọc kho cập nhật HQTN_HSPO QA")
	var dialog info
	dialog = info{Size: uint32(unsafe.Sizeof(dialog)), Parent: window, Message: message, Title: title}
	var username [513]uint16
	var password [257]uint16
	defer clear(password[:])
	copy(username[:], syscall.StringToUTF16("GitHub"))
	var save uint32
	prompt := syscall.NewLazyDLL("credui.dll").NewProc("CredUIPromptForCredentialsW")
	const flags = 0x40000 | 0x100000 | 0x80 | 0x8 | 0x2
	status, _, _ := prompt.Call(uintptr(unsafe.Pointer(&dialog)), uintptr(unsafe.Pointer(target)), 0, 0,
		uintptr(unsafe.Pointer(&username[0])), uintptr(len(username)),
		uintptr(unsafe.Pointer(&password[0])), uintptr(len(password)), uintptr(unsafe.Pointer(&save)), flags)
	if status == 1223 {
		return false, nil
	}
	if status != 0 {
		return false, syscall.Errno(status)
	}
	token := strings.TrimSpace(syscall.UTF16ToString(password[:]))
	if err := saveToken(token); err != nil {
		return false, err
	}
	return true, nil
}
