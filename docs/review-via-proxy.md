# Hạ tầng via/proxy — đã làm gì, còn gì phải làm

> Kèm theo `prompt.txt` "Hạ tầng Via/Proxy quét kênh Facebook / X / Instagram".
> Hợp đồng API ở [api.md](api.md#via--proxy-quét-facebook--x--instagram).
>
> **Vì sao đi đường này:** kênh nguồn là trang công khai của người khác nên
> không có page access token để dùng Graph API, và quy mô vài trăm trang khiến
> mọi dịch vụ tính tiền theo lượt đọc (X API v2, Apify) không kiểm soát được
> chi phí. Cái giá của đường đã chọn nằm ngay dưới đây.

---

## Đã xong và đã kiểm

### 1. Schema (migration `000031_scrape_via_proxy`)

`scrape_via`, `scrape_proxy`, `via_usage_log`.

Hai điểm lệch với bản mô tả trong prompt, đều có lý do:

**`cookies_secret_ref` → `cookies_encrypted`.** `pkg/secret` là một hộp mã hoá
AES-256-GCM, **không phải kho khoá-giá trị có địa chỉ để trỏ tới**. Cách đang
dùng trong repo là để ciphertext thẳng trong cột `*_encrypted`
(`ai_engine.api_key_encrypted`, `llm_api_key.api_key_encrypted`). Một cột "ref"
ở đây sẽ trỏ vào hư không.

**`via_usage_log` vẫn là bảng riêng.** Đã kiểm `fetch_error_stat` trước, đúng
như prompt yêu cầu: bảng đó gộp theo `(ngày, nền tảng, loại lỗi)` và **không có
cột nào trỏ tới via hay kênh** — nó được dựng để trả lời "có đáng mua proxy
không", một câu hỏi ở mức nền tảng. Nó không quy được lỗi về một via cụ thể, mà
đó lại là toàn bộ lý do tồn tại của bảng mới. Hai bảng không trùng nhau:
`fetch_error_stat` là số đếm gộp giữ mãi; `via_usage_log` là từng lượt, giữ 30
ngày.

Thêm một cột không có trong bản mô tả: **`daily_used_date`**. Bộ đếm hạn mức nhờ
nó mà **tự liền theo ngày** — lượt dùng đầu tiên của ngày mới thấy ngày lệch thì
tự đặt lại. Không có nó thì một lần cron lỡ nhịp sẽ khoá toàn bộ via ở "hết hạn
mức" cho tới khi có người để ý, mà đó đúng là lúc không ai để ý.

### 2. Máy trạng thái — đã kiểm trên Postgres thật

Toàn bộ logic chuyển trạng thái nằm trong **chính câu UPDATE**, không phải trong
Go. Lý do: hai worker cùng gặp lỗi trên một via sẽ chạy hai lượt, và kiểu
đọc-tính-rồi-ghi thì lượt sau đè lượt trước — số đếm đứng yên và via không bao
giờ chết.

Vì logic ở trong SQL nên nó được kiểm bằng SQL, trên Postgres thật (8 bước cho
via, 4 cho proxy):

```
via:   lỗi 1,2 -> vẫn active · lỗi 3 -> cooldown · bộ chọn bỏ qua via cooldown
       hết giờ -> active (GIỮ dấu cooldown_until) · lỗi login tiếp -> dead
       quét thành công -> xoá dấu, trừ hạn mức
       hết hạn mức -> không claim được · bộ đếm hôm qua KHÔNG chặn hôm nay
proxy: chặn 1,2 -> active · chặn 3 -> degraded (ra khỏi vòng chọn)
       vẫn chặn -> dead · KHÔNG có đường tự hồi sinh
```

Một điểm em sửa lại cho khớp prompt §2.2 sau khi kiểm: ban đầu via hồi sinh xong
phải đủ N lỗi nữa mới chết. Prompt nói "vẫn lỗi **ngay sau khi** hết cooldown →
dead", nên giờ chỉ cần **một** lỗi là chết — N lần đầu đã là bằng chứng, vòng
nghỉ là cơ hội, lần này là câu trả lời.

### 3. Chọn via/proxy

`ClaimScrapeVia` **chọn và đánh dấu trong MỘT câu** `UPDATE ... WHERE id = (SELECT
... FOR UPDATE SKIP LOCKED)`, không phải SELECT rồi UPDATE. Hai worker song song
mà đọc rồi mới ghi thì cả hai cùng thấy một via "nghỉ lâu nhất" và cùng dùng nó —
vòng xoay đứng im, một via gánh hết tải.

- Ưu tiên via `last_used_at` cũ nhất (round-robin theo thời gian nghỉ).
- Hết via → `domain.ErrNoViaAvailable`, **không phải sự cố**: vòng quét bỏ qua
  kênh đó và thử lại lượt sau.
- Hết proxy → **vẫn chạy**, đi thẳng bằng IP máy chủ. Với một kênh lẻ thì đi
  thẳng vẫn hơn không quét được gì, và số lần bị chặn sau đó tự nói ra rằng cần
  thêm proxy.

### 4. Bảo trì tự động

Job `scrape:sweep` chạy **mỗi giờ** (phút thứ 7 — lệch khỏi đầu giờ nơi lịch
quét dồn vào): hồi sinh via hết cooldown, đặt lại bộ đếm ngày, dọn nhật ký.

Tách khỏi `maintenance:cleanup` (chạy ngày) vì nhịp khác hẳn: cooldown chỉ 6
tiếng, gộp vào job ngày thì via vào cooldown lúc 4h sáng phải chờ tới 3h15 hôm
sau — 23 tiếng thay vì 6.

### 5. Giao diện (Cài đặt)

Đúng yêu cầu "không chấp nhận quản lý qua file cấu hình hoặc chỉ qua DB":

- **Sức khoẻ đàn via** — số via sống/tổng theo từng nền tảng, banner đỏ khi tụt
  dưới 30%. Mẫu số **không tính via bị tắt tay**: tắt bớt vài via không phải dấu
  hiệu hệ thống đang hỏng.
- **Bảng via** — trạng thái (badge màu), hạn mức còn lại hôm nay, số lỗi liên
  tiếp, lần dùng cuối, thời điểm hết cooldown. Thêm/sửa/tắt-bật/xoá.
- **Bảng proxy** — endpoint đã che user/pass, loại, trạng thái, tỉ lệ lỗi hôm
  nay. Dòng `datacenter` tự cảnh báo ngay tại chỗ.
- **Biểu đồ lượt quét theo giờ** — phần đỏ là lượt hỏng. Trả lời đúng một câu:
  lịch quét có bị dồn cục không.

Bảng via/proxy tự làm mới mỗi 30 giây: trạng thái đổi do **worker**, không do
thao tác trên màn hình này.

### 6. Bảo mật — đã kiểm trên DB thật

```
cookies_encrypted  = C6QYIm/TiLoUrCyVnisnKLBSS4jO…   (không chứa "c_user")
endpoint_encrypted = a1trFiwbXJ6DSDN9eKeZDpjmNM4e…   (không chứa mật khẩu)
endpoint_masked    = http://gw.example.net:8000       (đã cắt user/pass)
```

- Cookies/endpoint đi **một chiều**: API không bao giờ trả về.
- Phần che được **server tự cắt**, không nhận từ client — để không có đường nào
  lộ user/pass qua chính cột đó. URL không parse được thì trả rỗng chứ không trả
  nguyên bản.
- Audit ghi việc thêm/sửa/xoá via-proxy, **không bao giờ ghi nội dung** cookie
  hay endpoint.
- API từ chối đặt tay `status=dead` (đã thử: trả 400).

### 7. Adapter Facebook

- `FacebookScrapeAdapter` **bọc** adapter yt-dlp sẵn có và chỉ thay
  `FetchLatestPosts`. Bọc chứ không viết lại: hai adapter Facebook nghĩa là hai
  bộ regex nhận diện URL, và chúng sẽ lệch nhau ngay lần đầu Facebook đổi link.
- HTTP client riêng, gắn cookies của via + đi qua proxy được cấp, header của một
  trình duyệt thật, cache client theo endpoint proxy.
- **Phân loại lỗi** — phần quan trọng nhất, 10 case test. Cái bẫy chính: các
  trang này trả **HTTP 200 kèm trang đăng nhập** chứ không trả 401, nên chỉ nhìn
  mã trạng thái thì mọi via hỏng đều trông như thành công.
  Và dấu hiệu nghi-bot xét **trước** dấu hiệu đòi đăng nhập, vì trang checkpoint
  thường chứa cả hai — đọc nhầm nó là giết dần đàn via trong khi thứ hỏng thật
  là địa chỉ IP.

---

## 🔴 Chưa xong — phần chặn việc mở khoá Facebook

### Bộ phân tích trang chưa đối chiếu với dữ liệu thật

Đây là giới hạn thật, không phải việc còn dở dang có thể làm nốt bằng code.

Facebook nhúng nội dung trang trong các khối `<script type="application/json">`
của Relay. Cấu trúc khối đó **không có tài liệu, không có cam kết tương thích**,
và đổi theo từng bản triển khai. Bộ phân tích hiện bám vào các khoá ổn định nhất
quan sát được (`post_id`, `creation_time`, `message.text`), nhưng em **không có
cookies Facebook thật để đối chiếu**, nên không thể khẳng định nó chạy đúng trên
HTML thật.

Test hiện có chứng minh **hình dạng vào → ra** (thứ tự mới-nhất-trước, giải
escape unicode, tách hashtag, cắt theo limit), **không** chứng minh Facebook trả
về hình dạng đó.

Vì vậy `FACEBOOK_CHANNEL_SCAN=false` theo mặc định, và form Thêm kênh vẫn chặn
Facebook. Đúng như prompt §3 yêu cầu: *"Không gỡ `noChannelScan` trước khi
adapter đã test thật với dữ liệu thật"*.

**Việc cần anh làm để mở khoá:**

1. Thêm via thật ở **Cài đặt → Via**, dán cookies của một phiên Facebook đã đăng
   nhập (tài khoản dùng riêng cho việc này, không dính tới Business account).
2. Thêm ít nhất một proxy residential.
3. Đặt `FACEBOOK_CHANNEL_SCAN=true`, khởi động lại.
4. Thêm một kênh Facebook, bấm **Quét thử**, mở tab **Lịch sử quét**.
5. Số bài lấy về khớp thực tế → dùng được. Ra 0 bài hoặc sai nội dung → gửi em
   HTML thật của trang đó để chỉnh bộ phân tích.

Nếu bước 5 cho thấy cấu trúc đã đổi hẳn, phương án dự phòng là đọc qua endpoint
GraphQL nội bộ thay vì HTML — nhưng việc đó cũng cần đúng một mẫu request thật
mới làm được.

---

## Ba lỗ hổng phát hiện khi chạy thật, đã sửa

Sau khi có via + proxy thật, chạy lại toàn bộ đường quét thì lộ ra ba chỗ chưa
nối. Ghi lại vì cả ba đều thuộc loại "không gây lỗi biên dịch, không ai thấy":

**1. `FACEBOOK_CHANNEL_SCAN` chưa có trong `.env`.** Mặc định là `false` nên form
Thêm kênh vẫn chặn Facebook dù đã thêm via. Đây là lý do trực tiếp của thông báo
"Facebook không quét được cả trang…". Đã thêm `FACEBOOK_CHANNEL_SCAN=true`.

**2. `ErrNoViaAvailable` chỉ được TẠO ra, không ai bắt.** Lỗi này được sinh ở
`ScrapePool.Acquire` và mang sẵn ý nghĩa "bỏ qua, không phải sự cố" — nhưng
`service.Scan` xử nó như mọi lỗi khác: ghi `last_error` lên kênh, asynq retry ba
lần, vòng quét vào lịch sử với trạng thái lỗi.

Nghĩa là một via hết hạn mức ngày sẽ làm **mọi kênh Facebook đỏ lên** như thể
chúng hỏng. Trái hẳn yêu cầu §2.1. Giờ `scanBreaking`/`scanScheduled` nhận ra
lỗi này và kết thúc SẠCH: kênh không bị ghi lỗi, không retry, lượt sau thử lại.

**3. `SCRAPE_CONCURRENCY` và `SCRAPE_MIN_GAP` là cấu hình chết.** Khai trong
`config.Config`, có mặt trong `.env.example`, nhưng **không được đọc ở đâu cả**.
Người vận hành chỉnh chúng và không có gì xảy ra.

Giờ chúng dựng một `KeyGate` riêng cho Facebook/X/Instagram, tách khỏi gate của
yt-dlp. Tách là có lý do: một lần yt-dlp lấy video YouTube là request ẩn danh,
còn một lần tải trang Facebook mang theo cookies của tài khoản thật — nhịp của
cái sau phải chậm hơn, và nó không được ăn theo cấu hình của cái trước.

---

## 🟡 Chưa làm trong đợt này (có chủ ý)

### X và Instagram

Prompt chốt thứ tự **Facebook → X → Instagram**, và §7 nói "lặp lại bước 5 cho X,
rồi Instagram" *sau khi Facebook ổn định*. Toàn bộ hạ tầng (via, proxy, máy
trạng thái, HTTP client, phân loại lỗi, UI) đã dùng chung được — hai adapter còn
lại chỉ cần phần đọc trang của riêng chúng.

