# TÀI LIỆU ĐẶC TẢ YÊU CẦU (CẬP NHẬT)
## Voice Automation Tool — Hệ thống lấy/tạo Voice từ nội dung mạng xã hội

**Phiên bản:** 0.4 — cập nhật theo các quyết định đã chốt
**Ngày:** 07/09/2026

---

## -1. Cập nhật kiến trúc: Mô hình 3 tầng Danh sách → Bài Post → Voice

### Nguyên tắc
Ba thực thể **độc lập về CRUD** nhưng **phụ thuộc theo chuỗi**:

```
[Danh sách] --chạy--> tạo ra [Bài Post] --chạy--> tạo ra [Voice] --đăng--> multime.ai
   (List)                    (SourcePost)              (Voice)
```

| Thực thể | CRUD độc lập | Hành động "chạy" | Kết quả |
|---|---|---|---|
| **Danh sách** (Breaking/Định kỳ) | Thêm/Sửa/Xoá | "Chạy danh sách" — quét nguồn theo cấu hình (regex/tần suất) | Tạo ra 1 hoặc nhiều **Bài Post** mới (chưa có voice) |
| **Bài Post** | Thêm/Sửa/Xoá — có thể tạo **độc lập, không qua danh sách nào** (paste 1 URL tay) | "Chạy bài post" — thực thi Core Engine theo Hình thức thu thập đã chọn | Tạo ra 1 **Voice** |
| **Voice** | Thêm/Sửa/Xoá | "Đăng" — publish lên multime.ai | Voice ở trạng thái đã đăng, có URL trên multime.ai |

**Điểm mấu chốt bạn nêu:** dù tạo Voice bằng cách nào trong 3 hình thức (A/B/C), hệ thống **luôn lưu lại Bài Post từ URL trước**, rồi mới sinh Voice từ Bài Post đó — nghĩa là **không có đường tắt** đi thẳng từ URL sang Voice mà bỏ qua tầng Bài Post, kể cả ở F1 (nhập URL thủ công). Điều này giúp:
- Mọi Voice đều truy vết được về đúng 1 Bài Post nguồn duy nhất (audit rõ ràng).
- Cho phép **tạo lại Voice khác** từ cùng 1 Bài Post (đổi mode, đổi prompt) mà không cần fetch lại URL từ đầu.
- Cho phép **duyệt/lọc Bài Post trước khi tốn chi phí AI** tạo voice (đặc biệt hữu ích với Danh sách Định kỳ khi 1 lần quét ra nhiều bài).

### Cơ chế "tự động chạy tiếp" (auto-cascade) — đề xuất bổ sung

Nếu bắt buộc mọi thao tác đều phải qua 2 cú click riêng (chạy Danh sách → rồi vào từng Bài Post bấm chạy tiếp), sẽ **rất bất tiện** cho:
- **F1 (On-demand):** user muốn nhập URL và có voice ngay, không muốn thao tác 2 bước.
- **Danh sách Breaking (F2):** mục đích là tốc độ — không thể chờ user vào duyệt từng Bài Post rồi mới bấm chạy.

**Đề xuất:** thêm cờ cấu hình **`auto_process`** ở cả 2 tầng, mặc định khác nhau theo ngữ cảnh:

| Ngữ cảnh | `auto_process` (Post → Voice) | `auto_publish` (Voice → Đăng) |
|---|---|---|
| F1 — On-demand | Mặc định **bật** (tạo Post xong chạy luôn) — nhưng vẫn lưu Post như 1 bản ghi riêng, không phải "ảo" | Mặc định **tắt** — vẫn qua bước Preview trước khi đăng |
| Danh sách Breaking (F2) | Mặc định **bật** (ưu tiên tốc độ — quét ra bài khớp là chạy luôn sang Voice) | Cấu hình theo kênh (có thể bật auto-publish nếu tin tưởng nguồn, hoặc tắt để vào hàng đợi duyệt) |
| Danh sách Định kỳ (F3) | **Tuỳ chọn theo kênh** — có thể tắt để gom nhiều Bài Post rồi duyệt hàng loạt trước khi tạo Voice (tiết kiệm chi phí AI nếu có bài không cần) | Cấu hình theo kênh, tương tự Breaking |

