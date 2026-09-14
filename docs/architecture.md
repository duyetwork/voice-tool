# Kiến trúc hệ thống

## 1. Tổng thể

```
                                   ┌──────────────┐
                                   │   Next.js    │
                                   │  (dashboard) │
                                   └──────┬───────┘
                                          │ JWT / REST
                                   ┌──────▼───────┐
                                   │  cmd/api     │  chỉ CRUD + enqueue
                                   │  (Gin)       │
                                   └──┬────────┬──┘
                          Postgres    │        │   Redis (Asynq)
                          ┌───────────▼──┐  ┌──▼──────────────┐
                          │  PostgreSQL  │  │     Redis       │
                          └───────▲──────┘  └──┬──────────────┘
                                  │            │
                                  │     ┌──────▼────────┐
                                  └─────┤ cmd/worker    │
                                        │ Core Engine   │
                                        └──┬──┬──┬──┬───┘
                                           │  │  │  │
              ┌────────────────────────────┘  │  │  └──────────────┐
              ▼                               ▼  ▼                 ▼
     ┌─────────────────┐            ┌──────────────┐      ┌────────────────┐
     │ PlatformAdapter │            │ TTS/STT/LLM  │      │ MultimeClient  │
     │ (yt-dlp, API)   │            │ (bên thứ 3)  │      │  multime.ai    │
     └─────────────────┘            └──────┬───────┘      └────────────────┘
                                           ▼
                                   ┌───────────────┐
                                   │ S3 / MinIO    │
                                   └───────────────┘
```

Ba binary dùng chung 1 Go module và chung `internal/app` để wiring dependency:

| Binary | Việc | Số process |
|---|---|---|
| `api` | HTTP, chỉ CRUD + enqueue | 1 (auto-scale được) |
| `worker` | Core Engine + các task quét | 2 (scale ngang an toàn) |
| `scheduler` | Phát task theo lịch | **1 — bắt buộc** |

`api` chỉ nhận `domain.Enqueuer`; `service.Engine` (nơi giữ TTS/STT/LLM/Multime)
chỉ chạy trong `worker`.

**Vì sao scheduler phải tách và chỉ 1 process:** `asynq.PeriodicTaskManager` tự
enqueue theo cronspec của chính nó. Nếu để trong `worker` rồi chạy 2 worker thì
mỗi kênh bị quét 2 lần — tốn gấp đôi quota nền tảng và chi phí AI.

## 2. Kiến trúc phần backend

Clean architecture 4 tầng, phụ thuộc luôn hướng vào trong:

```
transport/http  ──►  service  ──►  domain (interface)  ◄──  infra (cài đặt)
                        │
                        └──►  repository (sqlc)
```

- **`domain`** không import bất cứ package nào của dự án. Định nghĩa entity,
  enum, error, và các *port*: `PlatformAdapter`, `TTSProvider`, `STTProvider`,
  `LLMProvider`, `MultimeClient`, `MultimeAuthenticator`, `AudioProber`,
  `Storage`, `Enqueuer`, `PlatformRegistry`.
- **`service`** là nơi duy nhất chứa business rule. Không biết gì về HTTP.
- **`infra`** cài đặt các port. Thêm nền tảng mới = thêm 1 file trong
  `infra/platform` và đăng ký vào registry ở `internal/app/app.go`.
- **`transport/http`** parse request, gọi service, map error sang HTTP status
  (`pkg/httpx.Fail`).

## 3. Luồng dữ liệu

### F1 — On-Demand

```
POST /source-posts
  → PlatformRegistry.Resolve(url)        # auto-detect nền tảng, lỗi rõ nếu không nhận ra
  → adapter.ExtractID(url)               # loại nội dung + post id
  → INSERT source_post (status=new)
  → audit_log: create source_post
  → nếu auto_process: enqueue voice:process
```

### F2 — Breaking (quét liên tục)

```
breaking:dispatch (tự re-enqueue mỗi BREAKING_SCAN_INTERVAL)
  → ListDueListBreakings(default_interval)
      # chỉ kênh đã quá khoảng nghỉ của CHÍNH NÓ:
      #   last_scanned_at + COALESCE(scan_interval, default) <= now()
  → bỏ kênh đang NGOÀI khung giờ của nó (ChannelSchedule.Allows)
  → enqueue breaking:scan cho từng kênh, tối đa BREAKING_SCAN_PARALLELISM song song
      (asynq.Unique để vòng trước chưa xong thì không dồn task;
       ProcessIn = ScanJitter(list_id) để N kênh không bắn cùng một giây)

breaking:scan
  → adapter.FetchLatestPosts(channel, list.scan_limit)
  → matchAny(list.regex_patterns, post.Text)?     # nhiều pattern, kết hợp OR
       khớp     → INSERT source_post (dedup bằng unique index) → enqueue voice:process nếu auto_process
       không    → INSERT skipped_log
  → TouchListBreakingScanned  (đánh mốc, kể cả khi không bắt được bài nào)
```

### F3 — Định kỳ

