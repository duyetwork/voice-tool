// Package multime là adapter tới nền tảng đích multime.ai.
//
// Hợp đồng API được đối chiếu từ repo multime-ai (trang /studio/upload →
// components/Studio/UploadVoice.tsx → lib/studioActions.ts):
//
//	Đăng nhập:
//	  POST {auth_base}/v1/public/auth/login
//	  headers: Content-Type: application/json, Scope: strongbody-ai
//	  body:    {"email":…,"password":…}
//	  resp:    {"code":0,"data":{"token":{"accessToken":…},"user":{"user":{"id":…}}}}
//
//	Đăng voice (upload audio + tạo bài đăng trong CÙNG 1 request):
//	  POST {voice_base}/v1/seller/voice-posts/upload
//	  headers: accept: application/json
//	           authorization: Bearer <accessToken>
//	           scope: strongbody-ai
//	  form:    audio_file, author_id, title, caption, source_lang, lang,
//	           visibility, is_public_download, category_id, category_ids[],
//	           hashtags[], image, scheduled_at
//	  resp:    {"code":0,"data":{"id":…, …}}  (code 0 = thành công)
//
//	`caption` KHÔNG được gửi: form đăng của chính multime luôn gửi caption
//	rỗng và giao diện chỉ đọc `title`, nên tiêu đề là toàn bộ phần chữ của
//	bài đăng — tối đa 200 ký tự, 1 dòng (maxLength của ô tiêu đề bên đó).
//
//	Danh bạ tài khoản (bốc tác giả bài đăng theo giới tính):
//	  GET {auth_base}/v1/admin/user?page=&limit=&order_by=id&order_dir=DESC
//	                               &filter_names=gender&filter_values=<male|female|other>
//	  headers: authorization: Bearer <accessToken>, scope: strongbody-ai
//	  resp:    {"code":0,"data":{"total":…,"data":[{"id":…,"email":…,"gender":…}]}}
//	  (đối chiếu strongbody-api: api/user/v1.ListUsersReq + dto.BuildWhere, nơi
//	   filter_names/filter_values thành mệnh đề `users.<name> = <value>`)
//
//	URL công khai: https://multime.ai/voice/<id>
package multime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

const (
	// scopeHeader là giá trị header Scope mà backend yêu cầu cho toàn bộ
	// voice API (multime-ai dùng 'strongbody-ai' ở mọi request).
	scopeHeader = "strongbody-ai"

	loginPath   = "/v1/public/auth/login"
	refreshPath = "/v1/admin/auth/refresh-token"
	uploadPath  = "/v1/seller/voice-posts/upload"
	usersPath   = "/v1/admin/user"
	// Danh mục quốc gia: KHÔNG dùng /v1/admin/countries — endpoint đó trả
	// {"code":403,"message":"unauthorized application"} với token của tài khoản
	// thường. Bản /buyer đọc được và cùng một bảng dữ liệu.
	countriesPath = "/v1/buyer/countries"

	// minDurationSeconds: UI của multime chặn voice ngắn hơn 15 giây.
	minDurationSeconds = 15
)

// Client dùng chung cho cả đăng nhập và đăng voice — cùng 1 hệ thống, cùng 1
// header Scope.
type Client struct {
	voiceBaseURL string
	authBaseURL  string
	siteURL      string

	visibility       string
	categoryIDs      []int64
	isPublicDownload bool

	http *http.Client
}

var (
	_ domain.MultimeClient        = (*Client)(nil)
	_ domain.MultimeAuthenticator = (*Client)(nil)
	_ domain.MultimeDirectory     = (*Client)(nil)
)

func New(cfg *config.Config) *Client {
	return &Client{
		voiceBaseURL:     strings.TrimRight(cfg.MultimeBaseURL, "/"),
		authBaseURL:      strings.TrimRight(cfg.MultimeAuthBaseURL, "/"),
		siteURL:          strings.TrimRight(cfg.MultimeSiteURL, "/"),
		visibility:       cfg.MultimeVisibility,
		categoryIDs:      cfg.MultimeCategoryIDs(),
		isPublicDownload: cfg.MultimePublicDownload,
		http:             &http.Client{Timeout: 10 * time.Minute},
	}
}

// envelope là format response chung của backend strongbody (GoFrame).
type envelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

// ---------------------------------------------------------------------------
// Đăng nhập
// ---------------------------------------------------------------------------