Làm trước khi Facebook chạy thật là viết ba bộ phân tích cùng lúc mà không bộ
nào được đối chiếu.

### Rải lịch quét trong ngày (§2.4)

**Chưa làm phần tự gán giờ quét.** Hệ thống đã có sẵn ba mảnh của việc này:

- `list_scheduled.fixed_times_min` — giờ chạy cố định theo múi giờ của kênh
  (migration 000017), scheduler đã dựng cron riêng cho từng mốc;
- `service.ScanJitter` — rải lệch **tất định theo id kênh**, nên mỗi kênh có một
  chỗ đứng cố định và chúng rải đều mãi mãi;
- `SCRAPE_CONCURRENCY` / `SCRAPE_MIN_GAP` — trần đồng thời và khoảng nghỉ theo
  nền tảng.

Việc còn thiếu là **tự gán `fixed_times_min` cho kênh mới** thay vì để người
dùng tự chọn. Em dừng lại ở đây vì nó đổi hành vi của cả hai loại kênh đang chạy
(kể cả YouTube/TikTok), mà phạm vi đợt này là via/proxy. Với vài trăm kênh thì
cần làm — nhưng nên làm thành một thay đổi riêng, có thể nhìn thấy tác động.

Trong lúc chưa có: đặt giờ cố định lệch nhau khi tạo kênh, và biểu đồ **Lượt quét
theo giờ** ở màn Cài đặt là thứ cho biết có bị dồn cục hay không.

---

## Trạng thái build

- `go build` / `go vet` / `gofmt` / `go test` — sạch. Thêm 2 tệp test: điều phối
  lỗi của bộ cấp phát (5 case), phân loại phản hồi HTTP (10 case).
- `tsc` / `eslint` / `next build` — sạch.
- Migration `000031` đã áp vào DB dev, `schema_migrations` = **31**, không dirty.
- Máy trạng thái via/proxy: kiểm bằng SQL trên Postgres thật, trên một database
  tạm rồi xoá.
- Đã gọi thật sau khi build: thêm/sửa/tắt/xoá via và proxy, `scrape-health`,
  `scrape-load`, và xác nhận cookies/endpoint là ciphertext trong DB. Dữ liệu
  thử đã xoá.
