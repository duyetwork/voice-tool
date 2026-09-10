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
| `user` | ✅ | ✅ | ❌ | ❌ |
| `editor` | ✅ | ✅ | ✅ | ❌ |
| `admin` | ✅ | ✅ | ✅ | ✅ |

Không có vai trò chỉ-xem: hệ thống không có đăng ký, đăng nhập được bằng tài
khoản multime nghĩa là dùng được. `editor` khác `admin` duy nhất ở quyền cấp
quyền.

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
| 403 | `forbidden` | role không đủ quyền (`user` gọi endpoint xoá, `editor` gọi endpoint cấp quyền) |
| 404 | `not_found` | không tìm thấy bản ghi |
| 409 | `already_published` | sửa/đăng lại Voice đã publish |
| 500 | `internal_error` | lỗi hệ thống |

## Phân trang

Mọi endpoint danh sách nhận `limit` (mặc định 20, tối đa 200) + `offset` và
trả về cùng một khung:

```json
{ "items": [...], "total": 137, "limit": 20, "offset": 40 }
```

`total` là tổng số bản ghi khớp bộ lọc (không phải số item trả về), nên UI dựng
được số trang thật. Ngoại lệ duy nhất là `GET /ai-engines` — bảng vài dòng, trả
toàn bộ trong `items` và không có `total`.

## Sắp xếp

Cùng bộ tham số cho mọi endpoint danh sách: `sort` (cột) + `dir` (`asc` |
`desc`, mặc định `desc`). Chỉ cột thời gian sắp xếp được, và mỗi bảng chỉ nhận
đúng cột của nó — giá trị lạ bị bỏ qua, rơi về mặc định thay vì trả lỗi:

| Endpoint | `sort` nhận | Mặc định |
|---|---|---|
| `/voices` | `created_at`, `published_at` | `created_at` desc |
| `/lists/breaking`, `/lists/scheduled` | `created_at`, `last_scanned_at` | `created_at` desc |
| `/source-posts`, `/prompts`, `/users`, `/audit-log` | — (chỉ có `created_at`) | `created_at` desc |

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
| POST | `/source-posts` | Luồng F1 từ URL. Body: `source_url`, `collect_mode` (A/B/C), `prompt_id?`, `language?` (mặc định `auto`), `platform?` (bỏ trống = tự nhận diện từ URL), `auto_process?` (mặc định `true`). Tự enqueue `post:metadata` để lấy nội dung/hashtag/ảnh bìa. **Text gõ tay không đi đường này** — xem `POST /voices` |
| GET | `/source-posts` | Query: `source_type`, `status`, `platform`, `collect_mode`, `language`, `created_by`, `created_from`, `created_to` (`YYYY-MM-DD`), `list_breaking_id`, `list_scheduled_id`, `limit`, `offset`. Mỗi item kèm `created_by_email` |
| GET | `/source-posts/:id` | |
| PATCH | `/source-posts/:id` | Sửa `collect_mode`, `prompt_id`, `language` — chỉ khi chưa `processing` |
| DELETE | `/source-posts/:id` | Cascade xoá Voice liên quan. **Chỉ admin** |
| POST | `/source-posts/:id/run` | → `202` `{status: "queued"}`. Enqueue `voice:process` |

Bài Post giữ luôn metadata gốc lấy từ nền tảng — `title`, `hashtags[]`,
`thumbnail_url`, `author_name`, `posted_at`. Worker ghi các field này khi chạy
`post:metadata`, và Voice sinh ra được điền sẵn từ đó nên form đăng bài không
phải gõ tay.

