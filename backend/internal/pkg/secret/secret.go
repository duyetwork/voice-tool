// Package secret mã hoá/giải mã các bí mật lưu trong DB.
//
// Hệ thống lưu access/refresh token multime của từng user để worker đăng voice
// thay họ. Đó là credential của hệ thống khác, không phải dữ liệu của mình, nên
// không để plaintext trong DB: một bản dump DB sẽ trở thành một xâu token dùng
// được ngay.
package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

// KeySize là độ dài khoá AES-256.
const KeySize = 32

var (
	ErrKeyMissing = errors.New("TOKEN_ENCRYPTION_KEY chưa được cấu hình")
	ErrKeySize    = fmt.Errorf("TOKEN_ENCRYPTION_KEY phải là %d byte sau khi decode base64", KeySize)
	ErrCiphertext = errors.New("dữ liệu mã hoá không hợp lệ")
)

// Box mã hoá bằng AES-256-GCM. Nonce sinh mới mỗi lần và được ghép vào trước
// ciphertext, nên cùng một token mã hoá 2 lần cho ra 2 chuỗi khác nhau.
type Box struct {
	aead cipher.AEAD
}

// NewBox nhận khoá dạng base64 (chuẩn hoặc raw, có/không padding).
func NewBox(base64Key string) (*Box, error) {
	if base64Key == "" {
		return nil, ErrKeyMissing
	}

	key, err := decodeKey(base64Key)
	if err != nil {
		return nil, err
	}
	if len(key) != KeySize {
		return nil, ErrKeySize
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("tạo cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("tạo GCM: %w", err)
	}
	return &Box{aead: aead}, nil
}

// GenerateKey sinh khoá mới dạng base64 — dùng cho `make gen-key`.
func GenerateKey() (string, error) {
	key := make([]byte, KeySize)
	if _, err := rand.Read(key); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(key), nil
}

// Encrypt trả về base64(nonce || ciphertext). Chuỗi rỗng vẫn là chuỗi rỗng để
// caller phân biệt được "chưa có token" và "token rỗng".
func (b *Box) Encrypt(plaintext string) (string, error) {
	if plaintext == "" {
		return "", nil
	}

	nonce := make([]byte, b.aead.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("sinh nonce: %w", err)
	}

	sealed := b.aead.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func (b *Box) Decrypt(encoded string) (string, error) {
	if encoded == "" {
		return "", nil
	}

	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrCiphertext, err)
	}
	nonceSize := b.aead.NonceSize()
	if len(raw) < nonceSize {
		return "", ErrCiphertext
	}

	plaintext, err := b.aead.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		// Sai khoá, hoặc dữ liệu bị sửa.
		return "", fmt.Errorf("%w: %v", ErrCiphertext, err)
	}
	return string(plaintext), nil
}

// EncryptPtr/DecryptPtr tiện cho các cột nullable của sqlc.
func (b *Box) EncryptPtr(plaintext string) (*string, error) {
	if plaintext == "" {
		return nil, nil
	}
	encoded, err := b.Encrypt(plaintext)
	if err != nil {
		return nil, err
	}
	return &encoded, nil
}

func (b *Box) DecryptPtr(encoded *string) (string, error) {
	if encoded == nil {
		return "", nil
	}
	return b.Decrypt(*encoded)
}

func decodeKey(s string) ([]byte, error) {
	for _, enc := range []*base64.Encoding{
		base64.StdEncoding, base64.RawStdEncoding,
		base64.URLEncoding, base64.RawURLEncoding,
	} {
		if key, err := enc.DecodeString(s); err == nil {
			return key, nil
		}
	}
	return nil, fmt.Errorf("TOKEN_ENCRYPTION_KEY không phải base64 hợp lệ")
}