Cách này giữ được đầy đủ tính linh hoạt của mô hình 3 tầng (ai muốn duyệt thủ công từng bước vẫn làm được) nhưng **không bắt buộc** user luôn phải thao tác thủ công — hệ thống vẫn "chạy xuyên suốt" theo mặc định hợp lý cho từng ngữ cảnh.

---

## 0. Các quyết định mới đã chốt (so với bản trước)

| # | Quyết định | Thay đổi so với bản trước |
|---|---|---|
| 1 | **Tách danh sách kênh F2 và F3 thành 2 danh sách độc lập** | Trước đây dùng chung 1 bảng Channel với flag `monitor_mode`. Nay: **Danh sách Breaking (F2)** và **Danh sách Định kỳ (F3)** là 2 thực thể riêng, không dùng chung bản ghi. |
| 2 | **Cú pháp nhận diện = Regex duy nhất** | Không còn khái niệm keyword/hashtag riêng — mọi điều kiện lọc đều được **quy về 1 Regex Pattern** (kể cả khi user chỉ gõ 1 từ khoá hay 1 hashtag, hệ thống tự chuyển thành regex tương đương). |
| 3 | **Danh sách Breaking (F2): không cấu hình tần suất quét** | Engine quét **liên tục/anytime**. Mỗi kênh trong danh sách này chỉ cần cấu hình: **Hình thức thu thập** (A/B/C) + **Regex Pattern**. |
| 4 | **Danh sách Định kỳ (F3): cấu hình tần suất theo từng kênh** | Mỗi kênh có: **Hình thức thu thập** + **Tần suất quét riêng** → cần 1 chức năng **Lập lịch (Scheduler)** quản lý các tần suất khác nhau giữa các kênh. |
| 5 | **Cả 2 danh sách đều hỗ trợ lọc theo Regex** | Không chỉ dùng regex để "bắt bài" (F2) — cả 2 màn danh sách kênh đều cho phép **search/filter bằng regex** trên tên kênh, nội dung, v.v. |
| 6 | **3 hình thức thu thập voice, đặt lại tên cho rõ nghĩa** | A → **Extract từ URL**; B → **Lấy text → AI gen → voice**; C → **Lấy text + Prompt → AI gen → voice**. |
| 7 | **Tự động phát hiện nền tảng & tự trích xuất ID bài đăng** | Bổ sung chức năng kỹ thuật: hệ thống tự nhận diện nền tảng từ URL và tự parse ID bài đăng cho mọi dạng (reel, post, short, video, story...), không cần user chọn tay nền tảng. |
| 8 | **Nền tảng hỗ trợ voice: tối đa hoá, mở rộng dần** | Facebook, TikTok, Instagram, X, YouTube, và có thể mở rộng thêm về sau (thiết kế theo kiểu adapter để dễ thêm nền tảng mới). |
| 9 | **Nơi đăng: multime.ai** | Xác nhận tên nền tảng đích chính thức. |
| 10 | **Ngôn ngữ có thể chọn ở nhiều cấp: Danh sách / Bài post / Voice** | Cần thiết kế theo cơ chế **override theo tầng** (xem mục 3.5). |
| 11 | **Danh sách Prompt mẫu** | Giữ nguyên như đã đề xuất — thư viện prompt dùng lại cho Mode C. |
| 12 | **AI TTS (Text-to-Voice) dùng các engine có sẵn** | Không tự xây dựng AI TTS — tích hợp với các nhà cung cấp TTS bên thứ 3 đã có sẵn trên thị trường, cần chức năng quản lý/lựa chọn engine. |
| 13 | **Cần lưu Nhật ký thao tác (Audit Log)** | Ghi lại: user nào, thao tác gì (CRUD), trên đối tượng nào (danh sách kênh, voice/post), thời gian nào. |
| 14 | **Phát triển theo giai đoạn** | Ưu tiên theo tính năng và theo nền tảng — xem lộ trình ở mục 6. |
| 15 | **Vòng đời file voice: chỉ lưu file khi chưa đăng** | Voice chưa đăng → lưu file voice thực tế trên storage nội bộ. Sau khi đăng thành công lên multime.ai → **xoá file voice**, chỉ giữ lại `multime_post_url` làm nguồn tham chiếu duy nhất. |

