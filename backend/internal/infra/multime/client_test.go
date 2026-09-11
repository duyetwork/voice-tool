package multime

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

func testCreds() domain.MultimeCredentials {
	return domain.MultimeCredentials{AccessToken: "tok-1", AuthorID: 4242}
}

func validPost() domain.VoicePostInput {
	return domain.VoicePostInput{
		AuthorID: 777, Title: "Bản tin sáng",
		Hashtags: []string{"tinnong"}, DurationSeconds: 42,
	}
}

func TestValidateRejectsWhatAPIWouldReject(t *testing.T) {
	c := &Client{}

	cases := []struct {
		name string
		post domain.VoicePostInput
	}{
		{"thiếu title", domain.VoicePostInput{
			AuthorID: 7, Hashtags: []string{"tin"}, DurationSeconds: 30,
		}},
		// Không còn hashtag mặc định trong cấu hình: thiếu hashtag là chặn,
		// không có gì bù vào.
		{"không hashtag", domain.VoicePostInput{AuthorID: 7, Title: "T", DurationSeconds: 30}},
		{"chưa chọn author", domain.VoicePostInput{
			Title: "T", Hashtags: []string{"tin"}, DurationSeconds: 30,
		}},
		{"ngắn hơn 15s", domain.VoicePostInput{
			AuthorID: 7, Title: "T", Hashtags: []string{"tin"}, DurationSeconds: 9,
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := c.validate(tc.post)
			if err == nil {
				t.Fatal("phải trả lỗi")
			}
			// Lỗi cấu hình/metadata -> retry vô nghĩa.
			if !domain.IsPermanent(err) {
				t.Errorf("phải là PermanentError, được: %v", err)
			}
		})
	}
}

func TestValidateAcceptsValidPost(t *testing.T) {
	if err := (&Client{}).validate(validPost()); err != nil {
		t.Errorf("post hợp lệ bị từ chối: %v", err)
	}
}

// Category cấu hình sẵn KHÔNG còn bù được hashtag thiếu: yêu cầu mới là mỗi
// voice phải có hashtag của riêng nó, bỏ trống thì không đăng.
func TestValidateRejectsMissingHashtagEvenWithCategories(t *testing.T) {
	c := &Client{categoryIDs: []int64{12}}
	err := c.validate(domain.VoicePostInput{AuthorID: 7, Title: "T", DurationSeconds: 30})
	if err == nil {
		t.Fatal("thiếu hashtag phải bị chặn")
	}
	if !strings.Contains(err.Error(), "hashtag") {
		t.Errorf("lỗi phải nói rõ thiếu hashtag, được: %v", err)
	}
}

func TestPublishVoiceRequiresCredentials(t *testing.T) {
	c := &Client{}

	for _, creds := range []domain.MultimeCredentials{
		{},
		{AccessToken: "tok"},
		{AuthorID: 1},
	} {
		_, err := c.PublishVoice(context.Background(), creds, []byte("A"), validPost())
		if !errors.Is(err, domain.ErrReloginRequired) {
			t.Errorf("thiếu credential phải trả ErrReloginRequired, được: %v", err)
		}
	}
}

func TestPostIDFrom(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string
	}{
		{"data là post", `{"id":123,"title":"x"}`, "123"},
		{"lồng trong voice_post", `{"voice_post":{"id":77}}`, "77"},
		{"lồng trong post", `{"post":{"id":88}}`, "88"},
		{"trả voice_post_id", `{"voice_post_id":99}`, "99"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := postIDFrom(json.RawMessage(tc.raw))
			if err != nil {
				t.Fatalf("postIDFrom lỗi: %v", err)
			}
			if got != tc.want {
				t.Errorf("postIDFrom(%s) = %q, muốn %q", tc.raw, got, tc.want)
			}
		})
	}

	if _, err := postIDFrom(json.RawMessage(`{"title":"không có id"}`)); err == nil {
		t.Error("thiếu id phải trả lỗi")
	}
}

