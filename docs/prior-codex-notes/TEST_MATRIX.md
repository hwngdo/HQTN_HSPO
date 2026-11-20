# Regression and acceptance test matrix

## A. Must-not-regress baseline
Before and after legal-document work, smoke-test the trusted v1.9.4.1 behavior:

### HS / Policy Engine
Representative codes mentioned in v1.9.4.1 test guidance:
- 29309090
- 25041000
- 25210000
- 24022090
- 71081210
- 38089490

Also run the existing 100-code regression dataset:
- `policy_data/regression_hs_v1.9.4.json`

Check:
- HS lookup works.
- selected-row/detail state resets correctly when code changes/clears.
- compact Policy Engine v2 UI remains compact.
- effective-date state works.
- no policy is concluded solely from HS when description/condition must be checked.

### Chemical/CAS
- Chemical declaration warning for Chapter 28/29 is preserved.
- Link/action to review Clause 7 exemptions remains available.
- CAS normalization rejects malformed CAS-like OCR strings.
- Valid CAS lookup still works.

### VAT
- Clear legally supported VAT mapping still returns the supported rate.
- Ambiguous reduction cases remain `8%/10% – Cần xác định` (or equivalent), never default to 8% without basis.

### LAN/deployment
- One-package release.
- Existing launcher/app still starts the server.
- Clients can connect over LAN as before.
- No internet dependency introduced.
- No new Python/runtime prerequisite for normal user use.

## B. Legal lookup acceptance
### Search
- Exact document number.
- Number typed without punctuation/spaces.
- Vietnamese keyword with accent.
- Same keyword without accent.
- issuing body filter.
- category/type filter.
- search by appendix/form identifier where indexed.

### Legal lineage
For a verified test fixture, cover:
1. Document A active, no amendments.
2. Document A amended by B but still active.
3. A partially active; Appendix II replaced by B.
4. A fully replaced by B.
5. B later amended by C (multi-hop chain).
6. Consolidated document exists but underlying legal-effect chain remains visible.

### Form lineage
For each fixture:
- old form visible in history but clearly not current.
- current form points to exact source document/file/location.
- effective date boundary is respected.
- unverified relation is never presented as current legal truth.

### Persistence
- Mark a relation/form as verified.
- Replace/update the legal module code.
- Verified data remains intact.
- Schema migration, if required, is explicit and backup-safe.

## C. Release checks
- Version string updated consistently in UI and version file.
- ZIP opens with integrity test.
- Start from a clean extracted folder.
- No absolute developer paths.
- No reference to `/mnt/data`, local build paths or unavailable tools.
- No stale test-only buttons/pages in production package.