**Không có trường mô tả.** `title` của Bài Post là **toàn bộ nội dung bài, trừ
hashtag** (hashtag đã có trường `hashtags[]` riêng), giữ nguyên xuống dòng, tối
đa 5.000 ký tự. Lý do: multime chỉ hiển thị `title` của voice post, nên tách
riêng một trường mô tả chỉ tạo dữ liệu chết. `voice.title` là chính nội dung đó
gộp về 1 dòng và cắt 200 ký tự — xem [Các trường của API đăng bài](#các-trường-của-api-đăng-bài).

Mỗi nền tảng gọi "nội dung" một kiểu, nên adapter dựng `title` theo 2 cách:

| Nhóm | Nội dung Bài Post là |
|---|---|
| Facebook, TikTok, Instagram, X, bài đọc qua Open Graph | Caption đầy đủ (`description` của yt-dlp / `og:description`); tiêu đề trang chỉ được ghép thêm khi nó KHÔNG phải bản cắt cụt của caption |
| YouTube | Tiêu đề video — phần mô tả dưới video là link/timestamp nên không đăng lại |

```bash
curl -X POST http://localhost:8080/api/v1/source-posts \
  -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"source_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ","collect_mode":"B","language":"vi"}'
```

### Hai kiểu nhập liệu đi hai đường khác nhau

| Nhập | Endpoint | Ra cái gì | Hình thức dùng được |
|---|---|---|---|
| URL bài đăng | `POST /source-posts` | **Bài Post** (rồi Voice nếu `auto_process`) | A, B, C |
| Text gõ tay | `POST /voices` | **Voice thẳng**, không có Bài Post | B, C — không có audio gốc để tách nên không dùng được A |

Text gõ tay **không** tạo Bài Post (khác với bản trước). Business rule #1 vẫn
đúng ở chỗ nó có ý nghĩa: Voice lấy từ một URL luôn phải đi qua Bài Post để
truy vết về bài gốc và để chạy lại với mode/prompt khác mà không fetch lại.
Text gõ tay không có URL, không có bài gốc, không có gì để fetch lại — Bài Post
sinh ra chỉ là bản ghi rỗng làm bẩn màn duyệt Bài Post (nơi để duyệt trước khi
tốn tiền AI). Xem migration `000011`, ràng buộc `ck_voice_origin`.

### Metadata lấy khi TẠO bài, không phải khi chạy Voice

`POST /source-posts` trả về ngay (không chờ mạng), rồi enqueue `post:metadata`.
Worker lấy `title`, `hashtags`, `thumbnail_url`, `author_name`, `posted_at` và
ghi lên chính Bài Post đó — áp dụng cho cả mode A, B, C vì việc
này không phụ thuộc mode. "Chạy Voice" (`POST /source-posts/:id/run`) từ đó chỉ
làm đúng một việc: tạo Voice.

Metadata lấy theo 2 đường, theo thứ tự:

1. **yt-dlp** — bài có media (video, reel, short).
2. **Thẻ Open Graph của trang** — bài viết dạng text, bài chỉ có ảnh. yt-dlp
   thoát ngay với "no video" nên không đọc được gì; thẻ `og:*` thì mọi nền tảng
   đều phát ra để link preview hoạt động.

Cả hai thất bại (nền tảng dựng tường đăng nhập) → `last_error` ghi lý do, nhưng
`status` **không** thành `failed`: bài vẫn chạy Voice được, chỉ thiếu phần điền
sẵn.

### Các trường của API đăng bài

`POST {voice_base}/v1/seller/voice-posts/upload` — multipart, upload audio và
tạo bài đăng trong cùng 1 request:

| Trường | Bắt buộc | Hệ thống điền từ |
|---|---|---|
| `audio_file` | ✅ | file voice trên storage |
| `author_id` | ✅ | id multime của **người tạo voice** (bài đăng dưới tên họ) |
| `title` | ✅ | `voice.title` — nội dung Bài Post gộp 1 dòng, cắt ở `domain.MaxVoiceTitleRunes` (200) |
| `caption` | — | **không gửi** — multime không dùng (form của chính họ luôn gửi rỗng) |
| `hashtags[]` | ✅¹ | `voice.hashtag`, fallback `MULTIME_DEFAULT_HASHTAGS` |
| `category_ids[]` | ✅¹ | `MULTIME_CATEGORY_IDS` |
| `image` | — | tải `voice.image_url` về rồi đính kèm (API nhận file, không nhận URL) |
| `source_lang` | luôn gửi | `voice.language`; rỗng = để multime tự nhận diện |
| `lang` | — | chỉ gửi khi ngôn ngữ khác `auto` |
| `visibility` | — | `MULTIME_VISIBILITY`, mặc định `public` |
| `is_public_download` | — | cấu hình |
| `scheduled_at` | — | chưa dùng |

¹ Phải có **ít nhất 1** trong `hashtags[]` / `category_ids[]`.

Ngoài ra API từ chối audio ngắn hơn 15 giây. Ba điều kiện bắt buộc (`title`,
hashtag/category, độ dài) được client chặn trước để lỗi hiện ra dưới dạng câu
giải thích thay vì HTTP 400 khó hiểu.

**`title` là toàn bộ phần chữ của bài đăng:**

- Giới hạn 200 ký tự / 1 dòng không phải con số tự đặt: ô tiêu đề ở form đăng
  của multime (`UploadVoice.tsx`) và form sửa (`MyVoices.tsx`) đều là `<input>`
  với `maxLength={200}`, và title là trường bắt buộc. Đổi chỗ duy nhất
  `domain.MaxVoiceTitleRunes` nếu multime nới giới hạn.
- Nguồn: `voice.title`, điền sẵn từ tiêu đề Bài Post (= nội dung bài, đã bỏ
  hashtag) gộp về 1 dòng rồi cắt ở ranh giới từ, thêm `…`. Không có gì dùng
  được thì rơi về text đã fetch/đã lưu, cuối cùng là `Voice <8 ký tự id>`.
- Sửa tay được ở form "Sửa metadata Voice"; API `PATCH /voices/:id` chặn tiêu
  đề quá 200 ký tự ngay tại chỗ thay vì để job publish fail sau.
- `caption` của API tồn tại nhưng hệ thống **không gửi**: giao diện multime chỉ
  đọc `title`, form đăng của chính họ luôn gửi caption rỗng.

### `last_error` nói rõ cái gì hỏng và phải làm gì

`last_error` của Bài Post và Voice là 1 câu tiếng Việt chỉ ra nguyên nhân cụ
thể và hành động tiếp theo, ví dụ:

| Tình huống | `last_error` |
|---|---|
| Bài bị nền tảng chặn | *"Bài này bị nền tảng chặn, phải đăng nhập mới xem được — cần cấu hình cookies cho yt-dlp"* |
| API key TTS sai | *"API key 3voices không hợp lệ hoặc đã bị thu hồi — khai lại key ở mục AI Engine (3voices: Invalid API key)"* |
| Hết credit 3voices | *"Tài khoản 3voices hết credit — nạp thêm rồi chạy lại"* |
| Vượt rate limit | *"3voices báo vượt giới hạn số request — hệ thống sẽ tự thử lại sau"* |
| Bài không có chữ nào | *"Bài này không có phụ đề lẫn nội dung text để đọc"* |

Nguyên văn lỗi (stderr yt-dlp, exit code, body của nhà cung cấp) chỉ đi vào log
của worker. Lỗi không khớp trường hợp nào ở trên thì trả **nguyên văn dòng đầu
đã cắt ngắn** chứ không nuốt thành câu chung chung kiểu "lỗi hệ thống, xem log":
người dùng không đọc được log của server, và một câu như thế biến mọi sự cố
khác nhau thành cùng một chữ.

Lỗi thuộc loại retry vô nghĩa (bài bị xoá, key sai, hết credit, text quá dài)
được đánh dấu `PermanentError` nên Asynq không thử lại; lỗi mạng, 429 và 5xx thì
có retry (3 lần, backoff 30s/60s/120s).

### Chống trùng theo ID bài đăng

Dedup theo `(platform, post_id_extracted)` trên toàn hệ thống, **không theo
URL**: cùng 1 bài có nhiều dạng link khác nhau.

```
facebook.com/watch/?ref=saved&v=2112466516312058  ┐
facebook.com/reel/2112466516312058                ┘ → cùng id 2112466516312058
```

| Luồng | Khi trùng |
|---|---|
| F2/F3 (quét tự động) | bỏ qua im lặng — trùng ở đây luôn là lỗi, không ai xác nhận được. Có thêm unique index `uq_source_post_dedup` làm lưới an toàn cho 2 worker quét song song |
| F1 (nhập tay) | `409 duplicate_post` kèm bài đã có; người dùng chọn **bỏ qua** hoặc **vẫn tạo mới** (`allow_duplicate: true`) |

Bài cũ **không bị sửa gì** trong cả 2 trường hợp.

```json
// 409 khi POST /source-posts trùng
{
  "error": "duplicate_post",
  "message": "Bài đăng này đã có trong hệ thống — chọn bỏ qua, hoặc vẫn tạo Bài Post mới từ cùng nội dung.",
  "existing": {
    "id": "…", "source_url": "…", "title": "…",
    "status": "processed", "collect_mode": "A",
    "created_at": "2026-09-09T09:12:00Z",
    "post_id_extracted": "2112466516312058"
  }
}
```

### Link nhận được

URL đi qua chuẩn hoá trước khi nhận diện nền tảng:

- **Bóc link bọc redirect**: `l.facebook.com/l.php?u=…`, `google.com/url?q=…`,
  `lm.facebook.com`, `l.messenger.com`, `zalo.me/redirect` (tối đa 3 lớp).
- **Bỏ tham số tracking**: `fbclid`, `igsh`, `mibextid`, `si`, `utm_*`… — cùng
  1 bài không còn trông như 2 URL khác nhau, dedup mới đúng.

Dạng link nhận được mỗi nền tảng:

| Nền tảng | Dạng |
|---|---|
| YouTube | `watch?v=`, `youtu.be/`, `/shorts/`, `/live/`, `/embed/`, `/v/`, `youtube-nocookie.com`, `music.` |
| Facebook | `/reel/`, `/videos/`, `/watch?v=`, `/posts/`, `/permalink.php?story_fbid=`, `/photo?fbid=`, `/groups/*/posts/*`, `fb.watch/`, `fb.me/`, và link share `/share/p|r|v/…` |
| TikTok | `@user/video/`, `@user/photo/`, `vm.`/`vt.tiktok.com/…`, `tiktok.com/t/…` |
| Instagram | `/p/`, `/reel(s)/`, `/tv/`, `/stories/…`, link share `/share/p|reel/…` |
| X | `/status/`, `/i/web/status/`, `t.co/…`, `twitter.com` |

Link không khớp pattern nào nhưng đúng nền tảng vẫn được nhận: ID suy ra từ
path và yt-dlp là bên quyết định tải được hay không.

## Voice

| Method | Path | Ghi chú |
|---|---|---|
| POST | `/voices` | **Chỉ nhận text gõ tay.** Body: `text` (bắt buộc, tối đa 20.000 ký tự), `collect_mode` (`B` hoặc `C`), `prompt_id?` (bắt buộc với C), `language?`. Trả về Voice ở trạng thái `processing` rồi enqueue `voice:text` — không tạo Bài Post nào. Voice từ URL vẫn đi qua `/source-posts` |
| GET | `/voices` | Query: `publish_status`, `source_post_id`, `platform`, `language`, `created_by`, `created_from`, `created_to`, `published_from`, `published_to`, `limit`, `offset`. Mỗi item kèm `platform`, `source_url`, `source_title`, `created_by_email` |
| GET | `/voices/:id` | |
| GET | `/voices/:id/audio` | Trả file voice để nghe thử; `?download=1` để tải về. Nhận token qua header **hoặc** `?token=` — thẻ `<audio>` không gắn được header `Authorization`. `400` nếu voice đã publish (file đã bị xoá theo business rule #2) |
| PATCH | `/voices/:id` | Sửa `title`, `hashtag`, `language`, `image_url`. `409` nếu đã publish, `400` nếu còn `processing` hoặc `title` quá 200 ký tự |
| POST | `/voices/:id/regenerate` | **Sửa lời đọc rồi đọc lại chính voice đó** (ghi đè file cũ). Body: `text` (bắt buộc), `collect_mode` (`B`/`C`), `prompt_id?`, `language?` → `202` + voice ở trạng thái `processing`. `409` nếu đã publish, `400` nếu đang `processing` |
| POST | `/voices/:id/ready` | `draft` → `ready` (đã duyệt, chờ đăng) |
| POST | `/voices/:id/publish` | → `202`. Enqueue `voice:publish` |
| DELETE | `/voices/:id` | Xoá cả file trên storage nếu còn. **Chỉ admin** |

> Không có `POST /voices`: Voice chỉ sinh ra từ việc chạy Bài Post
> (business rule #1).

`publish_status`: `processing` → `draft` → `ready` → `published`, hoặc `failed`.

`processing` là record được tạo **ngay lúc enqueue** `voice:process`, trước khi
có file — để bảng Voice hiện dòng "đang xử lý" thay vì trống trơn cho tới lúc
job xong. Record này chưa sửa/đăng được (`400`); worker điền file + metadata vào
đúng nó rồi chuyển sang `draft`, hoặc chuyển `failed` + `last_error` nếu lỗi.

Publish lên multime.ai yêu cầu Voice thoả 3 điều kiện, nếu không job sẽ fail với
`last_error` giải thích rõ (FE cũng disable nút Đăng kèm lý do):

| Điều kiện | Ghi chú |
|---|---|
| Có `title` | Tối đa 200 ký tự (`domain.MaxTitleRunes`). Mode B/C thiếu tiêu đề thì lấy câu đầu của text đọc ra |
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
| POST / GET / PATCH / DELETE | `/ai-engines`, `/ai-engines/:id` |

### AI Engine = API key TTS của từng người

Chỉ còn 1 nhà cung cấp TTS (3voices) nên `/ai-engines` không dùng để *chọn*
engine nữa, mà để mỗi người khai **API key của chính mình**: worker chạy TTS
bằng key của người tạo voice, nên quota và hoá đơn về đúng người đó.

Bản ghi chỉ còn 4 thông tin: key, **chủ sở hữu** (`user_id` — voice của người
này đọc bằng key này), **người khai** (`created_by` — khác chủ khi admin khai
hộ) và `last_used_at`. Tên gọi / provider / danh sách ngôn ngữ đã bỏ ở
migration `000010`: chỉ có 1 nhà cung cấp thì không có gì để đặt tên, còn danh
sách ngôn ngữ hỏi thẳng provider chính xác hơn là khai tay.

- `POST` body: `api_key` (bắt buộc), `user_ids?`.
  **Trả về mảng** `{"items": [...]}` — admin gán 1 key cho nhiều người thì mỗi
  người là một bản ghi riêng, nên sau này đổi/thu hồi key của từng người mà
  không đụng người còn lại. `user_ids` rỗng = key của chính mình.
- `PATCH` body: `api_key?`, `user_id?` (chuyển key sang người khác).
- **API key không bao giờ được trả về.** Response chỉ có `api_key_masked`
  (4 ký tự cuối); key lưu trong DB đã mã hoá AES-256-GCM bằng
  `TOKEN_ENCRYPTION_KEY`, cùng cơ chế với token multime.
- **`api_key` bỏ trống khi `PATCH` = giữ key cũ**, không phải xoá key — form
  không hiển thị key thật nên không có gì để gửi lại.
- **Quyền theo chủ sở hữu, không theo route:** admin xem/sửa/xoá key của mọi
  người và gán được key cho người khác (`user_ids` / `user_id`); các vai trò
  khác — kể cả `editor` — chỉ thấy và sửa key của chính mình, và `user_ids`
  gửi lên bị bỏ qua chứ không phải báo lỗi. Đụng vào key người khác trả `403`.
  Vì vậy `DELETE /ai-engines/:id` nằm ở nhóm "write" chứ không phải "chỉ
  admin" như các `DELETE` khác: key là của chính người dùng.
- `GET /ai-engines?user_id=<uuid>` lọc theo chủ sở hữu — chỉ có tác dụng với
  admin.
- `last_used_at` chỉ được đóng dấu khi TTS chạy **thành công**: key sai mà vẫn
  hiện "vừa dùng" thì người dùng tưởng key còn sống.

Voice chạy hình thức B/C mà chủ nhân chưa khai key thì job dừng với câu
*"Bạn chưa khai API key TTS — vào mục AI Engine thêm key 3voices rồi chạy lại"*
(trừ khi `.env` có key chung `THREEVOICES_API_KEY` làm dự phòng).

### Sửa lời đọc rồi tạo lại voice

`POST /voices/:id/regenerate` dùng cho cả Voice gõ tay lẫn Voice sinh từ Bài
Post. Thứ được sửa là **lời đọc** (`voice.input_text`), KHÔNG phải bài gốc trên
nền tảng — bài đó là dữ liệu của người khác.

- Có `input_text` thì worker đọc đúng nó và **không fetch lại nguồn**; nhờ vậy
  sửa xong là ra đúng lời mình chốt, không bị caption mới đè lên.
- Ghi đè lên chính bản ghi cũ (không tạo dòng mới): tiêu đề/hashtag/ảnh bìa đã
  điền vẫn giữ, và bảng không mọc thêm voice rác. File audio cũ bị xoá khỏi
  storage **sau khi** file mới ghi xong.
- Voice đã publish thì từ chối (`409`): bài bên multime đã có người nghe.

## Nhật ký thao tác

| Method | Path | Query |
|---|---|---|
| GET | `/audit-log` | `object_type`, `object_id`, `user_id`, `limit`, `offset`, `dir`. Mỗi item kèm `user_email` |

`object_type` ∈ `list_breaking`, `list_scheduled`, `source_post`, `voice`.
Append-only — không có endpoint sửa/xoá.

## Quản lý tài khoản (chỉ admin)

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/users` | Danh sách tài khoản (không trả `password_hash`) |
| PATCH | `/users/:id/role` | Body: `{ "role": "admin\|editor\|user" }`. Admin không tự hạ quyền mình |
| PATCH | `/users/:id/active` | Body: `{ "is_active": true\|false }`. Admin không tự vô hiệu hoá mình |

## Khác

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/healthz` | Không cần auth |
| GET | `/meta/platforms` | Danh sách nền tảng đang được tích hợp |
| GET | `/meta/collect-modes` | `[{mode, enabled, reason?}]` — hình thức nào đang bật. `reason` chỉ có khi tắt và nói rõ vì sao: người vận hành tự tắt trong `ENABLED_COLLECT_MODES`, hoặc thiếu provider (mode C mà `LLM_PROVIDER=mock` thì không có gì viết lại nội dung) |
