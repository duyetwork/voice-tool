# Kế hoạch deploy lên VPS (2 vCPU / 2GB RAM / 30GB)

> File tạm — chưa sửa dòng code nào. Chốt xong thì gộp phần còn dùng vào
> `docs/deployment.md` rồi xoá file này.

Máy đích: `103.121.89.215` (+ IPv6 `2403:6a40:0:89:2a2e:94ff:fe30:adc8`),
Ubuntu 24.04 x64, 2 vCPU / 2GB RAM / 30GB, đang đăng nhập bằng `root` + mật khẩu.

---

## 0. Ba quyết định phải chốt trước khi bắt đầu

| # | Câu hỏi | Đề xuất | Vì sao |
|---|---|---|---|
| 1 | **Tên miền** để chạy HTTPS | Một subdomain bạn sở hữu, ví dụ `voice.duyet.work`. Chưa có thì tạm dùng `103-121-89-215.sslip.io` (miễn phí, Let's Encrypt cấp cert được) | Không có tên miền thì không có HTTPS thật; mà không HTTPS thì token đăng nhập đi qua mạng ở dạng trần |
| 2 | **Lưu file voice ở đâu** | Dùng S3 sẵn có (`strongbody-files-api`, prefix `voice-tool/`) thay vì chạy MinIO trên VPS | Bỏ được ~250MB RAM + toàn bộ áp lực ghi đĩa. Đây là khoản tiết kiệm lớn nhất trên máy 2GB |
| 3 | **Build image ở đâu** | KHÔNG build trên VPS. Build ở máy bạn rồi đẩy image sang | `npm run build` của Next cần ~1.5–2GB RAM; trên máy 2GB nó sẽ bị OOM killer bắn giữa chừng, hoặc kéo cả Postgres chết theo |

### Về câu hỏi hostname "duyetwork"

Cần phân biệt 2 thứ khác nhau:

- **Hostname của máy** (`hostnamectl set-hostname ...`): chỉ là cái tên hiện
  trong shell và log. Đặt gì cũng chạy. Gợi ý: `voice-tool-prod` — đọc log 6
  tháng sau vẫn biết ngay máy này chạy gì. `duyetwork` cũng được, chỉ là không
  nói lên máy dùng làm gì.
- **Tên miền** (bản ghi DNS trỏ về `103.121.89.215`): đây mới là thứ quyết định
  người dùng gõ gì vào trình duyệt và Let's Encrypt cấp cert cho ai. Hostname
  của máy KHÔNG tự thành tên miền.

Nếu bạn sở hữu `duyet.work` thì tạo bản ghi:

```
A     voice.duyet.work    103.121.89.215    TTL 300
AAAA  voice.duyet.work    2403:6a40:0:89:2a2e:94ff:fe30:adc8    TTL 300
```

---

## 1. Hiện trạng repo: 5 file production được nhắc tới nhưng CHƯA TỒN TẠI

`docs/deployment.md` hướng dẫn chạy `docker-compose.prod.yml`, `Caddyfile`,
`.env.prod.example`, `scripts/deploy.sh`, `scripts/backup-db.sh` — nhưng không
file nào có thật (`scripts/` đang rỗng). Đây là việc phải làm đầu tiên, không
phải copy `docker-compose.yml` hiện tại lên vì bản đó là bản DEV:

| Bản dev đang có | Vấn đề khi lên production |
|---|---|
| `ports: 5432 / 6379 / 9000 / 9001` mở ra ngoài | Postgres, Redis, MinIO phơi thẳng ra Internet. Redis không mật khẩu = mất máy trong vài giờ |
| `adminer` + `asynqmon` | Không có xác thực. Adminer là cửa vào thẳng database |
| `worker replicas: 2`, `WORKER_CONCURRENCY=10` | 20 task song song trên 2 vCPU / 2GB → OOM |
| `build:` từ source | Build ngay trên VPS, xem mục 0.3 |
| Không có `mem_limit`, không giới hạn log | 1 container ăn hết RAM là chết cả máy; log không xoay vòng sẽ ăn dần 30GB |
| Không có reverse proxy / TLS | Chạy HTTP trần |

---

## 2. Ngân sách máy 2GB — con số cụ thể

### RAM

| Thành phần | Lúc rảnh | Đỉnh | `mem_limit` đề xuất |
|---|---|---|---|
| OS + docker daemon | 250MB | 350MB | — |
| Postgres (đã tune) | 120MB | 300MB | 384m |
| Redis | 30MB | 80MB | 128m |
| api | 30MB | 60MB | 192m |
| worker × **1** | 60MB | **600MB** (yt-dlp tải video + ffmpeg tách audio) | 768m |
| scheduler | 20MB | 30MB | 96m |
| frontend (Next standalone) | 90MB | 180MB | 256m |
| Caddy | 20MB | 40MB | 96m |
| *(MinIO nếu vẫn dùng)* | *150MB* | *300MB* | *384m* |

Tổng lúc rảnh ~620MB (có MinIO: ~770MB). Đỉnh khi worker chạy Mode A vượt
1.6GB → **bắt buộc có swap**, và `mem_limit` để một container phình không kéo
Postgres chết theo.

### Đĩa 30GB

| Mục | Ước tính |
|---|---|
| Ubuntu 24 | ~3GB |
| Docker images (7 image, worker nặng nhất vì có ffmpeg+python) | ~1GB |
| Postgres data | <200MB trong năm đầu (toàn dữ liệu chữ) |
| Redis AOF | <100MB |
| File voice chờ đăng | **biến động** — xem cảnh báo dưới |
| Log container (nếu cap 10MB×3×8 service) | ~250MB |
| Backup DB giữ 7 ngày | <500MB |
| **Còn trống** | ~24GB |

**Cảnh báo dung lượng:** 3voices trả về **WAV**, đo thực tế ~5,5MB mỗi phút
audio (voice 8 giây = 737KB). Business rule #2 xoá file ngay sau khi publish
thành công nên bình thường không tích luỹ — nhưng voice publish lỗi thì file
nằm lại. 100 voice × 3 phút chưa đăng = ~1,6GB.

→ Đề xuất (việc code, làm sau): **chuyển WAV sang MP3 128kbps bằng ffmpeg trước
khi upload** — worker đã có sẵn ffmpeg. Tiết kiệm ~5× dung lượng và cả băng
thông lúc đẩy lên multime.

### CPU

2 vCPU. Mode A (yt-dlp + ffmpeg) là thứ duy nhất ăn CPU thật. `WORKER_REPLICAS=1`
+ `WORKER_CONCURRENCY=2` là mức an toàn: 2 job nặng cùng lúc = mỗi job 1 vCPU.
Con số này cũng khớp giới hạn của 3voices (**10 request/phút, 2 job đồng thời**),
nên đặt cao hơn chỉ tổ ăn lỗi 429.

---

## 3. Kiến trúc triển khai

```
                        Internet
                     │ 80/443 (chỉ 2 cổng này mở)
                ┌────▼─────┐
                │  Caddy   │  voice.duyet.work — TLS tự động
                └──┬────┬──┘
        /api/*     │    │   còn lại
             ┌─────▼┐  ┌▼──────────┐
             │ api  │  │ frontend  │
             └──┬───┘  └───────────┘
                │   mạng nội bộ docker, KHÔNG mở cổng ra ngoài
     ┌──────────┼──────────┬──────────────┐
     ▼          ▼          ▼              ▼
  postgres    redis    worker ×1     scheduler ×1
                                          │
                              S3 (strongbody-files-api)
```

Frontend và API đứng chung 1 tên miền → build image frontend với
`NEXT_PUBLIC_API_URL=` (để trống) → bundle gọi `/api/v1` tương đối, không vướng
CORS, đổi tên miền không phải build lại.

---

## 4. Các bước thực hiện

### Giai đoạn A — Tạo file production trong repo (làm ở máy bạn)

| File | Nội dung chính |
|---|---|
| `docker-compose.prod.yml` | Không `build`, chỉ `image:`; bỏ adminer/asynqmon; postgres/redis **không** mở port ra host; thêm `mem_limit` + `logging` (json-file, max-size 10m, max-file 3) cho mọi service; `WORKER_REPLICAS=1`; thêm service `caddy` |
| `Caddyfile` | 1 site block: `/api/*` và `/healthz` → `api:8080`, còn lại → `frontend:3000`; bật `encode gzip` |
| `.env.prod.example` | Bản `.env.example` đã bỏ hết giá trị dev, ghi rõ biến nào bắt buộc đổi |
| `scripts/deploy.sh` | backup DB → pull image → `up -d` → chờ healthcheck → cảnh báo nếu scheduler ≠ 1 |
| `scripts/backup-db.sh` | `pg_dump` gzip vào `/var/backups/voice-tool`, giữ 7 bản, xoá bản cũ |
| `scripts/push-images.sh` *(nếu chọn cách "docker save")* | build 4 image ở máy local → `docker save … | ssh root@vps docker load` |
| `postgres.conf` (hoặc `command:` trong compose) | `shared_buffers=192MB`, `effective_cache_size=512MB`, `work_mem=4MB`, `maintenance_work_mem=64MB`, `max_connections=30` |

Cấu hình app cho máy nhỏ (trong `.env` production):

```
WORKER_REPLICAS=1
WORKER_CONCURRENCY=2          # dev đang 10
BREAKING_SCAN_PARALLELISM=2   # dev đang 4
DB_MAX_CONNS=8                # api + worker + scheduler cộng lại < max_connections=30
```

### Giai đoạn B — Gia cố VPS (khoảng 30 phút, làm 1 lần)

1. Đổi hostname: `hostnamectl set-hostname voice-tool-prod`
2. **Tạo swap 4GB** (bắt buộc):
   `fallocate -l 4G /swapfile && chmod 600 /swapfile && mkswap /swapfile && swapon /swapfile`,
   ghi vào `/etc/fstab`, đặt `vm.swappiness=10`.
3. Tạo user thường + `sudo`, copy SSH key (hiện *No keys added yet* — máy đang
   chỉ có mật khẩu cho root, đây là rủi ro lớn nhất lúc này).
4. Khoá SSH: `PermitRootLogin no`, `PasswordAuthentication no`, đổi cổng nếu muốn.
5. `ufw`: chỉ mở `22` (hoặc cổng SSH mới), `80`, `443`. Mặc định deny incoming.
   Nhớ mở cho **cả IPv6** — máy có IPv6 công khai.
6. `fail2ban` cho sshd.
7. `unattended-upgrades` cho bản vá bảo mật.
8. Timezone: `timedatectl set-timezone Asia/Ho_Chi_Minh`.
9. Cài Docker Engine + compose plugin (bản chính chủ, không dùng snap).
10. `/etc/docker/daemon.json`: đặt log driver mặc định `json-file` với
    `max-size: 10m`, `max-file: 3` — chốt chặn cuối để log không ăn hết đĩa.

### Giai đoạn C — Cấu hình ứng dụng

1. `git clone` repo vào `/opt/voice-tool` (chỉ để lấy compose + migrations +
   scripts; **không build ở đây**).
2. `cp .env.prod.example .env`, điền:
   - `POSTGRES_PASSWORD` — mật khẩu mạnh, sinh ngẫu nhiên
   - `JWT_SECRET` — `head -c 32 /dev/urandom | xxd -p -c 64`
   - `TOKEN_ENCRYPTION_KEY` — `head -c 32 /dev/urandom | base64` (giống nhau
     giữa api/worker/scheduler, đổi là mất toàn bộ token multime + API key TTS
     đã mã hoá)
   - `SITE_DOMAIN`, `BOOTSTRAP_ADMIN_EMAIL`
   - `S3_*` (hoặc giữ MinIO nếu chưa có S3)
   - `ENABLED_COLLECT_MODES=A,B,C`
   - `TTS_PROVIDER=3voices` (key TTS thật khai theo từng user trên UI, không đặt ở đây)
   - `LLM_PROVIDER` + `ANTHROPIC_API_KEY` **nếu** cần hình thức C — thiếu thì hệ
     thống tự tắt mode C kèm lý do, không chạy sai
3. `chmod 600 .env`.

### Giai đoạn D — Đưa image lên và chạy

Cách 1 (không cần registry, hợp lúc mới bắt đầu):

```bash
# máy bạn
docker compose -f docker-compose.prod.yml build
docker save voice-tool-api voice-tool-worker voice-tool-scheduler voice-tool-frontend \
  | gzip | ssh deploy@103.121.89.215 'gunzip | docker load'
```

Cách 2 (nên chuyển sang khi deploy thường xuyên): GitHub Actions build → đẩy lên
GHCR → VPS `docker compose pull`. Repo đã có `buildspec.yml` của CodeBuild nếu
muốn đi đường AWS ECR.

Rồi trên VPS:

```bash
cd /opt/voice-tool
docker compose -f docker-compose.prod.yml up -d      # migrate chạy tự động trước api/worker
docker compose -f docker-compose.prod.yml ps
curl -sf https://voice.duyet.work/healthz
```

### Giai đoạn E — Kiểm tra sau deploy

- [ ] `/healthz` trả 200 qua HTTPS, cert hợp lệ
- [ ] Đăng nhập bằng tài khoản strongbody, `/me` trả đúng role admin
- [ ] Thêm API key 3voices ở mục AI Engine → tạo 1 voice từ text → ra file, nghe thử được
- [ ] Tạo 1 voice Mode A từ 1 URL YouTube → kiểm tra yt-dlp không bị chặn (xem
      rủi ro #2 ở mục 5)
- [ ] Đăng thử 1 voice lên multime → file bị xoá khỏi storage sau khi đăng
- [ ] `docker stats` lúc đang chạy voice: RAM tổng < 1.6GB, swap dùng < 1GB
- [ ] `df -h` sau 1 ngày: dung lượng không tăng bất thường

### Giai đoạn F — Vận hành định kỳ

| Việc | Tần suất | Cách |
|---|---|---|
| Backup Postgres | Hằng ngày 03:00 | cron gọi `scripts/backup-db.sh`; **thêm bản sao ra ngoài máy** (rclone/S3) — backup nằm cùng máy không cứu được khi mất VPS |
| Rebuild image worker | 1–2 tháng | YouTube đổi cơ chế liên tục, `yt-dlp` cũ sẽ hỏng Mode A |
| Xem queue / DB | Khi cần | SSH tunnel, KHÔNG mở adminer/asynqmon ra Internet |
| Theo dõi đĩa + RAM | Hằng tuần | `df -h`, `free -h`, `docker stats` |
| Cập nhật OS | Tự động | unattended-upgrades |

---

## 5. Rủi ro đã biết trên cấu hình này

| # | Rủi ro | Mức | Giảm thiểu |
|---|---|---|---|
| 1 | **OOM khi Mode A gặp video dài** — ffmpeg + yt-dlp trên máy 2GB | Cao | Swap 4GB + `mem_limit` worker 768m + concurrency 2. Nếu vẫn chết: hạ concurrency về 1 |
| 2 | **YouTube chặn IP datacenter** — Mode A fail với "Sign in to confirm you're not a bot" dù local vẫn chạy | Cao | Đây là hạn chế của IP VPS, không phải lỗi code. Cần cấu hình cookies cho yt-dlp hoặc proxy dân dụng |
| 3 | Đĩa đầy vì file WAV chưa publish | Trung bình | Cảnh báo ở 80%; chuyển WAV→MP3 (việc code); dùng S3 thay MinIO |
| 4 | 3voices rate limit 10 req/phút | Trung bình | Concurrency 2; hệ thống đã tự retry với backoff khi gặp 429 |
| 5 | **1 máy = 1 điểm chết duy nhất**, không HA | Chấp nhận | Backup ra ngoài máy để dựng lại được trong ~30 phút |
| 6 | Mất `TOKEN_ENCRYPTION_KEY` = mất toàn bộ token multime + API key TTS đã lưu | Cao | Lưu key vào password manager NGAY khi sinh ra |
| 7 | RAM 2GB không đủ nếu sau này bật cả 3 hình thức chạy liên tục nhiều kênh | Trung bình | Nâng lên 4GB là bước tiếp theo rẻ nhất, chưa cần đổi kiến trúc |

---

## 6. Ước lượng công sức

| Việc | Thời gian |
|---|---|
| A. Viết 5–6 file production trong repo | 2–3 giờ |
| B. Gia cố VPS | 30 phút |
| C. Cấu hình `.env` + secrets | 20 phút |
| D. Build + đẩy image + chạy | 30–45 phút (lần đầu) |
| E. Kiểm tra sau deploy | 30 phút |
| F. Backup + cron + cảnh báo | 30 phút |
| **Tổng** | **~5 giờ** cho lần đầu; các lần sau `./scripts/deploy.sh` là xong |

---

## 7. Cần bạn trả lời để bắt đầu Giai đoạn A

1. **Tên miền** dùng cho hệ thống này là gì? (hay tạm dùng `sslip.io`?)
2. **S3**: dùng lại `strongbody-files-api` được không — nếu được, cho xin
   access key/secret + region, hay vẫn chạy MinIO trên VPS?
3. **Cách đẩy image**: `docker save | ssh` (đơn giản, làm được ngay) hay dựng
   GitHub Actions + GHCR (chuẩn hơn cho lâu dài)?
4. Có dùng **hình thức C** không? Nếu có thì cần `ANTHROPIC_API_KEY`.
5. Có sẵn tài khoản để nhận **cảnh báo** (email/Telegram) khi service chết hoặc
   đĩa gần đầy không?
