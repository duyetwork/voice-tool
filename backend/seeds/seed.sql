-- Dữ liệu mẫu cho môi trường dev. KHÔNG chạy trên production.
-- Chạy: make seed
--
-- Không seed tài khoản: hệ thống đăng nhập bằng SSO của strongbody/multime,
-- app_user được tạo tự động ở lần đăng nhập đầu tiên. Đặt BOOTSTRAP_ADMIN_EMAIL
-- trong .env để tài khoản đó thành admin.

INSERT INTO ai_engine (name, provider, supported_languages, is_active)
VALUES
  ('3voices.win', '3voices',
   ARRAY['vi','en','zh','ja','ko','fr','de','es','th'], TRUE),
  ('ElevenLabs Multilingual v2', 'elevenlabs',
   ARRAY['vi','en','ja','ko','zh','fr','de','es','it','pt','id','th'], FALSE),
  ('Mock TTS (dev)', 'mock', ARRAY['vi','en','ja','ko','zh'], FALSE)
ON CONFLICT DO NOTHING;

-- Prompt cần created_by: gán cho admin đầu tiên. Chưa có ai đăng nhập thì bỏ qua.
INSERT INTO prompt (name, content, created_by)
SELECT * FROM (VALUES
  (
    'Tin nhanh 30 giây',
    'Viết lại nội dung thành bản tin đọc trong khoảng 30 giây (80-100 từ). ' ||
    'Câu ngắn, thông tin quan trọng nhất lên đầu, giọng trung tính như phát thanh viên. ' ||
    'Bỏ mọi thông tin quảng cáo, kêu gọi theo dõi, hoặc link.',
    (SELECT id FROM app_user WHERE role = 'admin' ORDER BY created_at LIMIT 1)
  ),
  (
    'Kể chuyện thân mật',
    'Viết lại nội dung thành đoạn kể chuyện thân mật, ngôi thứ nhất, ' ||
    'độ dài 120-150 từ. Dùng từ ngữ đời thường, có 1 câu mở đầu gây tò mò ' ||
    'và 1 câu kết đọng lại. Không dùng thuật ngữ chuyên ngành.',
    (SELECT id FROM app_user WHERE role = 'admin' ORDER BY created_at LIMIT 1)
  )
) AS seed(name, content, created_by)
WHERE seed.created_by IS NOT NULL
ON CONFLICT DO NOTHING;