type loginData struct {
	User struct {
		User struct {
			ID        int64  `json:"id"`
			Email     string `json:"email"`
			FirstName string `json:"first_name"`
			LastName  string `json:"last_name"`
			FullName  string `json:"full_name"`
			Avatar    string `json:"profile_picture"`
		} `json:"user"`
		Role string `json:"role"`
	} `json:"user"`
	Token struct {
		AccessToken  string `json:"accessToken"`
		RefreshToken string `json:"refreshToken"`
	} `json:"token"`
	// Tài khoản bật 2FA: login không trả token mà trả pending_token.
	RequiresTOTP      bool `json:"requires_totp"`
	RequiresTOTPSetup bool `json:"requires_totp_setup"`
}

// Login xác thực với strongbody. Đây cũng là tài khoản dùng để đăng voice.
func (c *Client) Login(ctx context.Context, email, password string) (domain.MultimeSession, error) {
	payload, err := json.Marshal(map[string]string{"email": email, "password": password})
	if err != nil {
		return domain.MultimeSession{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.authBaseURL+loginPath,
		bytes.NewReader(payload))
	if err != nil {
		return domain.MultimeSession{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Scope", scopeHeader)

	var data loginData
	if err := c.do(req, &data); err != nil {
		// strongbody trả 400 kèm {"code":40014,"message":"Password is
		// incorrect"} khi sai mật khẩu, không phải 401. Ở endpoint đăng nhập
		// thì MỌI 4xx đều có nghĩa "không xác thực được" — trả 500 kèm nguyên
		// văn lỗi của họ vừa làm người dùng tưởng hệ thống hỏng, vừa dội cảnh
		// báo giả vào giám sát.
		if s := statusOf(err); isUnauthorized(err) || (s >= 400 && s < 500) {
			return domain.MultimeSession{}, domain.Explain("Sai email hoặc mật khẩu",
				fmt.Errorf("%w: %v", domain.ErrUnauthorized, err))
		}
		return domain.MultimeSession{}, fmt.Errorf("đăng nhập multime: %w", err)
	}
	if data.RequiresTOTP || data.RequiresTOTPSetup {
		return domain.MultimeSession{}, domain.ErrTOTPRequired
	}
	if data.Token.AccessToken == "" || data.User.User.ID == 0 {
		return domain.MultimeSession{}, fmt.Errorf("login multime trả về thiếu token hoặc user id")
	}

	u := data.User.User
	return domain.MultimeSession{
		UserID:       u.ID,
		Email:        firstNonEmpty(u.Email, email),
		FullName:     firstNonEmpty(u.FullName, strings.TrimSpace(u.FirstName+" "+u.LastName)),
		AvatarURL:    u.Avatar,
		AccessToken:  data.Token.AccessToken,
		RefreshToken: data.Token.RefreshToken,
	}, nil
}

// RefreshAccessToken đổi refresh token thành access token mới.
//
// Endpoint nằm dưới nhóm /admin nhưng nhận cả scope strongbody-ai (xem
// internal/router/admin/auth/auth.go của strongbody-api), và refresh token
// được gửi ở header Authorization.
func (c *Client) RefreshAccessToken(ctx context.Context, refreshToken string) (string, error) {
	if refreshToken == "" {
		return "", domain.ErrReloginRequired
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.authBaseURL+refreshPath, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+refreshToken)
	req.Header.Set("Scope", scopeHeader)

	var data struct {
		AccessToken string `json:"accessToken"`
	}
	if err := c.do(req, &data); err != nil {
		if isUnauthorized(err) {
			return "", domain.ErrReloginRequired
		}
		return "", fmt.Errorf("refresh token multime: %w", err)
	}
	if data.AccessToken == "" {
		return "", domain.ErrReloginRequired
	}
	return data.AccessToken, nil
}

// ---------------------------------------------------------------------------
// Đăng voice
// ---------------------------------------------------------------------------

// PublishVoice upload audio và tạo Voice Post trong 1 request.
//
// creds là của người bấm nút đăng (token gọi API); post.AuthorID là tài khoản
// ĐỨNG TÊN bài đăng, do người dùng chọn — hai thứ này không nhất thiết trùng
// nhau, nên không suy cái nọ ra cái kia.
func (c *Client) PublishVoice(
	ctx context.Context,
	creds domain.MultimeCredentials,
	audio []byte,
	post domain.VoicePostInput,
) (string, error) {
	if creds.AccessToken == "" || creds.AuthorID == 0 {
		return "", domain.ErrReloginRequired
	}
	if err := c.validate(post); err != nil {
		return "", err
	}

	url, err := c.upload(ctx, creds.AccessToken, post.AuthorID, audio, post)
	if err != nil && isUnauthorized(err) {
		// Caller (Engine) sẽ refresh token rồi gọi lại.
		return "", fmt.Errorf("%w: %v", domain.ErrTokenExpired, err)
	}
	return url, err
}

// validate chặn trước các điều kiện mà API sẽ từ chối, để lỗi hiện ra dưới
// dạng thông báo rõ ràng thay vì HTTP 400 khó hiểu.
func (c *Client) validate(post domain.VoicePostInput) error {
	if strings.TrimSpace(post.Title) == "" {
		return domain.Permanent(fmt.Errorf("%w: multime yêu cầu title cho voice post",
			domain.ErrInvalidInput))
	}
	if n := len([]rune(strings.TrimSpace(post.Title))); n > domain.MaxVoiceTitleRunes {
		return domain.Permanent(fmt.Errorf(
			"%w: tiêu đề %d ký tự, multime chỉ nhận tối đa %d",
			domain.ErrInvalidInput, n, domain.MaxVoiceTitleRunes))
	}
	if post.AuthorID <= 0 {
		return domain.Permanent(fmt.Errorf(
			"%w: chưa chọn tài khoản đứng tên bài đăng (author)", domain.ErrInvalidInput))
	}
	// Không còn hashtag mặc định trong cấu hình: hashtag là phần phân loại của
	// riêng từng bài, điền sẵn một thẻ chung cho mọi voice chỉ làm bẩn multime.
	// Bỏ trống thì chặn ở đây thay vì đăng ra một bài không ai tìm lại được.
	if len(post.Hashtags) == 0 {
		return domain.Permanent(fmt.Errorf(
			"%w: voice chưa có hashtag — multime yêu cầu ít nhất 1 hashtag cho mỗi bài đăng",
			domain.ErrInvalidInput))
	}
	if post.DurationSeconds > 0 && post.DurationSeconds < minDurationSeconds {
		return domain.Permanent(fmt.Errorf(
			"%w: voice dài %ds, multime yêu cầu tối thiểu %ds",
			domain.ErrInvalidInput, post.DurationSeconds, minDurationSeconds))
	}
	return nil
}

func (c *Client) upload(
	ctx context.Context,
	token string,
	authorID int64,
	audio []byte,
	post domain.VoicePostInput,
) (string, error) {
	body, contentType, err := c.buildForm(authorID, audio, post)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.voiceBaseURL+uploadPath, body)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Scope", scopeHeader)
	req.Header.Set("Content-Type", contentType)

	var raw json.RawMessage
	if err := c.do(req, &raw); err != nil {
		return "", err
	}

	id, err := postIDFrom(raw)
	if err != nil {
		return "", err
	}
	return c.siteURL + "/voice/" + id, nil
}

func (c *Client) buildForm(
	authorID int64,
	audio []byte,
	post domain.VoicePostInput,
) (io.Reader, string, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)

	fileName := post.FileName
	if fileName == "" {
		fileName = "voice.mp3"
	}
	part, err := mw.CreateFormFile("audio_file", fileName)
	if err != nil {
		return nil, "", err
	}
	if _, err := part.Write(audio); err != nil {
		return nil, "", err
	}

	_ = mw.WriteField("author_id", strconv.FormatInt(authorID, 10))
	_ = mw.WriteField("title", strings.TrimSpace(post.Title))
	// source_lang luôn gửi: rỗng nghĩa là để backend tự nhận diện.
	_ = mw.WriteField("source_lang", post.SourceLang)
	// lang chỉ gửi khi biết chắc — nếu không, backend điền theo kết quả nhận diện.
	if post.Language != "" {
		_ = mw.WriteField("lang", post.Language)
	}
	_ = mw.WriteField("visibility", firstNonEmpty(post.Visibility, c.visibility, "public"))
	_ = mw.WriteField("is_public_download", strconv.FormatBool(post.IsPublicDownload || c.isPublicDownload))

	for _, h := range post.Hashtags {
		_ = mw.WriteField("hashtags", h)
	}

	categoryIDs := post.CategoryIDs
	if len(categoryIDs) == 0 {
		categoryIDs = c.categoryIDs
	}
	for _, id := range categoryIDs {
		_ = mw.WriteField("category_ids", strconv.FormatInt(id, 10))
	}

	if len(post.ImageBytes) > 0 {
		name := post.ImageName
		if name == "" {
			name = "cover.jpg"
		}
		imagePart, err := mw.CreateFormFile("image", name)
		if err != nil {
			return nil, "", err
		}
		if _, err := imagePart.Write(post.ImageBytes); err != nil {
			return nil, "", err
		}
	}

	if err := mw.Close(); err != nil {
		return nil, "", err
	}
	return &buf, mw.FormDataContentType(), nil
}

// ---------------------------------------------------------------------------
// Danh bạ tài khoản Strongbody
// ---------------------------------------------------------------------------

// randomPickSize là số tài khoản lấy về ở lượt bốc thứ hai.
//
// Không lấy 1 dòng: tài khoản không có email thì không dùng được (email là thứ
// người dùng đối chiếu), nên bốc cả một nhúm rồi chọn trong đó vẫn rẻ hơn gọi
// lại API vài lần.
const randomPickSize = 20

// randomPageAttempts: số trang thử trước khi kết luận danh bạ không có ai dùng
// được. 3 là đủ: tài khoản thiếu email là ngoại lệ, không phải quy luật.
const randomPageAttempts = 3

// userListData là `data` của GET /v1/admin/user (strongbody-api trả
// {total, total_page, current_page, limit, data: []UserRes}).
type userListData struct {
	Total     int `json:"total"`
	TotalPage int `json:"total_page"`
	Data      []struct {
		ID        int64  `json:"id"`
		Email     string `json:"email"`
		Gender    string `json:"gender"`
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
		Avatar    string `json:"profile_picture"`
	} `json:"data"`
}

// RandomUser bốc ngẫu nhiên 1 tài khoản Strongbody theo giới tính.
//
// Hai lượt gọi, và đó là chủ ý: lượt đầu chỉ để biết danh bạ có bao nhiêu tài
// khoản khớp giới tính, lượt sau nhảy thẳng tới MỘT VỊ TRÍ BẤT KỲ trong số đó.
// Lấy đại trang đầu thì mọi voice sẽ đứng tên vài tài khoản mới nhất, không
// còn là ngẫu nhiên.
//
// Bốc theo VỊ TRÍ rồi mới quy ra số trang, chứ không bốc số trang: số trang phụ
// thuộc cỡ trang đang hỏi (`total_page` của lượt thăm dò limit=1 chính là tổng
// số tài khoản), nên lấy số đó làm trang cho lượt sau là trỏ ra ngoài danh sách
// và nhận về trang rỗng.
//
// Gọi bằng token của chính người đang dùng tool: quyền xem danh bạ là quyền bên
// Strongbody cấp cho tài khoản đó, tool không giữ tài khoản dịch vụ nào để
// mượn quyền.
func (c *Client) RandomUser(
	ctx context.Context,
	token string,
	gender domain.Gender,
	countryID int64,
) (domain.MultimeUser, error) {
	if token == "" {
		return domain.MultimeUser{}, domain.ErrReloginRequired
	}
	if !gender.Valid() {
		return domain.MultimeUser{}, domain.Permanent(fmt.Errorf(
			"%w: giới tính phải là male, female hoặc other", domain.ErrInvalidInput))
	}

	probe, err := c.listUsers(ctx, token, gender, countryID, 1, 1)
	if err != nil {
		return domain.MultimeUser{}, err
	}
	total := probe.Total
	if total <= 0 {
		total = probe.TotalPage // trang cỡ 1 dòng -> số trang = số tài khoản
	}
	if total <= 0 {
		return domain.MultimeUser{}, domain.Permanent(fmt.Errorf(
			"%w: Strongbody không có tài khoản nào khớp giới tính %s%s",
			domain.ErrNotFound, gender, countryNote(countryID)))
	}

	// Thử vài lần vì tài khoản thiếu email không dùng được (email là thứ người
	// dùng đối chiếu): rơi trúng một nhúm toàn tài khoản như vậy mà bỏ cuộc
	// ngay thì người dùng thấy lỗi ở một danh bạ hoàn toàn bình thường.
	for attempt := 0; attempt < randomPageAttempts; attempt++ {
		index := rand.IntN(total)
		listed, err := c.listUsers(ctx, token, gender, countryID, index/randomPickSize+1, randomPickSize)
		if err != nil {
			return domain.MultimeUser{}, err
		}

		if user, ok := pickUser(listed, index%randomPickSize, gender); ok {
			return user, nil
		}
	}

	return domain.MultimeUser{}, domain.Permanent(fmt.Errorf(
		"%w: không có tài khoản %s%s nào kèm email để đứng tên bài đăng",
		domain.ErrNotFound, gender, countryNote(countryID)))
}

// pickUser lấy đúng dòng đã bốc trong trang; dòng đó thiếu email thì lấy dòng
// dùng được gần nhất trong cùng trang, thay vì tốn thêm một lượt gọi API.
func pickUser(listed userListData, want int, gender domain.Gender) (domain.MultimeUser, bool) {
	usable := func(i int) (domain.MultimeUser, bool) {
		if i < 0 || i >= len(listed.Data) {
			return domain.MultimeUser{}, false
		}
		u := listed.Data[i]
		if u.ID == 0 || strings.TrimSpace(u.Email) == "" {
			return domain.MultimeUser{}, false
		}
		return domain.MultimeUser{
			ID:       u.ID,
			Email:    u.Email,
			Gender:   firstNonEmpty(u.Gender, string(gender)),
			FullName: strings.TrimSpace(u.FirstName + " " + u.LastName),
			Avatar:   u.Avatar,
		}, true
	}

	if user, ok := usable(want); ok {
		return user, true
	}
	for i := range listed.Data {
		if user, ok := usable(i); ok {
			return user, true
		}
	}
	return domain.MultimeUser{}, false
}

// listUsers gọi GET /v1/admin/user với bộ lọc giới tính.
// countryNote thêm mẩu "ở quốc gia X" vào thông báo lỗi khi có lọc quốc gia —
// không có nó thì người dùng tưởng cả giới tính đó không còn ai.
func countryNote(countryID int64) string {
	if countryID <= 0 {
		return ""
	}
	return fmt.Sprintf(" ở quốc gia #%d", countryID)
}

// Countries lấy danh mục quốc gia để người dùng chọn khi bốc author.
func (c *Client) Countries(ctx context.Context, token string) ([]domain.MultimeCountry, error) {
	if token == "" {
		return nil, domain.ErrReloginRequired
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.authBaseURL+countriesPath, nil)
	if err != nil {
		return nil, err
	}
	q := req.URL.Query()
	q.Set("page", "1")
	// Danh mục quốc gia hữu hạn (~250) nên lấy một lượt, không phân trang.
	q.Set("limit", "300")
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Scope", scopeHeader)

	var data countryListData
	if err := c.do(req, &data); err != nil {
		if isUnauthorized(err) {
			return nil, fmt.Errorf("%w: %v", domain.ErrTokenExpired, err)
		}
		return nil, fmt.Errorf("lấy danh mục quốc gia Strongbody: %w", err)
	}

	// GoFrame trả danh sách ở `data` hoặc `data.data` tuỳ endpoint — nhận cả hai
	// để không phụ thuộc vào chi tiết đó.
	rows := data.Data
	if len(rows) == 0 {
		rows = data.Items
	}

	out := make([]domain.MultimeCountry, 0, len(rows))
	for _, r := range rows {
		name := firstNonEmpty(r.Title, r.Name)
		if r.ID == 0 || name == "" {
			continue
		}
		out = append(out, domain.MultimeCountry{ID: r.ID, Name: name, Code: r.Code})
	}
	return out, nil
}

// countryRow: strongbody-api gọi tên quốc gia là `title` (internal/dto/country.
// CountryDto), không phải `name` — đọc cả hai để khỏi phụ thuộc chi tiết đó.
type countryRow struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Name  string `json:"name"`
	Code  string `json:"code"`
}

