# Deploy — đề xuất

Tài liệu này dựa trên hạ tầng thực tế của `strongbody-api` và `strongbody-web`
để voice-tool nằm cùng một hệ, dùng chung tooling và không phát sinh kiến thức
vận hành mới.

## Hạ tầng hiện có (đọc từ 2 repo tham chiếu)

| Thành phần | strongbody-api | strongbody-web |
|---|---|---|
| Region | `ap-southeast-1` | — |
| CI build | AWS CodeBuild + `buildspec.yml`, image tag theo branch | — |
| Registry | Amazon ECR | — |
| Runtime | **AWS App Runner** (`strongbody-api`, `strongbody-api-test`) | **AWS Amplify** (`next.config.js` nhắc giới hạn 220MB của Amplify) |
| Database | RDS PostgreSQL | — |
| Cache/Queue | Redis (`REDIS_HOST`/`REDIS_PORT`), AWS SQS cho messaging | — |
| Storage | S3 bucket `strongbody-files-api` (`ap-southeast-1`) | S3 |
| Secrets | biến môi trường trong `source-configuration` của App Runner | — |
| Branch | `live-v1` (prod), `dev` (test) | — |

## Vấn đề: App Runner không chạy được worker

App Runner **chỉ chạy HTTP service** — nó health-check bằng cổng HTTP và có thể
scale về 0. Worker và scheduler của voice-tool là process nền, không lắng nghe
HTTP, nên không deploy được lên App Runner. Đây là điểm khác biệt duy nhất so
với cách `strongbody-api` đang chạy.

Vì vậy có 2 phương án.

---

## Phương án A (đang dùng) — 1 máy chủ + Docker Compose + Caddy

Toàn bộ hệ thống trên 1 máy, dùng `docker-compose.prod.yml` và `Caddyfile` có
sẵn trong repo. Một tên miền phục vụ cả giao diện lẫn API:

```
                    Internet
                       │  443 (TLS tự động)
                 ┌─────▼─────┐
                 │   Caddy   │   voice.multime.ai
                 └──┬─────┬──┘
        /api/*      │     │      còn lại
              ┌─────▼─┐ ┌─▼────────┐
              │  api  │ │ frontend │
              └───┬───┘ └──────────┘
                  │  (mạng nội bộ, không mở cổng ra ngoài)
     ┌────────┬───┴────┬──────────┬─────────────┐
     ▼        ▼        ▼          ▼             ▼
  postgres  redis    minio    worker ×2    scheduler ×1
```

**Vì sao đứng chung 1 tên miền:** không phải cấu hình CORS, chỉ cần 1 chứng chỉ
TLS, và frontend gọi API bằng đường dẫn tương đối nên đổi tên miền không phải
build lại image.

### Yêu cầu

| Cần | Ghi chú |
|---|---|
| 1 máy Linux, 2 vCPU / 2GB trở lên | RAM lúc rảnh chỉ ~300MB; 2 vCPU là để chạy song song 2 voice (yt-dlp + ffmpeg ăn CPU) |
| Docker + Docker Compose | |
| Cổng 80 và 443 mở ra Internet | Caddy cần cổng 80 để xin chứng chỉ Let's Encrypt |
| 1 bản ghi DNS A | Ví dụ `voice.multime.ai` → IP máy chủ |

### Các bước

**1. Trỏ tên miền.** `multime.ai` đang dùng Route 53, nên thêm bản ghi trong
hosted zone của nó:

```
Name: voice        Type: A        Value: <IP máy chủ>        TTL: 300
```

Thêm subdomain không ảnh hưởng gì tới `multime.ai` gốc (đang trỏ CloudFront) và
cũng không ảnh hưởng bản ghi MX/email.

**2. Trên máy chủ:**

```bash
git clone <repo> /opt/voice-tool && cd /opt/voice-tool
cp .env.prod.example .env
vi .env          # điền SITE_DOMAIN, POSTGRES_PASSWORD, JWT_SECRET,
                 # TOKEN_ENCRYPTION_KEY, BOOTSTRAP_ADMIN_EMAIL, S3_*
```

Sinh 2 khoá bí mật (đừng dùng lại khoá trong `.env.example` — nó công khai):

```bash
docker run --rm alpine sh -c "head -c 32 /dev/urandom | base64"   # TOKEN_ENCRYPTION_KEY
docker run --rm alpine sh -c "head -c 32 /dev/urandom | xxd -p -c 64"  # JWT_SECRET
```

**3. Khởi động:**

```bash
docker compose -f docker-compose.prod.yml up -d --build
docker compose -f docker-compose.prod.yml logs -f caddy   # xem quá trình xin chứng chỉ
curl -sf https://voice.multime.ai/healthz
```

