# HQTN_HSPO 1.9.7-RC2.1 — UI & UAT 8970 (23/10/2026)

## Đã thực hiện
- Mở rộng vùng kéo-thả PDF/MSDS từ khoảng 12px padding sang tối thiểu **148px chiều cao** (126px mobile), vẫn chiếm toàn chiều ngang, kèm icon/tiêu đề/hướng dẫn.
- Giữ nguyên xử lý 10 PDF/lần, đọc file và OCR, code logic không sửa.
- Tạo **HQTN_HSPO_UAT_8970.exe** Windows x64, mặc định bind `127.0.0.1:8970` với trang preview riêng: **không Core, không dữ liệu production, không upload/OCR**.
- Có chế độ proxy vào Core *thử nghiệm ở máy khác* khi admin xác nhận đã tách biệt dữ liệu. Không chấp nhận loopback upstream.

## Đã kiểm thử
- `go test -race ./...`: PASS (4 bài).
- `go vet ./...`: PASS.
- `GOOS=windows GOARCH=amd64 go build`: PASS.
- Kiểm tra cấu trúc UI, giữ `hspoInitDrop()` và giới hạn 10 PDF: PASS.
- `node --check` inline JavaScript của trang chính và trang preview: PASS.
- Smoke HTTP cổng preview: kiểm tra riêng ở QA, xem các log (không phải Windows thật).

## Chưa kiểm thử/Chưa giải quyết
- Full Core ở cổng phụ trên cùng một máy: **chưa hỗ trợ**, vì Core gốc chưa xác minh tham số bind/data-dir độc lập; không có nguồn Core gốc.
- Đăng nhập, OCR LAN và tra cứu đầy đủ qua cổng 8970: **chưa có backend test độc lập**.
- Không nghiệm thu chuyển production chỉ vì UI preview chạy.
- Gói đầy đủ RC2.1 chứa binary Core/Tray/OCR 1.9.6.9 **không thay đổi** và có thể chiếm 8870–8872 nếu người dùng mở Launcher cũ; chỉ chạy preview 8970 khi đang dùng máy chủ chính.