type countryListData struct {
	Data  []countryRow `json:"data"`
	Items []countryRow `json:"items"`
}

func (c *Client) listUsers(
	ctx context.Context,
	token string,
	gender domain.Gender,
	countryID int64,
	page, limit int,
) (userListData, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.authBaseURL+usersPath, nil)
	if err != nil {
		return userListData{}, err
	}
	q := req.URL.Query()
	q.Set("page", strconv.Itoa(page))
	q.Set("limit", strconv.Itoa(limit))
	q.Set("order_by", "id")
	q.Set("order_dir", "DESC")
	// filter_names/filter_values là bộ lọc tổng quát của strongbody-api, dịch
	// thẳng thành `users.gender = ?` (xem dto.BuildWhere).
	q.Set("filter_names", "gender")
	q.Set("filter_values", string(gender))
	if countryID > 0 {
		q.Set("country_id", strconv.FormatInt(countryID, 10))
	}
	req.URL.RawQuery = q.Encode()
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Scope", scopeHeader)

	var data userListData
	if err := c.do(req, &data); err != nil {
		if isUnauthorized(err) {
			// Caller (service) refresh token rồi gọi lại.
			return userListData{}, fmt.Errorf("%w: %v", domain.ErrTokenExpired, err)
		}
		return userListData{}, fmt.Errorf("lấy danh sách tài khoản Strongbody: %w", err)
	}
	return data, nil
}

