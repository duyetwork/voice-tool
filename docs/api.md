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

| Role | Xem | Tạo/sửa/chạy/đăng voice | Xoá | Vận hành | Cấp quyền |
|---|---|---|---|---|---|
| `user` | ✅ | ✅ | ❌ | ❌ | ❌ |
| `editor` | ✅ | ✅ | ✅ | ✅ | ❌ |
| `admin` | ✅ | ✅ | ✅ | ✅ | ✅ |

Không có vai trò chỉ-xem: hệ thống không có đăng ký, đăng nhập được bằng tài
khoản multime nghĩa là dùng được.

**Vận hành** (`can_operate`) mở nhật ký thao tác và hạ tầng via/proxy. `user`
không vào được, và trên giao diện cả nhóm "Vận hành" bị ẩn khỏi sidebar — ẩn ở
UI đi kèm chặn ở API (`middleware.RequireOperate`), vì một mục bị ẩn mà gõ thẳng
URL vẫn ra dữ liệu thì việc ẩn đó chỉ là trang trí.

`editor` khác `admin` ở hai chỗ: cấp quyền, và phạm vi nhìn thấy via/proxy
(editor chỉ thấy của chính mình). Trong màn Cài đặt, editor dùng được tab "Via &
Proxy"; ba tab còn lại (LLM, Chi phí AI, Bị chặn) là cấu hình/số liệu của cả hệ
thống nên vẫn chỉ admin — tab vẫn hiện, bên trong là dòng báo không có quyền.

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
| GET | `/me` | — | `{ id, role, permissions: {can_write, can_delete, can_operate, can_manage_users} }` |

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
    "publish_when_ready": true,
    "tts_config": {
      "gender": "female",
      "age": "young adult",
      "pitch": "moderate pitch",
      "accent": "british accent",
      "speed": 1.0
    }
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
| `tts_config` | Cấu hình giọng đọc, lưu vào `voice.tts_config` (JSONB). Bỏ trống/`null` = giọng mặc định của nhà cung cấp. Chỉ có nghĩa với hình thức B và C — hình thức A lấy thẳng audio bài gốc, không qua TTS |

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
| POST | `/lists/breaking` | Body: `source_url`, `regex_patterns[]`, `collect_mode`, `prompt_id?`, `llm_api_set_id?`, `language_default?`, `auto_process?`, `auto_publish?`, `random_author?`, `status?`, `scan_limit?`, `scan_interval?`, `backfill_limit?`, `max_posts_per_run?`, `schedule?`, `country_id?` |
| GET | `/lists/breaking` | Query: `status`, `search` (regex lọc theo `source_url`), `limit`, `offset` |
| GET | `/lists/breaking/:id` | |
| PATCH | `/lists/breaking/:id` | Mọi field ở trên đều optional |
| DELETE | `/lists/breaking/:id` | **Chỉ admin** |
| POST | `/lists/breaking/:id/run` | → `202`. Quét thủ công 1 vòng để test regex |
| GET | `/lists/breaking/:id/skipped` | Bài đã xét rồi bỏ. Query `limit`, `offset` |
| GET | `/lists/breaking/:id/scans` | Lịch sử quét. Query `limit`, `offset` — xem mục **Lịch sử quét** |

`GET /lists/breaking/:id/skipped` trả:

```jsonc
{
  "items": [{
    "post_id_external": "7300000000000000000",
    "post_url":         "https://www.tiktok.com/@kenh/video/7300000000000000000",
    "text_excerpt":     "Nội dung đã đem so với regex, cắt còn 300 ký tự…",
    "reason":           "không khớp 3 regex_patterns",
    "checked_at":       "2026-09-15T08:12:00Z"
  }],
  "total": 412,
  "last_7_days": 412,   // cửa sổ cố định, so được giữa các kênh
  "limit": 20, "offset": 0
}
```

Đây là chỗ duy nhất trả lời được **"regex của kênh này có quá chặt không"**. Bảng
kênh chỉ đếm Bài Post đã tạo, nên một kênh đang bật, quét đều, mà không ra bài
nào trông y hệt một kênh chưa có bài mới.

`text_excerpt` và `post_url` mới có từ migration `000024` — bản ghi cũ hơn chỉ có
ID, và nhìn ID thì không kết luận được gì. Log giữ theo `SKIPPED_LOG_RETENTION`
(mặc định 7 ngày).

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
| `scan_limit` | `SCAN_LIMIT_DEFAULT` (20) | Cửa sổ quét: số bài mới nhất NHÌN mỗi vòng, 1..200 |
| `scan_interval` | `BREAKING_SCAN_INTERVAL` (60s) | Khoảng nghỉ riêng của kênh, tối thiểu 15s. Dạng `"30s"`, `"2m"` hoặc số giây |
| `backfill_limit` | `0` | Số bài CŨ lấy về ở vòng quét ĐẦU TIÊN, 0..200 |
| `max_posts_per_run` | `MAX_POSTS_PER_RUN_DEFAULT` (0 = không giới hạn) | Trần số Bài Post tạo ra mỗi vòng |

Response còn trả `last_scanned_at` — mốc vòng quét gần nhất, dùng để theo dõi
lịch chạy thực tế — và `backfill_done_at`, mốc vòng quét đầu đã chạy xong.

### `backfill_limit` — lấy bài cũ khi thêm kênh

