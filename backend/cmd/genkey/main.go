// Command genkey in ra 1 khoá AES-256 dạng base64 cho TOKEN_ENCRYPTION_KEY.
//
//	make gen-key
package main

import (
	"fmt"
	"os"

	"github.com/strongbody/voice-tool/backend/internal/pkg/secret"
)

func main() {
	key, err := secret.GenerateKey()
	if err != nil {
		fmt.Fprintln(os.Stderr, "không sinh được khoá:", err)
		os.Exit(1)
	}
	fmt.Println(key)
}