---

## 1. Kiến trúc cập nhật

### 1.1 Hai danh sách kênh tách biệt + mô hình 3 tầng

```
┌─────────────────────────┐        ┌─────────────────────────┐
│  Danh sách BREAKING (F2) │        │  Danh sách ĐỊNH KỲ (F3)  │
│  - Hình thức thu thập    │        │  - Hình thức thu thập    │
│  - Regex Pattern         │        │  - Tần suất quét         │
│  - (không có tần suất)   │        │  - Regex filter (search) │
│  - Regex filter (search) │        │                          │
└──────────┬───────────────┘        └──────────┬───────────────┘
           │ "Chạy danh sách"                   │ "Chạy danh sách" (theo Scheduler)
           ▼                                    ▼
                  ┌───────────────────────┐
                  │      BÀI POST          │ ◄── cũng có thể tạo tay, độc lập (F1)
                  │ (auto-detect nền tảng, │
                  │  parse ID, lưu text)   │
                  └──────────┬────────────┘
                             │ "Chạy bài post" (auto hoặc thủ công theo auto_process)
                             ▼
                  ┌───────────────────┐
                  │   Core Engine      │
                  │ - Extract/STT/TTS  │
                  │ - AI prompt process│
                  └─────────┬─────────┘
                             ▼
                  ┌───────────────────┐
                  │      VOICE         │
                  │ (metadata, ngôn    │
                  │  ngữ, hashtag...)  │
                  └─────────┬─────────┘
                             │ "Đăng" (auto hoặc thủ công theo auto_publish)
                             ▼
                     multime.ai
                             │
                             ▼
                  ┌───────────────────┐
                  │  Audit Log         │
                  │ (mọi CRUD + chạy)  │
                  └───────────────────┘
```

**Vì sao tách 2 danh sách thay vì dùng chung:** hai danh sách có bộ cấu hình khác nhau về bản chất (Breaking không có tần suất, Định kỳ bắt buộc có tần suất) — dùng chung 1 bảng sẽ có nhiều field NULL chéo nhau, dễ nhầm lẫn khi 1 kênh vừa thuộc cả hai. Tách riêng giúp UI, logic, và log rõ ràng hơn. Một kênh (cùng URL nguồn) vẫn có thể được thêm vào **cả 2 danh sách** như 2 bản ghi độc lập, mỗi bản ghi có cấu hình và lịch sử riêng.

**Vì sao thêm tầng Bài Post ở giữa:** xem mục -1 — Bài Post là điểm neo bắt buộc giữa URL nguồn và Voice, cho cả 3 luồng F1/F2/F3, giúp truy vết, tái sử dụng, và cho phép duyệt trước khi tạo voice khi cần.

### 1.2 3 hình thức thu thập voice (đặt lại tên)

| Mã | Tên | Cơ chế |
|---|---|---|
| **A** | Extract từ URL | Tải video/audio gốc, tách trực tiếp track giọng nói |
| **B** | Lấy text → AI gen → voice | Lấy caption/transcript gốc → đưa qua AI TTS đọc nguyên văn |
| **C** | Lấy text + Prompt → AI gen → voice | Lấy text gốc → xử lý qua AI theo Prompt mẫu đã chọn → đưa qua AI TTS |

### 1.3 Tự động phát hiện nền tảng & trích xuất ID bài đăng

