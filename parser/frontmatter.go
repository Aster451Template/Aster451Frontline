package parser

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"time"

	"frontline/models"

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"
)

var (
	ErrInvalidFrontMatter = errors.New("invalid front matter")
	ErrMissingDelimiter   = errors.New("front matter delimiter not found")
)

// ParseFrontMatter は Markdown からメタデータと本文を分離する。
func ParseFrontMatter(data []byte) (models.Document, []byte, error) {
	var doc models.Document

	// 先頭の空白を除去し、行単位に分割する。
	content := bytes.TrimLeft(data, " \t\r\n")
	lines := bytes.Split(content, []byte("\n"))

	// 開始デリミタを確認する。
	if len(lines) == 0 || strings.TrimSpace(string(lines[0])) != "=====" {
		return doc, nil, fmt.Errorf(
			"%w: opening delimiter is missing",
			ErrInvalidFrontMatter,
		)
	}

	// 終了デリミタを探す。
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(string(lines[i])) == "=====" {
			end = i
			break
		}
	}

	if end == -1 {
		return doc, nil, ErrMissingDelimiter
	}

	// デリミタの間を YAML として解析する。
	yamlData := bytes.Join(lines[1:end], []byte("\n"))

	if err := yaml.Unmarshal(yamlData, &doc); err != nil {
		return doc, nil, fmt.Errorf("parse front matter: %w", err)
	}

	// 必須フィールドを検証する。
	if strings.TrimSpace(doc.Title) == "" {
		return doc, nil, errors.New("front matter: title is required")
	}
	if strings.TrimSpace(doc.Path) == "" {
		return doc, nil, errors.New("front matter: path is required")
	}
	if strings.TrimSpace(doc.Author) == "" {
		return doc, nil, errors.New("front matter: author is required")
	}

	// ID がなければ生成する。
	// 呼び出し側は、この ID を index.json に保存して再利用すること。
	if doc.ID == uuid.Nil {
		doc.ID = uuid.New()
	}

	// 作成日時がなければ現在時刻を設定する。
	now := time.Now().UTC()

	if doc.CreatedAt.IsZero() {
		doc.CreatedAt = now
	}

	if doc.UpdatedAt.IsZero() {
		doc.UpdatedAt = doc.CreatedAt
	}

	// 本文を取り出す。
	body := bytes.Join(lines[end+1:], []byte("\n"))
	body = bytes.TrimSpace(body)

	return doc, body, nil
}
