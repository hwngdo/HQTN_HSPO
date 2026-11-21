# Phát hành GitHub và chuyển máy chủ HQTN_HSPO

Repo riêng tư `hwngdo/HQTN_HSPO` lưu mã nguồn, tài liệu thiết kế, ca kiểm thử tổng hợp và lịch sử phiên bản. Các thay đổi sau bản bàn giao đi qua nhánh làm việc, kiểm thử Windows và xét duyệt trước khi gắn tag phát hành. Dùng GitHub Releases để phân phối các gói đã kiểm tra. Dữ liệu vận hành đi qua quy trình chuyển máy chủ ngoại tuyến.

## Cập nhật chương trình

Gói `FULL_RELEASE.zip` dùng cho thay đổi giao diện, thuật toán biên dịch, API và bộ chạy Windows. Mỗi bản cần đủ Core, Launcher, Tray, OCR, tài nguyên web, ma trận tương thích dữ liệu và manifest SHA-256. Máy chủ chỉ đọc kênh đã cấu hình, tải vào thư mục staging, xác thực nguồn phát hành và chữ ký trên manifest, rồi chạy kiểm tra trước khi Admin duyệt. Sử dụng một tiến trình cập nhật riêng để dừng và khởi động đúng các thành phần của bản được cài. Sau khi chuyển sang thư mục phiên bản mới, chạy health check. Nếu khởi động lỗi, quay lại phiên bản trước cùng snapshot dữ liệu tương thích.

Core gốc vẫn cần nghiệm thu tương thích trước khi phát hành sản xuất. Bộ cài QA có luồng kiểm tra phiên bản trong Tray và trang cập nhật trên web, xác minh manifest Ed25519 và cài lại từ gói đã duyệt. Xem [hướng dẫn đóng gói và công bố Release của kênh QA](../installer/README_QA.md).

## Cập nhật dữ liệu qua GitHub

Dùng `DATA_UPDATE.zip` theo định dạng `HQTN_HSPO_DATA_V1` của UpdateGateway. Biểu thuế, CAS, danh mục kiểm tra và quy tắc dựa trên dữ liệu đi qua ZIP dữ liệu. Thay đổi mã thuật toán và JavaScript đi qua phát hành chương trình. Công cụ tạo gói hiện có nằm ở `apps/release-v198/tools/TAO_GOI_DU_LIEU.py`.

Đặt `HSPO_GITHUB_REPOSITORY=hwngdo/HQTN_HSPO` và cấp `HSPO_GITHUB_READ_TOKEN` cho tiến trình Gateway trên máy chủ test. Dùng quyền đọc Contents giới hạn ở repo này. Cất thông tin xác thực trong nơi cấu hình được bảo vệ của máy chủ.

1. Admin vào **Hệ thống → Dữ liệu** rồi bấm **Kiểm tra gói trên GitHub**. Gateway chọn bản công bố mới nhất có tệp đúng tên trong 30 bản gần nhất. Các bản nháp và bản thử phát hành được bỏ qua.
2. Máy chủ kiểm SHA-256 tải xuống theo metadata GitHub, rồi dùng bộ xem trước hiện có để kiểm schema, danh sách tệp, phiên bản ứng dụng và hồ sơ rà soát. SHA-256 chứng minh tệp tải về khớp tài sản mà GitHub công bố. Quy trình ký bản phát hành chương trình nằm ở mục trên.
3. Admin xem diff, áp dụng có xác nhận, sao lưu và khởi động lại trong cửa sổ bảo trì. Gateway đối chiếu SHA-256 của ZIP lúc áp dụng với ZIP đã xem trước. Giữ nhật ký và dùng bản sao lưu để rollback.

Khi làm gói nghiệp vụ, lưu ngày hiệu lực, mức xác minh và quan hệ sửa đổi, thay thế hoặc bãi bỏ theo [quy trình rà soát](DATA_PATCH_AND_LEGAL_REVIEW.md). Chỉ phát hành dữ liệu nghiệp vụ sau khi đã đối chiếu nguồn. Luồng tải GitHub hiện đã kiểm thử bằng phản hồi giả lập. Cần một release dữ liệu thử và token đọc trên máy QA để nghiệm thu GitHub thật trước khi dùng.

## Chuyển dữ liệu sang máy chủ mới

`scripts/server_migration.py` phục vụ chuyển dữ liệu giữa hai bản gốc **1.9.8** có Core, máy chủ runtime, tập dữ liệu JSON và biểu thuế trùng SHA-256. Công cụ tạo bản sao của tài khoản, lịch sử tra cứu và audit từ `userdata/`. Máy đích phải là bộ cài mới với `userdata/` chưa tồn tại. Thư mục TLS và thông tin xác thực kết nối của máy nguồn nằm ngoài phạm vi gói chuyển. Máy đích cần được cấp cấu hình kết nối và chứng chỉ phù hợp.

1. Dùng `export` và `restore` ở chế độ kiểm tra trước. Công cụ in số tệp thay vì nội dung tài khoản hoặc lịch sử.
2. Dừng ghi trên máy nguồn trước khi xuất thật. Đặt thư mục chuyển trên ổ bảo vệ bằng mã hóa và chuyển qua kênh nội bộ được phê duyệt. Dừng máy đích trước khi khôi phục.
3. Kiểm tra phiên bản và SHA tại máy đích rồi dùng `restore --apply`. Công cụ đặt thư mục đã xác minh vào máy đích chỉ sau khi chép xong toàn bộ. Chạy kiểm tra đăng nhập, lịch sử, chính sách và OCR trước khi chuyển máy trạm sang địa chỉ mới. Giữ nguyên máy nguồn để có thể quay lại khi nghiệm thu lỗi.

```powershell
python scripts/server_migration.py export D:\HSPO_CU E:\HSPO_CHUYEN
python scripts/server_migration.py export D:\HSPO_CU E:\HSPO_CHUYEN --apply
python scripts/server_migration.py restore E:\HSPO_CHUYEN D:\HSPO_MOI
python scripts/server_migration.py restore E:\HSPO_CHUYEN D:\HSPO_MOI --apply
```

Ca kiểm thử hiện tại dùng tài khoản và dữ liệu tổng hợp. Chuyển khác phiên bản hoặc thay lược đồ tài khoản cần bổ sung trình chuyển đổi và ca nghiệm thu riêng.
