package platform

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// ---------------------------------------------------------------------------
// HTTP client cho các adapter tự đọc trang (Facebook / X / Instagram)
// ---------------------------------------------------------------------------
//
// Ba nền tảng này không có extractor yt-dlp nào liệt kê được bài của một trang,
// nên phần "liệt kê kênh" phải tự gọi HTTP. Tệp này là phần dùng chung: gắn
// cookies của via, đi qua proxy được cấp, và ĐỔI PHẢN HỒI THÀNH ĐÚNG NHÃN LỖI
// mà hạ tầng via/proxy cần.
//
// Phân loại lỗi là phần quan trọng nhất ở đây, không phải phần tải trang: cả
// máy trạng thái via lẫn máy trạng thái proxy đều chạy bằng nhãn đó. Gắn nhầm
// `bot_block` thành `login_required` là giết via vì lỗi của proxy.

// scrapeTimeout — trần cho MỘT request đọc trang.
//
// 90 giây, nâng từ 45. Lý do là số đo thật: trang `/reels` của một số page
// Facebook (vd facebook.com/etnow/reels) hỏng đều đặn ở 45s với
// "context deadline exceeded ... while reading body" — tức là kết nối đã mở,
// header đã về, và hết giờ ĐANG ĐỌC THÂN. Những trang đó nặng vài MB JSON và
// đi qua proxy residential vốn chậm.
//
// Không nâng vô hạn: một kênh treo giữ luôn suất đồng thời của cả nền tảng, mà
// suất đó cố tình chỉ có vài cái. 90s là mức còn chấp nhận được cho một vòng
// quét chạy nền, và phần thân đọc dở vẫn được tận dụng — xem minUsableBody.
const scrapeTimeout = 90 * time.Second

// maxScrapeBody — trần dung lượng một trang.
//
// 8MB, hạ từ 16MB. Trang nặng nhất quan sát được còn cách xa mức này, còn trần
// cao hơn chỉ có tác dụng duy nhất là kéo dài một lần đọc đang hỏng.
const maxScrapeBody = 8 << 20

// minUsableBody — đọc dở nhưng đã được chừng này thì vẫn đem đi phân tích.
//
// VÌ SAO KHÔNG VỨT ĐI: io.ReadAll trả về CẢ phần đã đọc lẫn lỗi, và trước đây
// ta bỏ luôn phần đã đọc khi hết giờ. Nhưng dữ liệu cần lấy — khối JSON nhúng
// chứa danh sách bài — nằm ở phần đầu trang; phần đuôi chủ yếu là script và
// theo dõi. Vứt 6MB đã về chỉ vì thiếu 200KB cuối nghĩa là kênh đó không bao
// giờ quét được, trong khi mọi thứ cần thiết đã nằm trong tay.
//
// 256KB là ngưỡng để phân biệt "đọc dở nhưng có nội dung" với "gần như chưa
// nhận được gì" — trường hợp sau thì lỗi mới là câu trả lời đúng.
const minUsableBody = 256 << 10

// scrapeUserAgent — User-Agent gửi kèm mọi request.
//
// Phải là một trình duyệt thật: cookies của một phiên đăng nhập trên trình duyệt
// mà đi kèm User-Agent của thư viện HTTP là cặp đôi không tồn tại trong thực tế,
// và đó là thứ đầu tiên bị soi.
const scrapeUserAgent = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) " +
	"AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"

// ScrapeFetcher tải một URL bằng phiên (via + proxy) được cấp.
type ScrapeFetcher struct {
	// clients cache theo endpoint proxy: mỗi proxy một http.Client để tái dùng
	// kết nối. Dựng client mới cho từng request thì mỗi lượt quét là một lần bắt
	// tay TLS mới qua proxy — chậm, và tạo ra một dấu vết rất dễ nhận ra.
	clients *clientCache
}

func NewScrapeFetcher() *ScrapeFetcher {
	return &ScrapeFetcher{clients: newClientCache()}
}