func TestSplitHashtags(t *testing.T) {
	cases := []struct {
		raw  string
		want []string
	}{
		{"#tinnong #vietnam", []string{"tinnong", "vietnam"}},
		{"tinnong, vietnam", []string{"tinnong", "vietnam"}},
		{"#a,#b;c", []string{"a", "b", "c"}},
		{"#dup #dup", []string{"dup"}},
		{"", nil},
	}

	for _, tc := range cases {
		got := config.SplitHashtags(tc.raw)
		if len(got) != len(tc.want) {
			t.Errorf("SplitHashtags(%q) = %v, muốn %v", tc.raw, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("SplitHashtags(%q) = %v, muốn %v", tc.raw, got, tc.want)
				break
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Login
// ---------------------------------------------------------------------------

func TestLoginParsesSessionAndSendsScope(t *testing.T) {
	var gotPath, gotScope, gotBody string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotScope = r.Header.Get("Scope")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{
			"user":{"user":{"id":4242,"email":"bot@example.com","first_name":"Voice","last_name":"Bot",
			                "profile_picture":"https://cdn/a.png"},"role":"Seller"},
			"token":{"accessToken":"acc-1","refreshToken":"ref-1"}}}`)
	}))
	defer srv.Close()

	c := &Client{authBaseURL: srv.URL, http: srv.Client()}

	session, err := c.Login(context.Background(), "bot@example.com", "secret123")
	if err != nil {
		t.Fatalf("Login lỗi: %v", err)
	}

	if gotPath != loginPath {
		t.Errorf("path = %q, muốn %q", gotPath, loginPath)
	}
	if gotScope != scopeHeader {
		t.Errorf("scope = %q, muốn %q", gotScope, scopeHeader)
	}
	if !strings.Contains(gotBody, `"email":"bot@example.com"`) {
		t.Errorf("body không chứa email: %s", gotBody)
	}

	if session.UserID != 4242 {
		t.Errorf("UserID = %d, muốn 4242", session.UserID)
	}
	if session.AccessToken != "acc-1" || session.RefreshToken != "ref-1" {
		t.Errorf("token = %q/%q", session.AccessToken, session.RefreshToken)
	}
	// full_name rỗng -> ghép first + last.
	if session.FullName != "Voice Bot" {
		t.Errorf("FullName = %q, muốn %q", session.FullName, "Voice Bot")
	}
	if session.AvatarURL != "https://cdn/a.png" {
		t.Errorf("AvatarURL = %q", session.AvatarURL)
	}
}

func TestLoginMapsWrongPasswordToUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":401,"message":"Invalid email or password"}`)
	}))
	defer srv.Close()

	c := &Client{authBaseURL: srv.URL, http: srv.Client()}

	_, err := c.Login(context.Background(), "a@b.com", "wrong")
	if !errors.Is(err, domain.ErrUnauthorized) {
		t.Errorf("sai mật khẩu phải trả ErrUnauthorized, được: %v", err)
	}
}

// TestLoginDetects2FA: tài khoản bật 2FA không trả token mà trả pending_token.
func TestLoginDetects2FA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"requires_totp":true,"pending_token":"pt"}}`)
	}))
	defer srv.Close()

	c := &Client{authBaseURL: srv.URL, http: srv.Client()}

	_, err := c.Login(context.Background(), "a@b.com", "pw")
	if !errors.Is(err, domain.ErrTOTPRequired) {
		t.Errorf("2FA phải trả ErrTOTPRequired, được: %v", err)
	}
}

func TestRefreshAccessToken(t *testing.T) {
	var gotAuth, gotPath string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"accessToken":"acc-2"}}`)
	}))
	defer srv.Close()

	c := &Client{authBaseURL: srv.URL, http: srv.Client()}

	token, err := c.RefreshAccessToken(context.Background(), "ref-1")
	if err != nil {
		t.Fatalf("RefreshAccessToken lỗi: %v", err)
	}
	if token != "acc-2" {
		t.Errorf("token = %q, muốn acc-2", token)
	}
	// Refresh token đi ở header Authorization, không phải body.
	if gotAuth != "Bearer ref-1" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotPath != refreshPath {
		t.Errorf("path = %q, muốn %q", gotPath, refreshPath)
	}
}