**4. Backup hằng ngày** (Postgres chứa toàn bộ dữ liệu nghiệp vụ):

```bash
crontab -e
0 3 * * * cd /opt/voice-tool && ./scripts/backup-db.sh >> /var/log/voice-tool-backup.log 2>&1
```

### Deploy phiên bản mới

```bash
./scripts/deploy.sh
```

Script tự backup DB trước, `git pull`, build lại, chạy migration qua service
`migrate`, rồi cảnh báo nếu số scheduler khác 1.

### Xem database / hàng đợi trên production

Bản prod **không chạy** `adminer` và `asynqmon` — cả hai không có xác thực
riêng. Cần xem thì mở SSH tunnel từ máy bạn:

```bash
# Postgres
ssh -L 5432:localhost:5432 <user>@<ip>
docker compose -f docker-compose.prod.yml exec postgres psql -U voice -d voice_tool
```

### Bảo trì định kỳ

**Rebuild image worker mỗi 1-2 tháng.** `yt-dlp` được cài lúc build; YouTube đổi
cơ chế thường xuyên và bản cũ sẽ hỏng Mode A với lỗi kiểu "Sign in to confirm
you're not a bot".

```bash
docker compose -f docker-compose.prod.yml build --no-cache worker
docker compose -f docker-compose.prod.yml up -d worker
```

**Lưu ý về IP datacenter:** YouTube chặn `yt-dlp` từ IP trung tâm dữ liệu mạnh
hơn nhiều so với IP nhà/văn phòng. Nếu Mode A hay fail trên VPS trong khi chạy
ở máy local vẫn tốt, đây là nguyên nhân — cần cấu hình cookies hoặc proxy cho
`yt-dlp`, không phải lỗi hệ thống.

---

## Phương án B — ECS Fargate (khi worker cần scale vượt 1 máy)

Giữ nguyên CodeBuild + ECR, đổi runtime:

| Service | Runtime | Cấu hình |
|---|---|---|
| `voice-tool-api` | App Runner | 1 instance (0.5 vCPU / 1GB), auto-scale theo request — giống `strongbody-api` |
| `voice-tool-worker` | ECS Fargate | `desiredCount: 2`, 1 vCPU / 2GB mỗi task |
| `voice-tool-scheduler` | ECS Fargate | `desiredCount: 1`, 0.25 vCPU / 0.5GB |

**Bắt buộc với service scheduler:** đặt
`deploymentConfiguration: { maximumPercent: 100, minimumHealthyPercent: 0 }`.
Mặc định ECS chạy task mới trước khi tắt task cũ, tức là sẽ có 2 scheduler cùng
lúc trong lúc deploy → task theo lịch bị nhân đôi. Cấu hình trên buộc ECS tắt
task cũ trước.

Worker thì ngược lại: mặc định (200%/100%) là đúng, vì worker chạy song song
được và mọi task đều idempotent.

---

## Cấu hình worker — vì sao 1 API / 2 worker / 1 scheduler

Đúng như đề xuất trong `prompt.md`, và code đã tách thành 3 binary để làm được:

| Process | Số lượng | Lý do |
|---|---|---|
| `api` | 1 (auto-scale) | Chỉ CRUD + enqueue, rất nhẹ. Scale theo request nếu cần. |
| `worker` | **2** | Task nặng nhất là Mode A (`yt-dlp` tải video + `ffmpeg` tách audio) — nghẽn ở I/O và CPU. 2 process trên máy 2 vCPU giữ được 1 vCPU cho mỗi process khi cả hai đang tách audio. Mọi task idempotent (`ClaimSourcePostForProcessing`, `asynq.Unique`) nên scale ngang an toàn. |
| `scheduler` | **1 — bắt buộc** | `asynq.PeriodicTaskManager` tự enqueue theo cronspec của chính nó. 2 process = mỗi kênh bị quét 2 lần, tốn gấp đôi quota nền tảng và chi phí AI. |

`WORKER_CONCURRENCY=10` là số task **song song trong 1 process**. Với 2 process
là 20 task đồng thời — cao hơn nhiều so với nhu cầu ban đầu, nhưng phần lớn
task chỉ chờ I/O. Nếu thấy CPU đầy, giảm xuống 4-6 thay vì giảm số process.

**Lưu ý rate limit:** 3voices.win giới hạn 10 request/phút và 2 job đồng thời
cho mỗi user. Nếu bật TTS thật (Mode B/C), giảm `WORKER_CONCURRENCY` xuống 2
hoặc thêm rate limiter — hiện chưa có, và đây là lý do nữa để giai đoạn này tập
trung Mode A (không gọi TTS).

