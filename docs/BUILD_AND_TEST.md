# Build / test

Môi trường tham chiếu: Python 3.x, Go 1.23.x, Node 22.x cho static validation. Các module Go hiện không cần tải thư viện bên thứ ba.

Linux/macOS shell:

```bash
python3 scripts/verify_package.py
bash scripts/test_sources.sh
python3 scripts/extract_api_contract.py > docs/API_CONTRACT_DISCOVERED.md
python3 scripts/check_policy_snapshot.py
```

Windows (PowerShell):

```powershell
python scripts/verify_package.py
powershell -ExecutionPolicy Bypass -File scripts/build_qa_windows.ps1
```

Build Windows trong `build_qa_windows.ps1` tạo bốn EXE `*_REFERENCE_NOT_PRODUCTION.exe` và ba EXE `*_QA_NOT_PRODUCTION.exe` trong `dist/qa/`.

Sau khi Codex tích hợp source production: phải có pipeline riêng `build/release`, unit/contract/Windows tests, manifests/sha trước khi coi là hoàn chỉnh.

Trong quá trình Codex đang sửa source, có thể chạy `HSPO_SKIP_INTEGRITY=1 bash scripts/test_sources.sh` để không kiểm tra SHA snapshot cũ. Khi bàn giao release, cập nhật manifest SHA và xác minh lại toàn bộ.