// ---------------------------------------------------------------------------
// HTTP helpers
// ---------------------------------------------------------------------------

// unauthorizedError bọc lỗi 401 để PublishVoice biết cần đăng nhập lại.
type unauthorizedError struct{ err error }

func (e *unauthorizedError) Error() string { return e.err.Error() }
func (e *unauthorizedError) Unwrap() error { return e.err }

func isUnauthorized(err error) bool {
	var ue *unauthorizedError
	return errors.As(err, &ue)
}

// statusError giữ lại mã HTTP để nơi gọi tự quyết định nó nghĩa là gì: cùng
// một mã 400 có thể là "sai mật khẩu" ở endpoint đăng nhập nhưng là "payload
// sai" ở endpoint đăng bài.
type statusError struct {
	status int
	err    error
}

func (e *statusError) Error() string { return e.err.Error() }
func (e *statusError) Unwrap() error { return e.err }

// statusOf rút mã HTTP ra khỏi chuỗi lỗi; 0 nghĩa là lỗi không đến từ HTTP
// (đứt mạng, parse hỏng).
func statusOf(err error) int {
	var se *statusError
	if errors.As(err, &se) {
		return se.status
	}
	return 0
}

func (c *Client) do(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("gọi multime %s: %w", req.URL.Path, err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("đọc response multime: %w", err)
	}

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return &unauthorizedError{err: fmt.Errorf("multime %s trả về %d: %s",
			req.URL.Path, resp.StatusCode, truncate(string(raw), 300))}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		e := error(&statusError{
			status: resp.StatusCode,
			err: fmt.Errorf("multime %s trả về %d: %s",
				req.URL.Path, resp.StatusCode, truncate(string(raw), 500)),
		})
		// 4xx (trừ 429) là lỗi payload/cấu hình -> retry vô nghĩa.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
			return domain.Permanent(e)
		}
		return e
	}

	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return fmt.Errorf("parse envelope multime: %w", err)
	}
	// GoFrame trả code 0 khi thành công.
	if env.Code != 0 {
		return domain.Permanent(fmt.Errorf("multime %s lỗi code=%d: %s",
			req.URL.Path, env.Code, env.Message))
	}
	if out == nil || len(env.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(env.Data, out); err != nil {
		return fmt.Errorf("parse data multime: %w", err)
	}
	return nil
}

