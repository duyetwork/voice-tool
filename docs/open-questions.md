# Câu hỏi mở

> Trạng thái tổng thể của dự án (đã làm gì, rủi ro, việc tiếp theo) nằm ở
> **[docs/status.md](status.md)**. File này chỉ giữ những câu **chưa có câu
> trả lời**.

---

## 1. Kênh nguồn Facebook / TikTok / Instagram / X là của ai? 🔴

Chặn việc làm adapter cho 4 nền tảng này.

| Nền tảng | Đọc được kênh của mình | Đọc được kênh của người khác |
|---|---|---|
| Facebook / Instagram | ✅ Graph API (Page/Business account đã liên kết) | ❌ Không có API |
| TikTok | ✅ Display API (user đã cấp quyền) | ❌ Không có API |
| X | ✅ | ⚠️ API v2 thu phí, gói Basic ~10k tweet/tháng |

Nếu là kênh của người khác thì với TikTok/Instagram buộc phải scraping — cần
quyết định về pháp lý/vận hành **trước khi** code, vì nó thay đổi hẳn cách làm
adapter (proxy pool, chống chặn, chi phí).

## 2. Voice do F2/F3 tự tạo nên đăng dưới tài khoản nào? 🟡

Hiện tại: tài khoản của **người tạo ra danh sách kênh** (`list.created_by`).

Hệ quả (xem [status.md §3.1](status.md)): nếu người đó không đăng nhập
voice-tool lâu ngày, refresh token hết hạn và **mọi auto-publish của các kênh
họ tạo đều dừng**.

Phương án khác: dùng 1 tài khoản "bot" riêng cho toàn bộ voice tự động — auto
publish ổn định hơn, nhưng bài đăng không mang tên người thật.

## 3. `user` có được xem Nhật ký thao tác không? 🟢

Hiện có. Nếu coi audit log là dữ liệu vận hành thì nên giới hạn admin.

## 4. `DEFAULT_USER_ROLE` nên là `user` hay `editor`? 🟢

Hiện `user` — ai đăng nhập được bằng tài khoản multime là đăng voice được ngay,
khớp với "user bình thường có quyền post voice", nhưng không xoá được dữ liệu
của người khác. Đổi sang `editor` nếu muốn ai cũng xoá được.

## 5. Bài do voice-tool đăng có cần phân biệt với bài user tự đăng? 🟢

Trên multime hiện không phân biệt được. Nếu cần (để thống kê hoặc rollback hàng
loạt) thì chốt 1 hashtag/category riêng và đặt vào `MULTIME_DEFAULT_HASHTAGS` /
`MULTIME_CATEGORY_IDS`.

## 6. AI Engine chọn theo kênh hay 1 engine toàn hệ thống? 🟢

Hiện 1 engine mặc định toàn hệ thống (engine `is_active` đầu tiên). Muốn chọn
theo từng kênh thì thêm cột `ai_engine_id` vào `list_breaking` / `list_scheduled`.

---

## Đã chốt (không cần trả lời lại)

| Câu hỏi cũ | Kết luận |
|---|---|
| API post voice của multime | `POST /v1/seller/voice-posts/upload` — 1 request, xem [status.md §1.5](status.md) |
| Nhà cung cấp TTS | 3voices.win, adapter đã xong |
| Nhiều regex cho 1 kênh | Có, kết hợp OR, tối đa 20 |
| Tần suất quét tự do hay cố định | Tự do, tối thiểu 1 phút (F3) / 15 giây (F2) |
| Số worker | api 1 / worker 2 / scheduler 1 |
| Nơi lưu file voice | S3 dùng chung bucket `strongbody-files-api`, prefix `voice-tool/` |
| Dọn `skipped_log` | 7 ngày, job chạy 03:15 hằng ngày |
| Phân quyền | admin / editor / user — bỏ vai trò chỉ-xem |
| Đăng ký tài khoản | Không có — chỉ đăng nhập bằng SSO strongbody |
| Auto-detect nền tảng thất bại | Báo lỗi rõ, không cho chọn tay |
| 1 Bài Post ra nhiều Voice | Được, gọi `/run` nhiều lần |
| Cùng 1 bài ở cả 2 danh sách | Chỉ vào hệ thống 1 lần — dedup theo `(platform, post_id_extracted)` toàn hệ thống |
| Có giữ trường mô tả không | **Không.** multime chỉ hiển thị `title` (form đăng của họ luôn gửi `caption` rỗng) nên mô tả là dữ liệu chết. Tiêu đề Bài Post = toàn bộ nội dung bài trừ hashtag; tiêu đề Voice = nội dung đó gộp 1 dòng, cắt 200 ký tự |
