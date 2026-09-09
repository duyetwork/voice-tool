package platform

import (
	"regexp"
	"strings"
)

var (
	vttTimestampRe = regexp.MustCompile(`^\d{2}:\d{2}:\d{2}[.,]\d{3}\s*-->`)
	vttTagRe       = regexp.MustCompile(`</?[cviub][^>]*>|<\d{2}:\d{2}:\d{2}\.\d{3}>`)
	vttIndexRe     = regexp.MustCompile(`^\d+$`)
)

// ParseVTT gộp phụ đề WebVTT thành 1 đoạn text liền mạch, bỏ timestamp/tag và
// các dòng trùng lặp do auto-caption cuộn chữ.
func ParseVTT(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")

	var out []string
	var last string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		switch {
		case line == "",
			line == "WEBVTT",
			strings.HasPrefix(line, "NOTE"),
			strings.HasPrefix(line, "Kind:"),
			strings.HasPrefix(line, "Language:"),
			vttTimestampRe.MatchString(line),
			vttIndexRe.MatchString(line):
			continue
		}

		line = strings.TrimSpace(vttTagRe.ReplaceAllString(line, ""))
		if line == "" || line == last {
			continue
		}
		out = append(out, line)
		last = line
	}
	return strings.Join(out, " ")
}
