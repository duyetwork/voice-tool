# Xử lý lỗi hay gặp

| Hiện tượng | Nguyên nhân hay gặp |
|---|---|
| `api` restart liên tục | Thiếu `TOKEN_ENCRYPTION_KEY` hoặc `JWT_SECRET` — hệ thống fail-fast có chủ ý. Xem `docker compose logs api` |
| Đăng nhập báo 401 | Sai mật khẩu multime; hoặc tài khoản bị admin tắt. Nếu `MULTIME_BASE_URL` để trống thì mật khẩu chỉ cần ≥ 6 ký tự |
| Đăng nhập báo lỗi 2FA | Tài khoản bật 2FA — chưa hỗ trợ, phải tắt 2FA trên multime |
| Bấm "Tạo Bài Post" xong không thấy Voice | Xem http://localhost:8081 (job `voice:process` fail?) và `docker compose logs worker`. Hay gặp nhất: `yt-dlp` bị YouTube chặn |
| Nút **Đăng** bị mờ | Voice thiếu hashtag, hoặc audio ngắn hơn 15s — lý do hiện ngay dưới voice đó |
| Đăng báo "cần đăng nhập lại multime" | Token multime hết hạn và refresh cũng thất bại — đăng xuất rồi đăng nhập lại |
| `migrate` báo lỗi | Xem `docker compose logs migrate`. Nếu DB đã có dữ liệu cũ không khớp schema, nhanh nhất là `docker compose down -v` rồi dựng lại (mất dữ liệu) |

Nơi nhìn đầu tiên theo thứ tự: **Asynq monitor** (http://localhost:8081) →
`docker compose logs -f worker` → `docker compose logs -f api` → database
([docs/database.md](database.md)).
