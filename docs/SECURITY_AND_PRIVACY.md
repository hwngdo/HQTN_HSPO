# An toàn thông tin / quyền sử dụng

- Repo không đưa `users.json`, `search_history.jsonl`, dữ liệu doanh nghiệp, PDF MSDS nghiệp vụ, mật khẩu, tokens, private keys, logs.
- Trước khi nhập bản ZIP mới phải lọc `data/`, `logs/`, `diagnostics/`, `QA_EXPORT/`, và mọi file chứa user info/private key.
- Không chạy recovered Core trên LAN: bootstrap mẫu có hash mật khẩu rỗng; chưa có middleware bảo mật đầy đủ. Cần thay thế bằng account provisioning an toàn trước production.
- Session check cần server-side revoke khi khóa user/thay role/password; cookie `HttpOnly`, `SameSite`, `Secure` qua HTTPS, chống CSRF.
- Xác minh xác thực/phân quyền ngay server, không tin nút ẩn hoặc username do client gửi.
- PDF/MSDS lỗi chỉ ghi khi có quyền, có retention, gắn đúng người đang đăng nhập, cho Admin tải/đọc trong phạm vi phân quyền.
- Không đưa các tài liệu doanh nghiệp/bản ghi user thật vào Codex hay môi trường test nếu chưa được phép.
