# Dữ liệu pháp lý 2.0.1

`documents`: trạng thái văn bản. `has_forms` là tri-state: NULL chưa xác minh, 0 không có, 1 có.

`relations`: quan hệ AMENDS, SUPPLEMENTS, REPLACES, REPEALS, CONSOLIDATED_INTO; chỉ kết luận khi verified=1.

`forms`: từng mẫu/phụ lục có trạng thái riêng: VERIFIED_CURRENT, NEEDS_REVIEW, REPLACED, REPEALED, NOT_REVIEWED.

`relation_candidates`: ứng viên do quét nội dung phát hiện; chỉ phục vụ rà soát.
