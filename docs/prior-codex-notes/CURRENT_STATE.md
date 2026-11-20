# Current state handed to Codex

## Trusted baseline
The user supplied `HQTN_HSPO_v1.9.4.1_LAN_Server_Win64.zip`. This is the baseline that should be preserved while implementing the new legal-document capability.

Known v1.9.4.1 package facts:
- Main UI: `static/index.html`.
- Legal registry: `static/documents/registry.json` with 72 document entries.
- Registry fields include category, document number, title, issuing body, source file, PDF file, optional appendix file, note, effective date/note.
- Source documents include Word/DOC, PDF and generated PDFs.
- Application includes packaged launcher/core/runtime/OCR/tray EXEs and offline Tesseract assets.

## Rejected v2.0.1 attempt
A ChatGPT-built v2.0.1 package was produced but the user stated: **“Hiện tại em chưa đáp ứng được nội dung anh yêu cầu.”** Therefore it is not an accepted baseline.

What that attempt changed:
- Changed version text from v1.9.4.1 to v2.0.1.
- Added a top-level `Văn bản` navigation action that opened `static/modules/legal_lookup/index.html`.
- Added `static/modules/legal_lookup/data/legal_index.json` generated from the 72-document registry.
- Added `static/legal_data/legal_overrides.json` as a persistent overlay idea.
- Heuristically scanned Word/DOC text for amendment keywords and known document numbers.
- Heuristically counted `Mẫu` and `Phụ lục` keywords.

Why it is insufficient:
- It does not model legal effect deeply enough.
- It does not establish a verified chain of current form/appendix versions.
- It is too close to a file/keyword browser rather than a legal-lineage decision-support tool.
- It is visually/functionally separated from the main app rather than fully integrated.

## Reference files included in this handoff
`reference_current_attempt/` contains:
- `build_legal_module.py` — generator from the rejected attempt.
- `legal_lookup/index.html` — rejected module UI.
- `legal_lookup/legal_index.json` — generated index snapshot.
- `legal_lookup/module.json` — module metadata idea.
- `legal_overrides.json` — persistent overlay skeleton.
- `diffs/main_index_1941_to_201.diff` — exact main static UI diff between baseline and the rejected attempt.

Use these to understand what was tried, not as acceptance criteria.

## Recommended implementation direction
If the actual Codex repo includes application/server source, implement a proper reusable legal-data service/API there rather than keeping the feature as a purely static disconnected module.

Suggested split:
- legal ingestion/parser
- normalized legal registry DB/data
- verified relation/form lineage store
- API/service
- integrated main UI
- admin verification/update UI

Keep a stable module boundary internally, but distribute as one normal HQTN_HSPO package.
