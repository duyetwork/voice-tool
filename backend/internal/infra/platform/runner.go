package platform

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// CommandRunner tách việc gọi binary ngoài (yt-dlp, ffmpeg) ra khỏi adapter
// để test được bằng fake runner.
type CommandRunner interface {
	Run(ctx context.Context, name string, args ...string) ([]byte, error)
}

// ExecRunner chạy binary thật, có timeout và map path binary theo config.
type ExecRunner struct {
	Timeout time.Duration
	// Aliases map tên logic -> đường dẫn thật (YTDLP_PATH, FFMPEG_PATH).
	Aliases map[string]string
}

func NewExecRunner(timeout time.Duration, aliases map[string]string) *ExecRunner {
	if timeout <= 0 {
		timeout = 5 * time.Minute
	}
	return &ExecRunner{Timeout: timeout, Aliases: aliases}
}

func (r *ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if path, ok := r.Aliases[name]; ok && path != "" {
		name = path
	}
	ctx, cancel := context.WithTimeout(ctx, r.Timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("%s: %w: %s", name, err, truncate(stderr.String(), 500))
	}
	return stdout.Bytes(), nil
}

func truncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// splitHostPath parse URL và trả host + path, ok=false nếu URL không hợp lệ.
func splitHostPath(raw string) (host, path string, ok bool) {
	if !strings.Contains(raw, "://") {
		raw = "https://" + raw
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", "", false
	}
	return u.Host, u.Path, true
}