Có ở cả Breaking lẫn Định kỳ. Vòng quét ĐẦU TIÊN của một kênh không có mốc nào
để so, nên trước đây nó nuốt trọn cả cửa sổ quét: thêm kênh là lập tức có 20 Bài
Post từ bài đã đăng từ trước, và với `auto_process` thì 20 voice.

`backfill_limit` biến việc đó thành lựa chọn tường minh:

| Giá trị | Vòng quét đầu làm gì |
|---|---|
| `0` (mặc định) | Không lấy bài nào có sẵn — chỉ bài đăng SAU khi thêm kênh |
| `N` | Lấy thêm `N` bài gần thời điểm thêm kênh nhất |

Đếm theo **số bài**, không theo khoảng thời gian: danh sách nền tảng trả về vốn
đã là "N bài mới nhất". Lọc theo ngày đăng thì phải tin vào `posted_at`, mà
yt-dlp không trả trường này ổn định trên mọi nền tảng.

Chỉ có tác dụng **một lần**. Sau vòng quét đầu, `backfill_done_at` được ghi và
giá trị này không còn được đọc tới nữa — PATCH vẫn nhận nhưng không đổi gì.

Với kênh Breaking, những bài bị loại ở vòng đầu được nhớ trong
`backfill_excluded_ids`: kênh Breaking không có mốc đồng bộ, mỗi vòng nó xét lại
cùng một cửa sổ, nên không nhớ thì chính chúng sẽ quay lại ở vòng thứ hai như
thể vừa mới đăng.

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
| POST | `/lists/scheduled` | Body như Breaking nhưng thay `regex_patterns` bằng `scan_frequency`; cũng nhận `scan_limit?`, `max_posts_per_run?`, `backfill_limit?`, `llm_api_set_id?`, `schedule?`, `country_id?` (xem mục trên) |
| GET | `/lists/scheduled` | Query: `status`, `search`, `limit`, `offset` |
| GET | `/lists/scheduled/:id` | |
| PATCH | `/lists/scheduled/:id` | |
| DELETE | `/lists/scheduled/:id` | **Chỉ admin** |
| POST | `/lists/scheduled/:id/run` | → `202`. Quét thủ công 1 vòng, không chờ hết chu kỳ |
| GET | `/lists/scheduled/:id/scans` | Lịch sử quét. Query `limit`, `offset` |

`scan_frequency` nhận `"30m"`, `"6h"`, `"24h"` hoặc số giây (`"1800"`).
Tối thiểu 1 phút.

`max_posts_per_run` (mặc định `MAX_POSTS_PER_RUN_DEFAULT` = `0` = **không giới
hạn**) là trần số Bài Post tạo ra trong 1 vòng quét — chặn nổ chi phí AI khi
kênh đăng ồ ạt. Phần dư không mất, nó được xử lý ở vòng sau.

Trên PATCH, gửi `0` nghĩa là **bỏ trần** (cột trong DB về `NULL`). Bỏ hẳn field
ra khỏi body thì trần cũ giữ nguyên — hai điều đó khác nhau, và đây là lý do
`0` không thể là "không sửa".

## Quốc gia của kênh

`country_id` là **quốc gia của kênh**: mọi Bài Post và Voice do kênh đẻ ra mang
giá trị này, và tài khoản đứng tên bài (`random_author`) được bốc trong nhóm đó.
Bỏ trống = suy từ ngôn ngữ như trước. Trên PATCH, gửi `0` nghĩa là **gỡ quốc
gia** (id của Strongbody luôn > 0, nên `0` không đụng vào giá trị thật nào).

## Lịch sử quét

`GET /lists/breaking/:id/scans` và `GET /lists/scheduled/:id/scans` trả:

```jsonc
{
  "items": [{
    "id":                 "7c1e…",
    "started_at":         "2026-09-17T08:12:00Z",
    "finished_at":        "2026-09-17T08:12:04Z",  // null = đang chạy
    "status":             "success",               // running | success | error
    "trigger_kind":       "manual",                // auto = lịch chạy
    "triggered_by_email": "an@strongbody.ai",      // rỗng ở vòng auto
    "fetched":            20,
    "posts_created":      3,
    "voices_created":     3,
    "skipped":            17,
    "error":              ""
  }],
  "total": 1284,
  "runs_7d": 336, "posts_created_7d": 41, "voices_created_7d": 41, "failed_7d": 2,
  "limit": 20, "offset": 0
}
```

Kênh chỉ mang được trạng thái của **vòng gần nhất** (`last_scanned_at`,
`last_error`), và vòng sau ghi đè vòng trước. Đây là chỗ duy nhất trả lời được
"kênh này quét bao lâu một lần THẬT SỰ", "vòng vừa rồi do lịch chạy hay ai bấm",
và "mấy hôm nay nó có ra bài nào không".

`voices_created` đếm riêng khỏi `posts_created` vì kênh tắt `auto_process` vẫn
tạo Bài Post mà không tạo voice nào — gộp lại thì không phân biệt được "kênh
không bắt được bài" với "kênh bắt được bài nhưng không ai bảo nó đọc".

`trigger_kind` và `triggered_by_email` là hai trường riêng: email rỗng ở vòng
`manual` nghĩa là tài khoản đã bị xoá, khác hẳn vòng `auto` vốn không có ai
đứng sau.

