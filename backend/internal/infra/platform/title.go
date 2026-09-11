package platform

import (
	"regexp"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Tiêu đề Bài Post = TOÀN BỘ nội dung bài, trừ hashtag.
//
// Hệ thống không còn trường mô tả riêng: multime chỉ hiển thị `title` của voice
// post, nên nội dung bài gốc phải nằm trọn trong tiêu đề Bài Post, và tiêu đề
// Voice là 200 ký tự đầu của nó (xem domain.VoiceTitle).
//
// Cái khó là mỗi nền tảng gọi "nội dung" một kiểu, còn yt-dlp/Open Graph thì
// nhét cả hai vào cặp (title, description):
//
//   - Facebook / Instagram / TikTok / X: bài chỉ có caption. `title` mà yt-dlp
//     trả về là <title> của trang — thường là caption đã bị cắt cụt, kèm số
//     liệu tương tác và tên page:
//     "42K views · 824 reactions | Tin nóng hôm nay… | Kênh ABC | Facebook".
//     Nội dung thật nằm ở `description`.
//   - YouTube: `title` là tiêu đề video do người đăng đặt — đó CHÍNH LÀ nội
//     dung bài; `description` là phần chú thích/link bên dưới, đăng lại lên
//     multime là rác.
//
// Hai kiểu đó dùng hai hàm khác nhau — adapter biết mình là nền tảng nào nên
// không phải đoán: PostContent cho bài kiểu caption, PostContentTitled cho bài
// có tiêu đề riêng.
// ngược lại lấy `title` (bài có tiêu đề riêng).

// statPartRe khớp 1 số liệu tương tác đứng riêng: "42K views", "824 reactions",
// "42 N lượt xem" (bản tiếng Việt của Facebook dùng N = nghìn, Tr = triệu).
var statPartRe = regexp.MustCompile(
	`(?i)^\d[\d.,]*\s*(?:k|m|b|n|tr)?\s*` +
		`(views?|reactions?|comments?|shares?|likes?|plays?|followers?|subscribers?` +
		`|lượt xem|lượt thích|lượt chia sẻ|bình luận|chia sẻ|người theo dõi)$`)

// platformNameRe khớp đoạn chỉ là tên nền tảng — phần đuôi của <title> trang.
var platformNameRe = regexp.MustCompile(`(?i)^(facebook( watch)?|instagram|tiktok|twitter|youtube)$`)

// boilerplateTitleRe khớp tiêu đề do yt-dlp TỰ ĐẶT khi bài không có tiêu đề
// riêng: Instagram trả về "Video by <tên tài khoản>", đôi khi kèm tiền tố
// "Instagram". Đó là nhãn của công cụ, không phải chữ người đăng viết — đọc lên
// thành "vi-đê-ô bai duyet_nguyen" thì vô nghĩa. Bỏ hẳn, dùng caption
// (description) làm nội dung.
var boilerplateTitleRe = regexp.MustCompile(
	`(?i)^(instagram\s+)?(video|photo|image|post|reel|story|clip)\s+by\s+\S.*$`)

// authorSeparators: các dấu yt-dlp dùng để nối tên tài khoản vào trước nội
// dung. X trả về title dạng "<tên tài khoản> - <nội dung tweet>".
var authorSeparators = []string{" - ", " – ", " — ", ": ", " | "}

// excerptProbeRunes: số ký tự đầu dùng để nhận ra `title` là đoạn trích của
// `description`. Facebook cắt caption giữa chừng nên không so được cả chuỗi;
// 40 ký tự đủ dài để không trùng nhầm, đủ ngắn để nằm gọn trong phần bị cắt.
const excerptProbeRunes = 40

// PostContent dựng nội dung Bài Post cho nền tảng KIỂU CAPTION — Facebook,
// Instagram, TikTok, X, và mọi bài đọc qua Open Graph. Bài ở đó không có tiêu
// đề riêng: cặp (title, description) chỉ là hai mảnh của cùng một caption, nên
// ghép lại và bỏ phần lặp.
//
// Bỏ số liệu tương tác, bỏ tên nền tảng, bỏ hashtag, giữ nguyên xuống dòng,
// cắt theo domain.MaxPostTitleRunes. Hashtag bị bỏ vì đã đi vào trường
// `hashtags` riêng của API — để lại trong nội dung là lặp và ăn hết giới hạn
// ký tự của tiêu đề Voice.
// `author` là tên tài khoản đăng bài (uploader của yt-dlp), dùng để bóc phần
// tên bị nối vào đầu tiêu đề — để trống nếu không biết.
func PostContent(title, description, author string) string {
	title = domain.StripHashtags(cleanTitle(title, author))
	body := domain.StripHashtags(description)

	switch {
	case title == "":
		return domain.PostTitle(body)
	case body == "":
		return domain.PostTitle(title)
	case isExcerptOf(title, body):
		// "Tiêu đề" chỉ là bản cắt cụt của chính caption -> caption là đủ.
		return domain.PostTitle(body)
	default:
		// Hai mảnh khác nhau: giữ cả hai, tiêu đề trước.
		return domain.PostTitle(title + "\n\n" + body)
	}
}

// PostContentTitled dựng nội dung Bài Post cho nền tảng bài CÓ tiêu đề riêng do
// người đăng đặt — YouTube. Tiêu đề đó chính là nội dung bài; phần mô tả bên
// dưới video là chú thích/link/timestamp, đăng lại lên multime là rác.
//
// Chỉ khi video không có tiêu đề mới rơi về mô tả để không ra bài trắng.
func PostContentTitled(title, description, author string) string {
	if t := domain.StripHashtags(cleanTitle(title, author)); t != "" {
		return domain.PostTitle(t)
	}
	return domain.PostTitle(domain.StripHashtags(description))
}

// cleanTitle làm sạch tiêu đề thô của nền tảng qua 3 bước, theo thứ tự:
//
//  1. Bỏ tiêu đề rác do công cụ tự đặt ("Video by ..." của Instagram).
//  2. Bóc tên tài khoản bị nối vào đầu ("<tài khoản> - <nội dung>" của X).
//  3. Bỏ số liệu tương tác và tên nền tảng trong <title> của trang.
//
// Cả ba đều là chữ của MÁY chứ không phải của người đăng, mà tiêu đề Bài Post
// là thứ TTS sẽ đọc lên và multime sẽ hiển thị.
func cleanTitle(title, author string) string {
	title = strings.TrimSpace(title)
	if boilerplateTitleRe.MatchString(title) {
		return ""
	}
	return cleanPageTitle(stripAuthorPrefix(title, author))
}

// stripAuthorPrefix bóc "<tên tài khoản><dấu nối>" ở đầu tiêu đề.
//
// yt-dlp dựng title của X bằng cách nối tên tài khoản vào trước nội dung tweet,
// nên không bóc thì tiêu đề Bài Post (và tiêu đề Voice đăng lên multime) luôn
// mở đầu bằng tên tài khoản, còn TTS thì đọc luôn tên đó ra.
//
// Chỉ bóc khi phần còn lại vẫn có chữ: tiêu đề chỉ có mỗi tên tài khoản thì
// giữ nguyên còn hơn trả về rỗng.
func stripAuthorPrefix(title, author string) string {
	author = strings.TrimSpace(author)
	if author == "" || title == "" {
		return title
	}
	// Tài khoản hay được ghi kèm @: khớp cả "@vtv24" lẫn "vtv24".
	names := []string{author, "@" + strings.TrimPrefix(author, "@")}
	for _, name := range names {
		for _, sep := range authorSeparators {
			prefix := name + sep
			if len(title) > len(prefix) && strings.EqualFold(title[:len(prefix)], prefix) {
				if rest := strings.TrimSpace(title[len(prefix):]); rest != "" {
					return rest
				}
			}
		}
	}
	return title
}

// cleanPageTitle bỏ khỏi <title> của trang những đoạn không phải nội dung: số
// liệu tương tác và tên nền tảng. Không phát hiện số liệu thì giữ nguyên, vì
// tiêu đề YouTube hay có dấu "|" hợp lệ kiểu "Tin nóng 24h | VTV24".
func cleanPageTitle(title string) string {
	title = strings.Join(strings.Fields(title), " ")

	segments := strings.Split(title, "|")
	hasStats := false
	kept := make([]string, 0, len(segments))

	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		if seg == "" {
			continue
		}
		if isStatsSegment(seg) {
			hasStats = true
			continue
		}
		if platformNameRe.MatchString(seg) {
			continue
		}
		kept = append(kept, seg)
	}

	switch {
	case len(kept) == 0:
		// Không còn gì -> tiêu đề gốc toàn là rác.
		return ""
	case hasStats:
		// Đây là <title> của trang: đoạn đầu còn lại là caption, các đoạn sau
		// là tên page/nền tảng nên bỏ luôn.
		return kept[0]
	default:
		return strings.Join(kept, " | ")
	}
}

