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
| POST | `/source-posts` | Luồng F1 từ URL. Body: `source_url`, `collect_mode` (A/B/C), `prompt_id?`, `language?` (mặc định `auto`), `platform?` (bỏ trống = tự nhận diện từ URL), `auto_process?` (mặc định `true`), `voice?` (metadata điền sẵn — xem dưới). Tự enqueue `post:metadata` để lấy nội dung/hashtag/ảnh bìa. **Text gõ tay không đi đường này** — xem `POST /voices` |
| POST | `/images` | Tải ảnh bìa lên khi **chưa có voice nào** (màn tạo Voice điền metadata trước khi Bài Post tồn tại) — multipart, field `file`. Trả `{image_url, image_uploaded}` để gửi kèm trong `voice.image_url` |
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

Nội dung này cũng CHÍNH LÀ thứ TTS đọc ở hình thức B/C: những gì bảng Voice
hiển thị và ô "Nội dung đọc" điền sẵn đúng bằng những gì bạn sẽ nghe. Phụ đề
video chỉ dùng khi bài không có chữ nào (video thuần hình ảnh), và STT chỉ dùng
khi không có cả phụ đề. Bài chỉ gồm hashtag bị coi là không có nội dung — trước
đây TTS đọc nguyên chuỗi "#fyp #studytok" ra thành tiếng.

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

### Màn tạo Voice: điền sẵn metadata trong MỘT bước

`POST /source-posts` nhận thêm khối `voice` — metadata người dùng đã điền ở màn
tạo Voice, trước cả khi hệ thống chạm vào bài gốc:

```json
{
  "source_url": "https://www.tiktok.com/@user/video/123",
  "collect_mode": "A",
  "auto_process": true,
  "voice": {
    "title": "Tiêu đề tự gõ",
    "hashtag": "#tinnong",
    "language": "vi",
    "image_url": "https://…",
    "image_uploaded": true,
    "no_image": false,
    "author_id": 91650,
    "author_email": "a@strongbody.ai",
    "author_gender": "female",
    "publish_when_ready": true
  }
}
```

Luật duy nhất: **điền thì dùng của người dùng, bỏ trống thì lấy từ bài gốc** —
worker chỉ điền vào ô còn trống chứ không ghi đè (`Engine.buildVoice`). Ba chỗ
cần chú ý:

| Trường | Cách xử lý |
|---|---|
| `hashtag` | **Gộp** hashtag người dùng gõ với hashtag của bài gốc, bỏ trùng (không phân biệt hoa thường và dấu `#`), phần người dùng gõ đứng trước |
| `no_image` | Khác với để trống: để trống thì lấy ảnh bìa bài gốc, bật `no_image` là đăng bài **không ảnh** |
| `language` | Người dùng chọn thì đó là chốt — quyết định luôn giọng TTS, bước tự nhận diện không ghi đè |

`publish_when_ready: true` = tạo xong audio thì **tự đăng lên multime**, không
cần bấm nút nữa (cùng đường với `auto_publish` của Danh sách nguồn). Voice thiếu
điều kiện đăng (không author/hashtag/audio < 15s) sẽ dừng ở `failed` kèm
`last_error` như mọi lần đăng hỏng khác.

### Các trường của API đăng bài

`POST {voice_base}/v1/seller/voice-posts/upload` — multipart, upload audio và
tạo bài đăng trong cùng 1 request:

| Trường | Bắt buộc | Hệ thống điền từ |
|---|---|---|
| `audio_file` | ✅ | file voice trên storage |
| `author_id` | ✅ | `voice.author_id` — tài khoản Strongbody **bốc ngẫu nhiên theo giới tính người dùng chọn** (không suy ra từ người bấm Đăng) |
| `title` | ✅ | `voice.title` — nội dung Bài Post gộp 1 dòng, cắt ở `domain.MaxVoiceTitleRunes` (200) |
| `caption` | — | **không gửi** — multime không dùng (form của chính họ luôn gửi rỗng) |
| `hashtags[]` | ✅ | `voice.hashtag` — **không còn hashtag mặc định**, bỏ trống là không đăng được |
| `category_ids[]` | — | `MULTIME_CATEGORY_IDS` |
| `image` | — | `voice.image_url`: ảnh URL thì tải về rồi đính kèm, ảnh tải từ máy thì đọc thẳng từ storage (API nhận file, không nhận URL) |
| `source_lang` | luôn gửi | `voice.language`; rỗng = để multime tự nhận diện |
| `lang` | — | chỉ gửi khi ngôn ngữ khác `auto` |
| `visibility` | — | `MULTIME_VISIBILITY`, mặc định `public` |
| `is_public_download` | — | cấu hình |
| `scheduled_at` | — | chưa dùng |

