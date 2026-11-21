# Bộ cài HQTN_HSPO QA cho Windows

Chạy `HQTN_HSPO_QA_Setup_Win64.exe`, chọn thư mục riêng và đặt mật khẩu cho tài khoản `admin` khi cài lần đầu. Mở **HQTN_HSPO QA** từ menu Start. Bản cài chạy ở `http://127.0.0.1:18972/` để thử song song với bản QA trước đó và bản 1.9.8. Tài khoản, lịch sử và bản sao lưu dữ liệu nằm trong thư mục đã cài.

## Cập nhật offline

Đăng nhập Admin, mở **Hệ thống → Dữ liệu**, chọn ZIP và bấm **Kiểm tra gói ZIP offline**. Xem phiên bản, danh sách tệp và hồ sơ rà soát trước khi bấm **Áp dụng cập nhật**. Khởi động lại Server từ Tray và làm mới trình duyệt để nạp dữ liệu. Có thể chọn **Khôi phục** ở dòng bản sao lưu rồi khởi động lại.

`QA_DEMO_DATA_UPDATE.zip` dùng để thử luồng xem trước, áp dụng và khôi phục. Gói chỉ thêm tệp đánh dấu và đặt phiên bản dữ liệu thử `QA-demo-1`.

## Cập nhật online qua GitHub

Trên mỗi máy cài QA, nhấp phải biểu tượng Tray và chọn **Cấu hình quyền đọc GitHub**. Nhập fine-grained token giới hạn repo `hwngdo/HQTN_HSPO` với quyền **Contents: Read**. Windows lưu quyền đọc cho tài khoản Windows đang chạy QA. Có thể dùng lại lệnh này để thay token khi hết hạn. [Quyền đọc kho GitHub](https://docs.github.com/en/rest/releases/assets#get-a-release-asset)

- **Tray → Kiểm tra cập nhật** báo phiên bản đang chạy và phiên bản mới. Sau khi xác nhận, Tray tải bộ cài, kiểm tra chữ ký và SHA-256, sao lưu tệp chương trình, rồi cài và mở lại Server. Có thể mở **Xem cập nhật trên web** từ cùng menu.
- **Hệ thống → Cập nhật chương trình** dành cho Admin. Bấm **Kiểm tra cập nhật** để đọc ghi chú phát hành đã ký và xem hai phiên bản. Bấm **Tải và cài đặt** sau khi duyệt. Tray sẽ tiếp nhận gói và tự mở lại Server.
- **Hệ thống → Dữ liệu → Kiểm tra GitHub online** tải `DATA_UPDATE.zip` từ bản phát hành dữ liệu. Các bước duyệt và áp dụng giống offline.

Bộ cập nhật lưu nhật ký bộ cài trong `program_updates/install-*/setup.log`. Nếu bộ cài báo lỗi hoặc Core mới không lên, bộ cập nhật khôi phục tệp chương trình trước cập nhật và mở lại Tray. Phiên đăng nhập web được tạo lại sau khi Server khởi động lại.

## Build và kiểm thử

`scripts/build_installer_qa.ps1` nhận thư mục bộ QA có `SHA256SUMS.txt`, đường dẫn `ISCC.exe` của Inno Setup, số phiên bản, khóa ký và tệp ghi chú phát hành UTF-8. Script kiểm SHA-256 tài nguyên đầu vào, build Core và Tray với khóa công khai, rồi tạo bộ cài và manifest đã ký trong `dist/installer_qa/<phiên bản>/`. [Inno Setup](https://jrsoftware.org/inno-setup/downloads)

```powershell
$releaseKey = Join-Path $env:LOCALAPPDATA 'HQTN_HSPO_Release\qa-signing.ed25519'
.\scripts\build_installer_qa.ps1 -Baseline 'dist\HQTN_HSPO_v1.9.8_SEARCH_ACCOUNTS_QA' -InnoCompiler 'C:\...\ISCC.exe' -AppRelease '1.9.9' -SigningKey $releaseKey -ReleaseNotes 'installer\RELEASE_NOTES_QA.md'
```

Giữ khóa riêng trên máy phát hành và sao lưu trong nơi quản lý bí mật của anh. Nếu bắt đầu kênh mới, tạo khóa bằng `go run ./apps/release-v198/cmd/hspo-release -action keygen -key <đường-dẫn-khóa>`. Dùng cùng khóa cho các phiên bản sau để các máy đã cài xác minh được bản cập nhật.

Tạo GitHub Release với tag `qa-v<phiên bản>`. Đính kèm đúng cặp `HQTN_HSPO_QA_Setup_Win64.exe` và `HQTN_HSPO_QA_Release.json` từ cùng thư mục build. Công bố Release để máy khách đọc được. Bản nháp, bản prerelease và tag chỉ có commit được bỏ qua. [GitHub Releases](https://docs.github.com/en/rest/releases/releases#list-releases)

Trên máy chưa đăng ký bộ cài QA, chạy `python -B scripts/smoke_installer_windows.py <bộ-cài.exe> --program-manifest <manifest-đã-ký.json>`. Bài thử dùng thư mục tạm và dữ liệu tổng hợp để kiểm tra áp dụng ZIP, bàn giao bản cài đã ký, giữ tài khoản và dữ liệu, rollback dữ liệu và gỡ cài đặt.
