# HQTN_HSPO 1.9.7-RC2.1: Vùng kéo-thả lớn và cổng UAT 8970

## 1. Cải tiến giao diện
- CSS vùng kéo-thả **chiều cao tối thiểu 148 px** (126 px màn hình nhỏ), toàn bộ chiều ngang panel; icon + tiêu đề + gợi ý tối đa 10 PDF.
- Vẫn hỗ trợ nhấn/Enter/Space để chọn PDF và kéo-thả nhiều file. Không thay đổi engine OCR hay quy tắc HS/CAS.
- `static/index.html` chứa đầy đủ bản vá vào giao diện RC2 thực tế. `preview.html` là trang xem trước có cùng CSS và event kéo-thả.

## 2. Cổng test riêng 8970: thử ở 2 mức

### Mức A — Xem trước an toàn, chạy được ngay
1. Giải nén ZIP QA sang thư mục mới **khác** bộ đang dùng.
2. Chạy `MO_XEM_TRUOC_8970.cmd` trên Windows.
3. Mở http://127.0.0.1:8970; kéo PDF hoặc chọn tệp, kiểm tra vùng thả rộng, dễ thao tác.
4. Đóng cửa sổ CMD để dừng cổng UAT.

**Giới hạn**: Trang xem trước chỉ dùng File API trong trình duyệt để nhận tên/kích thước, **không tải file lên server, không chạy OCR, không truy cập tài khoản hay Core**. Đây KHÔNG PHẢI kiểm thử chức năng đầy đủ.

### Mức B — Proxy đến một Core TEST đã tách biệt trên máy thử KHÁC (chỉ Admin cấu hình)
- Khi đã có server thử nghiệm khác, dữ liệu, tài khoản, OCR, thư mục làm việc và cổng riêng:
`HQTN_HSPO_UAT_8970.exe -listen 127.0.0.1:8970 -root . -test-backend http://MAY-THU-RIENG:8870 -confirm-isolation I_HAVE_A_SEPARATE_TEST_CORE_AND_DATA`
- UAT 8970 sẽ cung cấp HTML đã vá, proxy phần API/tài nguyên sang Core thử nghiệm. Không cho phép localhost/127.0.0.1 làm upstream để tránh vô tình nối sang bản sản xuất.
- Phần OCR LAN có thể gọi 8872 qua URL tuyệt đối, nên cần cấu hình OCR đúng máy thử và xác nhận đường dẫn mạng trước khi nghiệm thu; gateway **không tự cấu hình OCR**.
- Để truy cập từ máy trạm qua LAN, cần triển khai proxy với địa chỉ bind LAN được kiểm soát và chỉ mở firewall cho mạng nội bộ. **Không công khai 8970 ra Internet.**
- Cookies HTTP thường phân biệt theo **hostname chứ không phải cổng**; để tách phiên tốt nhất dùng hostname UAT khác máy chủ chính và trình duyệt riêng.
- Không sử dụng 8970 để truy cập trực tiếp Core production 8870: như vậy sẽ dùng chung database/phiên, gây nguy hiểm.

## 3. Quy trình chuyển sang cổng chính (không phải tự đổi port để phát hành)
1. Thử UI trên 8970, sau đó test chức năng đủ trên Core test tách biệt.
2. Kiểm thử tài khoản, đăng nhập lại, OCR LAN, HS/VAT/CAS, lịch sử và cập nhật; backup dữ liệu trước khi chuyển.
3. Đóng phiên test, sao lưu Core/data đang chạy ở cổng 8870; triển khai bản đã nghiệm thu vào đúng đường dẫn sản xuất và kiểm tra dịch vụ trở lại trên **8870, 8871, 8872**.
4. Nếu không đạt, khôi phục toàn bộ app+data bản cũ. Cần máy Windows và người quản trị xác nhận, không tự động triển khai.

## 4. Giới hạn kỹ thuật hiện tại
- Core gốc 1.9.6.9 trong RC2 chỉ có EXE với listen address mặc định 8870/8871, chưa xác minh có cách cấu hình hai phiên chạy cùng một máy.
- Gói này KHÔNG khởi động bản Core thứ hai, KHÔNG tự sửa byte EXE, KHÔNG đảm bảo cùng máy chạy 2 Core đồng thời. Để UAT end-to-end, dùng Core test độc lập hoặc phát triển lại Core có tham số listen/data-dir được kiểm thử.
- Bản gốc 1.9.6.9 vẫn giữ nguyên. Đây là **gói phát triển QA, không phải release thay thế**.