Hai bảng danh sách kênh kèm thêm **`last_run_status`** — trạng thái vòng quét
gần nhất, đọc thẳng từ bảng này. Chuỗi rỗng = kênh chưa quét lần nào. Không
thêm cột "đang quét" lên kênh vì cột như vậy phải được xoá bởi chính tiến trình
vừa chết, nên nó sẽ kẹt ở "đang quét" đúng lúc cần tin nó nhất.

Lịch sử giữ theo `SCAN_RUN_RETENTION` (mặc định 30 ngày). Vòng quét treo ở
`running` quá 2 giờ được job dọn dẹp hằng ngày đóng lại thành `error`.

## Via / proxy quét Facebook · X · Instagram

Ba nền tảng này không liệt kê được bài của một trang qua `yt-dlp`, và kênh nguồn
là trang **công khai của người khác** nên không có API chính thức nào dùng được.
Việc quét chúng chạy bằng **via** (phiên đăng nhập) đi qua **proxy**.

> ⚠️ Cách làm này vi phạm điều khoản sử dụng của cả ba nền tảng và mang rủi ro
> pháp lý về scraping dữ liệu công khai. Chi phí thật của nó là **via chết liên
> tục phải thay** — chi phí vận hành, không phải một lần. Giữ hạ tầng này tách
> biệt hoàn toàn với Business account dùng để đăng bài: nếu hai bên chạm nhau,
> Meta khoá luôn tài khoản đăng bài.

Toàn bộ route cần quyền **Vận hành** (editor hoặc admin).

**Via và proxy có CHỦ SỞ HỮU** (`user_id`), y như API key TTS: admin thấy và sửa
của mọi người, editor chỉ thấy và sửa của chính mình. Phân định nằm ở service,
không phải ở route — gọi thẳng API cũng không đọc được via của người khác.
`created_by` là người khai, khác chủ khi admin thêm hộ.

| Method | Path | Ghi chú |
|---|---|---|
| GET | `/settings/vias` | Query `platform`, `owner`. **Không bao giờ trả về cookies**. `owner` chỉ có tác dụng với admin |
| POST | `/settings/vias` | Body: `platform`, `label`, `cookies`, `daily_quota?`, `user_id?` |
| PATCH | `/settings/vias/:id` | `label?`, `cookies?`, `daily_quota?`, `status?`, `user_id?`. Dán cookies mới cũng RESET sức khoẻ về `active` |
| DELETE | `/settings/vias/:id` | |
| GET | `/settings/proxies` | Query `platform`, `owner`. `endpoint` đã cắt user/pass |
| POST | `/settings/proxies` | Body: `label`, `endpoint`, `kind?`, `platform?`, `user_id?` |
| PATCH | `/settings/proxies/:id` | `label?`, `endpoint?`, `kind?`, `status?`, `user_id?` |
| DELETE | `/settings/proxies/:id` | |
| GET | `/settings/via-cookie-specs` | Nền tảng nào cần cookie gì + mẫu để dán |
| GET | `/settings/scrape-health` | Số via theo trạng thái, từng nền tảng |
| GET | `/settings/scrape-load` | Lượt quét theo giờ trong ngày. Query `days` |

`user_id` trong body = **gán bản ghi cho người khác**, và chỉ admin gửi được:
khi tạo thì giá trị của vai trò khác bị bỏ qua (form của họ không có ô đó), khi
sửa thì bị từ chối `403` — ở đó một chủ sở hữu khác là thay đổi người dùng cố ý
yêu cầu, và im lặng nuốt nó sẽ báo "đã lưu" cho một việc không hề xảy ra.

`/settings/scrape-health` và `/settings/scrape-load` cũng theo phạm vi đó: admin
thấy số của cả hệ thống, editor thấy số của riêng via mình.

### Lỗi nào là lỗi của kênh, lỗi nào chỉ là chờ lượt sau

| Nhãn | Xử lý | Vì sao |
|---|---|---|
| `ErrNoViaAvailable` | bỏ qua vòng, không ghi lỗi kênh | hạn mức ngày cạn / cả đàn via đang nghỉ |
| `rate_limit` | bỏ qua vòng, không retry | nền tảng vừa bảo "chờ vài phút"; retry chính là thứ nó cấm |
| `bot_block` | lỗi thật | không tự khỏi — đây là tín hiệu đi đổi proxy |
| `login_required` | lỗi thật | via phải được dán cookies mới |

Hai trường hợp đầu ghi vào `scan_run` là **lỗi kèm lý do**, không phải
"thành công, 0 bài" — dạng `Permanent` nên nói một lần, không retry. Trước đây
chúng trả về `nil` và lịch sử quét hiện một vòng thành công không lấy được bài
nào, không kèm một chữ giải thích; đó đúng là thứ người vận hành báo lại với
Instagram. Một vòng không làm được việc của nó là một vòng hỏng, dù lỗi không
phải của kênh.

Cả bốn đều được đếm vào `fetch_error_stat` nên tab "Bị chặn" vẫn thấy — bỏ qua
ở đây là bỏ qua việc RETRY, không phải bỏ qua việc ghi nhận.

Riêng **HTTP 401 kèm `require_login`**: Instagram trả mã này khi phiên của via
hỏng, nên phải đọc thân phản hồi trước khi kết luận. Quy hết 401/403 về "IP bị
chặn" nghĩa là hạ cấp rồi khai tử một proxy tốt, còn via hỏng — thứ thật sự cần
thay — vẫn nằm nguyên trong vòng xoay ở trạng thái khoẻ.

