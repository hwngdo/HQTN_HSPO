# PROMPT GIAO CODEX — sao chép nội dung này khi bàn giao

Bạn đang nhận repo HQTN_HSPO từ người dùng. **Không viết lại toàn bộ từ đầu**. Hãy giữ UI, dữ liệu và mô-đun đã có; xây tiếp vào các phần trống có chứng cứ.

1. Đọc `AGENTS.md`, `README.md`, `docs/STATUS_AND_GAPS.md` và manifests.
2. Dùng baseline FULL QA ngày 19/11/2026 đã nhập tại `imports/v1.9.8-full-qa/`. Khi có gói mới, dùng `scripts/import_release_baseline.py` để nhập riêng và so sánh frontend, policy, API và pháp lý trước khi tích hợp.
3. Lập bản đồ contract frontend/API bằng `scripts/extract_api_contract.py` và test contract; giữ nguyên URL/request/response trừ khi có adapter tương thích.
4. Kết hợp source `apps/core-recovered` và `apps/qa-gateway` bằng refactor có test, **không coi recovered core là production-ready**; đặc biệt sửa bootstrap admin trắng, authorization và CSRF/cookie.
5. Bổ sung source Launcher/Tray/OCR Bridge hoặc thay thế có regression; bàn giao toàn bộ source cho các EXE cuối cùng. Một `HQTN_HSPO.exe` sở hữu các child process.
6. Trước khi đổi phiên bản, chạy UAT độc lập trên 8970 (cổng API/Core và data riêng; OCR riêng hoặc proxy được phép sang OCR QA, không dùng hồ sơ thật).
7. Chuẩn hóa cập nhật data ZIP, kiểm tra văn bản hiệu lực/sửa đổi/thay thế/bãi bỏ, backup/rollback/audit; `scripts/create_data_patch.py` là tạo ZIP offline **không phải lệnh áp dụng tự động**.
8. Báo cáo từng hạng mục với: source changed, unit tests, Windows acceptance, rủi ro, artifact và SHA. Chỉ kết luận hoàn tất khi test production-like đạt.

**Ưu tiên thực hiện**: Core/account/Launcher → OCR LAN/diagnostics → History/permissions → CAS legal/exemptions → data-only update/legal validity → regression HS/VAT/FTA → production package.
