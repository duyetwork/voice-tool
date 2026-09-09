# API Reference

Base URL: `http://localhost:8080/api/v1`

Mọi endpoint (trừ `/auth/*` và `/healthz`) yêu cầu header:

```
Authorization: Bearer <access_token>
```

Access token hết hạn sau 15 phút; frontend tự gọi `/auth/refresh` một lần khi
gặp 401.

## Đăng nhập

Hệ thống **không có đăng ký**. Đăng nhập bằng tài khoản
**strongbody/multime.ai** — `POST /auth/login` proxy sang
`https://api-v2.strongbody.ai/v1/public/auth/login`, tạo `app_user` ở lần đầu
và phát JWT của voice-tool.

Tài khoản đăng nhập **chính là tài khoản đăng voice**: khi publish, worker dùng
access token của người tạo ra voice đó, nên bài xuất hiện trên multime dưới
đúng tài khoản của họ.

## Phân quyền

| Role | Xem | Tạo/sửa/chạy/đăng voice | Xoá | Cấp quyền |
|---|---|---|---|---|
| `viewer` | ✅ | ❌ | ❌ | ❌ |
| `user` | ✅ | ✅ | ❌ | ❌ |
| `admin` | ✅ | ✅ | ✅ | ✅ |

Tài khoản đăng nhập lần đầu nhận role `DEFAULT_USER_ROLE` (mặc định `user`).
Email trong `BOOTSTRAP_ADMIN_EMAIL` luôn được nâng lên `admin`.

Thao tác vượt quyền trả `403 forbidden`.

## Format lỗi

```json
{ "error": "invalid_request", "message": "mode C bắt buộc phải có prompt_id" }
```

| HTTP | `error` | Khi nào |
|---|---|---|
| 400 | `invalid_request` | payload sai, URL không nhận diện được, regex không hợp lệ, mode C thiếu prompt, tài khoản bật 2FA |
| 401 | `unauthorized` | thiếu/sai token, sai email-password |
| 403 | `forbidden` | role không đủ quyền (viewer gọi endpoint ghi) |
| 404 | `not_found` | không tìm thấy bản ghi |
| 409 | `already_published` | sửa/đăng lại Voice đã publish |
| 500 | `internal_error` | lỗi hệ thống |

---

## Auth

| Method | Path | Body | Trả về |
|---|---|---|---|
| POST | `/auth/login` | `email`, `password` (tài khoản multime) | `{ token: {access_token, refresh_token, expires_in}, user: {id, email, full_name, avatar, role} }` |
| POST | `/auth/refresh` | `refresh_token` | `{ token }` — làm mới JWT của voice-tool, không liên quan token multime |
| GET | `/me` | — | `{ id, role, permissions: {can_write, can_delete, can_manage_users} }` |

Không có `POST /auth/register`. Lỗi đăng nhập:

| HTTP | Khi nào |
|---|---|
| 401 | Sai email/mật khẩu, hoặc tài khoản bị admin tắt |
| 400 | Tài khoản bật 2FA (chưa hỗ trợ) |

## Bài Post (SourcePost)

| Method | Path | Ghi chú |
|---|---|---|
| POST | `/source-posts` | Luồng F1. Body: `source_url`, `collect_mode` (A/B/C), `prompt_id?`, `language?` (mặc định `auto`), `platform?` (bỏ trống = tự nhận diện từ URL), `auto_process?` (mặc định `true`) |
| GET | `/source-posts` | Query: `source_type`, `status`, `platform`, `collect_mode`, `language`, `created_by`, `created_from`, `created_to` (`YYYY-MM-DD`), `list_breaking_id`, `list_scheduled_id`, `limit`, `offset`. Mỗi item kèm `created_by_email` |
| GET | `/source-posts/:id` | |
| PATCH | `/source-posts/:id` | Sửa `collect_mode`, `prompt_id`, `language` — chỉ khi chưa `processing` |
| DELETE | `/source-posts/:id` | Cascade xoá Voice liên quan. **Chỉ admin** |
| POST | `/source-posts/:id/run` | → `202` `{status: "queued"}`. Enqueue `voice:process` |

Bài Post giữ luôn metadata gốc lấy từ nền tảng — `title`, `description`,
`hashtags[]`, `thumbnail_url`, `author_name`, `posted_at`. Worker ghi các field
này khi chạy `voice:process`, và Voice sinh ra được điền sẵn từ đó (tiêu đề, mô
tả, hashtag, ảnh bìa) nên form đăng bài không phải gõ tay.

```bash
curl -X POST http://localhost:8080/api/v1/source-posts \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"source_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","collect_mode":"B","language":"vi"}'
```

