// Package audio đọc metadata kỹ thuật của file audio bằng ffprobe.
//
// multime.ai lưu audio dưới dạng audio_asset với duration_ms, size_bytes,
// mime_type, codec, sample_rate — nên phải đo trước khi publish.
package audio

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/strongbody/voice-tool/backend/internal/domain"
)

// Runner tách việc gọi ffprobe ra khỏi adapter để test được.
type Runner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

type Prober struct {
	runner  Runner
	tempDir string
}

var _ domain.AudioProber = (*Prober)(nil)

func NewProber(runner Runner, tempDir string) *Prober {
	if tempDir == "" {
		tempDir = os.TempDir()
	}
	return &Prober{runner: runner, tempDir: tempDir}
}

type ffprobeOutput struct {
	Streams []struct {
		CodecName string `json:"codec_name"`
		CodecType string `json:"codec_type"`
		SampleRt  string `json:"sample_rate"`
		Duration  string `json:"duration"`
	} `json:"streams"`
	Format struct {
		FormatName string `json:"format_name"`
		Duration   string `json:"duration"`
		Size       string `json:"size"`
	} `json:"format"`
}

// Probe ghi bytes ra file tạm rồi đo bằng ffprobe. Lỗi ffprobe KHÔNG chặn
// luồng publish — caller nhận về thông tin rỗng và tự quyết định.
func (p *Prober) Probe(ctx context.Context, data []byte) (domain.AudioInfo, error) {
	info := domain.AudioInfo{SizeBytes: int64(len(data))}
	if len(data) == 0 {
		return info, fmt.Errorf("audio rỗng")
	}

	dir, err := os.MkdirTemp(p.tempDir, "probe-*")
	if err != nil {
		return info, fmt.Errorf("tạo temp dir: %w", err)
	}
	defer os.RemoveAll(dir)

	path := filepath.Join(dir, "audio.bin")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return info, fmt.Errorf("ghi file tạm: %w", err)
	}

	stdout, err := p.runner.Run(ctx, "ffprobe",
		"-v", "error", "-print_format", "json",
		"-show_format", "-show_streams", path)
	if err != nil {
		return info, fmt.Errorf("ffprobe: %w", err)
	}

	var out ffprobeOutput
	if err := json.Unmarshal(stdout, &out); err != nil {
		return info, fmt.Errorf("parse output ffprobe: %w", err)
	}

	for _, s := range out.Streams {
		if s.CodecType != "audio" {
			continue
		}
		info.Codec = s.CodecName
		info.SampleRate = atoi(s.SampleRt)
		if info.DurationSeconds == 0 {
			info.DurationSeconds = seconds(s.Duration)
		}
		break
	}
	if info.DurationSeconds == 0 {
		info.DurationSeconds = seconds(out.Format.Duration)
	}
	info.MimeType = mimeFor(out.Format.FormatName, info.Codec)
	return info, nil
}

// seconds làm tròn lên để voice 0.4s không thành 0 giây.
func seconds(raw string) int {
	f, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || f <= 0 {
		return 0
	}
	return int(math.Ceil(f))
}

func atoi(raw string) int {
	v, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil {
		return 0
	}
	return v
}

func mimeFor(formatName, codec string) string {
	formats := strings.Split(formatName, ",")
	for _, f := range formats {
		switch strings.TrimSpace(f) {
		case "mp3":
			return "audio/mpeg"
		case "wav":
			return "audio/wav"
		case "ogg":
			return "audio/ogg"
		case "webm":
			return "audio/webm"
		case "flac":
			return "audio/flac"
		case "mp4", "m4a":
			return "audio/mp4"
		}
	}
	switch codec {
	case "mp3":
		return "audio/mpeg"
	case "opus":
		return "audio/ogg"
	case "aac":
		return "audio/mp4"
	case "pcm_s16le":
		return "audio/wav"
	}
	return "application/octet-stream"
}