**Mô tả:** Khi user (hoặc worker) nhận 1 URL, hệ thống tự động:
1. Nhận diện nền tảng nguồn (Facebook/TikTok/Instagram/X/YouTube...) dựa trên domain/pattern URL.
2. Nhận diện **loại nội dung** trên nền tảng đó (reel, post, short, video, story, tweet...) — vì mỗi loại có cấu trúc URL và cách gọi API/crawl khác nhau.
3. Trích xuất **ID bài đăng** tương ứng để dùng cho việc fetch nội dung.

**Thiết kế đề xuất:** dùng cơ chế **adapter theo nền tảng** — mỗi nền tảng có 1 bộ regex/pattern riêng để nhận diện loại URL và parse ID, dễ mở rộng khi thêm nền tảng mới hoặc nền tảng đổi cấu trúc URL.

**Business rule:**
- Nếu không nhận diện được nền tảng hoặc loại nội dung → báo lỗi rõ ràng (không đoán mò), yêu cầu user kiểm tra lại URL.
- Log lại loại nội dung đã nhận diện (post/reel/short/video...) kèm theo Job, phục vụ audit và debug khi nền tảng đổi cấu trúc URL.

### 1.4 Cơ chế Regex dùng chung

- Mọi input "cú pháp nhận diện" (dù user gõ dạng từ khoá đơn giản hay hashtag) đều được **chuẩn hoá về regex** trước khi lưu — ví dụ user gõ `#tinnong` → hệ thống lưu thành regex tương đương (vd `#tinnong\b`), để nhất quán khi engine so khớp.
- Regex dùng cho 2 mục đích riêng biệt, cần phân biệt rõ trong thiết kế:
  1. **Regex điều kiện bắt bài** (chỉ áp dụng cho Danh sách Breaking – F2) — quyết định có tạo job hay không.
  2. **Regex filter/search trên danh sách** (áp dụng cho cả 2 danh sách) — chỉ dùng để tìm kiếm/lọc hiển thị, không ảnh hưởng việc thu thập.
- Cần validate regex hợp lệ ngay khi nhập (chặn lưu nếu sai cú pháp, hoặc cảnh báo nếu regex có nguy cơ hiệu năng thấp — catastrophic backtracking).

### 1.5 Cơ chế Ngôn ngữ theo tầng (List → Post → Voice)

| Tầng | Vai trò |
|---|---|
| **Danh sách (List-level)** | Ngôn ngữ mặc định áp dụng cho mọi bài thu thập từ kênh này (F2/F3), nếu không có override ở tầng dưới. |
| **Bài post (Post-level)** | Có thể ghi đè ngôn ngữ riêng cho 1 bài cụ thể (áp dụng rõ nhất ở F1 — user tự chọn mỗi lần; ở F2/F3 chỉ ghi đè khi user vào sửa thủ công sau khi job đã tạo). |
| **Voice (Output-level)** | Ngôn ngữ thực tế dùng để chạy AI TTS (Mode B/C) — mặc định lấy theo Post-level, nhưng có thể khác nếu hệ thống hỗ trợ tự nhận diện ngôn ngữ nguồn (tuỳ chọn nâng cao). |

**Business rule:** thứ tự ưu tiên là **Post-level > List-level > mặc định hệ thống**; Mode A (Extract từ URL) không áp dụng ngôn ngữ vì không qua TTS, chỉ dùng ngôn ngữ để gắn nhãn phân loại bài đăng.

### 1.6 Nhật ký thao tác (Audit Log)

**Mô tả:** Ghi lại toàn bộ hành động **CRUD** (Create/Read*/Update/Delete — Read không bắt buộc log) do user thực hiện trên 2 nhóm đối tượng:
1. **Danh sách kênh** (cả Breaking và Định kỳ): thêm kênh, sửa cấu hình (mode, regex, tần suất, ngôn ngữ...), xoá kênh, bật/tắt theo dõi.
2. **Voice/Post**: tạo mới (qua F1/F2/F3), sửa metadata, sửa/tạo lại voice, xoá voice/post.

