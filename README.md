# Voice Automation Tool

Công cụ tự động lấy/tạo voice từ nội dung mạng xã hội và đăng lên **multime.ai**,
theo mô hình 3 tầng:

```
Danh sách (List) → Bài Post (SourcePost) → Voice → multime.ai
```

3 luồng thu thập:

| Luồng | Tên | Cơ chế |
|---|---|---|
| **F1** | On-Demand | Dán 1 URL, xử lý ngay |
| **F2** | Breaking | Quét liên tục, chỉ lấy bài khớp Regex Pattern |
| **F3** | Định kỳ | Quét theo tần suất riêng từng kênh, lấy toàn bộ bài mới |

3 hình thức thu thập voice (áp dụng cho cả F1/F2/F3):

| Mã | Tên | Cơ chế |
|---|---|---|
| **A** | Extract từ URL | Tải video/audio gốc, tách trực tiếp track giọng nói |
| **B** | Text → TTS → Voice | Lấy caption/transcript (hoặc text gõ tay) → TTS đọc nguyên văn |
| **C** | Text + Prompt → TTS → Voice | Text gốc → LLM viết lại theo Prompt mẫu → TTS |

Nền tảng nguồn: YouTube, Facebook, TikTok, Instagram, X — hệ thống tự nhận diện
từ URL. Metadata bài gốc (nội dung bài, hashtag, ảnh bìa) được lấy về và điền
sẵn vào Voice, nên thường chỉ cần nghe thử rồi bấm Đăng. Không có trường mô tả:
multime chỉ hiển thị tiêu đề, nên tiêu đề Bài Post mang trọn nội dung bài (trừ
hashtag) và tiêu đề Voice là 200 ký tự đầu của nó — đúng giới hạn multime nhận.

Hình thức B/C đọc bằng TTS **3voices**: mỗi người tự khai API key của mình ở
mục **AI Engine** (key mã hoá trong DB, không bao giờ hiện lại), worker đọc
bằng key của chính người tạo voice nên quota/chi phí về đúng người đó. Admin
xem và quản lý được key của tất cả mọi người.

Nguồn cho B/C có 2 kiểu: **URL** (hệ thống tự lấy caption/transcript) hoặc
**gõ thẳng text** ở màn F1 — text nhập tay không có audio gốc nên chỉ dùng
được B/C. Mode C cần thêm LLM (`ANTHROPIC_API_KEY`). Muốn tắt bớt hình thức
nào thì sửa `ENABLED_COLLECT_MODES` trong `.env` (mặc định `A,B,C`) — UI hiển
thị mờ và API từ chối hình thức đã tắt.

**Tech stack:** Go 1.26 (3 binary `api` / `worker` / `scheduler`, Gin, pgx +
sqlc, Asynq) · PostgreSQL 16 · Redis · S3-compatible (MinIO dev / S3 prod) ·
Next.js 15 + Tailwind v4 + TanStack Query · Docker Compose.

---

## Chạy local

### Yêu cầu

| Cần | Dùng để |
|---|---|
| Docker + Docker Compose | Cách chạy được khuyến nghị — dựng đủ 9 service |
| Go 1.26+ | Chỉ cần nếu chạy backend trực tiếp, hoặc dùng `make gen-key` |
| Node 22+ | Chỉ cần nếu chạy frontend trực tiếp |

### Cách 1 — Docker Compose (khuyến nghị)

```bash
cp .env.example .env
```

Sửa 3 dòng trong `.env`:

```bash
JWT_SECRET=<chuỗi ngẫu nhiên bất kỳ>
BOOTSTRAP_ADMIN_EMAIL=<email multime của bạn>   # để lần đầu đăng nhập là admin
TOKEN_ENCRYPTION_KEY=<khoá base64 32 byte>      # xem cách sinh bên dưới
```

