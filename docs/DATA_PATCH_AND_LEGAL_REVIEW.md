# Cập nhật dữ liệu và rà soát hiệu lực văn bản

Đây là **quy chuẩn yêu cầu**, chưa phải bộ cài cập nhật data production.

- Cấu trúc ZIP: `manifest.json` (`schema_version`, `id`, `created_at`, `base_version`, `target_version`, `files[]` với SHA-256) và whitelist `policy_data/**`, `static/documents/**`, `static/*.json.gz`.
- Thử chế độ `dry-run`: kiểm tra SHA/file size, schema, trùng mã, CAS checksum, hiệu lực theo ngày, quan hệ văn bản; liệt kê `added/modified/deleted`. Tự động **không xóa** văn bản gốc.
- Bắt buộc lưu `legal_effect_review`: tên/số văn bản, ngày ban hành, ngày hiệu lực, văn bản sửa đổi/thay thế/bãi bỏ, link/bằng chứng gốc, ai kiểm tra, `verified/unverified` và thời điểm kiểm tra.
- Nội dung chưa được xác minh -> `chưa xác minh`, cảnh báo nghiệp vụ chỉ hỗ trợ quyết định.
- Backup toàn bộ trước apply, khóa ghi trong lúc swap, atomic switch, rollback về bản trước nếu fail, audit user/time/hash.
- Khoảng hiệu lực thuế/CO theo HS6/HS8 phải so cùng bảng thuế/FTA, đối tác, năm, thời điểm.
- Cập nhật dữ liệu **ưu tiên ZIP, không sửa source**. Chỉ sửa code khi nghiệp vụ/format/API thay đổi thực sự.

Xem `scripts/create_data_patch.py` để **tạo gói offline chưa áp dụng**; khi có full Core, Codex phát triển validator và applicator chuẩn.