// isStatsSegment: cả đoạn chỉ gồm các số liệu tương tác nối bằng "·".
func isStatsSegment(seg string) bool {
	parts := strings.Split(seg, "·")
	for _, p := range parts {
		if !statPartRe.MatchString(strings.TrimSpace(p)) {
			return false
		}
	}
	return len(parts) > 0
}

// isExcerptOf: `title` có phải chỉ là một đoạn trích của `body` không.
//
// So khớp phần ĐẦU đã chuẩn hoá thay vì cả chuỗi, vì nền tảng cắt tiêu đề giữa
// chừng và đính "…" vào cuối.
func isExcerptOf(title, body string) bool {
	// Chỉ so đoạn đầu, trước dấu "|": phần sau thường là tên page/kênh chứ
	// không phải nội dung, có so cũng không khớp.
	probe := normalizeForCompare(strings.Split(title, "|")[0])
	if probe == "" {
		return false
	}
	if r := []rune(probe); len(r) > excerptProbeRunes {
		probe = string(r[:excerptProbeRunes])
	}
	return strings.Contains(normalizeForCompare(body), probe)
}

// normalizeForCompare đưa 2 chuỗi về cùng một dạng để so: 1 dòng, thường hoá,
// bỏ dấu ba chấm của phần bị cắt.
func normalizeForCompare(s string) string {
	s = strings.ToLower(strings.Join(strings.Fields(s), " "))
	s = strings.ReplaceAll(s, "…", "")
	return strings.ReplaceAll(s, "...", "")
}