> **Bộ chọn của worker KHÔNG theo chủ sở hữu.** `ScrapePool` vẫn lấy via/proxy
> trên toàn hệ thống. Chủ sở hữu trả lời "ai được nhìn và sửa bản ghi này", không
> phải "lượt quét của ai được dùng via nào" — ghép hai thứ lại sẽ làm kênh của
> người chưa nuôi via im lặng ngừng ra bài.

`cookies` và `endpoint` đi **một chiều**: gửi lên được, không bao giờ trả về.
Cả hai được mã hoá AES-256-GCM bằng `TOKEN_ENCRYPTION_KEY` trước khi vào DB, y
như token multime và API key LLM. Bỏ trống khi PATCH = giữ giá trị cũ (giao diện
không có bản thật để gửi lại).

### Cookies cần lấy, theo từng nền tảng

Server **từ chối** via thiếu cookie mang danh tính — báo ngay lúc dán, không để
vòng quét phát hiện sau vài tiếng.

| Nền tảng | Bắt buộc | Vì sao |
|---|---|---|
| Facebook | `c_user`, `xs` | `c_user` là id tài khoản, `xs` là chính phiên. Thiếu một trong hai thì Facebook coi như khách |
| X | `auth_token`, `ct0` | `auth_token` là phiên; `ct0` là token CSRF mà mọi request đọc dữ liệu đều đòi |
| Instagram | `sessionid`, `ds_user_id` | `sessionid` là phiên; `ds_user_id` là id tài khoản đi kèm |

Cookie khác (`datr`, `sb`, `fr`, `csrftoken`, `guest_id`…) chép kèm cũng được,
**không bắt buộc**: thiếu chúng phiên vẫn chạy, và bắt buộc chúng chỉ làm người
dùng bị từ chối vì một thứ không quan trọng.

Lấy ở đâu: mở nền tảng trên trình duyệt đã đăng nhập → DevTools (F12) →
Application/Storage → Cookies.

`GET /settings/via-cookie-specs` trả đúng danh sách này kèm mẫu — form thêm via
đọc từ đó thay vì chép cứng, để hướng dẫn trên form và điều kiện server kiểm
không bao giờ lệch nhau.

### Định dạng endpoint proxy

Nhận **ba dạng**, vì đó là ba dạng thật sự tồn tại ngoài đời:

```
23.95.45.5:10356:u3qvdrto:9w6w0mc8c5     dạng nhà bán proxy hay giao — dán nguyên dòng
23.95.45.5:10356                          proxy không cần đăng nhập
socks5://user:pass@23.95.45.5:1080        khai scheme khi KHÔNG phải http
```

Dạng đầu là lý do việc chuẩn hoá tồn tại: gần như mọi nhà bán proxy residential
đều giao một danh sách `ip:port:user:pass`, và bắt người vận hành tự ghép tay
thành URL cho từng dòng là vừa mất thời gian vừa dễ sai — sai ở đây thì proxy
lặng lẽ không dùng được, mà triệu chứng lại giống hệt proxy bị chặn.

Không có scheme thì mặc định `http`. Mật khẩu chứa `@` hoặc `/` vẫn đúng: server
ghép bằng `url.UserPassword` chứ không nối chuỗi.

`status` chỉ nhận `active` | `disabled`. `cooldown` và `dead` là **kết luận của
hệ thống**, không đặt tay được — cho phép đặt tay thì con số trên bảng tổng quan
không còn nói lên điều gì về sức khoẻ thật của đàn via.

### Máy trạng thái

```
via:    active --(N lỗi login liên tiếp)--> cooldown --(hết giờ)--> active
        cooldown --(lỗi login ngay sau khi hồi)--> dead
        active <-> disabled                        (bật/tắt tay)

proxy:  active --(N lần bot_block liên tiếp)--> degraded --(vẫn bị chặn)--> dead
        active <-> disabled                        (bật/tắt tay)
```

Hai bệnh khác nhau, chữa bằng hai thứ khác nhau — và đây là phần dễ sai nhất:

| Nhãn lỗi | Nghĩa | Ai bị trách |
|---|---|---|
| `login_required` | nền tảng đòi đăng nhập | **via** (phiên hỏng) |
| `bot_block` | nền tảng nghi IP | **proxy** |
| `rate_limit`, còn lại | tần suất, mạng, bài bị xoá | không ai — chỉ ghi lại |

Đánh via chết vì một IP bị chặn là thay nhầm thứ đang hỏng, và ta vừa vứt đi một
tài khoản còn dùng được.

Proxy **không tự hồi sinh** theo thời gian, khác via: một IP đã bị Meta/X liệt
thì chờ bao lâu cũng vậy. Bật lại bằng tay qua PATCH là đường duy nhất.

### Hạn mức và lịch

`daily_quota` đếm theo **lượt quét kênh**: 1 lượt = 1 lần `FetchLatestPosts`
hoàn tất cho 1 kênh, kể cả khi bên trong phải tải nhiều trang nối tiếp. Không
đếm theo số bài, không đếm theo request HTTP con.

Hạn mức chỉ bị trừ khi lượt quét **thành công**: một request bị proxy làm hỏng
không phải lỗi của via và không đáng lấy mất một suất của nó.

Job `scrape:sweep` chạy **mỗi giờ** (phút thứ 7): hồi sinh via hết cooldown, đặt
lại bộ đếm ngày, dọn `via_usage_log` quá hạn. Mỗi giờ chứ không mỗi ngày vì
cooldown mặc định chỉ 6 tiếng — chạy theo ngày thì via nghỉ xong vẫn nằm ngoài
vòng xoay gần trọn một ngày nữa.

