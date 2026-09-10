# Rà soát dự án — trạng thái, rủi ro, việc tiếp theo

Cập nhật: sau khi nối luồng rời rạc thành 1 flow xuyên suốt (prompt.md) —
metadata bài gốc tự điền vào Voice, nghe thử/tải voice, 5 nền tảng nguồn,
ngôn ngữ auto-detect, bảng có chọn hàng loạt + bộ lọc.

Quy mô hiện tại: **10.4k dòng Go** (+886 dòng test), **3.0k dòng TypeScript**,
4 migration, 3 binary backend, 12 trang frontend.

---

## 1. Đã làm

### 1.1 Kiến trúc

| Thành phần | Trạng thái |
|---|---|
| Mô hình 3 tầng `Danh sách → Bài Post → Voice → multime.ai` | ✅ Đúng theo specs, enforce ở cả DB constraint và service |
| Clean architecture: `transport → service → domain ← infra` | ✅ `domain` không import package nào của dự án |
| 3 binary: `api` (1) / `worker` (2) / `scheduler` (1) | ✅ Tách vì `asynq.PeriodicTaskManager` không chạy được 2 instance |
| Queue Asynq: 4 task type + 1 job dọn dẹp | ✅ |
| Auth SSO strongbody + phân quyền 3 mức | ✅ Mới xong |

### 1.2 Luồng nghiệp vụ

| Luồng | Backend | Frontend |
|---|---|---|
| **F1** On-demand (URL → Bài Post → Voice) | ✅ | ✅ `/on-demand` |
| **F2** Breaking (quét liên tục, nhiều regex OR) | ✅ | ✅ `/lists/breaking` |
| **F3** Định kỳ (tần suất riêng từng kênh) | ✅ | ✅ `/lists/scheduled` |
| **Mode A** Extract audio từ URL | ✅ yt-dlp + ffprobe | ✅ mặc định trên UI |
| **Mode B** Text → TTS | ✅ code xong, chưa bật | ✅ |
| **Mode C** Text + Prompt → LLM → TTS | ✅ code xong, chưa bật | ✅ |
| **Publish** lên multime.ai | ✅ đúng API thật, đăng bằng tài khoản của user | ✅ `/voices` |
| Duyệt Bài Post trước khi tốn chi phí AI | ✅ | ✅ `/source-posts` |
| Prompt mẫu + AI Engine | ✅ | ✅ |
| Audit log (append-only) | ✅ | ✅ `/audit-log` |
| Quản lý tài khoản | ✅ | ✅ `/users` (chỉ admin) |

### 1.3 10 business rules trong specs

| # | Rule | Enforce ở đâu |
|---|---|---|
| 1 | Mọi Voice phải qua `SourcePost` | Không có `POST /voices`; `ck_source_post_origin` |
| 2 | Publish xong xoá file S3; publish lỗi giữ file | `MarkVoicePublished` + `ck_voice_published` |
| 3 | Regex là cơ chế duy nhất; keyword tự chuẩn hoá về regex | `pkg/validator` + `service.normalizePatterns` |
| 4 | Breaking quét liên tục, không cron | `breaking:dispatch` tự re-enqueue |
| 5 | Scheduled có tần suất riêng, sửa là lịch tự cập nhật | `worker/scheduler` đọc DB mỗi 30s |
| 6 | Chống lấy lặp: lỗi tạm thời không tiến mốc, lỗi vĩnh viễn thì tiến | `Scan.ScanScheduled` + `domain.PermanentError` + unique index |
| 7 | `auto_process`/`auto_publish` theo từng danh sách | Cột trên `list_*` |
| 8 | Audit log tự động cho 4 entity | `service.Audit` inject vào mọi service có mutation |
| 9 | Cascade ngôn ngữ Post > List > hệ thống | `service/language.go` |
| 10 | API chỉ CRUD + enqueue | Provider AI/Multime chỉ inject vào `service.Engine` (worker) |

### 1.4 10 câu trả lời trong `prompt.md`

