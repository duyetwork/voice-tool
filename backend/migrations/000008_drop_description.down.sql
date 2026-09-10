-- Trả lại cột mô tả (dữ liệu cũ đã mất khi drop, không khôi phục được).
ALTER TABLE source_post ADD COLUMN IF NOT EXISTS description TEXT;
ALTER TABLE voice       ADD COLUMN IF NOT EXISTS description TEXT;
