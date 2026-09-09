// Package validator chứa các validate dùng chung, trọng tâm là regex pattern.
package validator

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// maxPatternLen chặn pattern quá dài (rủi ro hiệu năng khi quét liên tục).
const maxPatternLen = 512

// NormalizePattern chuẩn hoá input của user về regex (business rule #3).
//
// Người dùng có thể gõ:
//   - hashtag:  "#tinnong"       -> "#tinnong\b"
//   - từ khoá:  "tin nóng"       -> "(?i)tin\s+nóng"
//   - nhiều từ: "a, b"           -> "(?i)(a|b)"
//   - regex sẵn: "/^BREAKING/i"  -> giữ nguyên phần thân, map cờ i
//
// Trả về pattern đã compile được, để lưu thẳng vào cột regex_pattern.
func NormalizePattern(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", fmt.Errorf("%w: regex pattern không được rỗng", domain.ErrInvalidInput)
	}

	pattern := s
	switch {
	case isDelimitedRegex(s):
		var err error
		pattern, err = unwrapDelimitedRegex(s)
		if err != nil {
			return "", err
		}
	case looksLikeRegex(s):
		// User gõ regex thuần — dùng nguyên văn.
	case strings.HasPrefix(s, "#"):
		pattern = regexp.QuoteMeta(s) + `\b`
	case strings.ContainsAny(s, ",;"):
		parts := splitList(s)
		if len(parts) == 0 {
			return "", fmt.Errorf("%w: danh sách từ khoá rỗng", domain.ErrInvalidInput)
		}
		quoted := make([]string, 0, len(parts))
		for _, p := range parts {
			quoted = append(quoted, keywordToPattern(p))
		}
		pattern = `(?i)(` + strings.Join(quoted, "|") + `)`
	default:
		pattern = `(?i)` + keywordToPattern(s)
	}

	if err := Validate(pattern); err != nil {
		return "", err
	}
	return pattern, nil
}

// Validate kiểm tra regex compile được và không quá dài (specs 1.4).
func Validate(pattern string) error {
	if len(pattern) > maxPatternLen {
		return fmt.Errorf("%w: pattern dài quá %d ký tự", domain.ErrRegexInvalid, maxPatternLen)
	}
	if _, err := regexp.Compile(pattern); err != nil {
		return fmt.Errorf("%w: %s", domain.ErrRegexInvalid, err.Error())
	}
	return nil
}

// Compile trả về regex đã compile để engine so khớp.
// RE2 (regexp của Go) không backtrack nên không có catastrophic backtracking.
func Compile(pattern string) (*regexp.Regexp, error) {
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, fmt.Errorf("%w: %s", domain.ErrRegexInvalid, err.Error())
	}
	return re, nil
}

// keywordToPattern: khoảng trắng trong từ khoá khớp linh hoạt 1+ whitespace.
func keywordToPattern(kw string) string {
	fields := strings.Fields(kw)
	quoted := make([]string, 0, len(fields))
	for _, f := range fields {
		quoted = append(quoted, regexp.QuoteMeta(f))
	}
	return strings.Join(quoted, `\s+`)
}

func splitList(s string) []string {
	raw := strings.FieldsFunc(s, func(r rune) bool { return r == ',' || r == ';' })
	out := make([]string, 0, len(raw))
	for _, r := range raw {
		if t := strings.TrimSpace(r); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// isDelimitedRegex nhận dạng dạng /pattern/flags.
func isDelimitedRegex(s string) bool {
	return len(s) > 2 && strings.HasPrefix(s, "/") && strings.LastIndex(s, "/") > 0
}

func unwrapDelimitedRegex(s string) (string, error) {
	end := strings.LastIndex(s, "/")
	body := s[1:end]
	flags := s[end+1:]
	if body == "" {
		return "", fmt.Errorf("%w: thân regex rỗng", domain.ErrRegexInvalid)
	}
	var goFlags string
	for _, f := range flags {
		switch f {
		case 'i':
			goFlags += "i"
		case 'm':
			goFlags += "m"
		case 's':
			goFlags += "s"
		default:
			return "", fmt.Errorf("%w: cờ regex %q không hỗ trợ", domain.ErrRegexInvalid, f)
		}
	}
	if goFlags != "" {
		return "(?" + goFlags + ")" + body, nil
	}
	return body, nil
}

// looksLikeRegex: có metacharacter -> coi như user chủ động viết regex.
func looksLikeRegex(s string) bool {
	return strings.ContainsAny(s, `\^$.|?*+()[]{}`)
}