```
PeriodicTaskManager (SyncInterval 30s, đọc list_scheduled từ DB)
  → có fixed_times_min → 1 cronspec "CRON_TZ=<tz> <phút> <giờ> * * <thứ>" cho MỖI mốc
    không có           → "@every <scan_frequency>"
  → scheduled:scan

scheduled:scan
  → ngoài khung giờ của kênh → bỏ vòng này
  → FetchLatestPosts(channel, list.scan_limit) → cắt phần mới hơn last_synced_post_id
  → cắt tiếp theo max_posts_per_run (giữ phần CŨ nhất, phần dư để vòng sau)
  → xử lý từ CŨ đến MỚI:
       ok               → tiến mốc
       lỗi vĩnh viễn    → tiến mốc (không kẹt vĩnh viễn)
       lỗi tạm thời     → dừng, KHÔNG tiến mốc (lần sau retry đúng bài đó)
```

Chống trùng: dedup theo `(platform, post_id_extracted)` toàn hệ thống, không
theo URL và không theo từng danh sách (xem [api.md](api.md#chống-trùng-theo-id-bài-đăng)).

### Lập lịch theo từng kênh

`scan_frequency` trả lời "bao lâu một lần", nhưng không trả lời "lúc nào". Kênh
tin tức không đăng lúc 3h sáng, và quét lúc đó là đốt hạn mức đúng vào lúc chẳng
thu được gì. Migration `000017` thêm cho cả hai loại kênh:

| Cột | Ý nghĩa |
|---|---|
| `timezone` | tên IANA, mặc định `Asia/Ho_Chi_Minh` |
| `active_from_min` / `active_to_min` | khung giờ, phút từ nửa đêm; cả hai NULL = 24/7; from > to = vắt qua nửa đêm |
| `active_weekdays` | `SMALLINT[]`, 0 = Chủ nhật … 6 = Thứ bảy; rỗng = mọi ngày |
| `fixed_times_min` | (chỉ Định kỳ) giờ chạy cố định — **thay thế** `scan_frequency` |

**Múi giờ là phần bắt buộc, không phải tuỳ chọn.** Lịch chạy theo giờ container
(UTC), nên "chỉ quét 6h–23h" mà tính theo UTC thì lệch 7 tiếng so với ý người
dùng — kênh nghỉ đúng lúc đang đăng bài. Ba binary đều `import _ "time/tzdata"`:
image thiếu `tzdata` thì `time.LoadLocation` im lặng rơi về UTC, tức là đúng cái
sai mà tính năng này sinh ra để sửa, và không có lỗi nào hiện ra để ai kịp nhận
thấy.

**Giờ lưu bằng số phút từ nửa đêm**, không phải kiểu `TIME`: giá trị đi qua JSON,
qua pgx và qua `<input type="time">` mà không phải đổi kiểu ở ba nơi.

**Khung giờ chặn ở handler, không ở cronspec.** Cron không diễn tả được khung
vắt qua nửa đêm hay tần suất không chia hết cho 60 phút; để hai nơi cùng quyết
định thì sớm muộn chúng nói khác nhau. Riêng `weekdays` thì cron diễn tả trọn
vẹn nên đưa luôn vào spec của lịch giờ-cố-định, để không phải đánh thức worker
vào ngày kênh nghỉ.

**Nút "Chạy ngay" KHÔNG bị khung giờ chặn**: nó enqueue thẳng `breaking:scan`,
và đó là yêu cầu tường minh của người dùng — im lặng bỏ qua thì họ bấm mà không
thấy gì xảy ra, cũng không biết vì sao.

### Giữ nhịp gọi nền tảng, và đo trước khi mua proxy

Rủi ro khi quét nhiều bài là có thật, không phải lý thuyết:
`infra/platform/ytdlperr.go` đã phải có nhãn riêng cho "confirm you're not a
bot" và HTTP 429 — tức là chúng đã xảy ra. Nhưng proxy là bậc gần cuối của thang
xử lý, không phải bậc đầu:

| Bậc | Việc | Trạng thái |
|---|---|---|
| 1 | Watermark `last_synced_post_id` giới hạn khối lượng phải lấy | đã có từ trước |
| 2 | Sàn tần suất (≥ 5 phút với Định kỳ) + rải lệch giờ chạy | **`service.ScanJitter`**, tất định theo id kênh |
| 3 | Mỗi nền tảng 1 lần gọi yt-dlp tại một thời điểm, có khoảng nghỉ giữa hai lần | **`service.PlatformGate`** (`PLATFORM_MIN_GAP`, mặc định 3s) |
| 4 | Tinh chỉnh yt-dlp (`--sleep-requests`, UA thật) | chưa làm |
| 5 | Cookies — gỡ được phần lớn chặn của FB/YouTube mà không cần proxy | chưa làm |
| 6 | **Đếm** số lần bị chặn theo từng nền tảng | **bảng `fetch_error_stat`** + màn Cài đặt |
| 7 | Proxy | **chưa làm — chờ số đếm ở bậc 6** |

`PlatformGate` giới hạn theo *từng nền tảng* chứ không toàn cục: chặn toàn cục
thì một kênh YouTube chậm sẽ chặn luôn việc quét TikTok, trong khi hai nền tảng
đó không chia sẻ hạn mức nào với nhau. Cùng một gate dùng cho cả quét kênh lẫn
tải bài — nền tảng chỉ thấy tổng số request, không quan tâm nó đến từ luồng nào.

Rải lệch dùng **băm id kênh**, không dùng ngẫu nhiên: ngẫu nhiên thì mỗi vòng
dispatch lại xáo lại thứ tự và hai kênh vẫn có lúc rơi trúng nhau; băm thì mỗi
kênh có một chỗ đứng cố định trong cửa sổ.

`fetch_error_stat` tách rõ loại chặn, vì cách xử lý khác hẳn nhau: `bot_block`
là thứ proxy giải quyết được, `login_required` thì phải có cookies (proxy không
giúp gì), còn `unavailable` (bài đã xoá) không phải bị chặn và không được làm
phồng con số dùng để quyết định. **Chưa xây phần quản lý proxy** — thiết kế đã
rõ (bảng `proxy`, gán theo người dùng, admin quản lý tập trung), chỉ là chưa có
dữ liệu chứng minh cần trả tiền cho nó.

### post:metadata

```
GetSourcePost
  → adapter.FetchMetadata(url)     # yt-dlp --dump-json
     → thất bại: thẻ Open Graph của trang (bài text / bài chỉ có ảnh)
  → UpdateSourcePostMetadata       # title (= toàn bộ nội dung bài, trừ
                                   # hashtag), hashtags, thumbnail, author,
                                   # posted_at
```

Chạy ngay khi tạo Bài Post, không tạo Voice. Thất bại chỉ ghi `last_error`,
KHÔNG đặt `status = failed` — bài vẫn chạy Voice được.

### voice:process (Core Engine)

API tạo sẵn record `voice` ở trạng thái `processing` **trước khi** enqueue, và
truyền `voice_id` trong payload. Nhờ vậy bảng Voice hiện dòng "đang xử lý" ngay
lúc bấm, thay vì trống trơn cho tới khi job xong.

```
ClaimSourcePostForProcessing   # new|failed → processing, idempotent khi retry
  → platform=text (bài CŨ, trước khi text đi thẳng ra Voice):
      nội dung lấy thẳng từ extracted_text, không gọi mạng
    còn lại:       adapter.FetchContent(postID, mode)
  → mode A: audio gốc
    mode B: text (nội dung bài → fallback phụ đề → fallback STT)
    mode C: text → LLM(prompt)
  → mode B/C: lưu extracted_text (text NGUỒN, không phải bản LLM viết lại)
              + ttsFor(người tạo voice) → TTS bằng API key của chính họ
  → ffprobe đo duration/sample_rate/mime (multime.ai cần cho audio_asset)
  → storage.Put(voices/<post_id>/<...>.mp3)
  → FinishVoice(voice_id)        # điền file + metadata, processing → draft
                                 # (payload không có voice_id = task cũ
                                 #  → INSERT voice như trước)
  → source_post.status = processed
  → audit_log: create voice
  → nếu list.auto_publish: enqueue voice:publish
```

`last_error` chỉ lưu 1 câu tiếng Việt (`domain.UserMessage`); nguyên văn stderr
của yt-dlp đi vào log. Bảng phân loại lỗi nằm ở
`internal/infra/platform/ytdlperr.go`.

Lỗi ở bất kỳ bước nào → `source_post.status = failed` + `last_error`, record
voice chuyển `processing → failed` (không để treo ở "đang xử lý"), và task được
Asynq retry 3 lần với backoff 30s/60s/120s (trừ lỗi `PermanentError`).

### Hình thức C bắt buộc có LLM thật

Mode C = *text nguồn → LLM viết lại theo Prompt mẫu → TTS đọc bản viết lại*.
3voices chỉ có TTS/STT, không có endpoint sinh hay viết lại text, nên bước giữa
phải là một LLM riêng.

"Có LLM thật" có **hai đường**: Bộ API key khai trong DB (đường chính, xem mục
dưới) hoặc `LLM_PROVIDER` + key trong `.env` (dự phòng cho dev). `app.New` tắt
hẳn mode C **chỉ khi cả hai đều không có** — provider là mock *và*
`llm_api_key` rỗng — rồi kèm lý do vào `ModeGate`: API từ chối sớm, FE hiện
đúng câu "cần LLM thật…" thay vì chữ "chưa hỗ trợ" chung chung. Trước đây mock
ghép `prompt + text` rồi trả về, nên voice đọc to cả câu lệnh dành cho AI; giờ
mock trả nguyên text nguồn để dù có lọt qua đâu đó cũng không bao giờ đọc prompt.

### LLM Router: bộ API key + chuỗi dự phòng

Trước đây có đúng 1 key LLM cho cả hệ thống, đọc từ `.env`. Vấn đề không phải
ở chỗ ít key mà ở chỗ **hạn mức**: mode C chạy tự động theo kênh, và một nhà
cung cấp cạn hạn mức là cả hệ thống đứng. Nên key trở thành *dữ liệu*, và có
một tầng đứng trên chọn key.

Ba bảng, migration `000016`:

| Bảng | Là gì |
|---|---|
| `llm_api_set` | 1 **bộ** = 1 túi key, có `visible_to_users` (toggle admin) |
| `llm_api_key` | key trong bộ: nhà cung cấp, key mã hoá, `priority`, và 3 cột **sức khoẻ** |
| `llm_api_set_user` | bộ dùng chung cho 1 hay nhiều người |

**Vì sao không tái dùng `ai_engine`.** Bảng đó đã được cố tình thu về "sổ key
TTS" ở migration `000010`: 1 bản ghi = 1 key = 1 chủ sở hữu, không tên, không
provider. Bộ API LLM là thứ ngược lại — 1 bản ghi = *nhiều* key của *nhiều* nhà,
dùng chung cho *nhiều* người. Nhét cả hai vào một bảng là làm sống lại đúng
những cột mà `000010` vừa bỏ đi.

**Thuật toán chọn — tất định, không xoay ngẫu nhiên:**

```
chain  = app_setting['llm.chain']        # rẻ trước, đắt sau
ứng viên = cho từng mắt xích trong chain:
             cho từng key cùng nhà, sắp theo (priority, id)
               bỏ key có disabled_at
               bỏ key còn cooldown_until > now()

với từng ứng viên:
  gọi Generate
  thành công      → MarkLLMAPIKeyOK  (reset consecutive_failures) → xong
  429 / hết quota → cooldown_until = now + Retry-After, hoặc backoff gấp đôi
                    theo consecutive_failures (1 phút → trần 6 giờ) → key kế
  401 / 403       → disabled_at = now() + last_error               → key kế
  5xx / mạng      → thử lại tại chỗ 2 lần có jitter, rồi mới sang key kế
  lỗi do NỘI DUNG → KHÔNG đổi key, chỉ đổi MODEL, tối đa 1 lần

hết ứng viên → lỗi "Bộ API đã cạn" kèm lý do của TỪNG key
```

Vòng ngoài là chuỗi dự phòng, vòng trong là key: thứ tự ưu tiên thuộc về
*model* (rẻ trước), không thuộc về key. Đảo lại nghĩa là key thứ nhất chạy hết
mọi model đắt tiền trước khi key thứ hai được thử ở model rẻ nhất.

**Không cần sticky cache.** `prompt.txt` yêu cầu "không xoay vòng ngẫu nhiên nếu
key/model hiện tại vẫn đang chạy tốt" — thứ tự tất định ở trên đã thoả điều đó:
chừng nào key đầu chuỗi còn khoẻ thì mọi request đều rơi vào đúng nó. Trạng thái
"còn khoẻ hay không" nằm trong DB chứ không trong RAM của process, nên một key
vừa bị 429 ở worker A thì worker B cũng biết đường tránh — thứ mà sticky cache
trong bộ nhớ không làm được.

**Phân biệt lỗi-do-key với lỗi-do-nội-dung là chỗ dễ sai nhất.** Lỗi nội dung
(bị chặn vì chính sách, prompt quá dài) mà cũng xoay key thì hệ thống đốt sạch
mọi key của mọi nhà trên cùng một input chắc chắn thất bại, rồi báo "hết quota"
— trong khi quota vẫn còn nguyên, và người dùng đi sửa sai chỗ. Việc phân loại
nằm ở **adapter** (`domain.LLMFail*`) vì chỉ adapter mới đọc được mã lỗi của nhà
mình: Gemini trả 400 cho cả "key sai" lẫn "input quá dài", OpenAI báo hết tiền
bằng 429 nhưng nó không tự hồi theo thời gian như rate-limit.

**Ghim ID model đầy đủ, cấm alias.** `domain.AllowedLLMModels` là danh sách
trắng, và `ValidateLLMChain` chặn mọi thứ ngoài nó — kể cả khi admin tự gõ vào
màn Cài đặt. Lý do: alias `gpt-5.6` không trỏ về Luna mà trỏ về Sol (đắt hơn
khoảng 25 lần); viết thiếu hậu tố thì hệ thống vẫn chạy đúng, không có lỗi nào
hiện ra, chỉ có hoá đơn đội lên — và vì mắt xích đó chỉ chạy khi Gemini đã cạn
nên rất lâu mới có ai nhận ra. Có test chặn ở `domain/llm_test.go`.

`voice.llm_model_used` lưu model **thật** đã chạy: cùng một bộ, hôm nay chạy
`gemini-2.5-flash-lite`, mai hết hạn mức thì chạy `gpt-5.6-luna` — không ghi lại
thì không đối chiếu được chất lượng hay chi phí của một voice cụ thể.

### Batch: gom lời gọi đang cùng chờ

Batch gộp N mẩu text vào 1 request. Nó chỉ có ích ở luồng tự động (một vòng quét
sinh ra nhiều bài một lượt); tạo voice lẻ trên UI bản chất là lô 1 mẩu.

Chỗ gom là **tầng router**, không phải chỗ quét kênh. Batch chỉ đúng khi N mẩu
text là N mẩu hệ thống *thật sự sẽ đọc*, mà text đó chỉ có sau khi worker đã tải
bài và lấy phụ đề — ở thời điểm quét kênh mới chỉ có caption, gom sớm hơn nghĩa
là viết lại một nội dung khác với nội dung sẽ được đọc.

```
voice:process (N task chạy song song)
   └→ rewriteIfNeeded → LLMRouter.Generate
        └→ llmBatcher.submit: đứng chờ tối đa wait_ms xem có ai
           cùng (bộ API, chủ sở hữu, Prompt mẫu) đang chờ không
             đủ size mẩu / vượt max_chars / hết giờ → 1 request GenerateBatch
```

Đầu ra dùng **structured output native** của từng nhà (Gemini `responseSchema`,
OpenAI `json_schema` strict, Anthropic tool bắt buộc), không phải "nhắc trong
prompt" — nhắc thì model vẫn có thể trả văn xuôi kèm ```json và đó là lỗi parse
ngẫu nhiên rất khó tái hiện. Kết quả khoá theo **chỉ số**, không theo thứ tự
mảng: model đảo thứ tự thì voice của bài A đọc nội dung của bài B, im lặng và
gần như không lần ra được sau khi đã đăng.

Lô hỏng vì lý do không phải hạn mức → **tự hạ về gọi lẻ từng mẩu**; một mẩu xấu
không được làm chết N−1 mẩu còn lại. Hết hạn mức thì không hạ, vì gọi lẻ cũng
hết hạn mức, chỉ nhân số request lên N lần rồi thất bại y như cũ.

Cấu hình ở `app_setting['llm.batch']` (`size` 5, `max_chars` 12000, `wait_ms`
2000). Ba con số này là **điểm khởi đầu phải đo lại**, không phải hằng số đúng
sẵn — đó chính là lý do chúng nằm trong DB chứ không trong code.

### voice:text (Voice gõ tay và tạo lại voice)

```
POST /voices              → INSERT voice (input_text, collect_mode, prompt_id,
                                          publish_status=processing)
POST /voices/:id/regenerate → UPDATE voice (input_text mới, publish_status=processing)
                          → enqueue voice:text
voice:text
  → GetVoice → bỏ qua nếu không còn ở trạng thái processing (idempotent)
  → mode C: LLM(prompt, input_text)        # dùng chung rewriteIfNeeded
  → ttsFor(người tạo) → TTS bằng API key của chính họ
  → ffprobe → storage.Put(voices/text/<voice_id>-<ts>.mp3)
  → FinishVoice(voice_id)                  # processing → draft
```

Luồng này không fetch gì: có `input_text` nghĩa là người dùng đã chốt lời đọc,
kể cả với Voice vốn sinh ra từ Bài Post (sửa lời đọc rồi bấm tạo lại). Không
dedup, không `post:metadata`. Tạo lại thì file audio cũ bị xoá **sau khi** DB đã
ghi file mới — xoá trước mà ghi hỏng là mất cả hai. Lỗi →
`voice.publish_status = failed` + `last_error`.

### voice:publish

```
GetVoice → đã published thì bỏ qua (idempotent)
  → storage.Get(key)
  → tải image_url về nếu có (API multime nhận file, không nhận URL)
  → creds.For(voice.created_by)   # token multime của NGƯỜI TẠO voice
  → multime.PublishVoice   # upload audio + tạo bài đăng trong 1 request:
                           #   POST /v1/seller/voice-posts/upload
                           #   401 -> refresh token -> thử lại 1 lần
  → MarkVoicePublished (set multime_post_url, voice_file_url = NULL)
  → storage.Delete(key)          # ghi DB trước, xoá file sau
  → audit_log: publish voice
```

Trước khi gọi API, client chặn sẵn 3 điều kiện mà multime sẽ từ chối: `title`
bắt buộc, ít nhất 1 hashtag/category, audio tối thiểu 15 giây. Lỗi trả về là
`PermanentError` nên Asynq không retry vô nghĩa, và FE disable nút Đăng kèm lý
do cụ thể.

Thất bại → `publish_status = failed` + `last_error`, **giữ nguyên file** để retry.

### TTS chạy bằng API key của từng người

`ai_engine` không còn là danh mục "chọn engine nào" — chỉ còn 1 nhà cung cấp
(3voices) nên mỗi bản ghi là **API key của một người**: `user_id` (chủ sở hữu),
`api_key_encrypted`, `created_by` (người khai — khác chủ khi admin khai hộ),
`last_used_at`.

```
ttsFor(owner, language)
  → GetAIEngineForUser(owner)     # key mới khai nhất của chủ sở hữu
     ├ có   → box.Decrypt(api_key) → ttsFactory.For(cred) → ThreeVoices
     │        → validate ngôn ngữ với provider.SupportedLanguages()
     └ không→ provider dự phòng trong .env (dev: mock)
              không có nốt → PermanentError kèm câu "vào mục AI Engine thêm key"
  ...TTS thành công → TouchAIEngineUsed(id)   # cột "dùng gần đây"
```

Danh sách ngôn ngữ hỏi thẳng provider chứ không khai tay trong DB: nhà cung cấp
là nơi duy nhất biết mình đọc được thứ tiếng nào, còn một bản sao trong DB thì
lệch dần theo thời gian.

Vì sao theo người tạo voice: quota và hoá đơn 3voices tính trên key, nên phải
rơi đúng vào người bấm chạy. Cùng lý do với việc publish bằng token multime của
`voice.created_by` — worker luôn hành động *thay mặt* một người cụ thể, không
có tài khoản hệ thống dùng chung.

Key mã hoá AES-256-GCM bằng `TOKEN_ENCRYPTION_KEY` (cùng `secret.Box` với token
multime) và không bao giờ ra khỏi backend: API chỉ trả 4 ký tự cuối.

### Voice gõ tay: text → Voice, không qua Bài Post

Hình thức B/C nhận nguồn từ URL **hoặc** đoạn text người dùng gõ, và hai nguồn
đi hai đường khác nhau:

```
URL  → POST /source-posts → source_post → voice:process → voice
Text → POST /voices       → voice (input_text, collect_mode, prompt_id)
                          → voice:text → TTS → cùng một record voice
```

Business rule #1 ("mọi Voice đều đi qua Bài Post") vẫn giữ ở chỗ nó có ý nghĩa:
Voice lấy từ một URL phải truy vết được về bài gốc, và phải chạy lại được với
mode/prompt khác mà không fetch lại URL. Text gõ tay không có URL, không có bài
gốc, không có gì để fetch lại — `source_post` sinh ra chỉ là bản ghi rỗng đứng
giữa, làm bẩn màn duyệt Bài Post (nơi để duyệt trước khi tốn tiền AI) mà không
thêm thông tin nào. `ck_voice_origin` (migration 000011) giữ đúng 2 dạng Voice
hợp lệ, không có dạng thứ ba.

Hai đường gặp lại nhau ở `rewriteIfNeeded` (mode C viết lại qua LLM) và
`ttsFor` (chọn API key TTS), nên hai luật đó chỉ có một bản cài đặt.

## 4. Các quyết định thiết kế đáng lưu ý

**Regex dùng RE2 (`regexp` của Go), không backtrack.** Nên không có nguy cơ
catastrophic backtracking; validate chỉ cần chặn pattern không compile được và
pattern quá dài (512 ký tự).

**Chống lấy lặp có 2 lớp.** `last_synced_post_id` là lớp nhanh; unique partial
index `(list_*_id, post_id_extracted)` là lớp chắc chắn — nếu marker bị lệch
(kênh đăng nhiều bài giữa 2 lần quét), DB vẫn không cho tạo trùng.

**`breaking:dispatch` thay cho cron.** Business rule yêu cầu quét liên tục,
không dùng cron cố định. Task tự enqueue lại chính nó sau mỗi vòng, kèm
`asynq.Unique` để nhiều instance worker không nhân đôi vòng lặp.

**Lịch F3 đọc trực tiếp từ DB.** `PeriodicTaskConfigProvider` được Asynq gọi
lại mỗi 30 giây, nên sửa `scan_frequency` qua API là lịch tự cập nhật — không
cần API riêng để re-register.

**Ngưỡng an toàn về tần suất.** `BREAKING_SCAN_INTERVAL` (mặc định 60s) chặn
quét quá dày ở F2; `minScanFrequency` (1 phút) chặn ở F3. Cả hai để tránh vượt
rate-limit của nền tảng nguồn.

**Client multime là 1 request, không phải 2.** Ban đầu tôi thiết kế
`MultimeClient` thành `UploadVoice` + `CreatePost` vì `strongbody-api` có
`POST /files/upload` riêng. Hợp đồng thật (đọc từ repo `multime-ai`) upload audio
và tạo bài đăng trong cùng 1 request multipart, nên interface đã rút còn 1
method `PublishVoice`. Token lấy qua `/v1/public/auth/login`, cache lại, tự
đăng nhập lại đúng 1 lần khi gặp 401.

**Ngôn ngữ mặc định là `auto`.** Đoán sai ngôn ngữ tệ hơn là không đoán: khi
Bài Post để `auto`, worker lấy ngôn ngữ nền tảng khai báo (yt-dlp `language`);
không có thì giữ `auto` và khi publish gửi `lang` rỗng để multime.ai tự nhận
diện từ audio. TTS cũng nhận chuỗi rỗng thay vì bị ép đọc sai giọng.

**Mọi nền tảng chạy trên cùng 1 backend fetch (yt-dlp).** `ytdlpCore` giữ phần
chung (metadata, tải audio, phụ đề, liệt kê kênh); mỗi nền tảng chỉ khai host
regex + pattern parse ID. Khác biệt duy nhất đáng kể: YouTube dựng lại được URL
chuẩn từ ID, còn TikTok/Facebook/Instagram/X thì không (cần cả @username hoặc
tên page), nên `FetchContent` nhận `PostRef{URL, PostID}` chứ không chỉ ID.

**Token đi qua query param, chỉ cho route phát audio.** Thẻ `<audio>` không
gắn được header `Authorization`, nên muốn thanh phát hiện sẵn trong bảng thì URL
phải tự mang theo token — `middleware.AuthMedia` chấp nhận `?token=`, và chỉ
route `GET /voices/:id/audio` dùng nó. Đổi lại `preload="none"`: mở bảng 50
voice không kéo về 50 file, trình duyệt chỉ tải khi người dùng bấm play.

**Nghe thử voice đi qua API, không qua URL storage.** Bucket là riêng tư và
host `minio:9000` chỉ tồn tại trong mạng Docker — trình duyệt không mở được
`voice_file_url`. `GET /voices/:id/audio` đọc file từ storage và trả kèm quyền
đã xác thực; FE tải blob rồi phát, nên không phải mở public bucket hay ký URL.

**Provider AI mặc định là `mock`.** Hệ thống chạy end-to-end được ngay khi chưa
có API key: `mock` TTS sinh file WAV im lặng dài tỉ lệ với số ký tự. Đổi sang
provider thật chỉ bằng biến môi trường.

**Đăng nhập là SSO của strongbody, không có xác thực cục bộ.** Hệ thống không
lưu mật khẩu; `POST /auth/login` proxy sang
`api-v2.strongbody.ai/v1/public/auth/login`, upsert `app_user` theo email rồi
phát JWT riêng. Lý do quan trọng hơn là tiện: tài khoản đăng nhập **chính là**
tài khoản đăng voice, nên bài xuất hiện trên multime dưới đúng tên người tạo,
không phải một tài khoản hệ thống dùng chung.

Hệ quả: access/refresh token của user phải lưu lại để worker publish thay họ
(user không cần online lúc F2/F3 chạy). Token là credential của hệ thống khác
nên được mã hoá AES-256-GCM bằng `TOKEN_ENCRYPTION_KEY` — một bản dump DB không
biến thành xâu token dùng được ngay. Hết hạn thì refresh 1 lần; refresh lỗi thì
trả lỗi vĩnh viễn yêu cầu user đăng nhập lại (không có cách nào tự sửa).

**Phân quyền 3 mức, chia theo nhóm route.** `user` đọc + tạo/sửa/chạy/đăng;
`editor` thêm xoá; `admin` thêm cấp quyền. Router chia 4 group
(`authed` / `writer` / `remover` / `admin`) thay vì kiểm tra role rải rác trong
từng handler — nhìn `router.go` là biết ai làm được gì.

**Text nguồn và text đọc là 2 thứ khác nhau.** `source_post.extracted_text` giữ
đúng text NGUỒN, không bao giờ giữ bản LLM viết lại: nếu lưu lẫn, chạy lại Bài
Post với prompt khác sẽ khiến LLM đọc chính đầu ra của nó ở lần trước. Bản LLM
viết lại chỉ được TTS đọc, không ghi đè text nguồn; nó chỉ lọt vào
`voice.title` khi bài gốc không có nội dung nào lấy được.

**`skipped_log` có retention, `audit_log` thì không.** `skipped_log` là dữ liệu
debug regex (mỗi vòng quét ghi tối đa `scan_limit` bản ghi cho mỗi kênh — có thể
lên hàng trăm nghìn/ngày), giữ 7 ngày. `audit_log` là append-only vĩnh viễn theo
specs 1.6.

## 5. Thêm 1 nền tảng nguồn mới

1. Tạo `internal/infra/platform/<tên>.go`, implement `domain.PlatformAdapter`
   (5 method: `Name`, `DetectPlatform`, `ExtractID`, `FetchContent`,
   `FetchLatestPosts`).
2. Đăng ký vào `platformadapter.NewRegistry(...)` trong `internal/app/app.go`.
3. Thêm test URL parsing theo mẫu `youtube_test.go`.

Không phải sửa service, handler hay DB.

## 6. Thêm 1 provider AI mới

Thêm file vào `internal/infra/ai/{tts,stt,llm}/` implement interface tương ứng,
rồi thêm 1 `case` trong hàm `New(cfg)` của package đó. Với TTS, thêm `case`
tương ứng trong `tts.Factory.For` và bỏ CHECK `provider = '3voices'` ở
`ai_engine`; ngôn ngữ hỗ trợ lấy từ `SupportedLanguages()` của chính provider,
không phải khai tay.

## 7. Cấu trúc thư mục

```
voice-tool/
├── backend/
│   ├── cmd/
│   │   ├── api/                  # HTTP API (chỉ CRUD + enqueue job)
│   │   ├── worker/               # Asynq consumer — chạy được nhiều process
│   │   ├── scheduler/            # Phát task theo lịch — CHỈ 1 process
│   │   └── genkey/               # sinh TOKEN_ENCRYPTION_KEY
│   ├── internal/
│   │   ├── app/                  # wiring dependency dùng chung 3 binary
│   │   ├── config/               # viper, đọc .env
│   │   ├── domain/               # entity, enum, error, PORTS (interface)
│   │   ├── repository/           # code sqlc sinh ra + queries/*.sql
│   │   ├── service/              # business logic
│   │   │   ├── engine.go         # Core Engine: Post → Voice → publish
│   │   │   ├── scan.go           # breaking scan / scheduled scan
│   │   │   ├── sourcepost.go     # tầng Bài Post
│   │   │   ├── voice.go          # tầng Voice
│   │   │   ├── list.go           # 2 danh sách kênh (F2 / F3)
│   │   │   ├── catalog.go        # Prompt mẫu + AI Engine
│   │   │   ├── audit.go          # Nhật ký thao tác
│   │   │   ├── auth.go           # đăng nhập SSO strongbody
│   │   │   ├── multimecreds.go   # token multime của từng user (mã hoá)
│   │   │   ├── user.go           # phân quyền admin/editor/user
│   │   │   ├── maintenance.go    # dọn skipped_log
│   │   │   └── language.go       # cascade ngôn ngữ theo tầng
│   │   ├── infra/                # cài đặt các port ra thế giới bên ngoài
│   │   │   ├── platform/         # adapter nền tảng nguồn + registry auto-detect
│   │   │   ├── ai/{tts,stt,llm}/ # adapter nhà cung cấp AI (3voices, elevenlabs…)
│   │   │   ├── audio/            # ffprobe — đo duration/sample_rate/mime
│   │   │   ├── multime/          # client nền tảng đích (strongbody-api)
│   │   │   ├── storage/          # S3/MinIO
│   │   │   └── postgres/         # pgx pool
│   │   ├── transport/http/       # router, handler, middleware
│   │   ├── worker/               # task định nghĩa, enqueuer, consumer, scheduler
│   │   └── pkg/                  # logger, jwt, httpx, validator (regex), secret (AES-GCM)
│   ├── migrations/               # golang-migrate
│   ├── seeds/                    # dữ liệu mẫu cho dev
│   ├── sqlc.yaml
│   └── Dockerfile                # multi-stage: target api / worker / scheduler
├── frontend/
│   ├── src/
│   │   ├── app/
│   │   │   ├── login/
│   │   │   └── (dashboard)/      # on-demand, lists/*, source-posts, voices,
│   │   │                         # prompts, ai-engines, audit-log
│   │   ├── components/           # app-shell, providers, ui/*
│   │   ├── hooks/use-api.ts      # TanStack Query hooks
│   │   ├── lib/                  # api client (JWT + auto refresh), utils
│   │   └── types/api.ts
│   └── Dockerfile
├── docs/
├── buildspec.yml                 # CodeBuild — build 3 image, push ECR
├── docker-compose.yml
├── Makefile
└── .env.example
```

## 8. Business rules đang được enforce trong code

| # | Rule | Enforce ở đâu |
|---|---|---|
| 1 | Mọi Voice phải đi qua `SourcePost`, không có đường tắt URL → Voice | Không có endpoint `POST /voices`; Voice chỉ sinh từ `POST /source-posts/:id/run` |
| 2 | Publish thành công → xoá file S3, set `voice_file_url = NULL`; publish lỗi → giữ file để retry | `MarkVoicePublished` + `Engine.publish`, ràng buộc `ck_voice_published` |
| 3 | Regex là cơ chế nhận diện duy nhất; keyword/hashtag được chuẩn hoá về regex trước khi lưu. Nhiều pattern/kênh, kết hợp OR | `pkg/validator.NormalizePattern` + `service.normalizePatterns` |
| 4 | Breaking không có tần suất — worker quét liên tục, không cron | `breaking:dispatch` tự re-enqueue sau mỗi vòng |
| 5 | Scheduled có `scan_frequency` riêng từng kênh, sửa là lịch tự cập nhật | `worker/scheduler` đọc trực tiếp DB qua `PeriodicTaskConfigProvider` |
| — | Không đăng ký; đăng nhập bằng SSO strongbody, voice đăng bằng tài khoản của chính user | `service.Auth.SignIn`, `service.MultimeCreds`, `Engine.publishAs` |
| — | Phân quyền user/editor/admin — `user` đăng được voice nhưng không xoá | `middleware.RequireWrite/RequireDelete/RequireAdmin`, router chia 4 group |
| 6 | Chống lấy lặp: lỗi tạm thời không tiến `last_synced_post_id`, lỗi vĩnh viễn thì tiến | `service.Scan.ScanScheduled` + `domain.PermanentError` + unique index dedup |
| 7 | `auto_process` / `auto_publish` cấu hình theo từng danh sách, không hard-code | Cột trên `list_breaking` / `list_scheduled`, đọc trong `Engine.autoPublishFor` |
| 8 | Audit Log ghi tự động cho 4 entity, service không tự viết log riêng lẻ | `service.Audit` được inject vào mọi service có mutation |
| 9 | Cascade ngôn ngữ: Post > List > mặc định hệ thống; Mode A chỉ gắn nhãn | `service/language.go`, `Engine.buildVoice` |
| 10 | API chỉ CRUD + enqueue, không gọi TTS/STT/LLM/Multime trong HTTP handler | Handler chỉ gọi `domain.Enqueuer`; provider chỉ được inject vào `service.Engine` (chạy trong worker) |
