-- ---------------------------------------------------------------------------
-- Via và proxy có CHỦ SỞ HỮU.
--
-- Trước migration này via/proxy là tài sản chung của cả hệ thống và chỉ admin
-- đụng tới được. Thực tế vận hành khác: via là tài khoản CÁ NHÂN của người
-- nuôi nó, proxy là thứ họ tự bỏ tiền mua — người khác nhìn thấy danh sách đó
-- không giúp được gì, mà mỗi dòng lại lộ ra họ đang giữ bao nhiêu tài khoản
-- trên nền tảng nào.
--
-- Hình dạng mượn nguyên của ai_engine (migration 000010), vì đây đúng là cùng
-- một bài toán: một bản ghi bí mật, một chủ sở hữu, và admin đứng trên tất cả
-- để khai hộ và gán lại. Dùng lại đúng hai cột `user_id` / `created_by` với
-- cùng ý nghĩa thì không có luật mới nào phải nhớ.
--
--   user_id    — CHỦ SỞ HỮU: người thấy và sửa được bản ghi này.
--   created_by — NGƯỜI KHAI: khác chủ khi admin thêm hộ.
--
-- KHÔNG có bảng nối nhiều-nhiều như llm_api_set_user. Một bộ API key LLM chia
-- cho nhiều người là hợp lý vì nó chỉ là hạn mức; còn một via là MỘT phiên đăng
-- nhập, và hai người cùng đổ tải lên một phiên là cách nhanh nhất giết nó.
-- ---------------------------------------------------------------------------

ALTER TABLE scrape_via   ADD COLUMN user_id UUID REFERENCES app_user(id);
ALTER TABLE scrape_proxy ADD COLUMN user_id UUID REFERENCES app_user(id);

-- Via/proxy đang có đều do admin khai, nên người khai chính là chủ. Không có
-- cách đoán nào tốt hơn, và để NULL thì chúng biến mất khỏi mọi danh sách.
UPDATE scrape_via   SET user_id = created_by WHERE user_id IS NULL;
UPDATE scrape_proxy SET user_id = created_by WHERE user_id IS NULL;

ALTER TABLE scrape_via   ALTER COLUMN user_id SET NOT NULL;
ALTER TABLE scrape_proxy ALTER COLUMN user_id SET NOT NULL;

-- Danh sách của một người, mới nhất trước — đúng câu truy vấn của màn quản lý.
CREATE INDEX idx_scrape_via_owner   ON scrape_via(user_id, created_at DESC);
CREATE INDEX idx_scrape_proxy_owner ON scrape_proxy(user_id, created_at DESC);
