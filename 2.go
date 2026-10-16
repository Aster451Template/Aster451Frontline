package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/yuin/goldmark"
	"gopkg.in/yaml.v3"
)

// HTML テンプレート用データ
type PageData struct {
	Title  string
	Author string
	Body   template.HTML
}

// Markdown ファイルを再帰的に探索
func (p *Paths) LoadMarkdownPaths() error {
	p.Paths = nil

	return filepath.WalkDir(p.repository, func(
		path string,
		entry fs.DirEntry,
		err error,
	) error {
		if err != nil {
			return err
		}

		if entry.IsDir() {
			return nil
		}

		if strings.EqualFold(filepath.Ext(entry.Name()), ".md") {
			p.Paths = append(p.Paths, path)
		}

		return nil
	})
}

// Front Matter を解析して本文と分離
func ParseFrontMatter(data []byte) (FrontMatter, []byte, error) {
	var meta FrontMatter

	content := strings.TrimSpace(string(data))
	lines := strings.Split(content, "\n")

	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "=====" {
		return meta, nil, fmt.Errorf(
			"front matter の開始区切りがありません",
		)
	}

	end := -1

	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "=====" {
			end = i
			break
		}
	}

	if end == -1 {
		return meta, nil, fmt.Errorf(
			"front matter の終了区切りがありません",
		)
	}

	yamlData := strings.Join(lines[1:end], "\n")

	if err := yaml.Unmarshal([]byte(yamlData), &meta); err != nil {
		return meta, nil, fmt.Errorf(
			"YAML の解析に失敗: %w", err,
		)
	}

	// ID がない場合は UUID を生成
	if meta.ID == uuid.Nil {
		meta.ID = uuid.New()
	}

	// 日時が未指定の場合は補完
	now := time.Now().UTC()

	if meta.CreatedAt.IsZero() {
		meta.CreatedAt = now
	}

	if meta.UpdatedAt.IsZero() {
		meta.UpdatedAt = meta.CreatedAt
	}

	body := []byte(strings.Join(lines[end+1:], "\n"))
	body = bytes.TrimSpace(body)

	return meta, body, nil
}

// Markdown を HTML に変換
func RenderMarkdown(meta FrontMatter, body []byte) ([]byte, error) {
	var rendered bytes.Buffer

	if err := goldmark.Convert(body, &rendered); err != nil {
		return nil, err
	}

	tmpl, err := template.ParseFiles("template/document.html")
	if err != nil {
		return nil, err
	}

	data := PageData{
		Title:  meta.Title,
		Author: meta.Author,
		Body:   template.HTML(rendered.String()),
	}

	var output bytes.Buffer

	if err := tmpl.Execute(&output, data); err != nil {
		return nil, err
	}

	return output.Bytes(), nil
}

// HTML を保存
func SaveHTML(path string, html []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	return os.WriteFile(path, html, 0644)
}

// JSON を保存
func SaveJSON(path string, data any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}

	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(path, jsonData, 0644)
}

// ドキュメント一覧からタグ辞書を生成
func BuildTagDictionary(
	documents []FrontMatter,
) []TagDictionary {
	tagMap := make(map[string][]string)

	for _, doc := range documents {
		// 同一文書内の重複タグを除去
		seen := make(map[string]bool)

		for _, tag := range doc.Tags {
			tag = strings.TrimSpace(tag)

			if tag == "" || seen[tag] {
				continue
			}

			seen[tag] = true
			tagMap[tag] = append(
				tagMap[tag],
				doc.ID.String(),
			)
		}
	}

	tags := make([]string, 0, len(tagMap))

	for tag := range tagMap {
		tags = append(tags, tag)
	}

	// JSON の生成結果を安定させる
	sort.Strings(tags)

	result := make([]TagDictionary, 0, len(tags))

	for _, tag := range tags {
		ids := tagMap[tag]
		sort.Strings(ids)

		result = append(result, TagDictionary{
			Tag:       tag,
			Documents: ids,
		})
	}

	return result
}