// postIDFrom lấy id voice post từ `data`. multime-ai chấp nhận nhiều dạng bọc
// (`data` là post, hoặc lồng trong voice_post/post/item/result) nên xử lý cả.
func postIDFrom(raw json.RawMessage) (string, error) {
	if len(raw) == 0 {
		return "", domain.Permanent(fmt.Errorf("multime không trả về dữ liệu voice post"))
	}

	var node map[string]json.RawMessage
	if err := json.Unmarshal(raw, &node); err != nil {
		return "", fmt.Errorf("parse voice post multime: %w", err)
	}

	if id, ok := numberField(node, "id"); ok {
		return id, nil
	}
	if id, ok := numberField(node, "voice_post_id"); ok {
		return id, nil
	}
	for _, key := range []string{"voice_post", "post", "item", "result", "data"} {
		nested, ok := node[key]
		if !ok {
			continue
		}
		if id, err := postIDFrom(nested); err == nil {
			return id, nil
		}
	}
	return "", domain.Permanent(fmt.Errorf("multime không trả về id của voice post"))
}

func numberField(node map[string]json.RawMessage, key string) (string, bool) {
	rawValue, ok := node[key]
	if !ok {
		return "", false
	}
	var n json.Number
	if err := json.Unmarshal(rawValue, &n); err != nil || n.String() == "" || n.String() == "0" {
		return "", false
	}
	return n.String(), true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
