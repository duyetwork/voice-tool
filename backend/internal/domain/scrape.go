package domain

import (
	"context"
	"net/url"
	"slices"
	"strings"
)

// ---------------------------------------------------------------------------
// Via / proxy — hạ tầng quét trang công khai của Facebook / X / Instagram
// ---------------------------------------------------------------------------
//
// VÌ SAO CÓ TẦNG NÀY: ba nền tảng đó không liệt kê được bài của một trang qua
// yt-dlp, và kênh nguồn là trang của NGƯỜI KHÁC nên không có API chính thức nào
// dùng được. Quyết định đã chốt là tự lấy bằng phiên đăng nhập + proxy.
//
// Đi kèm là những thứ không nằm trong code nhưng người vận hành phải biết: cách
// làm này vi phạm điều khoản sử dụng của cả ba nền tảng, mang rủi ro pháp lý về
// scraping, và chi phí thật của nó là via chết liên tục phải thay — chi phí vận
// hành chứ không phải chi phí một lần. Xem docs/review-via-proxy.md.

// Trạng thái của một via.
//
// Máy trạng thái:
//
//	active  --(N lỗi login liên tiếp)--> cooldown --(hết giờ)--> active
//	cooldown --(lại lỗi login ngay sau khi hồi)--> dead
//	disabled <-> active                            (người vận hành bật/tắt tay)
//
// `dead` và `disabled` tách nhau vì chúng trả lời hai câu khác nhau: `dead` là
// hệ thống kết luận via hỏng, `disabled` là người vận hành chủ động cất nó đi.
// Gộp lại thì một lần tắt tay trông giống một lần via chết, và bảng tổng quan
// đếm sai số via còn cứu được.
const (
	ViaActive   = "active"
	ViaCooldown = "cooldown"
	ViaDead     = "dead"
	ViaDisabled = "disabled"
)

// Trạng thái của một proxy. Không có `cooldown`: proxy hỏng vì IP bị liệt thì
// chờ không khỏi — xem MarkScrapeProxyBlocked.
const (
	ProxyActive   = "active"
	ProxyDegraded = "degraded"
	ProxyDead     = "dead"
	ProxyDisabled = "disabled"
)

// Loại proxy. `datacenter` được phép khai nhưng không nên dùng cho ba nền tảng
// này: Meta và X nhận ra dải IP datacenter gần như ngay lập tức.
const (
	ProxyResidential = "residential"
	ProxyMobile      = "mobile"
	ProxyDatacenter  = "datacenter"
)

// ScrapeResultSuccess là nhãn của một lượt quét chạy được, trong via_usage_log.
// Các nhãn lỗi dùng chung bộ chữ với FetchBlockKind để một lỗi chỉ có một tên
// trên toàn hệ thống.
const ScrapeResultSuccess = "success"

func ValidViaStatus(s string) bool {
	switch s {
	case ViaActive, ViaCooldown, ViaDead, ViaDisabled:
		return true
	}
	return false
}

func ValidProxyStatus(s string) bool {
	switch s {
	case ProxyActive, ProxyDegraded, ProxyDead, ProxyDisabled:
		return true
	}
	return false
}

func ValidProxyKind(k string) bool {
	switch k {
	case ProxyResidential, ProxyMobile, ProxyDatacenter:
		return true
	}
	return false
}

// ScrapePlatforms là các nền tảng cần via để quét kênh, theo đúng thứ tự ưu
// tiên đã chốt: Facebook trước, rồi X, rồi Instagram.
var ScrapePlatforms = []Platform{PlatformFacebook, PlatformX, PlatformInstagram}

// NeedsVia cho biết quét kênh của nền tảng này có phải đi qua via hay không.
// YouTube và TikTok thì không: yt-dlp liệt kê được kênh của chúng mà không cần
// đăng nhập, và đẩy chúng qua via chỉ là tự thêm một điểm hỏng.
func NeedsVia(p Platform) bool { return slices.Contains(ScrapePlatforms, p) }

