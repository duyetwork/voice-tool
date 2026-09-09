package secret

import (
	"errors"
	"strings"
	"testing"
)

func newTestBox(t *testing.T) *Box {
	t.Helper()

	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey lỗi: %v", err)
	}
	box, err := NewBox(key)
	if err != nil {
		t.Fatalf("NewBox lỗi: %v", err)
	}
	return box
}

func TestEncryptDecryptRoundTrip(t *testing.T) {
	box := newTestBox(t)
	token := "eyJhbGciOiJIUzI1NiJ9.multime-access-token"

	encoded, err := box.Encrypt(token)
	if err != nil {
		t.Fatalf("Encrypt lỗi: %v", err)
	}
	if strings.Contains(encoded, token) {
		t.Error("ciphertext không được chứa plaintext")
	}

	got, err := box.Decrypt(encoded)
	if err != nil {
		t.Fatalf("Decrypt lỗi: %v", err)
	}
	if got != token {
		t.Errorf("Decrypt = %q, muốn %q", got, token)
	}
}

func TestEncryptIsNonDeterministic(t *testing.T) {
	box := newTestBox(t)

	first, _ := box.Encrypt("same-token")
	second, _ := box.Encrypt("same-token")
	if first == second {
		t.Error("cùng plaintext phải cho 2 ciphertext khác nhau (nonce mới mỗi lần)")
	}
}

func TestEmptyStaysEmpty(t *testing.T) {
	box := newTestBox(t)

	if got, _ := box.Encrypt(""); got != "" {
		t.Errorf("Encrypt(\"\") = %q, muốn rỗng", got)
	}
	if got, _ := box.Decrypt(""); got != "" {
		t.Errorf("Decrypt(\"\") = %q, muốn rỗng", got)
	}
	if ptr, _ := box.EncryptPtr(""); ptr != nil {
		t.Error("EncryptPtr(\"\") phải trả nil để lưu NULL")
	}
	if got, _ := box.DecryptPtr(nil); got != "" {
		t.Errorf("DecryptPtr(nil) = %q, muốn rỗng", got)
	}
}

func TestDecryptWithWrongKeyFails(t *testing.T) {
	encoded, err := newTestBox(t).Encrypt("secret")
	if err != nil {
		t.Fatalf("Encrypt lỗi: %v", err)
	}

	other := newTestBox(t)
	if _, err := other.Decrypt(encoded); !errors.Is(err, ErrCiphertext) {
		t.Errorf("sai khoá phải trả ErrCiphertext, được: %v", err)
	}
}

func TestDecryptRejectsTamperedData(t *testing.T) {
	box := newTestBox(t)

	for _, bad := range []string{"không-phải-base64!!", "c2hvcnQ="} {
		if _, err := box.Decrypt(bad); !errors.Is(err, ErrCiphertext) {
			t.Errorf("Decrypt(%q) phải trả ErrCiphertext, được: %v", bad, err)
		}
	}
}

func TestNewBoxValidatesKey(t *testing.T) {
	if _, err := NewBox(""); !errors.Is(err, ErrKeyMissing) {
		t.Errorf("khoá rỗng phải trả ErrKeyMissing, được: %v", err)
	}
	// 16 byte -> AES-128, không đủ theo yêu cầu.
	if _, err := NewBox("MTIzNDU2Nzg5MDEyMzQ1Ng=="); !errors.Is(err, ErrKeySize) {
		t.Errorf("khoá sai độ dài phải trả ErrKeySize, được: %v", err)
	}
	if _, err := NewBox("khong-phai-base64-***"); err == nil {
		t.Error("khoá không phải base64 phải trả lỗi")
	}
}

func TestNewBoxAcceptsRawBase64(t *testing.T) {
	// Khoá 32 byte encode không padding — dạng người ta hay copy từ terminal.
	if _, err := NewBox("MDEyMzQ1Njc4OTAxMjM0NTY3ODkwMTIzNDU2Nzg5MDE"); err != nil {
		t.Errorf("phải nhận base64 raw (không padding): %v", err)
	}
}
