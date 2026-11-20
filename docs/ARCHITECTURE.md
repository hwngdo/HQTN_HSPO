# Kiến trúc tham chiếu

```text
Web client (static/index.html)
     |  HTTP/HTTPS 8870/8871 production
     v
Core HTTP API (gốc hiện chỉ còn binary)
  |-- HS & policy/VAT/FTA: JSON and *.json.gz read-only
  |-- CAS & chemical rules, Legal Reader (PDF + appendices, effective date)
  |-- Auth/User/History (server-side authorization + persistent storage)
  |-- OCR endpoint / Bridge 8872 -> PowerShell/Tesseract
  |-- Diagnostics / backups / data patch / rollback

Unified Launcher.exe -> Core, Tray, OCR Bridge (singleton)
UAT 8970 -> UAT Core and data directory *separate*; not proxy into production
```

### Định danh phiên bản

Không dùng một số tĩnh để thay thế trạng thái build: `/health` cần trả `app_version`, `build_hash`, `core_version`, `ocr_build`, `dataset_versions` độc lập. Tray phải đọc version từ cùng manifest với Launcher/Core.

### Source trees hiện có

- `apps/core-recovered/`: tài liệu/Go reconstruction, chưa sản xuất.
- `apps/qa-gateway/`: Go tests, account/diagnostic/gateway, chưa tích hợp.
- `apps/uat-preview-8970/`: cổng UI preview.
- `apps/legal-lookup-reference/`: Python module reference.
- `assets/static/`, `assets/policy_data/`: snapshot dữ liệu, UI và PDF từ RC2.1.

### Data / code separation

`policy_data/`, `static/documents/`, `static/*.json.gz` là tài nguyên cập nhật. Dữ liệu nhạy cảm user/history/logs nằm trong data dir riêng, tuyệt đối không thuộc gói data update hoặc source repo.
