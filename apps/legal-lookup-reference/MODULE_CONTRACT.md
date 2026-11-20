# MODULE CONTRACT — legal_lookup 1.0

Host contract ổn định:
- UI: `/modules/legal_lookup/`
- API: `/api/modules/legal_lookup/`
- Endpoint: `health`, `stats`, `search`, `document`, `file`.

Dữ liệu nghiệp vụ nằm trong `data/legal_lookup.db`. Khi nâng code module không ghi đè DB. Quan hệ văn bản và trạng thái mẫu biểu chỉ được coi là đã xác minh khi có cờ `verified`/`source_verified` tương ứng.