---

## Tham số quét

Mặc định là chỉ số tối ưu, chỉnh được theo từng kênh trên UI:

| Biến | Mặc định | Ý nghĩa |
|---|---|---|
| `BREAKING_SCAN_INTERVAL` | `60s` | Khoảng nghỉ giữa 2 vòng quét Breaking |
| `SCAN_LIMIT_DEFAULT` | `20` | Số bài lấy về mỗi vòng |
| `MAX_POSTS_PER_RUN_DEFAULT` | `50` | Trần Bài Post tạo ra mỗi vòng (chặn nổ chi phí AI) |
| `BREAKING_SCAN_PARALLELISM` | `4` | Số kênh quét song song |
| `SCHEDULER_SYNC_INTERVAL` | `30s` | Chu kỳ đọc lại lịch từ DB |
| `SKIPPED_LOG_RETENTION` | `168h` | Giữ log bài bị bỏ qua 7 ngày |

`TOKEN_ENCRYPTION_KEY` phải **giống nhau** giữa `api`, `worker` và `scheduler`:
api ghi token lúc đăng nhập, worker đọc lúc publish.

---

## Frontend — địa chỉ API là biến BUILD-TIME

`NEXT_PUBLIC_API_URL` được Next.js thay thẳng vào bundle JS **lúc build**, nên
đặt nó trong `environment` của container lúc chạy là vô tác dụng — đây là lỗi dễ
mắc và chỉ lộ ra khi deploy thật (trình duyệt người dùng gọi `localhost:8080`
của chính máy họ).

Hai cách dùng:

| Cách deploy | Giá trị | Kết quả |
|---|---|---|
| Frontend + API sau **cùng 1 tên miền** (reverse proxy) | để **trống** | Bundle gọi đường dẫn tương đối `/api/v1`; 1 image chạy được mọi môi trường, không cần biết trước domain, không vướng CORS |
| Frontend và API **khác tên miền/cổng** | URL tuyệt đối | Ví dụ `https://api.example.com/api/v1`. Đổi domain là phải build lại image |

```bash
docker build --build-arg NEXT_PUBLIC_API_URL= -t voice-tool-frontend ./frontend
```

Với reverse proxy, route cần có: `/api/*` → `api:8080`, còn lại → `frontend:3000`.

## Storage — lưu file voice ở đâu

**Đề xuất: dùng chung bucket `strongbody-files-api` với prefix riêng.**

```
s3://strongbody-files-api/voice-tool/voices/<source_post_id>/<post_id>-<ts>.mp3
```

Lý do:

- Credential và IAM policy đã có, không phải tạo bucket + key mới.
- File voice trong hệ thống này là **tạm thời**: theo business rule #2, publish
  thành công là xoá ngay, chỉ giữ `multime_post_url`. Dung lượng thực tế luôn ở
  mức "số voice đang chờ duyệt", không tăng theo thời gian.
- multime.ai cũng lưu trên S3 (`migrate-audio-asset-storage-key-urls` trong
  `strongbody-api` cho thấy họ đã chuyển từ GCS sang S3).

Cấu hình:

```bash
S3_BUCKET=strongbody-files-api
S3_REGION=ap-southeast-1
S3_ENDPOINT=https://s3.ap-southeast-1.amazonaws.com
S3_USE_PATH_STYLE=false        # AWS S3 thật dùng virtual-host style
```

Thêm **lifecycle rule** cho prefix `voice-tool/`: xoá object sau 30 ngày. Đây
là lưới an toàn cho file mồ côi (publish lỗi liên tục, hoặc record bị xoá bằng
SQL trực tiếp) — luồng bình thường đã tự xoá file rồi.

Dùng bucket riêng (`voice-tool-media`) chỉ nên chọn nếu cần tách hẳn quyền truy
cập giữa 2 hệ thống.

## Backup

| Dữ liệu | Cách backup | Lý do |
|---|---|---|
| PostgreSQL | RDS automated backup, retention **7 ngày** + PITR | Chứa toàn bộ cấu hình kênh, Bài Post, Voice metadata, `audit_log` (append-only) và token multime đã mã hoá |
| Redis | **Không cần** | Chỉ chứa hàng đợi task. Mất Redis = mất các job đang chờ; `breaking:dispatch` tự mồi lại khi scheduler khởi động, `scheduled:scan` tự lên lịch lại từ DB. Bài Post ở trạng thái `new` vẫn chạy lại được bằng nút "Chạy tạo Voice". |
| S3 | **Không cần versioning** | File voice là tạm thời và sẽ bị xoá theo design. Bản chính thức nằm trên multime.ai. |