### Thêm kênh Facebook / Instagram / X

Ba nền tảng này quét được cả kênh, mỗi nền tảng sau một cờ riêng:

| Nền tảng | Cờ | Nguồn dữ liệu |
|---|---|---|
| Facebook | `FACEBOOK_CHANNEL_SCAN` | HTML trang, moi khối JSON Relay |
| Instagram | `INSTAGRAM_CHANNEL_SCAN` | `/api/v1/users/web_profile_info` (JSON) |
| X | `X_CHANNEL_SCAN` | `syndication.twitter.com/srv/timeline-profile` |

Cả ba mặc định **tắt**, và form Thêm kênh vẫn hiện lý do chặn cho tới khi bật.
Ba cờ riêng chứ không một cờ chung: ba bộ phân tích bám vào ba thứ khác nhau và
hỏng độc lập nhau, nên khi một nền tảng đổi giao diện thì phải tắt được đúng nó
mà không làm đứt hai nền tảng đang chạy tốt.

Lý do mặc định tắt: các bộ phân tích bám vào cấu trúc nội bộ của những nền tảng
đó — không có tài liệu, không có cam kết tương thích — nên phải đối chiếu với
một trang thật, bằng via thật, trước khi mở khoá. Bật sớm thì người dùng tạo ra
hàng loạt kênh im lặng không ra bài.

#### Trần số bài lấy được mỗi lần quét

Ba nền tảng này **không phân trang được**. Một lần gọi trả bấy nhiêu là hết, và
phần thiếu không có đường nào lấy — đo trực tiếp ngày 17/09/2026:

| Nền tảng | Trần / lần gọi | Vì sao |
|---|---|---|
| Facebook | 10 | Chỉ những bài Facebook dựng sẵn trong HTML trang; phần còn lại trang tự tải thêm khi cuộn |
| Instagram | 12 | `web_profile_info` trả đúng 12 và không nhận tham số xin thêm |
| X | 20 | syndication bỏ qua **mọi** tham số phân trang — đã thử `max_id`, `until_id`, `cursor`, `max_position`, `count=200`: cùng một cửa sổ, cùng bài cũ nhất |
| YouTube / TikTok | không có trần | yt-dlp phân trang được (`--playlist-end`) |

Con số của X là số đo trên **phiên thật** (hai tài khoản cho 19 và 20). Gọi cùng
endpoint mà không có cookies thì trả về một bản đệm ~100 bài trộn lẫn nhiều năm
— đó không phải thứ vòng quét nhận được, đừng lấy làm trần.

Trần đi ra giao diện qua `/meta/platforms` → `channel_scan[].max_posts`, và form
Thêm kênh **khoá** ô nhập bằng nó (`max` + ép giá trị xuống khi biết nền tảng),
chứ không còn cho nhập 50 rồi cảnh báo — một con số không bao giờ đạt được thì
không nên nhập vào được. Đường quét cũng ép lại một lần nữa
(`domain.ClampChannelLimit`) để kênh thêm từ trước khi có trần không xin một
con số không ai giữ được.

**Thứ tự KHÔNG tin được, cả X lẫn Instagram.** Cả hai bộ phân tích sắp lại
theo thời gian đăng, giảm dần, TRƯỚC khi cắt `limit`:

- **X** — `entries` của syndication có biến thể trả 100 tweet mà 5 phần tử đầu
  lần lượt từ 2022, 2020, 2025, 2025, 2022 (đo trên @TF1Info). Lấy N phần tử đầu
  ra một nhúm bài ngẫu nhiên rải suốt 10 năm. Khoá xếp là `created_at`, dự phòng
  là snowflake trong `id_str`. **Không dùng `sort_index`** dù nó có mặt và trông
  như khoá xếp hạng: giá trị của nó là snowflake của *thời điểm trả lời*, giảm
  đúng 1 đơn vị mỗi phần tử — nó đánh số vị trí, không nói gì về tweet.
- **Instagram** — bài ghim nằm đầu mà không có cờ phân biệt; với trần 12 bài/lần
  gọi thì 3 bài ghim cũ chiếm 1/4 số suất của một vòng quét. Khoá xếp là
  `taken_at_timestamp`. Hệ quả: thứ tự ở đây khác thứ tự nhìn thấy trên trang
  thật, đổi lại "N bài mới nhất" đúng nghĩa là N bài mới nhất.

**URL bài Facebook** phải mang tên trang: `facebook.com/<trang>/posts/<id>`.
Dạng trần `facebook.com/<id>` chạy được với id của video/reel nên nhìn qua
tưởng đúng, nhưng với bài thường thì Facebook chuyển hướng sang một URL `pfbid…`
rồi trả 404 — triệu chứng là "Bài đăng không còn tồn tại trên nền tảng" cho một
bài vẫn đang sống. Không liên quan tới việc Facebook đổi tên miền:
`web.facebook.com/<id>` cũng 404 y hệt.

**Riêng X:** đường liệt kê dùng endpoint syndication (widget nhúng tweet) chứ
không phải GraphQL của x.com. GraphQL đòi ba thứ rotate độc lập — mã truy vấn
trong URL, khối `features`, bearer của web client — nên adapter bám vào chúng sẽ
hỏng vài tuần một lần và mỗi lần đều cần sửa code. Đổi lại, endpoint syndication
chỉ trả ~20 tweet gần nhất và chỉ với tài khoản công khai, đúng bằng nhu cầu ở
đây. Vẫn phải khai một via X để mở khoá vì `ScrapePool` là chỗ cấp proxy và đếm
hạn mức, dù cookies của via đó không được dùng để đọc danh sách.

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

