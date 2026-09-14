package service

import (
	"context"
	"log/slog"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/domain"
	"github.com/strongbody/voice-tool/backend/internal/repository"
)

const (
	// countryTTL — danh mục quốc gia đổi vài năm một lần; hỏi lại mỗi ngày đã là
	// thừa thãi, nhưng đủ rẻ để không phải nghĩ thêm về việc làm mới bằng tay.
	countryTTL = 24 * time.Hour
	// hashtagTTL — danh mục hashtag bên MultiMe là tập do người dùng bên đó tạo
	// ra liên tục, nhưng ~94.000 mục thì đồng bộ lại mỗi ngày một lần là đủ:
	// tag mới vẫn gõ tay thêm được ngay tại ô chọn.
	hashtagTTL = 24 * time.Hour
	// hashtagPageSize — số mục mỗi trang khi đồng bộ. 500 là cân bằng giữa số
	// request (~190 trang) và kích thước mỗi response.
	hashtagPageSize = 500
	// hashtagSyncTimeout — trần cho cả lượt đồng bộ nền.
	hashtagSyncTimeout = 30 * time.Minute
	// maxHashtags — trần số tag TRẢ VỀ mỗi lần hỏi. Không phải trần của danh
	// mục: danh mục nằm trong DB, đây chỉ là số dòng hiện trong dropdown.
	maxHashtags = 200
)

// CatalogCache phục vụ các danh mục mà modal Tạo Voice cần: Quốc gia, Hashtag,
// và thứ tự ưu tiên của Ngôn ngữ.
//
// Tồn tại để modal KHÔNG phải gọi API mỗi lần mở. Trước đây ô Quốc gia hỏi
// thẳng Strongbody, nghĩa là mở modal cũng phụ thuộc vào việc token bên đó còn
// hạn hay không — trong khi danh mục ấy đổi vài năm một lần.
//
// Hashtag thì không có danh mục nào để hỏi: client Strongbody hiện tại chỉ GỬI
// hashtags lúc đăng bài chứ không đọc về. Nguồn duy nhất có thật là chính dữ
// liệu hệ thống đã tích (Voice đã tạo, Bài Post đã lấy) — và vì mỗi Voice mang
// sẵn cả hashtag lẫn ngôn ngữ nên quan hệ tag ↔ ngôn ngữ là thứ QUAN SÁT ĐƯỢC,
// không phải dữ liệu bịa ra.
type CatalogCache struct {
	q     *repository.Queries
	users *MultimeUsers
	log   *slog.Logger

	// syncing chặn nhiều request cùng lúc cùng đi đồng bộ danh mục quốc gia.
	syncing sync.Mutex
	// hashtagSync tách riêng vì lượt đồng bộ hashtag chạy NỀN hàng chục phút —
	// dùng chung khoá với quốc gia thì nó khoá luôn cả việc đồng bộ quốc gia.
	hashtagSync sync.Mutex
}

func NewCatalogCache(q *repository.Queries, users *MultimeUsers, log *slog.Logger) *CatalogCache {
	return &CatalogCache{q: q, users: users, log: log}
}

// CountryView là 1 quốc gia đã kèm thứ tự hiển thị.
type CountryView struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	Code string `json:"code,omitempty"`
}

// HashtagView là 1 tag trong danh mục, đồng bộ từ MultiMe.
//
// Không có trường ngôn ngữ: bên MultiMe hashtag là `category` và entity đó
// không hề có ngôn ngữ (xem domain.MultimeHashtag).
type HashtagView struct {
	// ID là id category bên MultiMe — giữ để đối chiếu, tag gửi lúc đăng vẫn là
	// chuỗi tên.
	ID  int64  `json:"id"`
	Tag string `json:"tag"`
	// Slug là dạng MultiMe dùng trong URL.
	Slug string `json:"slug"`
	// Kind: system | user | campaign. `system` là tag MultiMe tuyển chọn.
	Kind       string `json:"kind"`
	IsFeatured bool   `json:"is_featured"`
}

