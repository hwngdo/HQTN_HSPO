# HQTN_HSPO — Bộ mã nguồn chuẩn hóa bàn giao Codex (20/11/2026)

> **TÌNH TRẠNG: SOURCE HANDOFF / KHÔNG PHẢI FULL ORIGINAL SOURCE / KHÔNG ĐƯỢC PHÁT HÀNH PRODUCTION.**
>
> Đây là kho mã nguồn chuẩn hóa từ các gói mà người dùng đã cung cấp và những phần QA được tái dựng.
> **Chưa có mã Go gốc đầy đủ** để tái biên dịch các EXE đang chạy. Không được khẳng định “đã khôi phục 100%”.

## Codex bắt đầu ở đâu?

Xem [ghi chú tích hợp v1.9.8](docs/INTEGRATION_V198.md) để biết dữ liệu đã nhập, mã được chuyển vào workspace và kết quả QA. Các mục bên dưới mô tả gói bàn giao ban đầu.

Xem [phát hành GitHub và chuyển máy chủ](docs/GITHUB_RELEASE_AND_SERVER_MIGRATION.md) để cấu hình xem trước ZIP dữ liệu và kiểm thử quy trình chuyển userdata.

1. Đọc `AGENTS.md` và `docs/STATUS_AND_GAPS.md` **trước khi sửa code**.
2. Đọc `docs/CODEX_START_HERE.md` và `docs/ARCHITECTURE.md`.
3. UI/dữ liệu offline có tại `assets/static/`, `assets/policy_data/`; snapshot từ **v1.9.7-RC2.1 (23/10/2026)**.
4. `apps/core-recovered/`: 4 file Go tái dựng tham khảo từ v1.9.4.1, **không đủ production**, có placeholder API và lối bootstrap yếu. Đừng khởi động trên LAN hoặc dùng dữ liệu thật.
5. `apps/qa-gateway/`: các thành phần tài khoản, chẩn đoán và gateway có unit test, **chưa nối Core thật**.
6. `apps/uat-preview-8970/`: trang xem trước UI độc lập cổng 8970, **không phải full UAT Core**.
7. `apps/legal-lookup-reference/`: module Legal Reader Python tham khảo, **không tự coi đã hợp nhất vào Core**.
8. Chạy `python3 scripts/verify_package.py` và `bash scripts/test_sources.sh` để kiểm tra phần có thể build.

## Giới hạn phiên bản

Bộ dữ liệu và giao diện từ **v1.9.8 FULL QA ngày 19/11/2026** đã được nhập vào `imports/v1.9.8-full-qa/`. Giao diện làm việc nằm trong `apps/release-v198/web/static/`. Dùng [ghi chú tích hợp](docs/INTEGRATION_V198.md) để chọn nguồn khi kiểm thử. Dữ liệu pháp lý trong cả hai snapshot cần được đối chiếu trước khi áp dụng.

## Mục tiêu hợp nhất lâu dài

- Một repo cho **Core + Launcher + Tray + OCR Bridge + giao diện** với build tái lập và version đồng bộ.
- Dữ liệu **HS/thuế/VAT/CO/CAS/chính sách/PDF pháp lý** nằm độc lập; cập nhật **qua ZIP dữ liệu** với validation, backup, rollback, audit.
- Test trên cổng UAT cách ly và dữ liệu thử; production 8870/8871, OCR 8872; UAT ưu tiên 8970 nhưng **phải có Core và data dir riêng**, không được proxy vào production.
- Giữ nghiệp vụ: công cụ hỗ trợ công chức, không tự quyết định mã HS, thuế hoặc hiệu lực văn bản.

## Các gói không đính kèm

Không đính kèm bản thực thi Windows, Tesseract installer, tệp người dùng (`users.json`), lịch sử tra cứu, PDF MSDS doanh nghiệp, log hoặc khóa/mật khẩu. Để chạy production phải có source Core thật hoặc Codex thay thế Core bằng implementation đã kiểm chứng tương thích đầy đủ.