Bản ghi gồm: key, **chủ sở hữu** (`user_id` — voice của người này đọc bằng key
này), **người khai** (`created_by` — khác chủ khi admin khai hộ), `is_active`,
bộ `key_status*` và `last_used_at`. Tên gọi / provider / danh sách ngôn ngữ đã
bỏ ở migration `000010`: chỉ có 1 nhà cung cấp thì không có gì để đặt tên, còn
danh sách ngôn ngữ hỏi thẳng provider chính xác hơn là khai tay.

- `POST` body: `api_key` (bắt buộc), `user_ids?`.
  **Trả về mảng** `{"items": [...]}` — admin gán 1 key cho nhiều người thì mỗi
  người là một bản ghi riêng, nên sau này đổi/thu hồi key của từng người mà
  không đụng người còn lại. `user_ids` rỗng = key của chính mình.
- `PATCH` body: `api_key?`, `user_id?` (chuyển key sang người khác),
  `is_active?` (bật/tắt key).
- `GET /ai-engines` xếp theo `created_at` giảm dần — key vừa thêm nằm ở dòng
  đầu, kể cả với admin đang nhìn key của cả nhà.

#### `is_active` — mỗi người đúng một key đang chạy

Trước migration `000027`, luật là ngầm: "key mới khai nhất thắng". Người có 2
key không có cách nào biết voice của mình đang chạy bằng key nào, càng không có
cách chọn. `is_active` biến luật đó thành một công tắc nhìn thấy được.

- `PATCH {"is_active": true}` **tắt mọi key khác của cùng chủ sở hữu** trước
  khi bật key này. Unique index `uq_ai_engine_active_per_user` ép luật này ở
  tầng dữ liệu, không chỉ ở UI.
- Key **đầu tiên** của một người tự động bật; key thứ hai trở đi vào ở trạng
  thái tắt — thêm key dự phòng không được âm thầm đổi key đang đọc.
- Xoá key đang bật, hoặc admin chuyển nó sang người khác, thì **key mới nhất
  còn lại của chủ cũ tự lên thay** (giữ đúng hành vi trước khi có công tắc).
  Key vừa chuyển sang chủ mới luôn ở trạng thái tắt.
- Tắt hết key = voice B/C của người đó dừng ở bước đọc, kèm câu hướng dẫn bật
  lại. Đây là lựa chọn hợp lệ, không phải lỗi.

#### `key_status` — key còn hạn hay hết credit

Ba cột `key_status`, `key_status_detail`, `key_status_at` ghi lại điều học được
từ **lần gọi TTS gần nhất** bằng key đó:

| `key_status` | Nghĩa | Người dùng phải làm gì |
|---|---|---|
| `unknown` | chưa chạy lần nào bằng key này | — |
| `ok` | lần đọc gần nhất ra audio | — |
| `no_credit` | 3voices trả `402` | nạp thêm credit |
| `invalid` | `401`/`403` — key sai hoặc bị thu hồi | khai lại key mới |
| `rate_limited` | `429` — gọi quá nhanh | chờ, không cần đổi key |

Hai điều quan trọng khi đọc mấy cột này:

- **Luôn là thông tin quá khứ.** Hệ thống *không* thăm dò 3voices định kỳ: mỗi
  lần hỏi là một request tính tiền của người dùng, để lấy đúng thứ mà lần đọc
  thật sẽ nói ra miễn phí. Vì vậy `key_status_at` là phần bắt buộc phải hiện
  cùng — "còn hạn" của tháng trước không nói gì về hôm nay.
- **Lỗi không liên quan tới key thì không ghi đè.** Mạng chập chờn, 3voices lỗi
  `5xx`, text quá dài — cả ba đều không chứng minh được gì về key, nên trạng
  thái cũ được giữ nguyên: một lần rớt mạng không được phép xoá dấu vết của lần
  hết credit.
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

Voice chạy hình thức B/C mà chủ nhân chưa khai key — hoặc đã tắt hết key — thì
job dừng với câu *"Bạn chưa khai hoặc chưa bật API key TTS nào — vào mục AI
Engine thêm key 3voices (hoặc bật lại key đã có) rồi chạy lại"* (trừ khi `.env`
có key chung `THREEVOICES_API_KEY` làm dự phòng).

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
| GET | `/settings` | `{llm_chain, llm_batch, allowed_models, providers, ai_prices}` |
| PATCH | `/settings` | Body: `llm_chain?`, `llm_batch?`, `ai_prices?` |
| GET | `/settings/fetch-stats` | Query `days` (1–365, mặc định 7) |
| GET | `/settings/ai-usage` | Query `days` (1–365, mặc định 30) |

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

#### `GET /settings/ai-usage` — đo trước khi tối ưu

Hai cần gạt ở trên (chuỗi dự phòng, batch) ảnh hưởng thẳng tới hoá đơn, nhưng
trước đây không có con số nào nói gạt xong rẻ hơn hay đắt hơn. Mỗi lần gọi nhà
cung cấp AI giờ ghi 1 dòng `ai_usage` — kể cả lần **thất bại** và kể cả các lần
retry, vì chúng vẫn bị tính token đầu vào.

