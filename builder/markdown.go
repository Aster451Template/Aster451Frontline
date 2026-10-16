package builder

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	"frontline/models"

	"github.com/yuin/goldmark"
)

// PageData は HTML テンプレートに渡すデータ。
type PageData struct {
	Title  string
	Author string
	Body   template.HTML
}

// RenderMarkdown は Markdown 本文を HTML に変換し、テンプレートを適用する。
func RenderMarkdown(
	doc models.Document,
	body []byte,
	templatePath string,
) ([]byte, error) {
	// Markdown を HTML に変換する。
	var rendered bytes.Buffer

	if err := goldmark.Convert(body, &rendered); err != nil {
		return nil, fmt.Errorf("convert markdown: %w", err)
	}

	// HTML テンプレートを読み込む。
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return nil, fmt.Errorf("parse HTML template: %w", err)
	}

	// Markdown から生成された HTML をテンプレートへ渡す。
	// template.HTML は HTML エスケープを無効化するため、
	// 信頼できない Markdown を入力する場合はサニタイズが必要。
	data := PageData{
		Title:  doc.Title,
		Author: doc.Author,
		Body:   template.HTML(rendered.String()),
	}

	var output bytes.Buffer

	if err := tmpl.Execute(&output, data); err != nil {
		return nil, fmt.Errorf("execute HTML template: %w", err)
	}

	return output.Bytes(), nil
}

// SaveHTML は HTML ファイルを指定されたパスへ保存する。
func SaveHTML(path string, content []byte) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create HTML directory: %w", err)
	}

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write HTML file: %w", err)
	}

	return nil
}
