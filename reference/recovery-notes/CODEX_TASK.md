# Task for Codex

Use this reconstructed Go source only as a starting baseline. The original Go source was not included in the shipped v1.9.4.1 ZIP.

1. Read `docs/RECOVERY_MAP.md` first.
2. Use the real v1.9.4.1 `static/index.html`, `policy_data/`, `static/*.json.gz`, `static/documents/`, PowerShell OCR worker, and QA/changelog files as the behavioral contract.
3. Reconcile the recovered backend with every API call made by the frontend.
4. Add tests for auth, permissions, verify persistence, sources/system/presence/history, chemical CAS normalization, backup/update, OCR bridge and shutdown.
5. Preserve existing HS, tariff, VAT, Policy Engine v2, quarantine and chemical datasets unchanged unless a separately approved requirement says otherwise.
6. For legal-document lookup, implement it as a modular package and keep legal verification data outside the replaceable module so upgrades do not overwrite reviewed state.
7. Legal rule: every legal document with appendices/forms must track the lifecycle of those forms independently. Later amending/replacing documents may amend/replace/repeal individual appendices/forms even when the parent document remains effective.