```jsonc
{
  "days": 30,
  "models": [{
    "kind": "llm", "provider": "gemini", "model": "gemini-2.5-flash-lite",
    "calls": 1240, "failed": 12,
    "input_tokens": 8200000, "output_tokens": 1900000,
    "characters": 0, "audio_seconds": 0,
    "cost_usd": 1.23, "has_price": true
  }],
  "daily": [ /* cùng hình dạng, thêm "day" */ ],
  "total_usd": 1.23,          // CHỈ cộng phần đã khai đơn giá
  "missing_prices": [{ "kind": "tts", "provider": "3voices", "model": "" }],
  "prices": [ /* bảng giá đang lưu */ ]
}
```

**`has_price: false` khác hẳn "miễn phí"** — nó nghĩa là chưa ai khai đơn giá,
và `cost_usd` của dòng đó không có ý nghĩa. `total_usd` cũng chỉ cộng phần đã
khai, vì một tổng trông có vẻ đầy đủ mà thiếu chính là con số người ta mang đi
báo cáo.

Đơn giá là **dữ liệu, không phải hằng số trong code**: giá của cả ba nhà đổi vài
lần một năm và khác nhau theo hợp đồng. Bảng giá cũ ghim trong Go sẽ không báo
lỗi — nó vẫn cho ra một con số, chỉ là con số sai. Khai qua `PATCH /settings`:

```jsonc
{ "ai_prices": [
  { "kind": "llm", "provider": "anthropic", "model": "claude-haiku-4-5-20251001",
    "input_per_mtok": 1.0, "output_per_mtok": 5.0 },
  { "kind": "tts", "provider": "3voices", "model": "", "per_mchars": 20.0 }
] }
```

`ai_prices` thay **toàn bộ** bảng (không sửa từng dòng). Giao diện dựng sẵn các
dòng từ `missing_prices` nên không phải gõ tay tên model — gõ sai một ký tự thì
dòng giá không khớp dòng usage nào và chẳng có gì báo lỗi.

Tiền tính lúc ĐỌC, không lưu vào từng dòng: khai nhầm rồi sửa lại là mọi con số
đúng ngay. Bản ghi giữ theo `AI_USAGE_RETENTION` (mặc định 90 ngày).

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
- `tts_config` ở body theo luật 3 trạng thái: **bỏ hẳn trường** = giữ nguyên
  giọng voice đang dùng; gửi `{}` = trả về giọng mặc định; gửi object có giá
  trị = đọc lại bằng giọng đó. Không dùng `COALESCE` được vì `null` phải phân
  biệt được với "không nhắc tới".

### Cấu hình giọng đọc (`tts_config`)

Lưu trên từng Voice (cột `voice.tts_config`, JSONB) chứ không phải trên API key:
cùng một key vẫn phải đọc mỗi bài một giọng khác nhau được, và worker chạy bất
đồng bộ nên giá trị phải nằm sẵn trên bản ghi.

| Trường | Giá trị hợp lệ |
|---|---|
| `gender` | `female`, `male` |
| `age` | `child`, `teenager`, `young adult`, `middle-aged`, `elderly` |
| `pitch` | `very low pitch`, `low pitch`, `moderate pitch`, `high pitch`, `very high pitch`, `whisper` |
| `accent` | `american accent`, `australian accent`, `british accent`, `canadian accent`, `chinese accent`, `indian accent`, `japanese accent`, `korean accent`, `portuguese accent`, `russian accent` |
| `speed` | Số thực trong khoảng `0.5`–`2.0`; `1.0` là bình thường |

Giá trị lạ bị chặn ngay ở API (`400`) kèm danh sách giá trị đúng — 3voices trả
`500` cho một từ khoá sai, và lúc đó lỗi đã nằm trong một job đã chết. Trường bỏ
trống rơi về mặc định của adapter (`female` / `young adult` / `moderate pitch` /
`speed 1.0`); riêng `accent` **không có mặc định** — không gửi nghĩa là giọng
chuẩn của ngôn ngữ đã chọn.

> **Cấu hình thắng giọng đã lưu.** Khi API key của user có `voice_id`, adapter
> vốn gọi `/tts/saved` — endpoint đó **bỏ qua** `gender`/`age`/`pitch`/`accent`.
> Nên chỉ cần `tts_config` có một trường, hệ thống chuyển sang `/tts/design` và
> không gửi `voice_id` nữa. Nếu không, người dùng chỉnh xong nghe lại thấy y hệt
> cũ mà không có chỗ nào giải thích. `speed` và `language` có tác dụng ở cả hai
> đường.

Ngôn ngữ đọc **không** nằm trong `tts_config`: nó đã là trường `language` của
Voice. Hai ô ngôn ngữ trong một form là hai nguồn sự thật.

### Lịch sử quét ghi lại SỐ BÀI ĐÃ XIN

`scan_run.requested_limit` = số bài vòng đó xin nền tảng, sau khi ép về trần.
Không suy ngược được từ kênh: `scan_limit` / `backfill_limit` là cấu hình hiện
tại và sửa lúc nào cũng được, còn lịch sử nói về quá khứ. Bảng hiện hai cột
cạnh nhau — "Bài xin" và "Bài xét" — vì câu hỏi đầu tiên khi thấy một con số
nhỏ luôn là "xin bao nhiêu". 0 = dòng có từ trước khi cột này tồn tại, giao
diện hiện dấu gạch chứ không hiện số 0.

