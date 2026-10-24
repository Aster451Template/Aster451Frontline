package utils

import (
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
)

// HashSourcePath は Markdown のリポジトリ内相対パスから
// SHA-256 ハッシュを生成する。
func HashSourcePath(path string) string {
	path = strings.ReplaceAll(path, `\`, "/")
	path = filepath.ToSlash(path)
	path = strings.TrimPrefix(path, "./")

	hash := sha256.Sum256([]byte(path))
	return hex.EncodeToString(hash[:])
}
