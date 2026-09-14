package domain

import "strings"

// ---------------------------------------------------------------------------
// Thứ tự hiển thị của danh mục Quốc gia / Ngôn ngữ
// ---------------------------------------------------------------------------

// countryPriority là thứ tự nghiệp vụ của ô chọn Quốc gia: thị trường chính
// lên trước, phần còn lại xếp sau theo alphabet.
//
// Không sắp theo alphabet thuần vì ô này dùng để lọc tài khoản author, và 90%
// lượt chọn rơi vào chục cái đầu — bắt người dùng cuộn qua Afghanistan để tới
// Việt Nam là bắt họ trả giá cho một quy tắc sắp xếp không ai cần.
//
// Khớp theo MÃ ISO là chính, tên chỉ là đường dự phòng. Lý do rất cụ thể:
// Strongbody trả tên ISO đầy đủ ("United Kingdom of Great Britain and Northern
// Ireland", "Korea (Republic of)"), nên khớp theo tên là một cuộc đoán không
// bao giờ đủ — còn mã thì chỉ có một dạng.
var countryPriority = []struct {
	name string
	// code là ISO 3166-1 alpha-2. Rỗng nghĩa là chỉ khớp được theo tên.
	code string
}{
	{"Vietnam", "VN"}, {"United States", "US"}, {"United Kingdom", "GB"},
	{"France", "FR"}, {"Germany", "DE"}, {"Japan", "JP"}, {"South Korea", "KR"},
	{"China", "CN"}, {"Switzerland", "CH"}, {"Singapore", "SG"},
	{"Canada", "CA"}, {"India", "IN"}, {"Brazil", "BR"}, {"Australia", "AU"},
	{"South Africa", "ZA"}, {"Thailand", "TH"}, {"Spain", "ES"},
	{"Bangladesh", "BD"}, {"Italy", "IT"}, {"Russia", "RU"}, {"Poland", "PL"},
	{"Netherlands", "NL"}, {"Belgium", "BE"}, {"Sweden", "SE"}, {"Norway", "NO"},
	{"Denmark", "DK"}, {"Finland", "FI"}, {"Portugal", "PT"}, {"Austria", "AT"},
	{"Greece", "GR"}, {"Czechia", "CZ"}, {"Hungary", "HU"}, {"Ireland", "IE"},
	{"Pakistan", "PK"}, {"Argentina", "AR"}, {"Bolivia", "BO"}, {"Chile", "CL"},
	{"Colombia", "CO"}, {"Ecuador", "EC"},
	// "Congo" trong danh sách nghiệp vụ không nói rõ nước nào; CD (CHDC Congo)
	// là nước đông dân hơn hẳn nên lấy nó.
	{"Congo", "CD"},
	{"Paraguay", "PY"}, {"Peru", "PE"}, {"Nepal", "NP"}, {"Uruguay", "UY"},
	{"Venezuela", "VE"}, {"Egypt", "EG"}, {"Algeria", "DZ"},
}

// lowestPriority là chỗ đứng của mọi quốc gia không có trong danh sách trên.
// Đủ lớn để không bao giờ đụng vào nhóm ưu tiên, và cùng một giá trị cho tất cả
// để chúng tự sắp theo tên (xem ORDER BY sort_order, name).
const lowestPriority = 9999

// CountrySortOrder trả vị trí sắp xếp của 1 quốc gia.
//
// CÓ MÃ ISO thì mã là câu trả lời DUY NHẤT — không so tên nữa. Đây là điểm dễ
// sai: so tên như một đường dự phòng nghe thì an toàn, nhưng "United States
// Minor Outlying Islands" bắt đầu đúng bằng "United States" và thế là nó chen
// lên hạng 2, đứng trước cả chính nước Mỹ. Một hàng đã khai mã mà mã đó không
// nằm trong danh sách thì nó KHÔNG phải nước ưu tiên, hết chuyện.
//
// So tên chỉ dành cho hàng không có mã, và phải khớp CHÍNH XÁC sau khi chuẩn
// hoá — không khớp tiền tố, vì cùng lý do trên.
func CountrySortOrder(name, code string) int32 {
	if iso := strings.ToUpper(strings.TrimSpace(code)); iso != "" {
		for i, want := range countryPriority {
			if want.code == iso {
				return int32(i)
			}
		}
		return lowestPriority
	}

	key := normalizeCountryName(name)
	for i, want := range countryPriority {
		if key == normalizeCountryName(want.name) {
			return int32(i)
		}
	}
	return lowestPriority
}

func normalizeCountryName(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(s))), " ")
}

// ---------------------------------------------------------------------------
// Ngôn ngữ
// ---------------------------------------------------------------------------

// countryLanguage map quốc gia trong danh sách ưu tiên sang ngôn ngữ chính của
// nó, để thứ tự ô Ngôn ngữ bám theo đúng thứ tự ô Quốc gia.
//
// Ngôn ngữ lặp lại ở nhiều quốc gia thì chỉ tính lần XUẤT HIỆN ĐẦU TIÊN: en đã
// lên hạng 2 nhờ United States, nên United Kingdom / Singapore / Canada không
// đẩy nó lên nữa và cũng không tự sinh ra thứ hạng mới.
var countryLanguage = map[string]string{
	"Vietnam": "vi", "United States": "en", "United Kingdom": "en",
	"France": "fr", "Germany": "de", "Japan": "ja", "South Korea": "ko",
	"China": "zh-CN", "Switzerland": "de", "Singapore": "en", "Canada": "en",
	"India": "hi", "Brazil": "pt-BR", "Australia": "en", "South Africa": "af",
	"Thailand": "th", "Spain": "es", "Bangladesh": "bn", "Italy": "it",
	"Russia": "ru", "Poland": "pl", "Netherlands": "nl", "Belgium": "nl",
	"Sweden": "sv", "Norway": "no", "Denmark": "da", "Finland": "fi",
	"Portugal": "pt", "Austria": "de", "Greece": "el", "Czechia": "cs",
	"Hungary": "hu", "Ireland": "ga", "Pakistan": "ur", "Argentina": "es",
	"Bolivia": "es", "Chile": "es", "Colombia": "es", "Ecuador": "es",
	"Congo": "fr", "Paraguay": "es", "Peru": "es", "Nepal": "ne",
	"Uruguay": "es", "Venezuela": "es", "Egypt": "ar", "Algeria": "ar",
}

// LanguageOrder là danh sách mã ngôn ngữ theo đúng thứ tự ưu tiên, suy ra từ
// countryPriority. Ngôn ngữ không có trong danh sách này xếp phía dưới, giữ
// nguyên thứ tự vốn có của chúng.
//
// Trả về mảng (không phải map) vì frontend cần đúng thứ tự, và tính một lần lúc
// khởi động thay vì mỗi request.
var LanguageOrder = buildLanguageOrder()

func buildLanguageOrder() []string {
	seen := make(map[string]bool, len(countryPriority))
	out := make([]string, 0, len(countryPriority))
	for _, country := range countryPriority {
		lang := countryLanguage[country.name]
		if lang == "" || seen[lang] {
			continue
		}
		seen[lang] = true
		out = append(out, lang)
	}
	return out
}