**Thông tin mỗi bản ghi log:**
| Trường | Mô tả |
|---|---|
| `user_id` | Ai thực hiện thao tác |
| `action` | create / update / delete |
| `object_type` | channel_breaking / channel_scheduled / voice_post |
| `object_id` | ID đối tượng bị tác động |
| `changes` | Trước/sau khi thay đổi (diff, cho update) |
| `timestamp` | Thời gian thao tác |

**Business rule:** log là **append-only** (không cho sửa/xoá bản ghi log), nhằm đảm bảo tính toàn vẹn khi cần truy vết.

---

## 2. Danh mục chức năng cập nhật

| Nhóm | Mã | Chức năng | Mô tả |
|---|---|---|---|
| **Quản lý danh mục** | A1 | Quản lý Prompt mẫu | CRUD thư viện prompt cho Mode C |
| | A2 | Quản lý AI Engine (TTS) | Danh sách các AI TTS có sẵn được tích hợp, chọn engine áp dụng theo kênh/job |
| **Danh sách Breaking (F2)** | B1 | Quản lý Danh sách Breaking | CRUD kênh: nguồn, Hình thức thu thập, Regex Pattern, ngôn ngữ default, `auto_process` |
| | B2 | Filter/Search theo Regex | Tìm kiếm trong danh sách bằng regex |
| | B3 | Chạy Danh sách (engine quét liên tục/anytime) | Worker chạy nền, đối chiếu regex ngay khi có bài mới → tạo **Bài Post** |
| | B4 | Log bài bị bỏ qua | Bài không khớp regex — lưu tối giản để debug |
| **Danh sách Định kỳ (F3)** | C1 | Quản lý Danh sách Định kỳ | CRUD kênh: nguồn, Hình thức thu thập, **tần suất quét**, ngôn ngữ default, `auto_process` |
| | C2 | Filter/Search theo Regex | Tương tự B2, áp dụng cho danh sách này |
| | C3 | Lập lịch (Scheduler) + Chạy Danh sách | Theo tần suất riêng từng kênh → tạo **Bài Post** cho các bài mới |
| | C4 | Chống lấy lặp | Dựa trên post_id đã xử lý gần nhất theo từng kênh |
| **Bài Post** | P1 | Quản lý Bài Post | Thêm (tay hoặc từ Danh sách) / Sửa / Xoá — sửa được Hình thức thu thập, Prompt, ngôn ngữ trước khi chạy |
| | P2 | Chạy Bài Post → tạo Voice | Thực thi Core Engine theo cấu hình của Bài Post |
| | P3 | Danh sách Bài Post (lọc theo nguồn/danh sách/trạng thái) | Xem các bài đã thu thập nhưng chưa/đã tạo voice |
| **Tạo Voice theo yêu cầu (F1)** | F1.1 | Nhập URL → auto-detect nền tảng/ID → tạo Bài Post | Không cần chọn tay nền tảng; luôn lưu Bài Post trước |
| | F1.2 | Chọn Hình thức thu thập (A/B/C) trên Bài Post | Kèm chọn Prompt mẫu nếu Mode C |
| | F1.3 | Nhập metadata & ngôn ngữ (post-level) | |
| | F1.4 | Chạy Bài Post & Preview trước khi đăng | Theo `auto_process` mặc định bật ở F1 |
| **Voice** | V1 | Quản lý Voice | Thêm (từ chạy Bài Post) / Sửa metadata / Tạo lại / Xoá |
| | V2 | Đăng Voice lên multime.ai | Thủ công hoặc auto theo `auto_publish` |
| | V3 | Danh sách Voice (lọc theo nguồn/trạng thái đăng) | Tổng hợp Voice từ cả F1/Breaking/Định kỳ |
| **Nhật ký thao tác** | L1 | Audit Log — Danh sách | Ai CRUD/chạy gì trên Breaking/Định kỳ |
| | L2 | Audit Log — Bài Post | Ai CRUD/chạy gì trên Bài Post |
| | L3 | Audit Log — Voice | Ai CRUD/đăng gì trên Voice |