func TestRefreshAccessTokenExpiredNeedsRelogin(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":401,"message":"Refresh token is invalid"}`)
	}))
	defer srv.Close()

	c := &Client{authBaseURL: srv.URL, http: srv.Client()}

	if _, err := c.RefreshAccessToken(context.Background(), "expired"); !errors.Is(err, domain.ErrReloginRequired) {
		t.Errorf("refresh token hết hạn phải trả ErrReloginRequired, được: %v", err)
	}
	if _, err := c.RefreshAccessToken(context.Background(), ""); !errors.Is(err, domain.ErrReloginRequired) {
		t.Error("refresh token rỗng phải trả ErrReloginRequired")
	}
}

// ---------------------------------------------------------------------------
// Publish
// ---------------------------------------------------------------------------

// TestPublishVoiceSendsExpectedForm dựng server giả theo đúng hợp đồng đã đọc
// từ repo multime-ai và kiểm tra request gửi ra khớp.
func TestPublishVoiceSendsExpectedForm(t *testing.T) {
	var (
		gotPath   string
		gotAuth   string
		gotScope  string
		gotFields = map[string][]string{}
		gotAudio  []byte
	)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotAuth = r.Header.Get("Authorization")
		gotScope = r.Header.Get("Scope")

		_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
		if err != nil {
			t.Errorf("content-type không parse được: %v", err)
		}
		mr := multipart.NewReader(r.Body, params["boundary"])
		for {
			part, err := mr.NextPart()
			if err != nil {
				break
			}
			data, _ := io.ReadAll(part)
			if part.FormName() == "audio_file" {
				gotAudio = data
				continue
			}
			gotFields[part.FormName()] = append(gotFields[part.FormName()], string(data))
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":0,"data":{"id":555}}`)
	}))
	defer srv.Close()

	c := &Client{
		voiceBaseURL: srv.URL,
		siteURL:      "https://multime.ai",
		visibility:   "public",
		http:         srv.Client(),
	}

	url, err := c.PublishVoice(context.Background(), testCreds(), []byte("AUDIO"),
		domain.VoicePostInput{
			FileName: "voice.mp3",
			// Khác hẳn creds.AuthorID (4242): bài phải lên dưới tên tài khoản
			// người dùng CHỌN, không phải tài khoản đang gọi API.
			AuthorID:        9001,
			Title:           "Bản tin sáng",
			Language:        "vi",
			SourceLang:      "vi",
			Hashtags:        []string{"tinnong", "vietnam"},
			DurationSeconds: 42,
		})
	if err != nil {
		t.Fatalf("PublishVoice lỗi: %v", err)
	}

	if url != "https://multime.ai/voice/555" {
		t.Errorf("URL bài đăng = %q", url)
	}
	if gotPath != uploadPath {
		t.Errorf("path = %q, muốn %q", gotPath, uploadPath)
	}
	if gotAuth != "Bearer tok-1" {
		t.Errorf("authorization = %q", gotAuth)
	}
	if gotScope != scopeHeader {
		t.Errorf("scope = %q, muốn %q", gotScope, scopeHeader)
	}
	if string(gotAudio) != "AUDIO" {
		t.Errorf("audio_file = %q", gotAudio)
	}

	want := map[string]string{
		// author_id là tài khoản người dùng chọn, không phải chủ access token.
		"author_id":          "9001",
		"title":              "Bản tin sáng",
		"source_lang":        "vi",
		"lang":               "vi",
		"visibility":         "public",
		"is_public_download": "false",
	}
	for field, value := range want {
		if got := gotFields[field]; len(got) != 1 || got[0] != value {
			t.Errorf("field %s = %v, muốn [%s]", field, got, value)
		}
	}
	// hashtags là field lặp lại, không phải 1 chuỗi ghép.
	if got := gotFields["hashtags"]; len(got) != 2 || got[0] != "tinnong" || got[1] != "vietnam" {
		t.Errorf("hashtags = %v, muốn [tinnong vietnam]", got)
	}
}

// TestPublishVoiceMapsExpiredTokenSoEngineCanRefresh: 401 phải lộ ra dưới dạng
// ErrTokenExpired để Engine biết cần refresh rồi thử lại.
func TestPublishVoiceMapsExpiredTokenSoEngineCanRefresh(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":401,"message":"token expired"}`)
	}))
	defer srv.Close()

	c := &Client{voiceBaseURL: srv.URL, siteURL: "https://multime.ai", http: srv.Client()}

	_, err := c.PublishVoice(context.Background(), testCreds(), []byte("A"), validPost())
	if !errors.Is(err, domain.ErrTokenExpired) {
		t.Errorf("401 phải trả ErrTokenExpired, được: %v", err)
	}
}

