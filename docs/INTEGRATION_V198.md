# Tích hợp HQTN_HSPO v1.9.8

Đây là nhánh làm việc cho QA. Bộ FULL QA ngày 19/11/2026 được nhập vào `imports/v1.9.8-full-qa/`. `IMPORT_MANIFEST.json` ghi SHA-256 của 273 tài nguyên nhập. Snapshot RC2.1 trong `assets/` vẫn là dữ liệu tham chiếu.

`apps/release-v198/web/static/` chứa bốn tệp giao diện từ bản tra cứu biểu thuế đã thử. Ba tệp JavaScript mới đóng gói cùng ứng dụng. Các tệp dữ liệu, tài liệu và JavaScript còn lại lấy từ bản FULL tương ứng. `apps/release-v198/INPUT_PROVENANCE.json` ghi SHA-256 trước khi chỉnh sửa của các tệp lấy từ bản phát hành và bản thử.

## Thành phần Windows

| Thành phần | Mã hiện có |
| --- | --- |
| `HQTN_HSPO.exe` | Mã trình khởi động bên ngoài trong `apps/release-v198/cmd/launcher` |
| `HQTN_HSPO_Tray.exe` | Mã Tray v1.9.8 trong `apps/release-v198/cmd/tray` |
| `HQTN_HSPO_UpdateGateway.exe` | Mã Gateway v1.9.8 và các bài kiểm thử trong `apps/release-v198/cmd/update-gateway` |
| `HQTN_HSPO_Core.exe` | Mã tái dựng tham chiếu. Một số endpoint còn trả 501 |
| `HQTN_HSPO_Launcher.exe` | Launcher máy chủ gốc của v1.9.8 chưa được đối chiếu với nguồn build tương ứng |
| `HQTN_HSPO_OCR_Bridge.exe` | Có OCR worker PowerShell và mã kiểm tra trạng thái trong bộ phát hành. Chưa có mã đầy đủ của Bridge |
| `HQTN_HSPO_TEST_Runtime_Server.exe` | Có EXE riêng trong v1.9.8. Chưa xác nhận mã preview tham chiếu build được hành vi tương đương |

Chạy `scripts/build_qa_windows.ps1` trên Windows với Go 1.23.2 hoặc mới hơn. Đầu ra QA nằm trong `dist/qa/`. Chạy kiểm thử cảnh báo HS6 trên dữ liệu nhập bằng lệnh sau.

```powershell
node apps/release-v198/web/check-tariff.cjs imports/v1.9.8-full-qa/static/tariff_2026.json.gz
```

Đặt `HSPO_QA_LEGACY_TARIFF` thành đường dẫn tuyệt đối tới biểu thuế đã nhập để chạy thêm kiểm thử tương thích ở Gateway. Core tham chiếu chỉ lắng nghe localhost, yêu cầu mật khẩu QA riêng khi khởi tạo và dừng nếu gặp tệp dữ liệu cũ không đọc được. Việc này ngăn ghi đè dữ liệu khi định dạng chưa hỗ trợ.

Sau khi build, chạy `python scripts/smoke_recovered_windows.py` để kiểm tra EXE Core tham chiếu trên localhost bằng tài khoản thử và thư mục tạm. Bài kiểm tra đăng nhập lại sau khi khởi động lại, rồi dùng `stage_legacy_userdata.py` để nhập dữ liệu mô phỏng cấu trúc v1.9.8 sang thư mục QA mới. Trình nhập giữ nguyên tệp nguồn, chuyển lịch sử JSONL thành mảng JSON và lưu bản sao nhật ký quản trị cùng lịch sử CAS dạng Excel trong `legacy_archive/`.

## Đọc MSDS trên bản tích hợp QA Windows

PDF có lớp chữ được đọc trong trình duyệt. Khi bộ đọc lớp chữ chưa tìm thấy mã CAS, giao diện trên bản Core QA gọi `POST /api/ocr` cùng địa chỉ đang mở, cổng mặc định 18971. Endpoint yêu cầu đăng nhập và chạy bộ OCR Tesseract đi kèm cục bộ.

Trong thư mục chạy QA, chép `apps/release-v198/tools/HQTN_HSPO_OCR_Worker.ps1` cạnh EXE, rồi đặt thư mục `tools/tesseract/` từ gói 1.9.8 sạch vào cùng thư mục chạy. Bộ Tesseract là phụ thuộc Windows. Mã nguồn Worker giữ nguyên từ bản cài 1.9.8 tại Project2, với SHA-256 `6f636286f1e72ac14ac8d438bc07700937af81a1363164b55b1c5c197fa80fc9`.

Core QA dùng thư mục tạm cho từng lần OCR và xóa tệp công việc khi kết thúc. Chức năng chạy trên localhost. Nghiệm thu từ máy trạm LAN vẫn thuộc bản phát hành đầy đủ.

## ZIP dữ liệu và hồ sơ rà soát

`apps/release-v198/tools/TAO_GOI_DU_LIEU.py` tạo manifest `HQTN_HSPO_DATA_V1` tương thích Gateway. Trình tạo gói tham chiếu trong `scripts/create_data_patch.py` dùng manifest khác. Dùng Gateway để xem trước, so SHA-256, sao lưu, áp dụng có phê duyệt và khôi phục. Gateway QA kiểm tra thêm danh sách `records`, mã HS, bảng `rates` và ngày rà soát hợp lệ theo lịch. Ngày rà soát và chuỗi bằng chứng chỉ xác nhận sự hiện diện của hồ sơ. Người rà soát vẫn cần đối chiếu văn bản, ngày hiệu lực và từng phụ lục bị sửa đổi hoặc thay thế theo `DATA_PATCH_AND_LEGAL_REVIEW.md`.

Nghiệm thu phát hành còn cần chạy với Core đầy đủ và dữ liệu mẫu tương thích v1.9.8, OCR từ máy trạm, khởi động lại và khôi phục trên cấu hình Windows/LAN cách ly. Sử dụng `ACCEPTANCE_CHECKLIST.md` để ghi nhận kết quả trước khi phát hành.

## Kết quả Windows ngày 20/11/2026

- Go 1.23.2 trên Windows amd64 build thành công ba EXE QA từ mã trình khởi động bên ngoài, Tray và UpdateGateway v1.9.8, cùng bốn EXE tham chiếu.
- `go test` và `go vet` đạt cho bốn module. Các ca UpdateGateway có xem trước, áp dụng, sao lưu, khôi phục với Core giả lập và kiểm tra schema bằng tệp biểu thuế v1.9.8 đã nhập.
- Core tham chiếu chạy được trên localhost với dữ liệu tạo trong thư mục tạm. Smoke test kiểm tra health, đăng nhập Admin, phiên đăng nhập, tệp JavaScript v1.9.8 và đăng nhập lại sau khi chạy lại EXE.
- Kiểm thử Node đọc biểu thuế v1.9.8 đã nhập và xác nhận chiều cảnh báo HS6 cho cặp mã 01013010 và 01013090.
- Bản phát hành v1.9.8 lưu `users.json` và `search_history.jsonl` trong `userdata/`. Core tham chiếu lưu `users.json` và `search_history.json` trong `data/`. Cần tiếp tục xác nhận thuật toán mật khẩu, hành vi nhật ký và quyền truy cập lịch sử trước khi chuyển tài khoản đang sử dụng.
