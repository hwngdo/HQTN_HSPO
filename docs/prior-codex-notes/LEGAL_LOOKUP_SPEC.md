# Legal document / form lookup — functional specification

## 1. User intent
The feature is not merely a list of legal files. It must help an officer determine the **currently applicable legal basis**, especially when later documents amend/supplement earlier documents and when the amendment changes an appendix/form/template.

Core question the UI should answer:
> For this legal document/procedure, what is the current chain of effect, and which exact appendix/form/template is currently applicable?

## 2. Mandatory legal-data model
Treat these as separate entities, not a single document row:

### LegalDocument
Suggested fields:
- `document_id`
- `document_no`
- `title`
- `document_type`
- `issuing_body`
- `issued_date`
- `effective_from`
- `effective_to`
- `status`: `ACTIVE`, `PARTIALLY_ACTIVE`, `REPEALED`, `REPLACED`, `NOT_YET_EFFECTIVE`, `UNKNOWN`
- `status_verified`: bool
- `source_file`, `pdf_file`
- `source_url` if maintained
- `notes`

### LegalRelation
Relations must be directional and article-aware when possible:
- `AMENDS`
- `SUPPLEMENTS`
- `REPLACES`
- `REPEALS`
- `CONSOLIDATES`
- `GUIDES`
- `CORRECTS`

Fields should include:
- source document
- target document
- relation type
- affected article/clause/appendix/form if known
- effective date of the relation
- evidence/source excerpt or locator
- `verification_status`: `AUTO_CANDIDATE`, `VERIFIED`, `REJECTED`

### AppendixOrForm
A form/appendix has its **own lifecycle**:
- stable identity (`form_id`)
- parent document/version
- name/number (e.g. Mẫu số 01, Phụ lục II)
- purpose/procedure
- source file/page/location
- effective_from/effective_to
- status (`CURRENT`, `REPLACED`, `REPEALED`, `UNKNOWN`)
- replacement form/version if any
- verification status

A parent document being active must never automatically make all historical forms under it current.

## 3. Amendment-chain behavior
When viewing a document, show:
- Original document.
- Later documents that amend/supplement/replace/repeal it.
- Earlier document(s) that it amends.
- Current effective state with explanation.
- Affected articles/clauses/appendices/forms where known.

When a later document changes a form:
- Mark the old form as superseded from the relevant effective date.
- Point to the new/current form.
- Preserve history so an officer can determine which form applied on a past date.

Date-sensitive lookup should be possible later; design data now so it is not blocked.

## 4. Search requirements
Search by:
- document number (with tolerant punctuation/spaces)
- title/keyword
- issuing body
- document type
- field/category
- effective status
- article/clause/appendix/form number or name
- amended/replacing document

Search should handle Vietnamese accents and common no-accent input.

## 5. UI requirements
Keep the visual language of v1.9.4.1; do not introduce a disconnected mini-app.

Recommended layout:
- Integrated top-level tab `Văn bản`.
- Left: searchable/filterable document list.
- Right: detail panel with compact tabs/sections:
  1. `Tổng quan`
  2. `Hiệu lực & quan hệ`
  3. `Phụ lục / Mẫu biểu`
  4. `Văn bản gốc`

At a glance show:
- current verified status
- whether there is a later amending/replacing document
- whether forms exist
- whether the current form version is verified

Important badges:
- `Đang hiệu lực`
- `Hiệu lực một phần`
- `Đã bị thay thế`
- `Đã bị bãi bỏ`
- `Chưa xác minh`
- `Mẫu hiện hành đã xác minh`
- `Mẫu cũ – đã bị thay`
- `Cần rà soát mẫu biểu`

Do not show “Chưa phát hiện” as if it were legally meaningful. Prefer wording such as `Chưa có dữ liệu xác minh trong hệ thống`.

## 6. Automatic extraction/detection
Automatic parsing may detect candidate relations from phrases such as sửa đổi/bổ sung/thay thế/bãi bỏ, but this is only an **ingestion aid**.

Rules:
- Store candidates separately from verified legal relations.
- Do not elevate candidates to legal status automatically.
- Avoid naïve keyword counts as the primary feature.
- Extract exact cited document number and affected provision if possible.
- If the text says “thay thế Phụ lục ...” or “Mẫu số ... được thay bằng ...”, create a candidate relation at the **form/appendix level**, not merely document level.

## 7. Data update workflow
Admin/update workflow should support:
1. Add/import a new legal document and source file.
2. Parse metadata/citations.
3. Show detected candidate relations.
4. Human verifies/rejects relations.
5. Human verifies document status.
6. Human verifies appendix/form changes.
7. Publish updated index.

Persistent verification data must survive module/code upgrades.

## 8. Integration with the rest of HQTN_HSPO
Other modules should be able to deep-link to a legal document, article/clause, or form by stable ID.
Examples:
- HS/Policy card -> legal basis detail.
- VAT result -> current legal source.
- Chemical warning -> Điều 6 / Khoản 7.
- Specialized policy -> exact appendix/list/form.

Avoid duplicating legal-status logic independently in each module. Prefer one reusable legal registry/service.

## 9. Acceptance examples
A correct implementation should support scenarios like:
- Document A is still partly effective, but a later Document B replaced only Appendix II. The UI must show A as partially effective and Appendix II from B as current.
- Document C amended Article 5 and replaced Form 03. Searching C, Article 5, or Form 03 should expose that relationship.
- A user opens an old form: the UI should visibly state it is superseded and link to the current form.
- The system has not verified the lineage: show `Chưa xác minh`, not “current”.

## 10. Rejected v2.0.1 attempt
The existing attempt in `reference_current_attempt/` is not accepted because it primarily:
- builds a flat index from `registry.json`,
- counts keywords like `Mẫu`/`Phụ lục`,
- heuristically detects document citations near amendment keywords,
- opens the legal module as an isolated page/iframe-like experience,
- does not provide a robust verified form-version lineage.

Reuse useful code only if it helps; redesign the data/UX where necessary.