// TestPublishVoiceBusinessErrorIsPermanent: envelope code != 0 là lỗi nghiệp vụ,
// retry vô nghĩa.
func TestPublishVoiceBusinessErrorIsPermanent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"code":50,"message":"hashtag is required"}`)
	}))
	defer srv.Close()

	c := &Client{voiceBaseURL: srv.URL, siteURL: "https://multime.ai", http: srv.Client()}

	_, err := c.PublishVoice(context.Background(), testCreds(), []byte("A"), validPost())
	if err == nil {
		t.Fatal("phải trả lỗi")
	}
	if !domain.IsPermanent(err) {
		t.Errorf("code != 0 phải là PermanentError, được: %v", err)
	}
	if !strings.Contains(err.Error(), "hashtag is required") {
		t.Errorf("phải giữ message của API: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

func TestMockLoginAndPublish(t *testing.T) {
	m := NewMock()

	session, err := m.Login(context.Background(), "dev@example.com", "password")
	if err != nil {
		t.Fatalf("Mock.Login lỗi: %v", err)
	}
	if session.UserID == 0 || session.AccessToken == "" {
		t.Fatalf("Mock.Login trả session rỗng: %+v", session)
	}
	// Cùng email phải cho cùng user id để dev không bị đổi author giữa các lần.
	again, _ := m.Login(context.Background(), "dev@example.com", "password")
	if again.UserID != session.UserID {
		t.Errorf("user id không ổn định: %d vs %d", again.UserID, session.UserID)
	}

	if _, err := m.Login(context.Background(), "", "password"); !errors.Is(err, domain.ErrUnauthorized) {
		t.Error("email rỗng phải trả ErrUnauthorized")
	}

	creds := domain.MultimeCredentials{AccessToken: session.AccessToken, AuthorID: session.UserID}
	url, err := m.PublishVoice(context.Background(), creds, []byte("A"), validPost())
	if err != nil {
		t.Fatalf("Mock.PublishVoice lỗi: %v", err)
	}
	if !strings.HasPrefix(url, "https://multime.ai/voice/mock-") {
		t.Errorf("URL mock = %q", url)
	}

	// Mock vẫn phải enforce ràng buộc thật để lỗi lộ ra ở dev.
	_, err = m.PublishVoice(context.Background(), creds, []byte("A"),
		domain.VoicePostInput{Title: "T", DurationSeconds: 5, Hashtags: []string{"x"}})
	if err == nil {
		t.Error("mock phải chặn voice ngắn hơn 15s")
	}
}

func TestUnauthorizedDetection(t *testing.T) {
	if isUnauthorized(errors.New("lỗi thường")) {
		t.Error("lỗi thường không được coi là unauthorized")
	}
	if !isUnauthorized(&unauthorizedError{err: errors.New("401")}) {
		t.Error("phải nhận ra unauthorizedError")
	}
}

// ---------------------------------------------------------------------------
// Danh bạ tài khoản
// ---------------------------------------------------------------------------

// genderListServer giả lập GET /v1/admin/user.
//
// `probe` là response của lượt thăm dò (limit=1, chỉ để biết có bao nhiêu
// trang), `page` là response của lượt bốc thật.
func genderListServer(t *testing.T, probe, listed string) (*Client, *[]url.Values) {
	t.Helper()
	var queries []url.Values

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		queries = append(queries, q)
		if r.URL.Path != usersPath {
			t.Errorf("path = %q, muốn %q", r.URL.Path, usersPath)
		}
		w.Header().Set("Content-Type", "application/json")
		if q.Get("limit") == "1" {
			_, _ = io.WriteString(w, probe)
			return
		}
		_, _ = io.WriteString(w, listed)
	}))
	t.Cleanup(srv.Close)

	return &Client{authBaseURL: srv.URL, http: srv.Client()}, &queries
}

// TestRandomUserFiltersByGenderAndPicksBeyondFirstPage khoá lại 2 điều kiện của
// việc "bốc ngẫu nhiên": lọc đúng giới tính, và KHÔNG phải lúc nào cũng trang
// đầu — lấy đại trang đầu thì mọi voice sẽ đứng tên vài tài khoản mới nhất.
func TestRandomUserFiltersByGenderAndPicksBeyondFirstPage(t *testing.T) {
	c, queries := genderListServer(t,
		`{"code":0,"data":{"total":500,"total_page":500,"data":[
			{"id":1,"email":"a@strongbody.ai","gender":"female"}]}}`,
		`{"code":0,"data":{"total":500,"total_page":25,"data":[
			{"id":77,"email":"solr@example.com","gender":"female","first_name":"Solr","last_name":"Nguyen"}]}}`)

	for i := 0; i < 20; i++ {
		user, err := c.RandomUser(context.Background(), "tok-1", domain.GenderFemale)
		if err != nil {
			t.Fatalf("RandomUser lỗi: %v", err)
		}
		if user.ID != 77 || user.Email != "solr@example.com" || user.Gender != "female" {
			t.Fatalf("tài khoản bốc ra = %+v", user)
		}
		if user.FullName != "Solr Nguyen" {
			t.Errorf("FullName = %q", user.FullName)
		}
	}

	pagesSeen := map[string]bool{}
	for _, q := range *queries {
		if q.Get("filter_names") != "gender" || q.Get("filter_values") != "female" {
			t.Fatalf("query thiếu bộ lọc giới tính: %v", q)
		}
		if q.Get("limit") != "1" {
			pagesSeen[q.Get("page")] = true
		}
	}
	if len(pagesSeen) < 2 {
		t.Errorf("20 lần bốc chỉ chạm %d trang — không phải ngẫu nhiên", len(pagesSeen))
	}
}

// Không có tài khoản nào khớp giới tính thì phải nói rõ và KHÔNG retry: danh bạ
// không tự mọc thêm người sau vài giây.
func TestRandomUserWithoutMatchIsPermanent(t *testing.T) {
	empty := `{"code":0,"data":{"total":0,"total_page":0,"data":[]}}`
	c, _ := genderListServer(t, empty, empty)

	_, err := c.RandomUser(context.Background(), "tok-1", domain.GenderOther)
	if !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("lỗi = %v, muốn ErrNotFound", err)
	}
	if !domain.IsPermanent(err) {
		t.Errorf("phải là PermanentError: %v", err)
	}
}

// Tài khoản không có email thì không đối chiếu được là ai -> bỏ qua.
func TestRandomUserSkipsAccountsWithoutEmail(t *testing.T) {
	c, _ := genderListServer(t,
		`{"code":0,"data":{"total":2,"total_page":2,"data":[{"id":1,"email":"","gender":"male"}]}}`,
		`{"code":0,"data":{"total":2,"total_page":1,"data":[
			{"id":1,"email":"","gender":"male"},
			{"id":2,"email":"ok@strongbody.ai","gender":"male"}]}}`)

	user, err := c.RandomUser(context.Background(), "tok-1", domain.GenderMale)
	if err != nil {
		t.Fatalf("RandomUser lỗi: %v", err)
	}
	if user.ID != 2 {
		t.Errorf("bốc trúng tài khoản không email: %+v", user)
	}
}

func TestRandomUserRejectsUnknownGender(t *testing.T) {
	if _, err := (&Client{}).RandomUser(context.Background(), "tok", "nam"); !errors.Is(err, domain.ErrInvalidInput) {
		t.Errorf("lỗi = %v, muốn ErrInvalidInput", err)
	}
}

// 401 phải lộ ra dưới dạng ErrTokenExpired để service refresh rồi gọi lại,
// giống đường publish.
func TestRandomUserMapsExpiredToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"code":401,"message":"token expired"}`)
	}))
	defer srv.Close()

	c := &Client{authBaseURL: srv.URL, http: srv.Client()}
	if _, err := c.RandomUser(context.Background(), "tok-1", domain.GenderMale); !errors.Is(err, domain.ErrTokenExpired) {
		t.Errorf("lỗi = %v, muốn ErrTokenExpired", err)
	}
}

func TestRandomUserRequiresToken(t *testing.T) {
	if _, err := (&Client{}).RandomUser(context.Background(), "", domain.GenderMale); !errors.Is(err, domain.ErrReloginRequired) {
		t.Errorf("lỗi = %v, muốn ErrReloginRequired", err)
	}
}