> Không bao gồm chức năng thông báo và quản trị hệ thống (phân quyền, quota) theo phạm vi đã thống nhất trước đó.

---

## 3. Mô tả chi tiết các chức năng thay đổi/ mới

### 3.1 B1 — Quản lý Danh sách Breaking

**Trường cấu hình mỗi kênh:**
| Trường | Bắt buộc | Ghi chú |
|---|---|---|
| URL/kênh nguồn | Có | Auto-detect nền tảng khi nhập |
| Hình thức thu thập (A/B/C) | Có | Kèm Prompt mẫu nếu chọn C |
| Regex Pattern (điều kiện bắt bài) | Có | Bắt buộc — vì đây là tiêu chí duy nhất quyết định lấy hay bỏ |
| Ngôn ngữ default | Có | Áp dụng theo cơ chế override ở mục 1.5 |
| Trạng thái (bật/tắt) | Có | Tạm dừng theo dõi mà không cần xoá |

**Không có** trường tần suất quét — vì bản chất là quét liên tục/anytime.

### 3.2 C1 — Quản lý Danh sách Định kỳ + C3 — Lập lịch

**Trường cấu hình mỗi kênh:**
| Trường | Bắt buộc | Ghi chú |
|---|---|---|
| URL/kênh nguồn | Có | Auto-detect nền tảng khi nhập |
| Hình thức thu thập (A/B/C) | Có | Kèm Prompt mẫu nếu chọn C |
| **Tần suất quét** | Có | Vd: mỗi giờ / mỗi 6 giờ / mỗi ngày — cấu hình riêng từng kênh |
| Ngôn ngữ default | Có | |
| Trạng thái (bật/tắt) | Có | |

**Chức năng Lập lịch (Scheduler):** cần quản lý tập trung để:
- Biết kênh nào "đến giờ" chạy tiếp theo, tránh 2 kênh cùng tần suất dồn task chạy cùng lúc gây quá tải.
- Cho phép xem "lịch chạy" tổng thể (kênh nào chạy khi nào) — phục vụ vận hành, không phải chỉ chạy cron mù.

### 3.3 A2 — Quản lý AI Engine (TTS)

**Mô tả:** Vì hệ thống dùng AI TTS có sẵn (bên thứ 3) thay vì tự xây dựng, cần 1 danh mục các engine đã tích hợp (vd Google TTS, ElevenLabs, Azure TTS...) để:
- Chọn engine áp dụng mặc định toàn hệ thống, hoặc theo từng kênh/job.
- Dễ dàng thêm/bớt engine khi có nhà cung cấp mới hoặc ngừng dùng 1 engine.

**Business rule:** mỗi engine có thể hỗ trợ tập ngôn ngữ khác nhau — cần validate ngôn ngữ đã chọn (mục 1.5) có được engine đang dùng hỗ trợ không, báo lỗi sớm nếu không.

---

## 4. Data Model cập nhật

**ChannelBreaking**
```
id, source_url, platform (auto-detected), content_type (post|reel|short|video...),
collect_mode (A|B|C), prompt_id (nullable), regex_pattern, language_default,
auto_process (bool), auto_publish (bool),
status, created_by, created_at
```

**ChannelScheduled**
```
id, source_url, platform (auto-detected), content_type,
collect_mode (A|B|C), prompt_id (nullable), scan_frequency,
language_default, last_synced_post_id,
auto_process (bool), auto_publish (bool),
status, created_by, created_at
```

**Prompt**
```
id, name, content, created_by, created_at
```

**AIEngine**
```
id, name, provider, supported_languages[], is_active
```