// NormalizeProxyEndpoint đổi chuỗi proxy người dùng dán vào thành URL chuẩn.
//
// Nhận ba dạng, vì đó là ba dạng thật sự tồn tại ngoài đời:
//
//	http://user:pass@host:port     URL đầy đủ
//	host:port:user:pass            dạng các nhà bán proxy hay giao
//	host:port                      proxy không cần đăng nhập
//
// Dạng thứ hai là lý do hàm này tồn tại. Gần như mọi nhà bán proxy residential
// đều giao một danh sách `ip:port:user:pass`, và bắt người vận hành tự ghép tay
// thành URL cho từng dòng là vừa mất thời gian vừa dễ sai — sai ở đây thì proxy
// lặng lẽ không dùng được, và triệu chứng lại giống hệt proxy bị chặn.
//
// Không đoán giao thức khác ngoài http: một endpoint `socks5` phải được khai
// tường minh, vì dùng nhầm giao thức thì request hỏng theo kiểu rất khó đọc.
func NormalizeProxyEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}

	// Đã có scheme thì tin người dùng, chỉ kiểm tra parse được.
	if strings.Contains(raw, "://") {
		u, err := url.Parse(raw)
		if err != nil || u.Host == "" {
			return ""
		}
		return u.String()
	}

	parts := strings.Split(raw, ":")
	switch len(parts) {
	case 2: // host:port
		if parts[0] == "" || parts[1] == "" {
			return ""
		}
		return "http://" + raw
	case 4: // host:port:user:pass
		host, port, user, pass := parts[0], parts[1], parts[2], parts[3]
		if host == "" || port == "" || user == "" {
			return ""
		}
		// url.UserPassword + String() lo phần escape: mật khẩu proxy hay có
		// ký tự @ hoặc : , và ghép chuỗi tay thì đúng những mật khẩu đó hỏng.
		u := url.URL{Scheme: "http", User: url.UserPassword(user, pass), Host: host + ":" + port}
		return u.String()
	default:
		return ""
	}
}

// MaskProxyEndpoint cắt bỏ user/pass khỏi URL proxy để hiện lên giao diện.
//
// Tính ở server chứ không nhận từ client: nếu để client gửi lên phần "đã che"
// thì chính cột đó trở thành một đường để user/pass rò ra ngoài, và không có gì
// kiểm tra được nó đã thật sự được che.
//
// URL không parse được thì trả về chuỗi rỗng chứ KHÔNG trả về nguyên bản: một
// endpoint dị dạng vẫn có thể chứa mật khẩu.
func MaskProxyEndpoint(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	u.User = nil
	return u.String()
}

// ---------------------------------------------------------------------------
// Cookies của via
// ---------------------------------------------------------------------------

// requiredCookies — những cookie BẮT BUỘC có để một phiên đăng nhập dùng được,
// theo từng nền tảng.
//
// Vì sao kiểm: dán cookies là thao tác thủ công, và cái sai thường gặp nhất là
// dán nhầm phần — copy cookie của trang khác, copy thiếu, hoặc copy nguyên khối
// JSON của một tiện ích mở rộng. Không kiểm ở đây thì via được lưu thành công,
// nằm trong vòng xoay, và mãi 6 tiếng sau mới lộ ra là hỏng qua một chuỗi lỗi
// "đòi đăng nhập" — lúc đó nó đã kéo theo cả một vòng quét thất bại.
//
// Chỉ liệt kê cookie MANG DANH TÍNH, không liệt kê cookie phụ trợ (datr, sb,
// fr, guest_id...): thiếu chúng thì phiên vẫn chạy, và bắt buộc chúng chỉ làm
// người dùng bị từ chối vì một thứ không quan trọng.
var requiredCookies = map[Platform][]string{
	// c_user = id tài khoản, xs = chính phiên đăng nhập. Thiếu một trong hai
	// thì Facebook coi như khách.
	PlatformFacebook: {"c_user", "xs"},
	// sessionid là phiên; ds_user_id là id tài khoản đi kèm.
	PlatformInstagram: {"sessionid", "ds_user_id"},
	// auth_token là phiên; ct0 là token CSRF mà mọi request đọc dữ liệu của X
	// đều đòi — có auth_token mà thiếu ct0 thì vẫn bị từ chối.
	PlatformX: {"auth_token", "ct0"},
}

// RequiredCookies trả danh sách cookie bắt buộc của một nền tảng.
func RequiredCookies(p Platform) []string { return requiredCookies[p] }

