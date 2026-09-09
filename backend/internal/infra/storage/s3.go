// Package storage cài đặt domain.Storage trên S3-compatible object storage
// (MinIO cho dev, AWS S3 cho production).
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/domain"
)

type S3Storage struct {
	client    *minio.Client
	bucket    string
	publicURL string // base URL công khai, nếu đứng sau CDN/reverse proxy
}

var _ domain.Storage = (*S3Storage)(nil)

func NewS3(ctx context.Context, cfg *config.Config) (*S3Storage, error) {
	endpoint, secure, err := parseEndpoint(cfg.S3Endpoint)
	if err != nil {
		return nil, err
	}

	// MinIO chỉ hỗ trợ path-style; AWS S3 thật dùng virtual-host style.
	lookup := minio.BucketLookupAuto
	if cfg.S3UsePathStyle {
		lookup = minio.BucketLookupPath
	}

	client, err := minio.New(endpoint, &minio.Options{
		Creds:        credentials.NewStaticV4(cfg.S3AccessKey, cfg.S3SecretKey, ""),
		Secure:       secure,
		Region:       cfg.S3Region,
		BucketLookup: lookup,
	})
	if err != nil {
		return nil, fmt.Errorf("khởi tạo S3 client: %w", err)
	}

	s := &S3Storage{
		client:    client,
		bucket:    cfg.S3Bucket,
		publicURL: strings.TrimRight(cfg.S3PublicBaseURL, "/"),
	}
	if err := s.ensureBucket(ctx); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *S3Storage) ensureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("kiểm tra bucket %s: %w", s.bucket, err)
	}
	if exists {
		return nil
	}
	if err := s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{}); err != nil {
		return fmt.Errorf("tạo bucket %s: %w", s.bucket, err)
	}
	return nil
}

func (s *S3Storage) Put(ctx context.Context, key string, data []byte, contentType string) (string, error) {
	_, err := s.client.PutObject(ctx, s.bucket, key, bytes.NewReader(data), int64(len(data)),
		minio.PutObjectOptions{ContentType: contentType})
	if err != nil {
		return "", fmt.Errorf("upload %s: %w", key, err)
	}
	return s.urlFor(key), nil
}

func (s *S3Storage) Get(ctx context.Context, key string) ([]byte, error) {
	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("mở object %s: %w", key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: object %s", domain.ErrNotFound, key)
		}
		return nil, fmt.Errorf("đọc object %s: %w", key, err)
	}
	return data, nil
}

func (s *S3Storage) Delete(ctx context.Context, key string) error {
	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil && !isNotFound(err) {
		return fmt.Errorf("xoá object %s: %w", key, err)
	}
	return nil
}

// KeyFromURL lấy lại object key từ URL đã lưu trong voice_file_url.
func (s *S3Storage) KeyFromURL(raw string) string {
	if raw == "" {
		return ""
	}
	if s.publicURL != "" && strings.HasPrefix(raw, s.publicURL) {
		return strings.TrimPrefix(strings.TrimPrefix(raw, s.publicURL), "/")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	p := strings.TrimPrefix(u.Path, "/")
	return strings.TrimPrefix(p, s.bucket+"/")
}

func (s *S3Storage) urlFor(key string) string {
	if s.publicURL != "" {
		return s.publicURL + "/" + key
	}
	scheme := "http"
	if s.client.EndpointURL().Scheme != "" {
		scheme = s.client.EndpointURL().Scheme
	}
	return fmt.Sprintf("%s://%s/%s/%s", scheme, s.client.EndpointURL().Host, s.bucket, key)
}

func parseEndpoint(raw string) (host string, secure bool, err error) {
	if raw == "" {
		return "", false, errors.New("S3_ENDPOINT là bắt buộc")
	}
	if !strings.Contains(raw, "://") {
		return raw, false, nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", false, fmt.Errorf("S3_ENDPOINT không hợp lệ: %w", err)
	}
	return u.Host, u.Scheme == "https", nil
}

func isNotFound(err error) bool {
	resp := minio.ToErrorResponse(err)
	return resp.Code == "NoSuchKey" || resp.StatusCode == 404
}
