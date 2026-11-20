# HQTN_HSPO project context

## 1. Purpose
HQTN_HSPO is a centralized lookup and decision-support system for customs officers. The user wants data and legal sources in one place, with clear provenance and conservative conclusions.

Core functional areas:
- HS lookup by code/name with 4/6/8-digit structure and HS master source.
- Import/export tariff, MFN, special preferential tariffs/FTA and C/O context.
- VAT logic with explicit legal basis and conservative fallback.
- Specialized-goods policy / quality-management policy.
- Chemicals/CAS/SDS/MSDS and chemical declaration warnings.
- Quarantine policy.
- Legal documents and source files.
- User management, search history, verification status, shared data, LAN deployment.

## 2. Baseline versions and key decisions
### v1.9.4
Policy Engine v2:
- Evaluates rules by HS + policy note + effective dates.
- States include: applying / needs comparison / transitional until date / expired / not-yet-effective.
- Must not conclude purely from HS when the law depends on description, purpose, composition, origin, condition, technical specifications or exclusions.
- Regression set contains 100 representative HS codes in `policy_data/regression_hs_v1.9.4.json`.

Priority rule groups include:
- Chemical import declaration Chapter 28/29 and exemptions under Clause 7.
- Civil cryptography NĐ 211/2025.
- Mineral policies.
- Currency/printing/gold-related NHNN rules.
- Health products, tobacco, printing, food safety, CITES, border-gate rules and other specialized policies.

### v1.9.4.1
UI-only refinement over v1.9.4:
- Compact policy cards.
- Prioritize Conclusion/Warning -> Conditions to check -> Legal basis.
- Technical/effective-date details move into expandable detail.
- No intended change to policy rules or datasets.

This is the current trusted functional baseline.

## 3. Important business rules already agreed
### VAT
- If HS and legal basis clearly establish a VAT rate, show the supported rate.
- If legal basis is not sufficiently clear, do not force 8%; show the base/possible rate such as `8%/10% – Cần xác định` and require officer verification.
- The tool supports the decision; it does not make the decision.

### HS/policy match
- A specialized-policy HS code that does not match `HS_MASTER` must be flagged for review.
- Do not silently map it to a “nearby” HS code.

### Chemical declaration
Current wording direction:
`Khai báo hóa chất nhập khẩu trước thông quan theo Điều 6 khoản 1 NĐ 26/2026/NĐ-CP. Cần kiểm tra hàng hóa thực tế và các trường hợp miễn tại khoản 7.`
- Provide a direct way to inspect Clause 7 / exemptions.
- A warning is a review signal, not automatic legal conclusion.

### OCR/CAS
- PDF/MSDS may be scan-only or vector-outline, so normal text extraction can fail.
- Tesseract offline was introduced after browser/PDF.js/Windows OCR fallback issues.
- CAS normalization must preserve valid CAS shape (two hyphens) and validate checksum where appropriate.
- Known historical OCR/CAS bad outputs included `3004111-12-7` and `28-16-0`; normalization should reject/fix invalid candidates, not pass them as valid CAS.
- Desired scan flow: Fast first (~1800 px); tile/fallback only if needed; progress percentage; user can switch tabs; automatically run lookup after CAS extraction.

## 4. Deployment and UX expectations
- Server: Windows machine on LAN.
- Clients: browsers; no per-client install.
- Some work PCs have restricted internet/proxy; core capability must work offline/LAN.
- Distribution must be simple: one ZIP/package, extract and run existing launcher/app.
- User strongly dislikes deployment that requires Python, command line, manual module copying or multiple servers.

## 5. Source/data organization visible in v1.9.4.1 package
Important paths:
- `static/index.html` — main web UI.
- `static/documents/registry.json` — legal document registry (72 documents in baseline).
- `static/documents/...` — Word/PDF/generated PDF/appendices.
- `policy_data/hs_policy.json`
- `policy_data/chemical_policy.json`
- `policy_data/chemical_rules.json`
- `policy_data/quarantine_policy.json`
- `policy_data/policy_engine_v2.json`
- `policy_data/legal_audit_33_v1.9.3.6.json`
- `policy_data/regression_hs_v1.9.4.json`
- `static/tariff_2026.json.gz`
- `static/hs_master_31_2022.json.gz`
- `static/quality_policy_2026.json.gz`
- `static/quarantine_policy_2026.json.gz`

The distributed package also includes compiled Windows executables for launcher/core/runtime/OCR/tray. Do not assume their source is available in this handoff package; use the actual Codex repo as source of truth if it contains source code.

## 6. Product philosophy
- Favor traceability over confidence theater.
- Every legal/policy claim should be traceable to a source document and, where relevant, exact article/clause/appendix.
- Distinguish: source data / automatically detected candidate / human verified status.
- Absence of a warning is not proof that no policy applies.
- Auto-detection may create a review queue, but it must not silently turn into an authoritative legal status.

## 7. Versioning and upgrade principle
The user wants future upgrades to be cheap:
- Internally, features may be modular.
- Persistent verified data must be outside replaceable module code or migrated safely.
- A future module upgrade should not require re-learning or re-entering verification work.
- End-user packaging must still be one simple release artifact.