// MissingCookies trả những cookie bắt buộc còn thiếu trong chuỗi đã dán.
//
// So khớp theo TÊN COOKIE đứng trước dấu `=`, không phải tìm chuỗi con: tên
// `xs` xuất hiện bên trong giá trị của một cookie khác là chuyện bình thường,
// và tìm chuỗi con sẽ báo "có" cho một phiên thật ra thiếu nó.
func MissingCookies(p Platform, raw string) []string {
	want := requiredCookies[p]
	if len(want) == 0 {
		return nil
	}
	have := map[string]bool{}
	for _, part := range strings.Split(raw, ";") {
		name, _, ok := strings.Cut(strings.TrimSpace(part), "=")
		if ok {
			have[strings.TrimSpace(name)] = true
		}
	}
	var missing []string
	for _, name := range want {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return missing
}

// ---------------------------------------------------------------------------

// ScrapeSession là bộ (via, proxy) đã được cấp cho MỘT lượt quét.
//
// Cookies và endpoint proxy đã giải mã sẵn: adapter chỉ việc dùng, không cần
// biết chúng được cất ở đâu. Chúng KHÔNG được ghi log ở bất kỳ đâu.
type ScrapeSession struct {
	ViaID   string
	ViaName string
	Cookies string
	// ProxyID rỗng = không có proxy nào khả dụng, request đi thẳng bằng IP máy
	// chủ. Vẫn chạy chứ không chặn: với một kênh lẻ thì đi thẳng còn hơn không
	// quét được gì, và số lần bị chặn sẽ tự nói ra rằng cần thêm proxy.
	ProxyID  string
	ProxyURL string
}

// ScrapePool cấp và thu hồi (via, proxy) cho từng lượt quét.
//
// Là interface để adapter không phải biết tới DB: adapter nhận một phiên, chạy
// xong thì báo kết quả về, và toàn bộ máy trạng thái nằm ở phía cài đặt.
type ScrapePool interface {
	// Acquire cấp một phiên cho nền tảng này. Trả ErrNoViaAvailable khi không
	// còn via nào dùng được — gọi phải BỎ QUA lượt quét đó chứ không được coi
	// là sự cố: hết via là chuyện bình thường khi hạn mức ngày đã cạn.
	Acquire(ctx context.Context, platform Platform) (*ScrapeSession, error)
	// Release báo kết quả của lượt quét: nil = thành công. Nó là nơi duy nhất
	// máy trạng thái của via/proxy được cập nhật.
	Release(ctx context.Context, s *ScrapeSession, owner ScrapeOwner, postsFound int, cause error)
}

// ScrapeOwner cho biết lượt quét này thuộc kênh nào, để via_usage_log trả lời
// được "via nào đang hỏng ở kênh nào". Cả hai rỗng = lượt chạy thử tay.
type ScrapeOwner struct {
	Platform  Platform
	Breaking  *string
	Scheduled *string
}

// ---------------------------------------------------------------------------
// Trần số bài lấy được trong MỘT lượt quét
// ---------------------------------------------------------------------------

// channelPostCap — số bài nhiều nhất một lượt quét lấy được, theo từng nền tảng.
//
// Ba nền tảng chạy bằng via KHÔNG phân trang được: một lần gọi trả bấy nhiêu là
// hết, và phần thiếu không có đường nào lấy. Đã đo trực tiếp ngày 17/09/2026:
//
//	Facebook   1–10 bài  — chỉ những bài Facebook dựng sẵn trong HTML trang;
//	                       phần còn lại trang tự tải thêm khi cuộn.
//	Instagram  12 bài    — web_profile_info trả đúng 12 và không nhận tham số
//	                       xin thêm.
//	X          ~20 bài   — endpoint syndication bỏ qua MỌI tham số phân trang
//	                       (đã thử max_id / until_id / cursor / max_position /
//	                       count=200: cùng một cửa sổ, cùng bài cũ nhất).
//
// Con số của X là số đo trên PHIÊN THẬT (hai tài khoản khác nhau cho 19 và 20).
// Gọi cùng endpoint mà không có cookies thì trả về một bản đệm ~100 bài trộn
// lẫn nhiều năm — đừng lấy con số đó làm trần, nó không phải thứ vòng quét nhận
// được.
//
// YouTube và TikTok không có trần: yt-dlp phân trang được (`--playlist-end`).
//
// VÌ SAO LÀ DỮ LIỆU Ở ĐÂY chứ không phải một phương thức của adapter: giao diện
// cần con số này để KHOÁ ô nhập ngay lúc thêm kênh, tức là trước khi có bất kỳ
// lượt quét nào. Một giá trị tra được mà không cần dựng adapter là đúng hình
// dạng của nhu cầu đó.
var channelPostCap = map[Platform]struct {
	max    int
	reason string
}{
	PlatformFacebook: {10,
		"Facebook chỉ dựng sẵn vài bài trong HTML của trang; phần còn lại trang tự tải thêm khi cuộn, không lấy được."},
	PlatformInstagram: {12,
		"Instagram trả đúng 12 bài mỗi lần gọi và không cho xin thêm."},
	PlatformX: {20,
		"X trả khoảng 20 bài mỗi lần gọi và bỏ qua mọi tham số phân trang."},
}

// MaxChannelPosts trả trần của một nền tảng, và lý do. 0 = không có trần.
func MaxChannelPosts(p Platform) (int, string) {
	limit, ok := channelPostCap[p]
	if !ok {
		return 0, ""
	}
	return limit.max, limit.reason
}

// ClampChannelLimit ép một con số người dùng đặt về đúng trần của nền tảng.
//
// Gọi ở ĐƯỜNG QUÉT chứ không chỉ ở form: kênh thêm từ trước khi có trần vẫn
// đang giữ giá trị cũ (50), và xin 50 ở một nơi chỉ trả về 12 thì con số ghi
// vào lịch sử quét là một lời hứa không ai giữ được.
func ClampChannelLimit(p Platform, limit int) int {
	max, _ := MaxChannelPosts(p)
	if max > 0 && limit > max {
		return max
	}
	return limit
}
