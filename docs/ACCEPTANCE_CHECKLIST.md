# Acceptance Checklist — phải nghiệm thu trên Windows

- [ ] Source có thể build lại **mọi EXE cuối cùng**, không dùng binary độc quyền không có nguồn trừ phụ thuộc bên ngoài đã ghi rõ.
- [ ] `HQTN_HSPO.exe` mở Core/Tray/OCR một lần, không loop/UAC, Start/Stop/Restart/Startup HKCU hoạt động, thoát sạch.
- [ ] Production 8870/8871 và OCR 8872, UAT 8970 cách ly **data dir và auth**; không chạm users/history thật.
- [ ] Quản lý tài khoản CRUD, đổi tên, role, khóa/mở khóa, đổi mật khẩu: persist/restart, deny locked login/revoke session.
- [ ] Tra cứu HS8/HS6, VAT MFN/FTA/CO, quy tắc HS6 so sánh đúng thời kỳ, không tạo cảnh báo thiếu căn cứ.
- [ ] Hóa chất/CAS, tiền chất, nhiều danh mục; ngoại lệ 100-37-8/108-01-0 được hiển thị đúng; mở PDF đúng trang theo APP/STT.
- [ ] OCR MSDS LAN nhiều tệp: đọc CAS thứ hai và nhiều CAS, lịch sử và error export đúng phân quyền.
- [ ] Lịch sử user chỉ bản thân; admin xem toàn bộ; 4 số liệu tổng thống nhất.
- [ ] Data update ZIP: signed/hash, backup, dry-run, rollback, audit và rà soát hiệu lực pháp lý.
- [ ] Khởi động lại/khôi phục dữ liệu không thay đổi nghiệp vụ; chạy UAT với dữ liệu giả, không dùng dữ liệu DN thật.
- [ ] Tray/Core/OCR/frontend hiển thị đúng version lấy từ một manifest.
- [ ] Documentation, SHA, source ZIP, release ZIP và UAT report đi cùng phát hành.

## Kiểm thử thủ công tài khoản trên Core QA Windows

Mở PowerShell tại thư mục mã nguồn chuẩn hóa. Các lệnh dưới đây tạo một thư mục QA riêng trong thư mục tạm của Windows và chạy trên localhost cổng 18970. Giữ cửa sổ PowerShell mở trong lúc kiểm thử.

```powershell
$qaRoot = Join-Path $env:TEMP ('hspo-account-ui-' + [guid]::NewGuid())
New-Item -ItemType Directory -Path $qaRoot | Out-Null
Copy-Item -Recurse .\apps\release-v198\web\static -Destination $qaRoot
$env:HSPO_ALLOW_INSECURE_RECOVERY_QA = 'I_ACKNOWLEDGE_INSECURE_QA_ONLY'
$env:HSPO_RECOVERY_QA_ADMIN_PASSWORD = 'Hspo-QA-Account-2026!'
.\dist\qa\HQTN_HSPO_Recovered_Core_REFERENCE_NOT_PRODUCTION.exe -root $qaRoot -http 18970
```

Mở [trang QA tài khoản](http://127.0.0.1:18970/) và đăng nhập bằng `admin` với mật khẩu QA ở trên. Dùng một cửa sổ ẩn danh để thử tài khoản thường.

1. Tạo hai tài khoản thử có họ tên và mật khẩu. Kiểm tra họ tên, rồi sửa họ tên của tài khoản tạo trước. Để trống ô mật khẩu khi sửa tên và đăng nhập lại bằng mật khẩu cũ.
2. Trong cửa sổ tài khoản thường, thử đổi mật khẩu với mật khẩu hiện tại nhập sai. Giao diện phải báo lỗi và mật khẩu cũ vẫn đăng nhập được. Nhập đúng để đổi, rồi xác nhận mật khẩu mới dùng được.
3. Giữ tài khoản thường đang đăng nhập. Trong cửa sổ admin, sửa tài khoản, bỏ chọn **Tài khoản đang hoạt động** và lưu. Tài khoản phải xuống cuối danh sách. Tải lại trang tài khoản thường và thử đăng nhập, hệ thống phải từ chối.
4. Mở khóa từ cửa sổ admin. Tài khoản về nhóm đang hoạt động. Đăng nhập mới bằng mật khẩu hiện tại để sử dụng tiếp.
5. Thử đặt lại mật khẩu của tài khoản thường từ cửa sổ admin. Phiên cũ phải hết hiệu lực và mật khẩu vừa đặt phải đăng nhập được.
6. Nhấn Ctrl+C trong PowerShell, rồi chạy lại lệnh EXE với cùng `$qaRoot` để kiểm tra họ tên, mật khẩu và trạng thái được giữ sau khi khởi động lại.

Bài kiểm thử tự động dùng dữ liệu giả và tự dọn thư mục tạm khi kết thúc.

```powershell
python -B scripts/smoke_recovered_windows.py
```