**SourcePost** (tầng "Bài Post" — trung tâm của kiến trúc mới)
```
id, source_type (F1|Breaking|Scheduled), list_id (nullable — null nếu tạo tay/độc lập),
source_url, platform (auto-detected), content_type (post|reel|short|video...),
post_id_extracted, extracted_text (caption/transcript, nếu đã fetch),
collect_mode (A|B|C), prompt_id (nullable), language,
status (new|processed|failed), created_by, created_at
```

**Voice** (tầng cuối — thay thế Job+Post cũ)
```
id, source_post_id, ai_engine_id,
voice_file (nullable — chỉ tồn tại khi CHƯA đăng, xoá sau khi đăng thành công),
description, hashtag, language, image_url (nullable),
publish_status (draft|ready|published|failed),
multime_post_url (nullable — có giá trị sau khi đăng thành công),
created_by, created_at, published_at
```

**Business rule — vòng đời file voice:**
- Khi Voice ở trạng thái `draft`/`ready` (chưa đăng): hệ thống lưu **file voice thực tế** (`voice_file`) để phục vụ nghe thử/preview/chỉnh sửa trước khi đăng.
- Ngay sau khi **đăng thành công** lên multime.ai (`publish_status = published`): hệ thống **xoá file voice khỏi storage nội bộ**, chỉ giữ lại `multime_post_url` làm nguồn tham chiếu duy nhất — tránh lưu trùng dữ liệu (file vừa có trên multime.ai, vừa có trên hệ thống) và tiết kiệm chi phí storage.
- Nếu đăng **thất bại** (`publish_status = failed`): vẫn giữ nguyên `voice_file` để cho phép retry đăng lại mà không cần tạo lại voice từ đầu.
- Hệ quả: sau khi đã đăng, chức năng "Nghe lại" ở màn Chi tiết Voice (nếu có) cần phát trực tiếp từ `multime_post_url` thay vì từ file nội bộ (vì file đã bị xoá).

**SkippedLog** (cho Breaking)
```
id, channel_id, post_id, regex_checked, reason, checked_at
```

**AuditLog**
```
id, user_id, action (create|update|delete|run|publish), object_type (list_breaking|list_scheduled|source_post|voice),
object_id, changes (json diff), timestamp
```

---

## 5. Câu hỏi còn cần confirm

1. 1 kênh (cùng URL) được thêm vào **cả 2 danh sách** (Breaking + Định kỳ) — có tạo 2 bản ghi độc lập hoàn toàn không liên quan nhau, hay cần liên kết để tránh xử lý trùng 1 bài 2 lần?
2. Regex điều kiện bắt bài (Breaking) — cho phép nhiều Regex/kênh (kết hợp OR) hay chỉ 1 pattern duy nhất?
3. "Quét liên tục/anytime" — có giới hạn tối thiểu (vd không quét dày quá X giây/lần để tránh vượt rate-limit của nền tảng) không, hay hoàn toàn không giới hạn?
4. Tần suất quét của Danh sách Định kỳ — cho phép nhập tự do (vd "mỗi 37 phút") hay chỉ chọn từ danh sách cố định (15p/1h/6h/1 ngày...)?
5. AI Engine (TTS) — user được chọn engine theo từng kênh/job, hay hệ thống tự chọn cố định 1 engine duy nhất cho toàn bộ hệ thống?
6. Audit Log có cần hiển thị ra UI cho user xem, hay chỉ lưu nội bộ phục vụ tra soát khi có sự cố?
7. Khi auto-detect nền tảng thất bại (URL lạ/không nhận diện được) — có cho phép user tự chọn tay nền tảng + nhập ID thủ công như phương án dự phòng không?
8. **[Mới]** `auto_process` (Bài Post → Voice) và `auto_publish` (Voice → Đăng) — cấu hình theo từng Danh sách/kênh như đề xuất ở mục -1, hay cần cấu hình chi tiết hơn (vd theo từng Bài Post riêng lẻ)?
9. **[Mới]** 1 Bài Post có được phép **chạy lại nhiều lần** để tạo nhiều Voice khác nhau (đổi mode/prompt) không, hay mỗi Bài Post chỉ sinh ra đúng 1 Voice?
10. **[Mới]** Khi 1 Bài Post được tạo tay (không qua Danh sách nào, `list_id = null`) — Bài Post đó có hiển thị trong màn Danh sách Bài Post chung hay tách riêng khỏi các bài từ Breaking/Định kỳ?