// Get tải một trang. Lỗi trả về ĐÃ được gắn nhãn domain.FetchBlockKind, nên
// ScrapePool chỉ việc đọc nhãn để biết nên trách via hay trách proxy.
func (f *ScrapeFetcher) Get(
	ctx context.Context, s *domain.ScrapeSession, rawURL string, headers map[string]string,
) ([]byte, error) {
	client, err := f.clients.get(s.ProxyURL)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, fmt.Errorf("dựng request %s: %w", rawURL, err)
	}
	// Bộ header của một trình duyệt thật. Thiếu Accept-Language thì nhiều trang
	// trả về bản rút gọn khác hẳn bản người dùng thấy, và bộ phân tích sẽ không
	// tìm thấy gì mà cũng không báo lỗi.
	req.Header.Set("User-Agent", scrapeUserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")
	req.Header.Set("Accept-Language", "en-US,en;q=0.9,vi;q=0.8")
	req.Header.Set("Sec-Fetch-Mode", "navigate")
	req.Header.Set("Upgrade-Insecure-Requests", "1")
	if s.Cookies != "" {
		req.Header.Set("Cookie", s.Cookies)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	resp, err := client.Do(req)
	if err != nil {
		// Lỗi mạng: KHÔNG gắn nhãn nào. Không biết proxy hỏng hay nền tảng
		// chặn, và đoán bừa ở đây là giết nhầm via hoặc proxy.
		return nil, fmt.Errorf("tải %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	// Trần đọc: một trang bất thường (hoặc một trang lỗi trả về stream vô hạn)
	// không được phép ngốn hết bộ nhớ của worker.
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxScrapeBody))
	if err != nil {
		// Đọc dở mà đã có đủ nội dung thì DÙNG phần đó — xem minUsableBody.
		// Phân tích một trang cụt còn hơn không quét được kênh đó lần nào.
		if len(body) < minUsableBody {
			return nil, fmt.Errorf("đọc phản hồi %s: %w", rawURL, err)
		}
	}

	if blocked := classifyScrapeResponse(resp, body); blocked != nil {
		return nil, blocked
	}
	return body, nil
}

// ---------------------------------------------------------------------------
// Phân loại phản hồi
// ---------------------------------------------------------------------------

// loginMarkers — dấu hiệu nền tảng ĐÒI ĐĂNG NHẬP, tức là phiên của via hỏng.
//
// Điểm khó: các trang này trả về HTTP 200 kèm trang đăng nhập chứ không trả 401.
// Chỉ nhìn mã trạng thái thì mọi via hỏng đều trông như thành công, và vòng quét
// lặng lẽ trả về 0 bài mãi mãi.
var loginMarkers = regexp.MustCompile(
	`(?i)"__typename"\s*:\s*"LoginPage"` + // Facebook GraphQL
		`|/login/\?next=` + // Facebook chuyển hướng
		`|You must log in to continue` +
		`|<title>\s*Log (?:in|into) Facebook` +
		`|name="login_source"` + // form đăng nhập Facebook
		`|"require_login"\s*:\s*true` + // Instagram
		`|accounts/login/\?next=`) // Instagram chuyển hướng

// botMarkers — dấu hiệu nền tảng nghi NGUỒN GỌI, tức là lỗi của proxy/IP.
var botMarkers = regexp.MustCompile(
	`(?i)confirm you.re not a bot` +
		`|unusual (?:traffic|activity) from your` +
		`|/checkpoint/` + // Facebook đưa vào checkpoint
		`|Please verify you are a human` +
		`|Attention Required!\s*\|\s*Cloudflare` +
		`|captcha`)

// classifyScrapeResponse đổi một phản hồi thành lỗi đã gắn nhãn, hoặc nil nếu
// trang đọc được.
//
// THỨ TỰ KIỂM TRA LÀ CÓ CHỦ Ý: dấu hiệu nghi-bot xét TRƯỚC dấu hiệu đòi đăng
// nhập. Khi nền tảng đẩy vào checkpoint, trang trả về thường chứa cả hai — và
// đọc nhầm nó thành "đòi đăng nhập" sẽ giết dần đàn via trong khi thứ hỏng thật
// sự là địa chỉ IP.
func classifyScrapeResponse(resp *http.Response, body []byte) error {
	switch resp.StatusCode {
	case http.StatusTooManyRequests:
		return domain.FetchBlocked(domain.FetchBlockRateLimit, domain.Explain(
			"Nền tảng đang giới hạn tần suất — giảm số kênh quét cùng lúc",
			fmt.Errorf("HTTP 429 từ %s", resp.Request.URL.Host)))
	case http.StatusUnauthorized, http.StatusForbidden:
		// ĐỌC THÂN PHẢN HỒI TRƯỚC khi kết luận. Mặc định của nhánh này là "IP bị
		// chặn", đúng với 403 của Facebook/X (checkpoint theo IP), nhưng KHÔNG
		// đúng với Instagram: endpoint `/api/v1/users/web_profile_info` trả
		//
		//	HTTP 401 {"message":"Please wait a few minutes before you try again.",
		//	          "require_login":true, ...}
		//
		// khi phiên của via hỏng. Đo trực tiếp ngày 17/09/2026. Không kiểm thân
		// phản hồi ở đây thì mỗi via Instagram hết hạn lại bị tính thành một lần
		// proxy bị chặn: proxy tốt bị hạ cấp rồi khai tử, còn via hỏng — thứ
		// thật sự cần thay — vẫn nằm nguyên trong vòng xoay ở trạng thái khoẻ.
		if loginMarkers.Match(snippetOf(body)) {
			return domain.FetchBlocked(domain.FetchBlockLogin, domain.Explain(
				"Phiên đăng nhập của via đã hỏng — cần dán cookies mới",
				fmt.Errorf("HTTP %d kèm dấu hiệu đòi đăng nhập từ %s",
					resp.StatusCode, resp.Request.URL.Host)))
		}
		return domain.FetchBlocked(domain.FetchBlockBot, domain.Explain(
			"Nền tảng từ chối request — nhiều khả năng IP đang bị chặn",
			fmt.Errorf("HTTP %d từ %s", resp.StatusCode, resp.Request.URL.Host)))
	case http.StatusNotFound, http.StatusGone:
		return domain.Permanent(domain.FetchBlocked(domain.FetchBlockUnavailable, domain.Explain(
			"Trang không còn tồn tại hoặc đã đổi địa chỉ",
			fmt.Errorf("HTTP %d từ %s", resp.StatusCode, resp.Request.URL.Host))))
	}

	// Trang 200 nhưng là trang chặn — xem ghi chú ở loginMarkers.
	snippet := snippetOf(body)
	if botMarkers.Match(snippet) {
		return domain.FetchBlocked(domain.FetchBlockBot, domain.Explain(
			"Nền tảng đang nghi ngờ IP máy chủ (checkpoint/captcha) — cần đổi proxy",
			fmt.Errorf("trang chặn từ %s", resp.Request.URL.Host)))
	}
	if loginMarkers.Match(snippet) {
		return domain.FetchBlocked(domain.FetchBlockLogin, domain.Explain(
			"Phiên đăng nhập của via đã hỏng — cần dán cookies mới",
			fmt.Errorf("trang đăng nhập từ %s", resp.Request.URL.Host)))
	}

	if resp.StatusCode >= 500 {
		// 5xx thường tự khỏi: không gắn nhãn, không trách ai.
		return fmt.Errorf("nền tảng trả HTTP %d", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("nền tảng trả HTTP %d", resp.StatusCode)
	}
	return nil
}

// snippetOf cắt phần đầu thân phản hồi để đem đi dò dấu hiệu chặn.
//
// Chỉ soi phần đầu: dấu hiệu chặn luôn nằm ở khối <head>/JSON đầu trang, còn
// quét regex trên 16MB HTML cho mỗi request là phí vô ích.
func snippetOf(body []byte) []byte {
	if len(body) > 256<<10 {
		return body[:256<<10]
	}
	return body
}

// ---------------------------------------------------------------------------

type clientCache struct {
	mu      sync.Mutex
	clients map[string]*http.Client
}

func newClientCache() *clientCache {
	return &clientCache{clients: map[string]*http.Client{}}
}

// get trả http.Client đi qua proxy chỉ định. Chuỗi rỗng = đi thẳng.
func (c *clientCache) get(proxyURL string) (*http.Client, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if client, ok := c.clients[proxyURL]; ok {
		return client, nil
	}

	transport := &http.Transport{
		MaxIdleConns:        20,
		IdleConnTimeout:     90 * time.Second,
		TLSHandshakeTimeout: 20 * time.Second,
	}
	if proxyURL != "" {
		parsed, err := url.Parse(proxyURL)
		if err != nil {
			return nil, fmt.Errorf("endpoint proxy không hợp lệ: %w", err)
		}
		transport.Proxy = http.ProxyURL(parsed)
	}

	client := &http.Client{
		Transport: transport,
		Timeout:   scrapeTimeout,
		// KHÔNG tự đi theo chuyển hướng sang trang đăng nhập: đi theo thì cái
		// nhận được là HTML của trang login và bộ phân loại phải đoán lại từ
		// đầu. Dừng ở đây thì chính URL chuyển hướng là bằng chứng rõ nhất.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 5 {
				return fmt.Errorf("quá nhiều chuyển hướng")
			}
			if strings.Contains(req.URL.Path, "/login") ||
				strings.Contains(req.URL.Path, "/checkpoint") {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	c.clients[proxyURL] = client
	return client, nil
}