## Voice

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/voices` | Query: `publish_status`, `source_post_id`, `platform`, `language`, `created_by`, `created_from`, `created_to`, `published_from`, `published_to`, `limit`, `offset`. Mỗi item kèm `platform`, `source_url`, `source_title`, `created_by_email` |
| GET | `/voices/:id` | |
| GET | `/voices/:id/audio` | Trả file voice để nghe thử; `?download=1` để tải về. Nhận token qua header **hoặc** `?token=` — thẻ `<audio>` không gắn được header `Authorization`. `400` nếu voice đã publish (file đã bị xoá theo business rule #2) |
| PATCH | `/voices/:id` | Sửa `title`, `description`, `hashtag`, `language`, `image_url`. `409` nếu đã publish |
| POST | `/voices/:id/ready` | `draft` → `ready` (đã duyệt, chờ đăng) |
| POST | `/voices/:id/publish` | → `202`. Enqueue `voice:publish` |
| DELETE | `/voices/:id` | Xoá cả file trên storage nếu còn. **Chỉ admin** |

> Không có `POST /voices`: Voice chỉ sinh ra từ việc chạy Bài Post
> (business rule #1).

Publish lên multime.ai yêu cầu Voice thoả 3 điều kiện, nếu không job sẽ fail với
`last_error` giải thích rõ (FE cũng disable nút Đăng kèm lý do):

| Điều kiện | Ghi chú |
|---|---|
| Có `title` | Thiếu thì hệ thống tự lấy dòng đầu của `description` |
| Có ít nhất 1 hashtag | Thiếu thì dùng `MULTIME_DEFAULT_HASHTAGS` |
| `duration_seconds` ≥ 15 | Giới hạn của multime.ai, không bỏ qua được |
| Token multime của người tạo voice còn hiệu lực | Hết hạn thì tự refresh; refresh lỗi thì `last_error` = "cần đăng nhập lại multime" |

## Danh sách Breaking (F2)

| Method | Path | Ghi chú |
|---|---|---|
| POST | `/lists/breaking` | Body: `source_url`, `regex_patterns[]`, `collect_mode`, `prompt_id?`, `language_default?`, `auto_process?`, `auto_publish?`, `status?`, `scan_limit?`, `scan_interval?` |
| GET | `/lists/breaking` | Query: `status`, `search` (regex lọc theo `source_url`), `limit`, `offset` |
| GET | `/lists/breaking/:id` | |
| PATCH | `/lists/breaking/:id` | Mọi field ở trên đều optional |
| DELETE | `/lists/breaking/:id` | **Chỉ admin** |
| POST | `/lists/breaking/:id/run` | → `202`. Quét thủ công 1 vòng để test regex |

`regex_patterns` là **mảng** — nhiều pattern kết hợp **OR**, khớp 1 pattern là
bắt bài. Mỗi phần tử nhận cả input thô, backend chuẩn hoá về regex:

| User gõ | Lưu vào DB |
|---|---|
| `#tinnong` | `\#tinnong\b` |
| `tin nóng` | `(?i)tin\s+nóng` |
| `bão, lũ` | `(?i)(bão\|lũ)` |
| `/^BREAKING/i` | `(?i)^BREAKING` |
| `\bCPI\b` | `\bCPI\b` (giữ nguyên) |

Tối đa 20 pattern/kênh; pattern trùng nhau sau khi chuẩn hoá bị loại tự động.

**Tham số quét** (bỏ trống = dùng chỉ số tối ưu của hệ thống trong `.env`):

| Field | Mặc định | Ý nghĩa |
|---|---|---|
| `scan_limit` | `SCAN_LIMIT_DEFAULT` (20) | Số bài mới nhất lấy về mỗi vòng, 1..200 |
| `scan_interval` | `BREAKING_SCAN_INTERVAL` (60s) | Khoảng nghỉ riêng của kênh, tối thiểu 15s. Dạng `"30s"`, `"2m"` hoặc số giây |

Response còn trả `last_scanned_at` — mốc vòng quét gần nhất, dùng để theo dõi
lịch chạy thực tế.

## Danh sách Định kỳ (F3)

| Method | Path | Ghi chú |
|---|---|---|
| POST | `/lists/scheduled` | Body như Breaking nhưng thay `regex_patterns` bằng `scan_frequency`; thêm `scan_limit?`, `max_posts_per_run?` |
| GET | `/lists/scheduled` | Query: `status`, `search`, `limit`, `offset` |
| GET | `/lists/scheduled/:id` | |
| PATCH | `/lists/scheduled/:id` | |
| DELETE | `/lists/scheduled/:id` | **Chỉ admin** |

`scan_frequency` nhận `"30m"`, `"6h"`, `"24h"` hoặc số giây (`"1800"`).
Tối thiểu 1 phút.

`max_posts_per_run` (mặc định `MAX_POSTS_PER_RUN_DEFAULT` = 50) là trần số Bài
Post tạo ra trong 1 vòng quét — chặn nổ chi phí AI khi kênh đăng ồ ạt. Phần dư
được xử lý ở vòng sau. `0` = không giới hạn.

## Danh mục

| Method | Path |
|---|---|
| POST / GET / PATCH | `/prompts`, `/prompts/:id` |
| DELETE | `/prompts/:id` — **chỉ admin** |
| POST / GET / PATCH | `/ai-engines`, `/ai-engines/:id` |
| DELETE | `/ai-engines/:id` — **chỉ admin** |

`GET /ai-engines?only_active=true` để lọc engine đang bật.

## Nhật ký thao tác

| Method | Path | Query |
|---|---|---|
| GET | `/audit-log` | `object_type`, `object_id`, `user_id`, `limit`, `offset` |

`object_type` ∈ `list_breaking`, `list_scheduled`, `source_post`, `voice`.
Append-only — không có endpoint sửa/xoá.

## Quản lý tài khoản (chỉ admin)

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/users` | Danh sách tài khoản (không trả `password_hash`) |
| PATCH | `/users/:id/role` | Body: `{ "role": "admin\|editor\|viewer" }`. Admin không tự hạ quyền mình |
| PATCH | `/users/:id/active` | Body: `{ "is_active": true\|false }`. Admin không tự vô hiệu hoá mình |

## Khác

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/healthz` | Không cần auth |
| GET | `/meta/platforms` | Danh sách nền tảng đang được tích hợp |
| GET | `/meta/collect-modes` | `[{mode, enabled}]` — hình thức thu thập nào đang bật (`ENABLED_COLLECT_MODES`) |
