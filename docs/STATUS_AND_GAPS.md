# Tình trạng mã nguồn — Sự thật kỹ thuật

Ngày lập: 20/11/2026. Xem [ghi chú tích hợp v1.9.8](INTEGRATION_V198.md) để theo dõi kết quả QA và các giới hạn còn lại.

| Thành phần | Đã có trong repo | Đủ build thành production? |
|---|---|---|
| UI HTML/JS và thư viện dữ liệu snapshot RC2.1 | Có, nguyên trạng theo snapshot | Không; gắn Core phiên bản thích hợp mới hoạt động |
| Core Go | 4 file tái dựng từ v1.9.4.1 | **Không**: endpoint 501 và chưa bảo mật đầy đủ |
| Account Admin API/session | Có module QA + test | **Không**: chưa nối Core đang chạy |
| Launcher / Supervisor | Có source Go tái dựng và logic QA | **Không**: chưa nghiệm thu tiến trình Windows thật |
| Tray icon | Có mã v1.9.8 trong `apps/release-v198/cmd/tray` và EXE build QA | Chưa nghiệm thu cùng Core và Launcher đầy đủ |
| OCR Bridge | Script PowerShell worker/health và EXE ở bộ phân phối ngoài repo | **Không có nguồn Go gốc** |
| Legal Reader | Có module Python tham chiếu | Chưa kiểm định hợp nhất Core |
| PDF pháp lý | Có snapshot RC2.1 | **Hiệu lực chưa được thẩm định lại đến 20/11/2026** |
| v1.9.8 FULL QA ngày 19/11/2026 | Đã nhập 273 tài nguyên, có manifest SHA-256 trong `imports/v1.9.8-full-qa/` | Dùng làm baseline QA |

## Các chức năng bắt buộc Codex tái dựng/tích hợp

1. Core production đầy đủ theo tất cả endpoint UI: login/logout/session, users, admin users, account password, history, verification, presence, sources, system, HS/CAS, OCR, backups, update/rollback, CSV/XLSX, shutdown, TLS.
2. Quyền server-side: admin/normal, khóa user từ chối login/revoke session, chống tự khóa admin cuối cùng; migration `displayName` → `display_name`, transactional update + atomic write.
3. Một Launcher thống nhất, singleton, giới hạn retry/backoff, quản lý Core/Tray/OCR, Startup HKCU có bật/tắt, không UAC vòng lặp.
4. OCR LAN hoạt động từ máy trạm, lỗi MSDS upload riêng về server và Admin xuất ZIP, ẩn dữ liệu nhạy cảm, retention kiểm soát.
5. Thống kê toàn hệ thống và lịch sử chỉ người dùng sở hữu/admin truy cập; log CAS/MSDS nhất quán.
6. Legal Reader: chính xác theo từng Điều/Khoản/Điểm, hiệu lực theo ngày và ngoại lệ; điều hướng đúng PDF/trang theo CAS + phụ lục/STT.
7. Nghiệm thu Windows: start/stop/restart, cổng UAT riêng, UI, LAN OCR, user persistence, rollback, data-only updates, bảo vệ sản xuất.

## Các mốc không được tự đánh dấu 'hoàn thành'

- `go test`/`go vet` hiện tại chỉ chứng minh **module QA** chạy được, không phải Core production.
- Cổng 8970 preview chỉ minh họa UI; không phải UAT end-to-end.
- Các file PDF/JSON trong snapshot cũ **không** chứng minh tình trạng hiệu lực của văn bản ngày 20/11/2026.
- Không thể giải quyết Tray/OCR Bridge bằng sửa nhãn hiển thị phiên bản mà không đồng bộ build thật.
