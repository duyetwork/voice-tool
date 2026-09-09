// Command devtoken phát 1 access token để gọi API khi dev/kiểm thử, không cần
// đăng nhập qua SSO strongbody.
//
// CHỈ dùng ở máy dev: nó ký bằng đúng JWT_SECRET trong .env, nên token phát ra
// có quyền như người dùng thật.
//
//	go run ./cmd/devtoken <user-uuid> [role]
package main

import (
	"fmt"
	"os"

	"github.com/google/uuid"

	"github.com/strongbody/voice-tool/backend/internal/config"
	"github.com/strongbody/voice-tool/backend/internal/pkg/jwt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, "dùng: go run ./cmd/devtoken <user-uuid> [role]")
		os.Exit(1)
	}
	userID, err := uuid.Parse(os.Args[1])
	if err != nil {
		fmt.Fprintf(os.Stderr, "user id không hợp lệ: %v\n", err)
		os.Exit(1)
	}
	role := "admin"
	if len(os.Args) > 2 {
		role = os.Args[2]
	}

	cfg, err := config.Load(".", "./backend", "..")
	if err != nil {
		fmt.Fprintf(os.Stderr, "đọc config: %v\n", err)
		os.Exit(1)
	}

	pair, err := jwt.NewManager(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL).
		Issue(userID, "dev@local", role)
	if err != nil {
		fmt.Fprintf(os.Stderr, "phát token: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(pair.AccessToken)
}
