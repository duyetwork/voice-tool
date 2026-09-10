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
//	URL công khai: https://multime.ai/voice/<id>
package multime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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
	defaultHashtags  []string
	isPublicDownload bool

	http *http.Client
}

var (
	_ domain.MultimeClient        = (*Client)(nil)
	_ domain.MultimeAuthenticator = (*Client)(nil)
)

func New(cfg *config.Config) *Client {
	return &Client{
		voiceBaseURL:     strings.TrimRight(cfg.MultimeBaseURL, "/"),
		authBaseURL:      strings.TrimRight(cfg.MultimeAuthBaseURL, "/"),
		siteURL:          strings.TrimRight(cfg.MultimeSiteURL, "/"),
		visibility:       cfg.MultimeVisibility,
		categoryIDs:      cfg.MultimeCategoryIDs(),
		defaultHashtags:  cfg.MultimeDefaultHashtags(),
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
		if isUnauthorized(err) {
			return domain.MultimeSession{}, domain.ErrUnauthorized
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

// PublishVoice upload audio và tạo Voice Post trong 1 request, dùng credential
// của chính user sở hữu voice.
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

	url, err := c.upload(ctx, creds.AccessToken, creds.AuthorID, audio, post)
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
	if len(post.Hashtags) == 0 && len(post.CategoryIDs) == 0 &&
		len(c.defaultHashtags) == 0 && len(c.categoryIDs) == 0 {
		return domain.Permanent(fmt.Errorf(
			"%w: multime yêu cầu ít nhất 1 hashtag hoặc category — thêm hashtag cho voice, "+
				"hoặc đặt MULTIME_DEFAULT_HASHTAGS", domain.ErrInvalidInput))
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

	hashtags := post.Hashtags
	if len(hashtags) == 0 {
		hashtags = c.defaultHashtags
	}
	for _, h := range hashtags {
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
		e := fmt.Errorf("multime %s trả về %d: %s",
			req.URL.Path, resp.StatusCode, truncate(string(raw), 500))
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