// CatalogView gom cả 3 danh mục vào 1 response.
//
// Một lần gọi thay vì ba: modal cần cả ba cùng lúc, và mỗi lần tải là một lần
// chờ. Frontend cache nguyên khối này nên mở modal lần sau không gọi lại.
type CatalogView struct {
	Countries []CountryView `json:"countries"`
	// Hashtags là PHẦN ĐẦU danh mục (tag MultiMe tuyển chọn), không phải toàn
	// bộ: danh mục có ~94.000 mục. Tìm phần còn lại qua /meta/hashtags?q=.
	Hashtags []HashtagView `json:"hashtags"`
	// LanguageOrder là mã ngôn ngữ theo thứ tự ưu tiên (suy ra từ thứ tự quốc
	// gia). Frontend dùng để sắp lại danh sách ngôn ngữ tĩnh của nó.
	LanguageOrder []string `json:"language_order"`
}

// Catalog trả cả 3 danh mục, tự đồng bộ phần đã cũ.
//
// `actor` chỉ dùng khi phải hỏi Strongbody (đồng bộ quốc gia lần đầu / hết hạn):
// token là của chính người đang thao tác, tool không mượn quyền của ai.
func (c *CatalogCache) Catalog(ctx context.Context, actor uuid.UUID) (CatalogView, error) {
	countries, err := c.Countries(ctx, actor)
	if err != nil {
		return CatalogView{}, err
	}
	hashtags, err := c.Hashtags(ctx, actor, "")
	if err != nil {
		return CatalogView{}, err
	}
	return CatalogView{
		Countries:     countries,
		Hashtags:      hashtags,
		LanguageOrder: domain.LanguageOrder,
	}, nil
}

// Countries đọc danh mục từ DB, đồng bộ lại từ Strongbody khi rỗng hoặc quá cũ.
func (c *CatalogCache) Countries(ctx context.Context, actor uuid.UUID) ([]CountryView, error) {
	state, err := c.q.CountriesSyncedAt(ctx)
	if err != nil {
		return nil, err
	}
	if state.Total == 0 || time.Since(state.SyncedAt) > countryTTL {
		if err := c.syncCountries(ctx, actor); err != nil {
			// Còn dữ liệu cũ thì cứ dùng: danh mục quốc gia lỗi thời vài ngày
			// vẫn dùng được, còn modal không mở được thì không.
			if state.Total == 0 {
				return nil, err
			}
			c.log.WarnContext(ctx, "không đồng bộ được danh mục quốc gia, dùng bản đã lưu",
				"error", err)
		}
	}

	rows, err := c.q.ListCountries(ctx)
	if err != nil {
		return nil, err
	}
	out := make([]CountryView, 0, len(rows))
	for _, r := range rows {
		out = append(out, CountryView{ID: r.ID, Name: r.Name, Code: deref(r.Code)})
	}
	return out, nil
}

func (c *CatalogCache) syncCountries(ctx context.Context, actor uuid.UUID) error {
	c.syncing.Lock()
	defer c.syncing.Unlock()

	// Người khác vừa đồng bộ xong trong lúc ta chờ khoá thì thôi.
	if state, err := c.q.CountriesSyncedAt(ctx); err == nil &&
		state.Total > 0 && time.Since(state.SyncedAt) <= countryTTL {
		return nil
	}

	countries, err := c.users.Countries(ctx, actor)
	if err != nil {
		return err
	}
	for _, country := range countries {
		if err := c.q.UpsertCountry(ctx, repository.UpsertCountryParams{
			ID:        country.ID,
			Name:      country.Name,
			Code:      nilIfEmpty(country.Code),
			SortOrder: domain.CountrySortOrder(country.Name, country.Code),
		}); err != nil {
			return err
		}
	}
	c.log.InfoContext(ctx, "đồng bộ danh mục quốc gia", "total", len(countries))
	return nil
}