| # | Yêu cầu | Kết quả |
|---|---|---|
| 1 | API post voice từ multime | Tìm trong `C:\multime-ai` → `POST /v1/seller/voice-posts/upload` (1 request, không phải 2 bước) |
| 2 | TTS 3voices.win | Adapter xong theo hợp đồng API; chưa bật vì ưu tiên Mode A |
| 3 | Nhiều regex | `regex_patterns TEXT[]`, OR, tối đa 20/kênh, tự loại trùng |
| 4 | Tham số quét điều chỉnh được | `scan_limit`, `scan_interval`, `max_posts_per_run` theo kênh + 6 biến `.env` |
| 5 | Đề xuất deploy | [deployment.md](deployment.md) — 1 EC2 + compose, phương án B là ECS Fargate |
| 6 | Tối ưu worker | api 1 / worker 2 / scheduler 1 (bắt buộc 1) |
| 7 | Nơi lưu file + backup | S3 dùng chung bucket, prefix `voice-tool/`; RDS backup 7 ngày; Redis không cần backup |
| 8 | Dọn `skipped_log` | Giữ 7 ngày, job chạy 03:15 hằng ngày |
| 9 | Role admin/editor/user | ✅ (xem 1.5) |
| 10 | Không signup, chỉ signin + cấp quyền | ✅ (xem 1.5) |

### 1.5 Đăng nhập SSO + phân quyền (yêu cầu mới nhất)

**Bỏ hoàn toàn xác thực cục bộ.** Không còn `/auth/register`, không còn cột
`password_hash` có giá trị, không còn bcrypt.

```
POST /api/v1/auth/login {email, password}
  → POST https://api-v2.strongbody.ai/v1/public/auth/login  (Scope: strongbody-ai)
  → upsert app_user (email là khoá), lưu accessToken + refreshToken (mã hoá AES-256-GCM)
  → phát JWT của voice-tool
```

Tài khoản đăng nhập **chính là** tài khoản đăng voice: khi publish, worker lấy
token của người tạo ra voice đó và gọi API với `author_id` của họ. Voice xuất
hiện trên multime dưới đúng tài khoản đó, không phải một tài khoản hệ thống
dùng chung.

Token hết hạn → refresh 1 lần qua `GET /v1/admin/auth/refresh-token` → nếu
refresh cũng thất bại thì trả lỗi vĩnh viễn yêu cầu user đăng nhập lại.

