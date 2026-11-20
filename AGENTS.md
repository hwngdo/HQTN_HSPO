# AGENTS.md — Nguyên tắc bắt buộc khi Codex phát triển HQTN_HSPO

## Không được làm
- **Không** tuyên bố nguồn này là original full source, hoặc phát hành production dựa trên Go recovered chưa đủ API.
- **Không** đổi `HQTN_HSPO.exe` / tự cài đè hệ thống chỉ vì `go build` thành công.
- **Không** sửa hàng loạt quy tắc HS/VAT/CAS, xóa căn cứ hay tự suy diễn tình trạng hiệu lực pháp lý.
- **Không** dùng dữ liệu người dùng thật trong unit test hoặc đưa `users.json`, history, MSDS của doanh nghiệp vào repo.
- **Không** trộn Go backend từ nhiều bản không đồng nhất thành một entrypoint chưa được test đầy đủ.
- **Không** tạo thêm nhiều package vá lẻ: trước tiên ưu tiên ZIP dữ liệu, chỉ sửa source khi tính năng cần code.

## Quy trình bắt buộc
1. Xác nhận version baseline thực tế, SHA-256, API contract; ghi nguồn từng file.
2. Cập nhật dữ liệu chính sách trước hết bằng ZIP độc lập, có `effective_from`, `effective_to` (nếu biết), quan hệ `amends/replaces/repeals`, chứng cứ đối chiếu; nếu chưa kiểm chứng, ghi `unverified`, không tự kết luận.
3. Với mỗi văn bản pháp lý mới: kiểm tra ngày hiệu lực, thay thế/bãi bỏ/sửa đổi những văn bản nào đã có trong thư viện, cập nhật quan hệ và các cảnh báo phụ thuộc; tách ngày đăng ký/ban hành/ngày hiệu lực.
4. Luôn có backup, dry-run, diff, rollback, audit; không tự cập nhật chính sách đang khai báo nếu nguồn không được xác minh.
5. Mỗi lần thay code, chạy contract/regression HS, VAT/FTA, CAS/tiền chất, Legal Reader, account/session, OCR LAN, lịch sử/đăng nhập, Launcher/Tray.
6. Môi trường UAT phải có **data dir và cổng riêng**, không dùng chung tệp production. Trước khi phát hành test Windows thực tế.
7. Release artifacts: `FULL_RELEASE.zip` + `SOURCE_HANDOFF.zip` + `DATA_UPDATE.zip` (nếu chỉ cập nhật data), kèm manifest SHA, ngày, kết quả QA và rollback.

## Nghiệp vụ ưu tiên
- Kiểm tra CAS theo từng nguồn/phụ lục/STT; ngoại lệ Phụ lục III cho CAS 100-37-8/108-01-0 phải có test riêng.
- Lỗi OCR đọc CAS thứ hai STR-1838 phải được bảo vệ bằng regression.
- Cảnh báo `Thuế suất HS6` phải so cùng loại thuế, năm/FTA/đối tác và hiệu lực; không cảnh báo tràn lan.
- Thông tin hiệu lực của Luật/Nghị định/Thông tư phải lưu theo văn bản và thời điểm, không gán “còn hiệu lực” bằng suy luận.
- Ứng dụng chỉ hỗ trợ công chức quyết định; hiển thị chưa xác minh khi thiếu căn cứ.