### Xoá kênh, xoá Prompt mẫu

Xoá một kênh **không xoá** Bài Post và Voice của nó: `source_post.list_*_id`
chuyển về NULL, còn `source_type` vẫn nói đúng bài đó sinh ra từ đâu. Nhật ký
quét và `via_usage_log` của kênh thì xoá theo (CASCADE) — chúng chỉ có nghĩa khi
kênh còn.

Xoá **Prompt mẫu** bị chặn khi còn kênh hoặc Bài Post dùng nó (khoá ngoại NO
ACTION), nhưng KHÔNG bị chặn bởi voice đã tạo — voice giữ lại `prompt_id` NULL,
đúng tiền lệ `voice.ai_engine_id`.

> Ràng buộc CHECK phải **hợp với** quy tắc xoá của chính khoá ngoại nó canh.
> `ck_source_post_origin` và `ck_voice_prompt` từng đòi cột phải khác NULL trong
> khi khoá ngoại khai `ON DELETE SET NULL` — hai điều đó không thể cùng đúng, và
> hậu quả là không xoá được kênh nào, không xoá được prompt nào. Xem migration
> 000034. Thêm CHECK mới trên một cột có `ON DELETE SET NULL` thì phải tự hỏi
> câu này trước.

Lỗi ràng buộc của Postgres được **dịch thành câu nói rõ phải làm gì** trước khi
lên giao diện (`service.wrapDB`); nguyên văn vẫn nằm trong log. Ràng buộc chưa
có câu dịch thì giữ nguyên văn — tên ràng buộc là manh mối duy nhất để tra.

## Nhật ký thao tác

Cần quyền **Vận hành** (editor hoặc admin) — trước đây mọi vai trò đọc được.
Chuyển cùng lúc với via/proxy vì cả nhóm "Vận hành" bị ẩn khỏi sidebar của
`user`, và một mục bị ẩn mà API vẫn trả dữ liệu thì việc ẩn đó không có nghĩa.

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
| GET | `/meta/collect-modes` | `[{mode, enabled, reason?}]` — hình thức nào đang bật. `reason` chỉ có khi tắt và nói rõ vì sao: người vận hành tự tắt trong `ENABLED_COLLECT_MODES`, hoặc không có LLM thật nào (mode C tắt khi `LLM_PROVIDER=mock` **và** trong DB chưa có Bộ API key nào). Trạng thái mode C đọc lại từ DB ngay trong lúc chạy (nhớ tạm 30s) — thêm Bộ API key xong, tải lại trang là thấy bật, **không cần restart** |
| GET | `/meta/voice-style` | `{genders, ages, pitches, accents, speed: {min, max}, has_saved_voice}` — bộ giá trị hợp lệ cho `tts_config`, để form không phải chép cứng bộ từ khoá của nhà cung cấp (xem [Cấu hình giọng đọc](#cấu-hình-giọng-đọc-tts_config)). `has_saved_voice` = API key của **chính người đang đăng nhập** có khai `voice_id` hay không; form dùng nó để cảnh báo trước rằng chỉnh cấu hình sẽ thay giọng đã lưu đó |
| GET | `/meta/publish` | `{category_ids, min_duration_seconds}` — điều kiện multime đòi ở 1 bài đăng |
| GET | `/meta/health` | `{failed_voices, users_need_relogin, channels_with_error, ok}` — những thứ đang hỏng **âm thầm**. Giao diện đọc mỗi phút và hiện thanh cảnh báo ở mọi trang khi `ok = false`. `users_need_relogin` chỉ đếm người đã mất token multime **mà đang đứng tên kênh đang bật**: token của họ là thứ worker dùng để auto-publish, hỏng thì mọi voice của các kênh đó fail mà bảng kênh vẫn hiện "Đang bật" (rủi ro 🔴 ở [status.md §3.1](status.md)). Chỉ chính họ đăng nhập lại mới sửa được |
| GET | `/meta/authors/random` | Query: `gender` (`male`/`female`/`other`, bắt buộc), `country_id` (tuỳ chọn). Trả `{author: {id, email, gender, full_name, avatar_url}}` — **bốc ngẫu nhiên 1 tài khoản** bên Strongbody (`GET /v1/admin/user` + `filter_names=gender` + `country_id`) bằng token của người đang đăng nhập; tool không giữ bản sao danh bạ. `id` chính là `author_id` khi đăng voice. Mỗi lần gọi là một lần bốc mới |
| GET | `/meta/catalog` | `{countries, hashtags, language_order}` — **cả 3 danh mục của modal Tạo Voice trong 1 lần gọi**. Frontend cache vĩnh viễn trong phiên: mở modal lần sau không gọi lại, mọi thao tác search chạy trên dữ liệu này. `countries` đọc từ bảng `country` (đồng bộ từ Strongbody mỗi 24h, sắp theo thứ tự nghiệp vụ — Việt Nam trước); `hashtags` là `[{tag, count, languages}]` dựng từ chính dữ liệu hệ thống; `language_order` là mã ngôn ngữ theo thứ tự suy ra từ thứ tự quốc gia |
| GET | `/meta/countries` | `{countries: [{id, name, code}]}` — danh mục quốc gia để lọc author. Đọc từ `GET /v1/buyer/countries` của Strongbody (bản `/v1/admin/countries` trả `403 unauthorized application` với token tài khoản thường) |
