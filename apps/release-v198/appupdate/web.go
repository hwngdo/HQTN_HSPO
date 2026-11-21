package appupdate

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"hspo/release-v198/dataupdate"
)

func WebHandler(root string, trayManaged func() bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		reply := func(code int, body any) {
			w.WriteHeader(code)
			_ = json.NewEncoder(w).Encode(body)
		}
		fail := func(code int, message string) {
			reply(code, map[string]any{"ok": false, "error": message})
		}
		current, err := InstalledVersion(root)
		if err != nil {
			fail(409, err.Error())
			return
		}
		token := Token()
		action := strings.TrimPrefix(r.URL.Path, dataupdate.RoutePrefix)
		if action == "program-status" {
			key, _ := publicKey()
			reply(200, map[string]any{"ok": true, "current": current, "configured": token != "", "managed": trayManaged() && len(key) > 0})
			return
		}
		if !trayManaged() {
			fail(409, "hãy mở bản cài QA bằng Tray để cập nhật")
			return
		}
		offer, err := Check(r.Context(), current, token)
		if err != nil {
			fail(502, err.Error())
			return
		}
		if action == "program-check" {
			var release *Manifest
			if offer != nil {
				release = &offer.Manifest
			}
			reply(200, map[string]any{"ok": true, "current": current, "release": release})
			return
		}
		var approval struct {
			Version string
		}
		if json.NewDecoder(io.LimitReader(r.Body, 1024)).Decode(&approval) != nil ||
			offer == nil || approval.Version != offer.Manifest.Version {
			fail(409, "bản phát hành đã thay đổi. Hãy kiểm tra và xác nhận lại")
			return
		}
		if err := Queue(r.Context(), root, token, offer); err != nil {
			fail(502, err.Error())
			return
		}
		reply(200, map[string]any{"ok": true, "version": offer.Manifest.Version})
	})
}