`TOKEN_ENCRYPTION_KEY` **không có giá trị mặc định** — thiếu nó thì `api` và
`worker` fail-fast ngay lúc khởi động (có chủ ý, để không chạy với khoá rác).
Sinh khoá bằng 1 trong 3 cách:

```bash
make gen-key                     # cần Go
openssl rand -base64 32          # cần openssl
docker run --rm alpine sh -c "head -c 32 /dev/urandom | base64"   # chỉ cần Docker
```

Rồi bật hệ thống:

```bash
docker compose up -d --build
```

Lần đầu build khoảng 3-5 phút (image worker phải cài `yt-dlp` + `ffmpeg`).
Service `migrate` chạy toàn bộ migration rồi tự thoát — đó là bình thường, không phải
lỗi. Xong thì kiểm tra:

```bash
docker compose ps           # 9 service running/healthy, migrate ở trạng thái exited (0)
docker compose logs -f api  # thấy dòng "api đang lắng nghe" là ổn
```

| Service | URL | Ghi chú |
|---|---|---|
| **Frontend** | http://localhost:3000 | Vào đây trước |
| API | http://localhost:8080/api/v1 | `GET http://localhost:8080/healthz` để kiểm tra sống |
| **Adminer** (xem database) | http://localhost:8085 | Xem [docs/database.md](docs/database.md) |
| Asynq monitor (xem queue/job) | http://localhost:8081 | Job đang chạy, đang chờ, đã fail |
| MinIO console (xem file voice) | http://localhost:9001 | Đăng nhập `minioadmin` / `minioadmin` |

Nạp dữ liệu mẫu (3 AI engine + 2 prompt cho Mode C) — chạy **sau khi đã đăng
nhập lần đầu**, vì prompt cần gán `created_by` cho tài khoản admin:

```bash
docker compose exec -T postgres psql -U voice -d voice_tool < backend/seeds/seed.sql
```

Dừng / khởi động lại / xoá sạch:

```bash
docker compose down            # dừng, GIỮ dữ liệu
docker compose down -v         # dừng và XOÁ toàn bộ dữ liệu (postgres + redis + minio)
docker compose up -d --build   # build lại sau khi sửa code
docker compose restart api     # restart 1 service
```

### Cách 2 — chạy code trực tiếp (dev nhanh hơn, hot reload FE)

```bash
make infra          # chỉ dựng postgres + redis + minio bằng Docker
make setup          # tạo .env, cài sqlc + golang-migrate + npm install

# Sửa .env: đổi host trong 3 biến này từ tên container sang localhost
#   DATABASE_URL=postgres://voice:voice@localhost:5432/voice_tool?sslmode=disable
#   REDIS_ADDR=localhost:6379
#   S3_ENDPOINT=http://localhost:9000

make migrate-up

make api            # terminal 1
make worker         # terminal 2  (chạy được nhiều instance song song)
make scheduler      # terminal 3  (CHỈ 1 instance — nhiều hơn là task bị nhân đôi)
make web            # terminal 4
```

`make help` liệt kê toàn bộ target.

Cần gọi API mà không muốn đăng nhập qua SSO (viết test, thử curl):

```bash
make dev-token user=<uuid trong bảng app_user>   # in ra 1 access token
```

Tiện ích này ký bằng đúng `JWT_SECRET` trong `.env` nên **chỉ dùng ở máy dev**;
nó không nằm trong image Docker nào.

### Đăng nhập

Hệ thống **không có đăng ký** — đăng nhập bằng tài khoản **multime.ai** của bạn,
và voice sẽ được đăng lên chính tài khoản đó.

- `MULTIME_BASE_URL` trỏ tới voice API thật (mặc định trong `.env.example`):
  dùng đúng email + mật khẩu multime của bạn.
- Để **trống** `MULTIME_BASE_URL`: chạy chế độ mock — đăng nhập bằng email bất
  kỳ + mật khẩu ≥ 6 ký tự, publish sinh URL giả. Đủ để thử toàn bộ luồng mà
  không đăng bài thật.

