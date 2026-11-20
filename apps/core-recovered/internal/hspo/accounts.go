package hspo

import (
	"crypto/subtle"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"strings"
)

type accountUpdate struct {
	Action      string  `json:"action"`
	Username    string  `json:"username"`
	Target      string  `json:"target"`
	DisplayName *string `json:"display_name"`
	LegacyName  *string `json:"displayName"`
	Password    string  `json:"password"`
	Role        string  `json:"role"`
	Active      *bool   `json:"active"`
}

func accountError(w http.ResponseWriter, status int, message string) {
	jsonOut(w, status, map[string]any{"ok": false, "error": message})
}

// commitUsers publishes changes only after the account file has been replaced.
// The caller holds a.mu for the entire account update.
func (a *App) commitUsers(w http.ResponseWriter, users []User) bool {
	if err := a.writeUsers(users); err != nil {
		log.Printf("save QA accounts: %v", err)
		accountError(w, http.StatusInternalServerError, "Không lưu được tài khoản")
		return false
	}
	a.Users = users
	return true
}

// revokeSessions is called with a.mu held.
func (a *App) revokeSessions(username, keepToken string) {
	for token, owner := range a.Sessions {
		if owner == username && token != keepToken {
			delete(a.Sessions, token)
		}
	}
}

func (a *App) adminUsersAPI(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		accountError(w, http.StatusMethodNotAllowed, "Phương thức không được hỗ trợ")
		return
	}
	var q accountUpdate
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q); err != nil {
		accountError(w, http.StatusBadRequest, "Dữ liệu tài khoản không hợp lệ")
		return
	}
	username := strings.TrimSpace(q.Username)
	target := strings.TrimSpace(q.Target)
	if username == "" {
		username = target
	}
	if username == "" || (target != "" && !strings.EqualFold(username, target)) {
		accountError(w, http.StatusBadRequest, "Tài khoản không hợp lệ")
		return
	}
	if q.DisplayName == nil {
		q.DisplayName = q.LegacyName
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	admin, err := a.sessionUser(r)
	if err != nil {
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	if admin.Role != "admin" {
		accountError(w, http.StatusForbidden, "Không có quyền thực hiện")
		return
	}

	users := append([]User(nil), a.Users...)
	index := -1
	for i := range users {
		if strings.EqualFold(users[i].Username, username) {
			index = i
			break
		}
	}
	action := q.Action
	if action == "" {
		action = "edit"
		if index < 0 {
			action = "add"
		}
	}

	result := map[string]any{"ok": true}
	var revoked string
	switch action {
	case "add":
		if index >= 0 {
			accountError(w, http.StatusConflict, "Tài khoản đã tồn tại")
			return
		}
		if strings.TrimSpace(q.Password) == "" {
			accountError(w, http.StatusBadRequest, "Nhập mật khẩu cho tài khoản mới")
			return
		}
		users = append(users, User{Username: username, Role: "user", Active: true, CreatedAt: now()})
		index = len(users) - 1
		fallthrough
	case "edit":
		if index < 0 {
			accountError(w, http.StatusNotFound, "Không tìm thấy tài khoản")
			return
		}
		user := &users[index]
		if q.DisplayName != nil {
			user.DisplayName = strings.TrimSpace(*q.DisplayName)
		}
		switch q.Role {
		case "admin":
			user.Role = "admin"
		case "user", "staff":
			user.Role = "user"
		case "":
		default:
			accountError(w, http.StatusBadRequest, "Quyền tài khoản không hợp lệ")
			return
		}
		if q.Active != nil {
			user.Active = *q.Active
		}
		if q.Password != "" {
			if strings.TrimSpace(q.Password) == "" {
				accountError(w, http.StatusBadRequest, "Mật khẩu không được để trống")
				return
			}
			user.Password = hashPassword(q.Password)
		}
		if user.Active && (user.Password == "" || user.Password == hashPassword("")) {
			accountError(w, http.StatusBadRequest, "Cần đặt mật khẩu trước khi mở khóa tài khoản")
			return
		}
		if !user.Active || q.Password != "" {
			revoked = user.Username
		}
	case "delete":
		if index < 0 {
			accountError(w, http.StatusNotFound, "Không tìm thấy tài khoản")
			return
		}
		revoked = users[index].Username
		if a.hasAccountRecords(revoked) {
			users[index].Active = false
			result["disabled_instead_of_deleted"] = true
		} else {
			users = append(users[:index], users[index+1:]...)
		}
	default:
		accountError(w, http.StatusBadRequest, "Thao tác tài khoản không hợp lệ")
		return
	}
	hasActiveAdmin := false
	for _, user := range users {
		if user.Role == "admin" && user.Active {
			hasActiveAdmin = true
			break
		}
	}
	if !hasActiveAdmin {
		accountError(w, http.StatusConflict, "Hệ thống cần ít nhất một admin đang hoạt động")
		return
	}
	if !a.commitUsers(w, users) {
		return
	}
	if revoked != "" {
		a.revokeSessions(revoked, "")
		delete(a.Presence, revoked)
	}
	jsonOut(w, http.StatusOK, result)
}

func (a *App) hasAccountRecords(username string) bool {
	for _, h := range a.History {
		if strings.EqualFold(h.Username, username) {
			return true
		}
	}
	for _, v := range a.Verify {
		if strings.EqualFold(v.L1User, username) || strings.EqualFold(v.L2User, username) {
			return true
		}
	}
	return false
}

func (a *App) accountPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		accountError(w, http.StatusMethodNotAllowed, "Phương thức không được hỗ trợ")
		return
	}
	var q struct {
		Current string `json:"current_password"`
		New     string `json:"new_password"`
	}
	if json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&q) != nil ||
		q.Current == "" || strings.TrimSpace(q.New) == "" {
		accountError(w, http.StatusBadRequest, "Nhập mật khẩu hiện tại và mật khẩu mới")
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	user, err := a.sessionUser(r)
	if err != nil {
		accountError(w, http.StatusUnauthorized, "Chưa đăng nhập")
		return
	}
	if subtle.ConstantTimeCompare([]byte(user.Password), []byte(hashPassword(q.Current))) != 1 {
		accountError(w, http.StatusBadRequest, "Mật khẩu hiện tại không đúng")
		return
	}
	users := append([]User(nil), a.Users...)
	for i := range users {
		if users[i].Username == user.Username {
			users[i].Password = hashPassword(q.New)
			break
		}
	}
	if !a.commitUsers(w, users) {
		return
	}
	cookie, _ := r.Cookie(sessionCookie)
	a.revokeSessions(user.Username, cookie.Value)
	jsonOut(w, http.StatusOK, map[string]any{"ok": true})
}
