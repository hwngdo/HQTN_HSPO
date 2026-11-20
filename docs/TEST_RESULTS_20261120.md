# Kết quả chuẩn hóa và kiểm thử — 20/11/2026

**Trạng thái: Bàn giao mã nguồn tham khảo; chưa phải nguồn production đầy đủ.**

## Kết quả kiểm thử thực hiện trong phiên
- `go test ./...` + `go vet ./...`: đạt cho `core-recovered`, `qa-gateway`, `uat-preview-8970` (recovered Core không có unit test riêng).
- `go test -race ./...`: đạt ở `qa-gateway`.
- Cross-build bằng Go 1.23.2 sang `windows/amd64`: đạt cho 4 entrypoints Core reference, Launcher reference, QA Gateway, UAT Preview.
- Node 22 `node --check`: đạt đối với 2 đoạn JavaScript inline trong giao diện snapshot RC2.1.
- Tệp JSON trong `policy_data` đọc được: đạt; `chemical_policy.json` có 1.257 CAS không trùng; 3 CAS có `cas_valid=0` phải đối chiếu nguồn.
- Kiểm tra SHA-256 toàn bộ tệp nguồn trong `SOURCE_SHA256.json`: đạt.

## Không được coi là đã kiểm thử
- Tích hợp API production, đăng nhập thật, Tray/Windows Startup, OCR LAN end-to-end và UAT trên máy trạm Windows.
- Tình trạng hiệu lực của văn bản đến ngày 20/11/2026; mọi văn bản trong gói là snapshot cũ.
- Rebuild chính xác toàn bộ EXE v1.9.8 từ source — chưa thể vì thiếu Go original.

## Xác minh lại trong thư mục làm việc trên ổ D

- Đối chiếu SHA-256 và kích thước của 273 tài nguyên đã nhập từ v1.9.8 FULL QA đều đạt.
- Ca kiểm thử HS6 trên biểu thuế v1.9.8 đã nhập đạt.
- EXE Core tham chiếu chạy trên localhost với dữ liệu mô phỏng. Các ca khởi động, đăng nhập, phiên, phục vụ tệp giao diện, đăng nhập lại sau khi khởi động lại và đọc tài khoản/lịch sử mô phỏng v1.9.8 đều đạt.
- Kết quả build và kiểm thử các thành phần v1.9.8 đã ghi tại [ghi chú tích hợp](INTEGRATION_V198.md). Lượt xác minh trên ổ D chạy trên EXE QA có sẵn.

## Kết quả khảo sát trước khi sửa API tài khoản

Đối chiếu request từ `apps/release-v198/web/static/index.html` với mã Core tham chiếu. Kiểm tra EXE QA sẵn có trên localhost, với thư mục dữ liệu tạm, một admin và hai người dùng giả.

| API | Cách kiểm thử | Kết quả trên bản QA hiện tại |
| --- | --- | --- |
| `POST /api/login`, `GET /api/me`, `POST /api/logout` | Đăng nhập, kiểm tra phiên, đăng xuất, gọi lại `/api/me` | Đạt. Sau đăng xuất, `/api/me` trả 401. |
| `POST /api/admin/users`, `GET /api/users` | Admin thêm người dùng từ giao diện, kiểm tra họ tên trong danh sách. Người dùng thường thử gọi API quản trị. | Phân quyền đạt, trả 403 cho người dùng thường. Tên bị bỏ trống vì UI gửi `display_name` còn Core chỉ đọc `displayName`. |
| `POST /api/admin/users`, `GET /api/me` | Để người dùng đăng nhập, admin khóa rồi mở khóa tài khoản. Kiểm tra phiên cũ ở cả hai thời điểm. | Khi khóa, cả phiên cũ và lần đăng nhập mới đều trả 401. Khi mở khóa, phiên cũ lại trả 200. Phiên cũ phải tiếp tục trả 401 cho đến khi đăng nhập lại. |
| `POST /api/account/password` | Dùng tài khoản giả, nhập sai mật khẩu hiện tại và thử đăng nhập bằng mật khẩu cũ. | Lỗi. API báo 200 và đăng nhập bằng mật khẩu cũ thất bại. Mã Core đọc `password` trong khi UI gửi `current_password` và `new_password`. |
| `POST /api/search-history`, `GET /api/search-history` | Mỗi người dùng tra một mã khác nhau. So kết quả của admin và hai người dùng. | Tổng chung là 2. Admin thấy cả hai dòng, mỗi người dùng thấy dòng của mình. `by_user` vẫn chứa tổng theo người dùng toàn hệ thống và `limit=1` bị bỏ qua. |
| `GET /api/search-history/export.csv`, `GET /api/history.xlsx` | Admin bấm hai nút xuất lịch sử. | Cả hai API trả 501. |