Trước mỗi lần deploy có migration: `aws rds create-db-snapshot` thủ công.

## Dọn dẹp dữ liệu

| Bảng | Chính sách | Cấu hình |
|---|---|---|
| `skipped_log` | Xoá bản ghi > **7 ngày**, job `maintenance:cleanup` chạy 03:15 hằng ngày | `SKIPPED_LOG_RETENTION=168h` |
| `audit_log` | **Không xoá** — append-only theo specs 1.6 | — |
| `source_post` / `voice` | Không tự xoá; user xoá thủ công trên UI | — |

Vì sao `skipped_log` phải dọn: mỗi vòng quét ghi tối đa `scan_limit` bản ghi cho
**mỗi kênh**. Với 20 kênh, `scan_limit=20`, chu kỳ 60s → tối đa ~576k bản
ghi/ngày. Đây là dữ liệu debug regex (trả lời câu hỏi "kênh có bài mà sao hệ
thống không bắt?"), không phải audit trail, nên 7 ngày là đủ.

## CI/CD

`buildspec.yml` ở gốc repo dùng đúng khuôn của `strongbody-api`: build image,
tag theo branch, push ECR, xuất `imageDetail.json`. Khác biệt: repo này build
**3 target** (`api`, `worker`, `scheduler`) từ cùng 1 Dockerfile.

```bash
aws codebuild start-build \
  --project-name voice-tool-build \
  --region ap-southeast-1 \
  --source-version main
```

## Secrets

Không đặt secret trong `buildspec.yml`. Dùng SSM Parameter Store (SecureString)
hoặc Secrets Manager, inject vào App Runner qua `source-configuration` /
vào ECS task definition qua `secrets`:

```
/voice-tool/prod/JWT_SECRET
/voice-tool/prod/TOKEN_ENCRYPTION_KEY
/voice-tool/prod/DATABASE_URL
/voice-tool/prod/THREEVOICES_API_KEY
/voice-tool/prod/ANTHROPIC_API_KEY
```

Không còn credential của multime trong secret: mỗi user tự đăng nhập, token của
họ nằm trong DB (đã mã hoá bằng `TOKEN_ENCRYPTION_KEY`).

**`TOKEN_ENCRYPTION_KEY` phải giống nhau giữa `api`, `worker` và `scheduler`** —
api ghi token lúc đăng nhập, worker đọc lúc publish. Khác khoá là publish fail
toàn bộ.

## Checklist trước khi lên production

- [ ] Đổi `JWT_SECRET` (mặc định trong `.env.example` là placeholder).
- [ ] Đặt `SITE_DOMAIN` và trỏ bản ghi DNS A về IP máy chủ **trước khi** khởi
      động — Caddy cần DNS đúng mới xin được chứng chỉ.
- [ ] Build frontend với `NEXT_PUBLIC_API_URL` **để trống** (bản
      `docker-compose.prod.yml` đã đặt sẵn).
- [ ] `APP_ENV=production` (bật Gin release mode + log JSON).
- [ ] `S3_USE_PATH_STYLE=false` khi dùng AWS S3 thật.
- [ ] Kiểm tra `/healthz` của api.
- [ ] Xác nhận **chỉ 1** container/task scheduler đang chạy.
- [ ] Mở `asynqmon` (hoặc port-forward) để theo dõi queue lần đầu chạy.
- [ ] `make gen-key` để sinh `TOKEN_ENCRYPTION_KEY` riêng (khoá trong
      `.env.example` là khoá dev, công khai trong repo).
- [ ] Đặt `BOOTSTRAP_ADMIN_EMAIL` = email admin đầu tiên.
- [ ] Tài khoản multime dùng cho voice-tool phải **tắt 2FA** (chưa hỗ trợ TOTP).
- [ ] Đặt cron `./scripts/backup-db.sh` chạy hằng ngày.
- [ ] Lịch rebuild image worker mỗi 1-2 tháng để cập nhật `yt-dlp`.
- [ ] **Không publish `adminer` (8085), `asynqmon` (8081) và MinIO console (9001)
      ra Internet.** Cả ba đều không có xác thực riêng (Adminer thì có form
      login nhưng dùng thẳng credential DB). Trên production: bỏ 3 service này
      khỏi compose, hoặc chỉ truy cập qua SSH tunnel
      (`ssh -L 8085:localhost:8085 <host>`).
- [ ] Đặt `MULTIME_DEFAULT_HASHTAGS` — multime yêu cầu mỗi bài đăng có ít nhất
      1 hashtag, nếu không auto-publish sẽ fail với voice chưa gắn hashtag.
