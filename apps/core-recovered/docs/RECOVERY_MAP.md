# HQTN_HSPO v1.9.4.1 — Go recovery map

This package is a **reconstructed, compile-ready baseline**, not the original source.
It was derived from the user-owned v1.9.4.1 distribution using:

- `go version -m` metadata
- function/type names retained in Go binary strings
- API calls and response fields used by `static/index.html`
- shipped JSON datasets and QA/changelog documents

## Confirmed build metadata

Both `HQTN_HSPO_Core.exe` and `HQTN_HSPO.exe` report:

- Go `1.23.2`
- `GOOS=windows`
- `GOARCH=amd64`
- `CGO_ENABLED=0`
- `-trimpath=true`
- command-line-arguments build path

## Confirmed Core symbols recovered from binary

`main.(*App).analyzeChem`, `auth`, `chemResult`, `ensureTLS`, `historyXLSX`, `load`, `login`, `logout`, `me`, `routes`, `saveHistory`, `saveUsers`, `saveVerify`, `searchHS`, `serveCert`, `shutdown`, `sourcesAPI`, `systemAPI`, `userFromReq`, `usersAPI`, `verify`, `writeHistoryXLSX`.

Confirmed type names include `App`, `ChemResult`, `Chemical`, `HSRecord`, `HistoryItem`, `Membership`, `Src`, `User`, `VerifyItem`.

## Confirmed Launcher symbols

`hiddenCmd`, `killName`, `msg`, `runPS`, `waitOCR`, `waitWeb`, `openBrowser`-equivalent behavior inferred from strings, plus `main`.

## Confirmed endpoints visible in v1.9.4.1 frontend/binary

- `/health`
- `/api/login`, `/api/logout`, `/api/me`
- `/api/users`, `/api/admin/users`, `/api/account/password`
- `/api/verify`
- `/api/chem/analyze`
- `/api/sources`, `/api/system`, `/api/presence`
- `/api/search-history`, `/api/search-history/export.csv`, `/api/history.xlsx`
- `/api/announcements`
- `/api/admin/backup`, `/api/admin/backups`, `/api/admin/update-preview`, `/api/admin/update-apply`, `/api/admin/rollback`
- `/api/shutdown`
- `/api/ocr` plus local OCR bridge routes `/ocr/start`, `/ocr/progress`, `/ocr/result`

Binary strings also contain `/api/hs`; v1.9.4.1 frontend mainly performs HS/tariff/policy lookup client-side from gzipped static datasets, so this may be retained compatibility code.

## Network information recovered

Binary strings contain `0.0.0.0:8870`, `0.0.0.0:8871`, `https://127.0.0.1:8871`, and the cookie name `hspo_session`.

## What is implemented here

Implemented: basic auth/session, users, verify, CAS-shape extraction, source stats, system/LAN info, presence, search-history, static-file serving, local shutdown, and a small launcher.

Placeholders remain for backup/update, XLSX/CSV exports, announcements, OCR bridge, TLS certificate generation, and full chemical policy enrichment. Those areas should be restored by Codex by reading the existing frontend and the shipped PowerShell/OCR scripts.

## Critical instruction to Codex

Do **not** replace the stable frontend/data or alter HS/Policy/VAT/CAS business rules merely to make the recovered backend easier. Treat the shipped v1.9.4.1 runtime data and `static/index.html` as the behavioral contract, and fill backend gaps to satisfy that contract.