Qua đọc mã, thao tác `action: delete` của giao diện cũng cần xử lý riêng. Core hiện dùng chung đường ghi tài khoản và chưa áp dụng quy tắc vô hiệu hóa khi tài khoản có lịch sử. Ca xóa chưa chạy trên EXE.

## Kết quả sửa API tài khoản trên Windows QA

- Tạo tài khoản lưu đúng `display_name`, chấp nhận trường `displayName` từ client cũ. Từ chối tài khoản trùng và tài khoản mới thiếu mật khẩu. Sửa họ tên riêng lẻ giữ mật khẩu, quyền và trạng thái hiện tại.
- Đổi mật khẩu từ tài khoản thường kiểm tra mật khẩu hiện tại, giữ phiên thực hiện và thu hồi các phiên khác. Admin đặt lại mật khẩu sẽ thu hồi các phiên của tài khoản đó.
- Khóa tài khoản thu hồi phiên và xóa trạng thái đang kết nối. Mở khóa yêu cầu đăng nhập lại. API và giao diện xếp tài khoản bị khóa xuống cuối danh sách.
- Từ chối khóa, hạ quyền hoặc xóa admin đang hoạt động cuối cùng. Khi xóa tài khoản có bản ghi nghiệp vụ trong Core QA, tài khoản được khóa và bản ghi được giữ lại.
- `go test` và `go vet` đạt trên bốn module. Kiểm thử Windows qua HTTP đạt cho bản Core QA vừa build, gồm đổi họ tên, mật khẩu, khóa và mở khóa, thứ tự danh sách và lưu dữ liệu sau khởi động lại. Kiểm tra cú pháp JavaScript và ca HS6 đều đạt.

[Quy trình kiểm thử trên giao diện](ACCEPTANCE_CHECKLIST.md#kiểm-thử-thủ-công-tài-khoản-trên-core-qa-windows).

## Phân quyền lịch sử và bố cục thống kê QA

- Tổng và số liệu theo nhóm tra cứu là số liệu chung. Chi tiết và số liệu theo tài khoản trả về theo quyền, dùng tên đăng nhập để phân biệt tài khoản trùng họ tên. Admin xem được toàn bộ. Số lượt tra cứu cá nhân loại trừ dòng ghi nhận tệp MSDS.
- Ca Windows dùng hai người dùng trùng họ tên, hai lượt tra HS và một dòng MSDS mô phỏng. Kiểm tra phạm vi dữ liệu của từng người, admin và giới hạn một dòng. Hai API xuất lịch sử yêu cầu quyền admin và còn trả 501 cho admin.
- Nút xuất CAS Excel được đưa vào thanh thao tác. Bảng thống kê giảm khoảng cách và chiều cao các thẻ.

## Cam kết an toàn
Source `core-recovered` đã bổ sung guard **chống khởi động vô ý**; chỉ QA reference với biến môi trường `HSPO_ALLOW_INSECURE_RECOVERY_QA=I_ACKNOWLEDGE_INSECURE_QA_ONLY` mới cho chạy. Không chạy bằng dữ liệu thật hoặc mở trên LAN.