---

## 6. Lộ trình phát triển theo giai đoạn

Ưu tiên theo 2 trục: **tính năng** (feature) và **nền tảng** (platform) — mỗi giai đoạn thu hẹp phạm vi để đi nhanh, giảm rủi ro, rồi mở rộng dần.

### Giai đoạn 1 — Nền tảng lõi (MVP)
**Mục tiêu:** chứng minh được luồng end-to-end cơ bản nhất, nhưng **dựng đúng mô hình 3 tầng ngay từ đầu** (List → Bài Post → Voice) để tránh phải refactor lớn ở giai đoạn sau — dù giai đoạn này chưa có Danh sách nào.
- Tính năng: **F1 (Tạo Voice theo yêu cầu)** — chỉ Mode A (Extract từ URL) + Mode B (Text → Voice). Dựng sẵn entity `SourcePost` và `Voice` tách biệt (dù UI có thể gộp 2 bước làm 1 nhờ `auto_process = true`).
- Nền tảng: **YouTube** (API ổn định, dễ tích hợp nhất).
- Đăng thử lên multime.ai (xác nhận luồng publish hoạt động đúng).
- Audit Log cơ bản (ghi log, chưa cần UI xem).

### Giai đoạn 2 — Mở rộng thu thập & Prompt
- Tính năng: thêm **Mode C** (Text + Prompt), **Quản lý Prompt mẫu** (A1), **Quản lý AI Engine** (A2).
- Nền tảng: thêm **Facebook**.
- Bổ sung Auto-detect nền tảng & trích xuất ID (thay vì chỉ hard-code cho YouTube).

### Giai đoạn 3 — Danh sách Định kỳ (F3)
- Tính năng: **Danh sách Định kỳ** (C1) + **Lập lịch** (C3) + chống lấy lặp (C4).
- Nền tảng: thêm **X (Twitter)**.
- Regex filter/search trên danh sách (C2).

### Giai đoạn 4 — Danh sách Breaking (F2)
**Đây là tính năng phức tạp/rủi ro cao nhất (quét liên tục, chi phí API cao) nên để sau khi các phần nền tảng đã ổn định.**
- Tính năng: **Danh sách Breaking** (B1) + Engine quét liên tục (B3) + Regex điều kiện bắt bài + Log bài bị bỏ qua (B4).
- Nền tảng: mở rộng tiếp **TikTok, Instagram** (nếu khả thi về API) cho F1/F3; F2 vẫn giữ giới hạn YouTube/Facebook/X do yêu cầu tốc độ.

### Giai đoạn 5 — Hoàn thiện & tối ưu
- Cơ chế ngôn ngữ theo tầng đầy đủ (List → Post → Voice).
- Audit Log có UI tra cứu.
- Tối ưu chi phí AI Engine, mở rộng thêm nền tảng mới nếu có nhu cầu.

---

## 7. Bước tiếp theo
1. Xác nhận các câu hỏi ở mục 5 (đặc biệt #1, #2, #5 — ảnh hưởng kiến trúc).
2. Duyệt lộ trình phát triển ở mục 6, điều chỉnh nếu thứ tự ưu tiên khác với mong muốn thực tế.
3. Thiết kế wireframe cho 2 màn Danh sách (Breaking, Định kỳ) với sự khác biệt rõ về trường cấu hình.
4. Khảo sát & chọn nhà cung cấp AI TTS cụ thể để tích hợp ở Giai đoạn 1.