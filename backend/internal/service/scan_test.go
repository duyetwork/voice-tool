package service

import (
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func TestNormalizePatternsOR(t *testing.T) {
	patterns, err := normalizePatterns([]string{"tin nóng", "#khancap", `\bCPI\b`})
	if err != nil {
		t.Fatalf("normalizePatterns lỗi: %v", err)
	}
	if len(patterns) != 3 {
		t.Fatalf("muốn 3 pattern, được %d: %v", len(patterns), patterns)
	}

	compiled, err := compileAll(patterns)
	if err != nil {
		t.Fatalf("compileAll lỗi: %v", err)
	}

	// Khớp bất kỳ 1 pattern là bắt bài.
	for _, text := range []string{
		"TIN NÓNG: cháy lớn",
		"Thông báo #khancap từ ban chỉ đạo",
		"Chỉ số CPI tháng 8 tăng",
	} {
		if !matchAny(compiled, text) {
			t.Errorf("phải khớp: %q", text)
		}
	}
	if matchAny(compiled, "hôm nay trời đẹp") {
		t.Error("không được khớp text không liên quan")
	}
}

func TestNormalizePatternsDedup(t *testing.T) {
	patterns, err := normalizePatterns([]string{"bão", "bão", " bão "})
	if err != nil {
		t.Fatalf("normalizePatterns lỗi: %v", err)
	}
	if len(patterns) != 1 {
		t.Errorf("pattern trùng phải bị loại, được %v", patterns)
	}
}

func TestNormalizePatternsRejects(t *testing.T) {
	if _, err := normalizePatterns(nil); err == nil {
		t.Error("danh sách rỗng phải trả lỗi")
	}
	if _, err := normalizePatterns([]string{"   "}); err == nil {
		t.Error("toàn khoảng trắng phải trả lỗi")
	}
	if _, err := normalizePatterns([]string{"(chưa đóng"}); err == nil {
		t.Error("regex sai cú pháp phải trả lỗi")
	}

	tooMany := make([]string, maxRegexPatterns+1)
	for i := range tooMany {
		tooMany[i] = "kw" + string(rune('a'+i%26))
	}
	if _, err := normalizePatterns(tooMany); err == nil {
		t.Error("vượt trần số pattern phải trả lỗi")
	}
}

func TestNewerThan(t *testing.T) {
	posts := []domain.RemotePost{{PostID: "p5"}, {PostID: "p4"}, {PostID: "p3"}}

	if got := newerThan(posts, nil); len(got) != 3 {
		t.Errorf("lần quét đầu phải lấy hết, được %d", len(got))
	}
	if got := newerThan(posts, ptr("p4")); len(got) != 1 || got[0].PostID != "p5" {
		t.Errorf("chỉ lấy bài mới hơn mốc, được %v", got)
	}
	if got := newerThan(posts, ptr("p5")); len(got) != 0 {
		t.Errorf("mốc là bài mới nhất thì không còn gì, được %v", got)
	}
	// Mốc không còn trong danh sách (kênh đăng nhiều bài giữa 2 lần quét).
	if got := newerThan(posts, ptr("p0")); len(got) != 3 {
		t.Errorf("mốc lệch thì lấy hết để không bỏ sót, được %d", len(got))
	}
}

func TestRolePermissions(t *testing.T) {
	cases := []struct {
		role                  domain.Role
		write, delete, manage bool
	}{
		{domain.RoleAdmin, true, true, true},
		// User bình thường: đăng được voice nhưng KHÔNG xoá được.
		{domain.RoleUser, true, false, false},
		{domain.RoleViewer, false, false, false},
	}

	for _, c := range cases {
		if c.role.CanWrite() != c.write {
			t.Errorf("%s CanWrite = %v, muốn %v", c.role, c.role.CanWrite(), c.write)
		}
		if c.role.CanDelete() != c.delete {
			t.Errorf("%s CanDelete = %v, muốn %v", c.role, c.role.CanDelete(), c.delete)
		}
		if c.role.CanManageUsers() != c.manage {
			t.Errorf("%s CanManageUsers = %v, muốn %v", c.role, c.role.CanManageUsers(), c.manage)
		}
	}

	for _, invalid := range []domain.Role{"superuser", "editor", ""} {
		if invalid.Valid() {
			t.Errorf("role %q phải bị từ chối", invalid)
		}
	}
}