| Role | Xem | Tạo/sửa/chạy/**đăng voice** | Xoá | Cấp quyền |
|---|---|---|---|---|
| `user` | ✅ tất cả | ✅ | ❌ | ❌ |
| `editor` | ✅ tất cả | ✅ | ✅ | ❌ |
| `admin` | ✅ tất cả | ✅ | ✅ | ✅ |

Không có vai trò chỉ-xem: đăng nhập được bằng tài khoản multime nghĩa là dùng
được. `editor` chỉ khác `admin` ở quyền cấp quyền.

Router chia 4 nhóm (`authed` / `writer` / `remover` / `admin`) thay vì kiểm tra
role rải rác trong handler. Tài khoản đăng nhập lần đầu nhận `DEFAULT_USER_ROLE`
(mặc định `user`); email trong `BOOTSTRAP_ADMIN_EMAIL` luôn được nâng lên admin.

### 1.6 Đã xoá (code/config thừa)

| Thứ bị xoá | Vì sao |
|---|---|
| `domain.Enqueuer.RegisterScheduledScan` / `UnregisterScheduledScan` | Chỉ có bản cài đặt no-op, không ai gọi — lịch F3 đọc trực tiếp từ DB |
| `domain.AudioMetadata`, `domain.PostMetadata`, `MultimeClient.UploadVoice`/`CreatePost` | API thật là 1 request; interface 2 bước là giả định sai ban đầu |
| `MULTIME_SCOPE`, `MULTIME_UPLOAD_PATH`, `MULTIME_UPLOAD_FOLDER`, `MULTIME_CREATE_POST_PATH`, `MULTIME_CATEGORY_ID`, `MULTIME_API_KEY`, `MULTIME_AUTHOR_ID`, `MULTIME_EMAIL`, `MULTIME_PASSWORD` | Path đã chốt cứng trong code; không còn tài khoản hệ thống dùng chung |
| `GOOGLE_TTS_CREDENTIALS_JSON`, `AZURE_TTS_KEY`, `AZURE_TTS_REGION` | Không có adapter — config đọc vào rồi không ai dùng |
| `YOUTUBE_API_KEY`, `FACEBOOK_ACCESS_TOKEN`, `TIKTOK_ACCESS_TOKEN`, `INSTAGRAM_ACCESS_TOKEN`, `X_BEARER_TOKEN` | Cùng lý do. YouTube dùng yt-dlp nên không cần key; nền tảng mới sẽ tự khai biến của nó |
| `BOOTSTRAP_ADMIN_PASSWORD`, `service.User.EnsureBootstrapAdmin` | Không còn mật khẩu cục bộ |
| `middleware.RequirePublish` | Trùng hoàn toàn với `RequireWrite` — 2 tên cho 1 quy tắc |
| Tài khoản mẫu trong `seeds/seed.sql` | Không còn mật khẩu cục bộ; app_user tạo tự động khi đăng nhập |
| `@tanstack/react-query-devtools`, `lucide-react` | Có trong package.json nhưng không import ở đâu |
| `internal/infra/redis/`, `backend/api/`, `internal/transport/http/dto/`, `deploy/` | Thư mục rỗng |

### 1.7 Test

| Package | Nội dung |
|---|---|
| `pkg/validator` | Chuẩn hoá regex (hashtag, từ khoá, danh sách OR, delimiter, regex thuần), chặn pattern sai/quá dài |
| `pkg/secret` | Mã hoá/giải mã round-trip, nonce khác nhau mỗi lần, sai khoá bị từ chối, validate độ dài khoá |
| `infra/platform` | Parse URL YouTube (watch/youtu.be/shorts/live/m.), từ chối nền tảng khác, parse VTT |
| `infra/multime` | httptest server theo hợp đồng thật: header, tên field, hashtag là field lặp lại, 2FA, refresh token, 401 → `ErrTokenExpired`, `code != 0` → lỗi vĩnh viễn |
| `service` | Nhiều regex OR + loại trùng, `newerThan` (chống lấy lặp), cascade ngôn ngữ, ma trận quyền 3 role |

Chạy: `make test`. Không có test nào cần Postgres/Redis.

---

## 2. Bạn cần cung cấp gì

### 2.1 Bắt buộc để chạy thật

| Việc | Ghi chú |
|---|---|
| **Xác nhận `MULTIME_BASE_URL`** | Đang mặc định `https://voice-api.strongbody.ai` (lấy từ `.env.example` của multime-ai). Cần biết có host riêng cho môi trường test không |
| **1 tài khoản multime để test** | Đăng nhập vào voice-tool bằng tài khoản đó, voice sẽ xuất hiện trên chính tài khoản đó. Tài khoản này **không được bật 2FA** (xem 3.4) |
| **`BOOTSTRAP_ADMIN_EMAIL`** | Email nào sẽ là admin đầu tiên |
| **Sinh `TOKEN_ENCRYPTION_KEY`** | `make gen-key`. Khoá trong `.env.example` chỉ dùng cho dev |

### 2.2 Cần khi bật Mode B/C (chưa gấp)

| Biến | Dùng cho |
|---|---|
| `THREEVOICES_API_KEY` (+ `THREEVOICES_VOICE_ID`) | TTS |
| `ANTHROPIC_API_KEY` | Mode C — LLM viết lại theo prompt |
| `OPENAI_API_KEY` | STT, chỉ khi video không có phụ đề |

### 2.3 Cần quyết định

| Câu hỏi | Ảnh hưởng |
|---|---|
| Kênh nguồn Facebook/TikTok/Instagram/X là **kênh của mình** hay **kênh người khác**? | Quyết định được hay không làm adapter — xem 4.2 |
| Voice do worker tự tạo (F2/F3) đăng dưới tài khoản nào? | Hiện là tài khoản người **tạo danh sách kênh**. Xem 3.1 |
| Có cho `user` xem `/audit-log` không? | Hiện có. Nếu coi là dữ liệu vận hành thì nên giới hạn admin |
| `DEFAULT_USER_ROLE` = `user` hay `editor`? | Hiện `user` (đăng nhập được là đăng voice được, nhưng không xoá được). Đổi sang `editor` nếu muốn ai cũng xoá được |

---

## 3. Cảnh báo — xung đột và rủi ro

### 3.1 🔴 Token multime hết hạn làm auto-publish dừng âm thầm

Voice của F2/F3 được đăng bằng token của người **tạo ra danh sách kênh**
(`list.created_by`). Người đó không cần online, nhưng token của họ có thời hạn.

- Access token hết hạn → hệ thống tự refresh, không ảnh hưởng gì.
- **Refresh token cũng hết hạn** (người đó không đăng nhập voice-tool lâu ngày)
  → mọi auto-publish của các kênh họ tạo đều fail với `last_error` =
  "cần đăng nhập lại multime". Job không retry (lỗi vĩnh viễn), Voice nằm ở
  trạng thái `failed` chờ người xử lý.

**Chưa có cảnh báo chủ động** — phải vào màn Voice mới thấy. Cách giảm rủi ro:
người phụ trách vận hành đăng nhập voice-tool định kỳ, hoặc bổ sung việc gửi
thông báo khi có Voice `failed` (xem 4.1).

### 3.2 🔴 Xung đột hạn mức và định danh với multime.ai

voice-tool **đăng bài thật vào hệ thống production của multime**, dùng chính
tài khoản người dùng. Nghĩa là:

- Bài do voice-tool đăng và bài user tự đăng trên `/studio/upload` **không phân
  biệt được** trên multime. Nếu cần phân biệt (để thống kê, hoặc để rollback
  hàng loạt), nên chốt 1 hashtag/category riêng và đặt vào
  `MULTIME_DEFAULT_HASHTAGS` / `MULTIME_CATEGORY_IDS`.
- F2 Breaking bật `auto_publish` có thể đăng rất nhiều bài trong thời gian
  ngắn. Chưa biết multime có rate limit cho `/voice-posts/upload` hay không.
  **Nên bật `auto_publish` cho 1 kênh trước và theo dõi**, đừng bật đồng loạt.
- Chưa có API xoá/hạ bài trên multime trong code. Đăng sai thì phải vào
  `/studio` xử lý tay.

### 3.3 🟡 Đổi `TOKEN_ENCRYPTION_KEY` làm mất token đang lưu

Token được mã hoá bằng khoá này. Đổi khoá (hoặc deploy môi trường mới với khoá
khác) → token cũ không giải mã được → **mọi user phải đăng nhập lại**. Dữ liệu
khác không ảnh hưởng. Hệ thống xử lý êm (báo `ErrReloginRequired`, không crash),
nhưng auto-publish sẽ đứng đến khi có người đăng nhập lại.

### 3.4 🟡 Tài khoản bật 2FA không đăng nhập được

`strongbody-api` trả `requires_totp: true` + `pending_token` thay vì token khi
tài khoản bật 2FA. voice-tool phát hiện và báo lỗi rõ ("tài khoản đang bật 2FA,
chưa hỗ trợ đăng nhập") nhưng **chưa làm bước xác minh TOTP**. Tài khoản dùng
cho voice-tool phải tắt 2FA, hoặc cần bổ sung luồng nhập mã (xem 4.1).

### 3.5 🟡 Endpoint refresh token nằm dưới nhóm `/admin`

`GET /v1/admin/auth/refresh-token` là chỗ duy nhất trong `strongbody-api` expose
`RefreshToken`, và nó nhận scope `strongbody-ai` nên user thường dùng được.
Nhưng đây là **đường dẫn dễ bị đổi** khi họ refactor router. Nếu refresh bắt đầu
trả 404, đây là chỗ cần kiểm tra đầu tiên.

### 3.6 🟡 Trùng 1 bài giữa 2 danh sách

Cùng 1 URL kênh thêm vào cả Breaking và Định kỳ → 2 bản ghi độc lập, dedup
riêng theo từng list → **cùng 1 bài có thể ra 2 Bài Post và 2 Voice, đăng 2
lần**. Đây là giả định đã nêu từ đầu (specs mục 5, câu 1) và chưa được xác nhận
lại. Nếu không muốn, cần đổi unique index sang `(platform, post_id_extracted)`.

### 3.7 🟢 Ràng buộc của multime chặn một phần Mode B/C

multime yêu cầu audio **≥ 15 giây**. TTS một caption ngắn dễ ra voice dưới 15s
và bị từ chối. Hệ thống chặn trước với lỗi rõ ràng, nhưng nghĩa là Mode B/C
chỉ dùng được với nội dung đủ dài.

### 3.8 🟢 Rate limit của 3voices khi bật TTS

3voices giới hạn 10 request/phút và 2 job đồng thời/user. `WORKER_CONCURRENCY`
mặc định 10 × 2 worker = 20 task song song → sẽ vượt hạn mức. **Chưa có rate
limiter.** Khi bật Mode B/C phải giảm `WORKER_CONCURRENCY` xuống 2 hoặc làm
limiter (xem 4.1).

### 3.9 🟡 Công cụ quan sát khi dev không có bảo vệ

`docker-compose.yml` mở 3 UI để dev quan sát hệ thống: Adminer (8085),
asynqmon (8081), MinIO console (9001). **Không cái nào có xác thực riêng** —
Adminer có form login nhưng dùng thẳng credential DB, asynqmon thì mở thẳng.
Chỉ dùng ở local hoặc qua SSH tunnel; đừng publish ra Internet
(xem checklist trong [deployment.md](deployment.md)).

### 3.10 🟢 Phần đã chạy thật và phần chưa

Đã chạy thật trên stack Docker đầy đủ:
- 4 migration apply sạch lên Postgres.
- yt-dlp lấy audio + metadata từ YouTube (video và shorts).
- Đăng voice lên multime.ai thành công bằng tài khoản thật.
- Nghe thử / tải file voice qua API.

**Chưa** chạy thật:
- 4 adapter Facebook / TikTok / Instagram / X: mới test phần parse URL bằng unit
  test. Các nền tảng này thường chặn tải khi không đăng nhập, nên nhiều khả năng
  cần cookie/credential — chưa biết trước cho tới khi thử.
- Mode B/C (TTS/LLM thật) — đang tắt bằng `ENABLED_COLLECT_MODES`.
- F2/F3 quét kênh theo lịch ở quy mô thật.

---

## 3b. Đã xử lý từ phản hồi chạy thật (prompt.md)

Luồng Mode A đã chạy thật: extract audio từ YouTube và đăng lên multime.ai
thành công. 8 điểm rời rạc phát hiện khi dùng đã được nối lại:

| # | Yêu cầu | Cách làm |
|---|---|---|
| 1 | Lấy đủ metadata bài gốc, auto-fill vào form tạo voice | `source_post` thêm `title` (= toàn bộ nội dung bài, trừ hashtag), `hashtags[]`, `thumbnail_url`, `author_name`, `posted_at`; worker ghi khi fetch, Voice sinh ra điền sẵn tiêu đề/hashtag/ảnh bìa |
| 2 | Nghe thử + tải voice trước khi đăng | `GET /voices/:id/audio` (`?download=1`) stream từ storage qua API — bucket riêng tư, `minio:9000` không mở được từ trình duyệt |
| 3 | Metadata "Nền tảng" + hỗ trợ nền tảng khác | Cột nền tảng hiện ở cả 3 bảng, ô chọn tuỳ chọn ở form F1; thêm adapter Facebook, TikTok, Instagram, X dùng chung `ytdlpCore` |
| 4 | Bảng có cột người tạo | 4 query danh sách JOIN `app_user` trả kèm `created_by_email` |
| 5 | Mode B/C chưa hỗ trợ thì để inactive | `ENABLED_COLLECT_MODES=A`; `/meta/collect-modes` cho FE hiển thị mờ, service từ chối mode chưa bật |
| 6 | Sửa ngôn ngữ ngay trên bảng | Ô chọn ngôn ngữ inline ở bảng Bài Post, Voice và 2 bảng danh sách kênh |
| 7 | Ngôn ngữ nên auto-detect | `DEFAULT_LANGUAGE=auto`: lấy ngôn ngữ nền tảng khai báo, không có thì gửi `lang` rỗng để multime tự nhận diện |
| 8 | Chọn nhiều để thao tác hàng loạt + bộ lọc | Checkbox + thanh hành động hàng loạt (chạy/đăng/đổi ngôn ngữ/tạm dừng/xoá); lọc theo nền tảng, ngôn ngữ, người tạo, ngày tạo, ngày đăng |

---

## 4. Việc tiếp theo

### 4.1 Ưu tiên 1 — chạy thật luồng Mode A + publish

1. `make gen-key` → điền `TOKEN_ENCRYPTION_KEY`, `BOOTSTRAP_ADMIN_EMAIL`.
2. `docker compose up -d --build` → kiểm tra toàn bộ migration apply sạch.
3. Đăng nhập bằng tài khoản multime thật → xác nhận `app_user` được tạo với
   `strongbody_user_id` đúng.
4. `/on-demand`: dán 1 URL YouTube, Mode A → xác nhận có file voice nghe được.
5. Thêm hashtag cho voice → bấm Đăng → xác nhận bài xuất hiện trên
   `https://multime.ai/voice/<id>` dưới đúng tài khoản đó.
6. Xác nhận file trên S3 đã bị xoá và `voice_file_url = NULL`.

Bước 5 là bước kiểm chứng quan trọng nhất — nó xác nhận toàn bộ hợp đồng API tôi
đọc từ code là đúng.

### 4.2 Sau khi luồng chính chạy được

| Việc | Vì sao chưa làm |
|---|---|
| Kiểm chứng adapter Facebook / X / TikTok / Instagram bằng URL thật | Đã code và test phần parse URL; chưa chạy yt-dlp thật với 4 nền tảng này — nhiều nền tảng chặn tải nếu không đăng nhập (xem mục 2.3) |
| Rate limiter cho TTS provider | Chỉ cần khi bật Mode B/C ở quy mô lớn |
| Luồng xác minh TOTP khi login | Chỉ cần nếu tài khoản dùng voice-tool bắt buộc bật 2FA |
| Cảnh báo khi có Voice `failed` / token hết hạn | Specs ghi rõ "không bao gồm chức năng thông báo", nhưng 3.1 cho thấy cần ít nhất 1 chỗ hiển thị số Voice lỗi |
| Endpoint batch upload của multime (`/upload-batch`, 20 file) | Luồng hiện tại đăng từng voice; chỉ cần khi tối ưu throughput |
| Chọn AI Engine theo từng kênh | Hiện 1 engine mặc định toàn hệ thống. Cần thêm cột `ai_engine_id` vào `list_*` |
| Dashboard số liệu (voice/ngày, tỉ lệ lỗi, chi phí AI) | Chưa có yêu cầu, nhưng là thứ đầu tiên cần khi chạy thật ở quy mô |

### 4.3 Nợ kỹ thuật đã biết

| Món | Mức độ |
|---|---|
| Chưa có integration test chạy trên Postgres thật (testcontainers) | 🟡 Migration và query phức tạp (partial unique index, `ListDueListBreakings`) chỉ được verify bằng mắt |
| `service/engine.go` đã 400+ dòng, gánh cả 3 mode + publish | 🟢 Còn đọc được, nhưng thêm 1 mode nữa thì nên tách |
| Chưa có OpenAPI spec (chỉ có `docs/api.md` viết tay) | 🟢 Đủ cho 1 frontend; cần khi có client thứ 2 |
| Frontend chưa có test | 🟢 Logic đều ở backend; FE chủ yếu là form + table |