// Hashtags trả danh mục tag đã đồng bộ từ MultiMe.
//
// `q` rỗng = phần đầu danh mục (tag MultiMe tuyển chọn). Có `q` thì lọc NGAY
// TRONG DB chứ không đẩy hết về trình duyệt: danh mục có ~94.000 mục, tải hết
// một lượt là vài MB cho mỗi lần mở modal.
//
// KHÔNG lọc theo ngôn ngữ, vì không có gì để lọc: bên MultiMe hashtag là
// `category` và entity đó không có trường ngôn ngữ nào (xem
// domain.MultimeHashtag). Bịa ra một quan hệ ở đây sẽ giấu mất phần lớn tag
// khỏi người dùng dựa trên một tiêu chí không tồn tại.
func (c *CatalogCache) Hashtags(ctx context.Context, actor uuid.UUID, q string) ([]HashtagView, error) {
	c.ensureHashtags(ctx, actor)

	rows, err := c.q.ListHashtags(ctx, repository.ListHashtagsParams{
		Q:   normalizeHashtagQuery(q),
		Lim: maxHashtags,
	})
	if err != nil {
		return nil, err
	}
	out := make([]HashtagView, 0, len(rows))
	for _, r := range rows {
		out = append(out, HashtagView{
			ID:         r.ID,
			Tag:        r.Name,
			Slug:       r.Slug,
			Kind:       r.VoiceTagKind,
			IsFeatured: r.IsFeatured,
		})
	}
	return out, nil
}

// normalizeHashtagQuery đưa từ khoá về đúng dạng `normalized_name` của MultiMe
// (chữ thường, không khoảng trắng) để so khớp tiền tố ăn được index.
func normalizeHashtagQuery(q string) string {
	q = strings.ToLower(strings.TrimSpace(q))
	q = strings.ReplaceAll(q, "#", "")
	return strings.Join(strings.Fields(q), "")
}

// ensureHashtags khởi động đồng bộ khi danh mục rỗng hoặc quá cũ.
//
// Đồng bộ CHẠY NỀN vì nó là ~190 request phân trang: bắt một lần mở modal chờ
// chừng ấy là không dùng được. Trang ĐẦU chạy đồng bộ ngay (nhanh, và nhờ
// order_by=id ASC nó chính là nhóm tag MultiMe tuyển chọn), phần đuôi dài chạy
// tiếp phía sau — người dùng có cái để chọn ngay, danh mục đầy dần.
func (c *CatalogCache) ensureHashtags(ctx context.Context, actor uuid.UUID) {
	state, err := c.q.HashtagsSyncedAt(ctx)
	if err != nil {
		c.log.WarnContext(ctx, "không đọc được mốc đồng bộ hashtag", "error", err)
		return
	}
	if state.Total > 0 && time.Since(state.SyncedAt) <= hashtagTTL {
		return
	}
	if !c.hashtagSync.TryLock() {
		return // đã có người đang đồng bộ
	}

	// Trang đầu: chạy ngay để modal có dữ liệu dùng được.
	page, err := c.syncHashtagPage(ctx, actor, 1)
	if err != nil {
		c.hashtagSync.Unlock()
		c.log.WarnContext(ctx, "không đồng bộ được danh mục hashtag", "error", err)
		return
	}

	// Phần còn lại: context riêng, vì request tạo ra nó kết thúc trước.
	go func() {
		defer c.hashtagSync.Unlock()
		bg, cancel := context.WithTimeout(context.WithoutCancel(ctx), hashtagSyncTimeout)
		defer cancel()

		for p := 2; p <= page.totalPages; p++ {
			if _, err := c.syncHashtagPage(bg, actor, p); err != nil {
				c.log.WarnContext(bg, "dừng đồng bộ hashtag giữa chừng",
					"error", err, "trang", p, "tổng_trang", page.totalPages)
				return
			}
		}
		c.log.InfoContext(bg, "đồng bộ xong danh mục hashtag",
			"tổng", page.total, "số_trang", page.totalPages)
	}()
}

type hashtagPage struct {
	total      int
	totalPages int
}

func (c *CatalogCache) syncHashtagPage(
	ctx context.Context,
	actor uuid.UUID,
	page int,
) (hashtagPage, error) {
	tags, total, err := c.users.VoiceHashtags(ctx, actor, page, hashtagPageSize)
	if err != nil {
		return hashtagPage{}, err
	}
	for _, t := range tags {
		if err := c.q.UpsertHashtag(ctx, repository.UpsertHashtagParams{
			ID:             t.ID,
			Name:           t.Name,
			NormalizedName: t.NormalizedName,
			Slug:           t.Slug,
			VoiceTagKind:   t.VoiceTagKind,
			IsFeatured:     t.IsFeatured,
			Ordering:       t.Ordering,
		}); err != nil {
			return hashtagPage{}, err
		}
	}

	pages := (total + hashtagPageSize - 1) / hashtagPageSize
	return hashtagPage{total: total, totalPages: pages}, nil
}