Tài khoản đăng nhập lần đầu nhận role `DEFAULT_USER_ROLE` (mặc định `user`);
email trong `BOOTSTRAP_ADMIN_EMAIL` luôn là `admin`. Chi tiết phân quyền:
[docs/api.md](docs/api.md).

### Thử luồng chính (Mode A — extract audio từ YouTube)

1. Mở http://localhost:3000, đăng nhập.
2. Vào **F1 — Theo yêu cầu**, dán 1 URL YouTube, để `Hình thức thu thập` = **A**.
3. Bấm **Tạo Bài Post**. Job `voice:process` được đẩy vào queue.
4. Xem tiến trình ở http://localhost:8081 (queue `default`) hoặc
   `docker compose logs -f worker`.
5. Vào **Voice** — bấm **▶ Nghe thử** (hoặc **⤓ Tải về**) để kiểm tra file.
6. Tiêu đề/hashtag/ảnh bìa đã điền sẵn từ bài gốc — bấm **Sửa metadata**
   nếu muốn đổi, rồi bấm **Đăng**.

### Thử Mode B/C (TTS đọc text)

1. Vào **AI Engine** → **Thêm API key**, dán key 3voices của bạn (dạng
   `sk-ov-…`). Key được mã hoá trước khi lưu và không hiển thị lại — bảng chỉ
   còn 4 ký tự cuối. Chưa có key thì job TTS dừng kèm câu nhắc khai key.
   Admin thấy key của mọi người và gán được 1 key cho nhiều tài khoản cùng lúc
   (ô **Người dùng** trong form thêm key).
2. Vào **F1 — Theo yêu cầu** (hoặc bấm **+ Tạo Voice** ở màn **Voice** — cùng
   một form), chọn tab **Nhập text** rồi gõ/dán nội dung, hoặc để tab **Từ URL**
   nếu muốn hệ thống tự lấy nội dung.
3. `Hình thức thu thập` = **B** (đọc nguyên văn) hoặc **C** (LLM viết lại theo
   Prompt mẫu trước khi đọc — cần chọn prompt và có `ANTHROPIC_API_KEY`).
4. Bấm nút tạo — xong là màn hình nhảy sang **Voice**, dòng voice mới hiện ở
   trạng thái *Đang xử lý* rồi chuyển sang *Nháp* khi worker đọc xong.

Nhập bằng **text** thì KHÔNG sinh Bài Post: text gõ tay không có bài gốc nào để
truy vết nên Voice được tạo thẳng. Nhập bằng **URL** thì vẫn qua Bài Post như
luồng A.

Lưu ý: multime từ chối audio ngắn hơn 15 giây, nên text quá ngắn sẽ tạo được
voice nhưng không đăng được.

Gặp lỗi: xem [docs/troubleshooting.md](docs/troubleshooting.md).

---

## Deploy production

```bash
cp .env.prod.example .env && vi .env        # điền SITE_DOMAIN + các khoá bí mật
docker compose -f docker-compose.prod.yml up -d --build
```

Một tên miền (`SITE_DOMAIN`) phục vụ cả giao diện lẫn API qua Caddy, TLS tự
động. Chi tiết từng bước: [docs/deployment.md](docs/deployment.md).

## Tài liệu

- **Trạng thái dự án, rủi ro, việc tiếp theo: [docs/status.md](docs/status.md)**
- Kiến trúc, cấu trúc thư mục, business rules: [docs/architecture.md](docs/architecture.md)
- API + phân quyền: [docs/api.md](docs/api.md)
- Deploy, vận hành, storage, backup: [docs/deployment.md](docs/deployment.md)
- Xử lý lỗi hay gặp: [docs/troubleshooting.md](docs/troubleshooting.md)
- Việc còn cần chốt: [docs/open-questions.md](docs/open-questions.md)