Ngoài ra API từ chối audio ngắn hơn 15 giây. Bốn điều kiện bắt buộc
(`author_id`, `title`, hashtag, độ dài) được chặn trước ở cả API `/publish` lẫn
client multime, để lỗi hiện ra dưới dạng câu giải thích thay vì HTTP 400 khó hiểu.

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
| Bài không có chữ nào | *"Bài này không có nội dung text để đọc (không có caption/tiêu đề lẫn phụ đề)"* |

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
| GET | `/voices` | Query: `publish_status`, `source_post_id`, `platform`, `language`, `created_by`, `created_from`, `created_to`, `published_from`, `published_to`, `limit`, `offset`. Mỗi item kèm `platform`, `source_url`, `source_title`, `source_extracted_text` (text đã đưa cho TTS), `created_by_email`, `author_id`, `author_email`, `author_gender`. **`publish_status` lọc theo trạng thái hiển thị** — xem ghi chú bên dưới |
| GET | `/voices/:id` | |
| GET | `/voices/:id/audio` | Trả file voice để nghe thử; `?download=1` để tải về. Nhận token qua header **hoặc** `?token=` — thẻ `<audio>` không gắn được header `Authorization`. `400` nếu voice đã publish (file đã bị xoá theo business rule #2) |
| PATCH | `/voices/:id` | Sửa `title`, `hashtag`, `language`, `image_url`, và bộ `author_id` + `author_email` + `author_gender`. `409` nếu đã publish, `400` nếu còn `processing` hoặc `title` quá 200 ký tự |
| POST | `/voices/:id/image` | **Tải ảnh bìa từ máy** — multipart, field `file` (JPG/PNG/WEBP/GIF, tối đa 8MB). Lưu vào storage, đánh dấu `image_uploaded` và **xoá khỏi storage ngay sau khi đăng lên multime**. `409` nếu đã publish |
| GET | `/voices/:id/image` | Ảnh bìa đã tải lên, để thẻ `<img>` hiển thị được — bucket riêng tư nên không lộ URL storage. Nhận token qua header **hoặc** `?token=`. `404` nếu voice dùng ảnh từ URL bài gốc (link công khai, trình duyệt tự tải) |
| POST | `/voices/:id/regenerate` | **Sửa lời đọc rồi đọc lại chính voice đó** (ghi đè file cũ). Body: `text` (bắt buộc), `collect_mode` (`B`/`C`), `prompt_id?`, `language?` → `202` + voice ở trạng thái `processing`. `409` nếu đã publish, `400` nếu đang `processing` |
| POST | `/voices/:id/ready` | `draft` → `ready` (đã duyệt, chờ đăng) |
| POST | `/voices/:id/publish` | → `202`. Enqueue `voice:publish`. `400` nếu voice chưa chọn author hoặc chưa có hashtag |
| DELETE | `/voices/:id` | Xoá cả file trên storage nếu còn. **Chỉ admin** |

> Không có `POST /voices`: Voice chỉ sinh ra từ việc chạy Bài Post
> (business rule #1).

`publish_status`: `processing` → `draft` → `ready` → `published`, hoặc `failed`.

Ngoài 5 giá trị đó, API nhận thêm **`incomplete` — "Chưa đủ điều kiện"**: voice
đang ở `draft`/`ready` nhưng còn thiếu thứ multime đòi (chưa chọn author, chưa
có hashtag/tiêu đề, audio ngắn hơn 15 giây). Đây là trạng thái **suy ra lúc
đọc**, không có trong cột `publish_status` của DB — cột đó là trạng thái quy
trình, còn cái lọc là trạng thái người dùng nhìn thấy trên bảng.

Tách khỏi `failed` vì hai thứ khác hẳn nhau: `failed` là đã gửi lên multime và
hỏng (có `last_error`), `incomplete` là người dùng chưa điền xong và tự sửa
được. Mỗi voice rơi vào đúng một trạng thái, nên tổng số dòng của các bộ lọc
bằng đúng tổng số voice.

`processing` là record được tạo **ngay lúc enqueue** `voice:process`, trước khi
có file — để bảng Voice hiện dòng "đang xử lý" thay vì trống trơn cho tới lúc
job xong. Record này chưa sửa/đăng được (`400`); worker điền file + metadata vào
đúng nó rồi chuyển sang `draft`, hoặc chuyển `failed` + `last_error` nếu lỗi.

Publish lên multime.ai yêu cầu Voice thoả 4 điều kiện; `POST /voices/:id/publish`
trả `400` ngay nếu thiếu author/hashtag, các điều kiện còn lại làm job fail với
`last_error` giải thích rõ (FE cũng disable nút Đăng kèm lý do):

| Điều kiện | Ghi chú |
|---|---|
| Có `author_id` | Tài khoản Strongbody đứng tên bài đăng — chọn giới tính rồi hệ thống bốc ngẫu nhiên (`GET /meta/authors/random`) |
| Có `title` | Tối đa 200 ký tự (`domain.MaxTitleRunes`). Mode B/C thiếu tiêu đề thì lấy câu đầu của text đọc ra |
| Có ít nhất 1 hashtag | **Bắt buộc** — không còn hashtag mặc định trong cấu hình |
| `duration_seconds` ≥ 15 | Giới hạn của multime.ai, không bỏ qua được |
| Token multime của người tạo voice còn hiệu lực | Hết hạn thì tự refresh; refresh lỗi thì `last_error` = "cần đăng nhập lại multime" |

## Danh sách Breaking (F2)

| Method | Path | Ghi chú |
|---|---|---|
| POST | `/lists/breaking` | Body: `source_url`, `regex_patterns[]`, `collect_mode`, `prompt_id?`, `llm_api_set_id?`, `language_default?`, `auto_process?`, `auto_publish?`, `status?`, `scan_limit?`, `scan_interval?`, `schedule?` |
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

### `schedule` — lịch quét theo từng kênh

Có ở cả Breaking lẫn Định kỳ. Bỏ trống cả object = quét 24/7 (hành vi cũ).

```jsonc
{
  "schedule": {
    "timezone": "Asia/Ho_Chi_Minh",  // tên IANA, không phải offset
    "active_from_min": 360,          // 06:00 — số phút từ nửa đêm
    "active_to_min": 1380,           // 23:00
    "active_weekdays": [1,2,3,4,5],  // 0 = Chủ nhật … 6 = Thứ bảy; rỗng = mọi ngày
    "fixed_times_min": [480, 720],   // 08:00 và 12:00 — CHỈ Danh sách Định kỳ
    "clear_window": false            // true = bỏ khung giờ, quay lại 24/7
  }
}
```

- **`timezone` là phần bắt buộc của khung giờ**, không phải tuỳ chọn: lịch chạy
  theo giờ container (UTC), nên "6h–23h" mà không nói múi giờ thì lệch 7 tiếng
  so với ý người dùng.
- `active_from_min` > `active_to_min` nghĩa là khung **vắt qua nửa đêm**
  (22:00–06:00). Chỉ có một trong hai đầu thì bị từ chối `400` — không có cách
  nào diễn giải "từ 6h" mà không tự bịa ra đầu còn lại.
- `fixed_times_min` **thay thế** `scan_frequency`: có giá trị thì tần suất bị bỏ
  qua. Với kênh đăng theo giờ cố định, đây vừa đúng hơn vừa rẻ hơn hẳn.
- `clear_window` cần cờ riêng vì trong JSON, thiếu trường vừa có nghĩa "không
  sửa" vừa có nghĩa "xoá" — không có cờ thì không bao giờ bỏ được khung giờ đã
  đặt.
- Nút **"Chạy ngay"** (`POST /lists/breaking/:id/run`) không bị khung giờ chặn:
  đó là yêu cầu tường minh của người dùng.

### `llm_api_set_id` — bộ API cho quét tự động

Hình thức C của kênh gọi LLM mà **không có ai bấm nút để chọn bộ**, nên bộ phải
nằm sẵn trên kênh. Không gán thì mode C của kênh đó không chạy (job dừng với câu
hướng dẫn). Bộ được sao chép sang từng Voice lúc tạo, không đọc lại lúc worker
chạy — kênh có thể bị sửa trong lúc bài còn nằm trong hàng đợi, và lúc đó voice
phải chạy bằng đúng bộ đã chọn.

## Danh sách Định kỳ (F3)

| Method | Path | Ghi chú |
|---|---|---|
| POST | `/lists/scheduled` | Body như Breaking nhưng thay `regex_patterns` bằng `scan_frequency`; thêm `scan_limit?`, `max_posts_per_run?`. Cũng nhận `llm_api_set_id?` và `schedule?` (xem mục trên) |
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
| POST / GET / PATCH / DELETE | `/llm-api-sets`, `/llm-api-sets/:id` |
| POST | `/llm-api-sets/:id/keys` |
| PATCH / DELETE | `/llm-api-keys/:key_id` |

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

### Bộ API key LLM (`/llm-api-sets`)

Tab "LLM Model" của màn AI Engine. Một **bộ** = túi key của nhiều nhà LLM, dùng
chung cho một hoặc nhiều người — khác hẳn `ai_engine` (1 key TTS của 1 người),
vì chuỗi dự phòng chỉ có ý nghĩa khi trong tay có key của nhiều nhà cùng lúc.

```jsonc
// POST /llm-api-sets
{
  "name": "Bộ chung phòng nội dung",
  "note": "mua tháng 9",
  "visible_to_users": false,       // CHỈ admin đặt được
  "user_ids": ["<uuid>", "..."],   // chia sẻ cho ai
  "keys": [
    { "provider": "gemini", "api_key": "...", "label": "công ty", "priority": 0 }
  ]
}
```

**Luật xem/sửa** (chặn ở service, gọi thẳng API cũng không lách được):

| Việc | Ai |
|---|---|
| Thấy bộ | người tạo + người được chia sẻ + mọi người nếu `visible_to_users` |
| Sửa bộ / thêm-sửa-xoá key | người tạo hoặc admin (`can_manage` trong response) |
| Bật `visible_to_users` | **chỉ admin** — bật lên là mở hạn mức của một nhóm cho cả hệ thống |

- API **không bao giờ** trả key thật, chỉ `api_key_masked` (4 ký tự cuối).
- `PATCH /llm-api-keys/:key_id` với `api_key` bỏ trống = **giữ key cũ**. Dán key
  mới thì đồng thời **reset sức khoẻ** (`disabled_at`, `cooldown_until`,
  `consecutive_failures`): người ta vào đây chính vì key hỏng, dán key mới mà
  vẫn bị router bỏ qua thì không ai hiểu vì sao.
- `user_ids` ở `PATCH` là **thay toàn bộ** danh sách chia sẻ, không phải thêm
  dồn: form gửi lên đúng những ai được tick, nên bỏ tick phải là gỡ quyền.
- Mỗi key có `health` ∈ `ok` | `cooldown` | `disabled`:

  | health | Nghĩa | Phải làm gì |
  |---|---|---|
  | `cooldown` | hết hạn mức, đang nghỉ tới `cooldown_until` | chờ — tự khỏi |
  | `disabled` | key sai hoặc bị thu hồi (`last_error` nói rõ) | dán key mới |

  Hai thứ này tách riêng vì cách xử lý khác hẳn nhau: chờ bao lâu cũng không cứu
  được một key đã bị thu hồi.

- `GET /llm-api-sets` (danh sách) trả `key_counts` theo từng nhà, không trả key.
  Chỉ `GET /llm-api-sets/:id` mới kèm mảng `keys`.
- Bộ bị xoá thì key con đi theo (`ON DELETE CASCADE`), còn Voice đã sinh ra vẫn
  giữ nguyên và chỉ mất con trỏ (`ON DELETE SET NULL`).

### Cài đặt hệ thống (`/settings`) — chỉ admin

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/settings` | `{llm_chain, llm_batch, allowed_models, providers}` |
| PATCH | `/settings` | Body: `llm_chain?`, `llm_batch?` |
| GET | `/settings/fetch-stats` | Query `days` (1–365, mặc định 7) |

```jsonc
{
  "llm_chain": [                                  // rẻ trước, đắt sau
    { "provider": "gemini",    "model": "gemini-2.5-flash-lite" },
    { "provider": "gemini",    "model": "gemini-2.5-flash" },
    { "provider": "openai",    "model": "gpt-5.6-luna" },
    { "provider": "anthropic", "model": "claude-haiku-4-5-20251001" }
  ],
  "llm_batch": { "enabled": true, "size": 5, "max_chars": 12000, "wait_ms": 2000 }
}
```

**`allowed_models` là danh sách trắng, và `PATCH` từ chối mọi model ngoài nó.**
Lý do: alias `gpt-5.6` không trỏ về Luna mà trỏ về Sol, đắt hơn khoảng 25 lần —
viết thiếu hậu tố thì hệ thống vẫn chạy đúng, không có lỗi nào hiện ra, chỉ có
hoá đơn đội lên; và vì mắt xích đó chỉ chạy khi Gemini đã cạn nên rất lâu mới có
ai nhận ra. Frontend đọc `allowed_models` từ API chứ không giữ bản sao — bản sao
sẽ lệch ngay ở lần thêm model tiếp theo.

`size` / `max_chars` / `wait_ms` là **điểm khởi đầu phải đo lại**, không phải
hằng số đúng sẵn — đó chính là lý do chúng nằm trong DB.

`GET /settings/fetch-stats` trả `[{day, platform, kind, count, last_at}]` — số
lần từng nền tảng chặn ta, gộp theo ngày. Đây là dữ liệu để trả lời một câu hỏi
duy nhất: **có đáng mua proxy không**.

| `kind` | Proxy có giúp không |
|---|---|
| `bot_block` | **có** — nền tảng nghi IP máy chủ |
| `rate_limit` | một phần — giảm tần suất trước đã |
| `login_required` | **không** — cần cookies |
| `geo_blocked`, `unavailable`, `timeout`, `other` | không (`unavailable` = bài đã xoá, không phải bị chặn) |

### Danh mục Quốc gia / Hashtag lưu trong DB

Modal Tạo Voice **không gọi API cho từng ô chọn** nữa: cả ba danh mục về một
lần qua `GET /meta/catalog` rồi nằm trong cache của trình duyệt.

- **Quốc gia** — bảng `country`, đồng bộ từ Strongbody khi rỗng hoặc quá 24h.
  Trước đây mỗi lần mở modal là một lần gọi sang Strongbody, tức là một thao tác
  thường ngày phụ thuộc vào việc token bên đó còn hạn hay không — trong khi danh
  mục ấy đổi vài năm một lần. Đồng bộ lỗi mà bảng đã có dữ liệu thì vẫn trả bản
  cũ: danh mục lỗi thời vài ngày vẫn dùng được, modal không mở được thì không.
- **Hashtag** — bảng `hashtag`. **Không có API danh mục hashtag nào để hỏi**:
  client Strongbody chỉ *gửi* `hashtags` lúc đăng bài chứ không đọc về. Nguồn
  duy nhất có thật là dữ liệu hệ thống đã tích — hashtag của Voice đã tạo và của
  Bài Post đã lấy.
- **Quan hệ hashtag ↔ ngôn ngữ** là thứ **quan sát được**, không phải danh mục
  ai khai: mỗi Voice mang sẵn cả hashtag lẫn ngôn ngữ, nên cặp `(tag, language)`
  đến thẳng từ dữ liệu. Dùng để **gợi ý** (tag cùng ngôn ngữ xếp lên đầu), không
  để giới hạn — người dùng vẫn chọn được tag của ngôn ngữ khác và vẫn gõ được
  tag mới.

### Bốc tài khoản author diễn ra lúc ĐĂNG, không phải lúc chọn

Chọn author trên form giờ chỉ lưu **giới tính** (`author_gender`) và **quốc gia
lọc** (`author_country_id`). Việc bốc ra một tài khoản cụ thể lùi xuống bước
`voice:publish` trong worker (`Engine.ensureAuthor`).

- `POST /voices/:id/publish` chấp nhận voice chưa có `author_id`, miễn là đã có
  `author_gender`. Không có cả hai thì chặn ngay ở API.
- Worker bốc bằng token Strongbody của **người tạo voice**, rồi ghi
  `author_id`/`author_email` lên chính voice đó — đăng lỗi rồi retry thì dùng
  đúng tài khoản đã bốc, không bốc ra người khác ở lần thử thứ hai.
- Voice đã có `author_id` (người dùng bốc tay) thì giữ nguyên, không bốc đè.
- `PATCH /voices/:id` đổi `author_gender` hoặc `author_country_id` mà **không**
  kèm `author_id` sẽ **xoá** tài khoản đã bốc trước đó: giữ lại nghĩa là bài lên
  multime dưới tên một người không khớp thứ người dùng vừa chọn.

Vì sao lùi: chọn giới tính là thao tác của form, còn bốc là một lần gọi mạng
sang Strongbody. Gộp hai thứ khiến mỗi lần đổi ý là một lần chờ — và kết quả bốc
sớm cũng không "giữ chỗ" được gì bên Strongbody trong lúc voice nằm trong hàng đợi.

### Ảnh bìa: "Lấy ảnh từ nguồn" thay cho "Không có ảnh"

Mặc định **không** lấy ảnh nguồn. Ba trường trong `voice` quyết định ảnh cuối cùng:

| `no_image` | `image_url` | Kết quả |
|---|---|---|
| `false` | rỗng | Lấy ảnh bìa của bài gốc |
| `false` | có | Dùng ảnh người dùng tải lên |
| `true` | rỗng | Đăng **không kèm ảnh** |

Tải ảnh lên là tự bỏ "lấy ảnh từ nguồn" — hai nguồn ảnh không cùng thắng được,
và bắt người dùng đoán cái nào thắng là một lỗi thiết kế. Không có ảnh **không**
chặn việc đăng.

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
| GET | `/meta/collect-modes` | `[{mode, enabled, reason?}]` — hình thức nào đang bật. `reason` chỉ có khi tắt và nói rõ vì sao: người vận hành tự tắt trong `ENABLED_COLLECT_MODES`, hoặc không có LLM thật nào (mode C bị tắt khi `LLM_PROVIDER=mock` **và** trong DB chưa có Bộ API key nào) |
| GET | `/meta/publish` | `{category_ids, min_duration_seconds}` — điều kiện multime đòi ở 1 bài đăng |
| GET | `/meta/authors/random` | Query: `gender` (`male`/`female`/`other`, bắt buộc), `country_id` (tuỳ chọn). Trả `{author: {id, email, gender, full_name, avatar_url}}` — **bốc ngẫu nhiên 1 tài khoản** bên Strongbody (`GET /v1/admin/user` + `filter_names=gender` + `country_id`) bằng token của người đang đăng nhập; tool không giữ bản sao danh bạ. `id` chính là `author_id` khi đăng voice. Mỗi lần gọi là một lần bốc mới |
| GET | `/meta/catalog` | `{countries, hashtags, language_order}` — **cả 3 danh mục của modal Tạo Voice trong 1 lần gọi**. Frontend cache vĩnh viễn trong phiên: mở modal lần sau không gọi lại, mọi thao tác search chạy trên dữ liệu này. `countries` đọc từ bảng `country` (đồng bộ từ Strongbody mỗi 24h, sắp theo thứ tự nghiệp vụ — Việt Nam trước); `hashtags` là `[{tag, count, languages}]` dựng từ chính dữ liệu hệ thống; `language_order` là mã ngôn ngữ theo thứ tự suy ra từ thứ tự quốc gia |
| GET | `/meta/countries` | `{countries: [{id, name, code}]}` — danh mục quốc gia để lọc author. Đọc từ `GET /v1/buyer/countries` của Strongbody (bản `/v1/admin/countries` trả `403 unauthorized application` với token tài khoản thường) |
