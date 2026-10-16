package main

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"

	"frontline/builder"
	"frontline/models"
	"frontline/parser"
	"frontline/utils"
)

const (
	sourceDir    = "./public"
	outputDir    = "./dirc"
	templatePath = "./template/document.html"
	indexPath    = "./dirc/index.json"
)

// loadMarkdownPaths は Markdown ファイルを再帰的に検索する。
func loadMarkdownPaths(root string) ([]string, error) {
	var paths []string

	err := filepath.WalkDir(root, func(
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

		if strings.EqualFold(filepath.Ext(path), ".md") {
			paths = append(paths, path)
		}

		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Markdown files: %w", err)
	}

	return paths, nil
}

// loadDocumentIndex は既存のインデックスを読み込む。
// 初回実行時にファイルがなければ空のインデックスを返す。
func loadDocumentIndex(path string) (models.DocumentIndex, error) {
	index := models.DocumentIndex{
		Documents: []models.Document{},
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return index, nil
	}
	if err != nil {
		return index, fmt.Errorf("read index: %w", err)
	}

	if err := json.Unmarshal(data, &index); err != nil {
		return index, fmt.Errorf("decode index: %w", err)
	}

	if index.Documents == nil {
		index.Documents = []models.Document{}
	}

	return index, nil
}

// latestCommitSHA は履歴の先頭にある最新コミット SHA を返す。
func latestCommitSHA(history []models.DocumentHistory) string {
	if len(history) == 0 {
		return ""
	}

	return history[0].CommitSHA
}

// build は Markdown を処理し、HTML と JSON を生成する。
func build() error {
	// 1. 既存インデックスを読み込む。
	oldIndex, err := loadDocumentIndex(indexPath)
	if err != nil {
		return err
	}

	// SourceHash から既存ドキュメントを検索できるようにする。
	oldDocuments := make(map[string]models.Document)
	for _, doc := range oldIndex.Documents {
		if doc.SourceHash != "" {
			oldDocuments[doc.SourceHash] = doc
		}
	}

	// 2. Markdown ファイルを検索する。
	markdownPaths, err := loadMarkdownPaths(sourceDir)
	if err != nil {
		return err
	}

	newDocuments := make([]models.Document, 0, len(markdownPaths))
	activeHashes := make(map[string]bool)

	// 3. 各 Markdown を処理する。
	for _, markdownPath := range markdownPaths {
		// リポジトリ内の相対パスを作成する。
		sourcePath, err := filepath.Rel(".", markdownPath)
		if err != nil {
			return fmt.Errorf("resolve source path: %w", err)
		}
		sourcePath = filepath.ToSlash(sourcePath)

		// パスのハッシュを生成する。
		sourceHash := utils.HashSourcePath(sourcePath)
		activeHashes[sourceHash] = true

		// Markdown を読み込む。
		data, err := os.ReadFile(markdownPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", sourcePath, err)
		}

		// Front Matter と本文を分離する。
		doc, body, err := parser.ParseFrontMatter(data)
		if err != nil {
			return fmt.Errorf("parse %s: %w", sourcePath, err)
		}

		// 既存文書なら UUID を引き継ぐ。
		oldDoc, exists := oldDocuments[sourceHash]
		if exists {
			doc.ID = oldDoc.ID
		} else if doc.ID == uuid.Nil {
			doc.ID = uuid.New()
		}

		doc.SourceHash = sourceHash
		doc.Paths.SourcePath = sourcePath
		doc.Paths.HTMLPath = filepath.ToSlash(
			filepath.Join(outputDir, doc.ID.String()+".html"),
		)

		// Git の履歴を取得する。
		history, err := builder.GetDocumentHistory(".", sourcePath)
		if err != nil {
			return fmt.Errorf("get history for %s: %w", sourcePath, err)
		}
		doc.History = history

		// Git の最新コミット日時を更新日時として利用する。
		if len(history) > 0 {
			doc.UpdatedAt = history[0].CreatedAt
		}

		htmlPath := filepath.Join(outputDir, doc.ID.String()+".html")

		// 既存 HTML があり、最新コミットも変わっていなければ
		// HTML の再生成を省略する。
		unchanged := exists &&
			latestCommitSHA(oldDoc.History) == latestCommitSHA(history) &&
			latestCommitSHA(history) != "" &&
			fileExists(htmlPath)

		if !unchanged {
			htmlContent, err := builder.RenderMarkdown(
				doc,
				body,
				templatePath,
			)
			if err != nil {
				return fmt.Errorf("render %s: %w", sourcePath, err)
			}

			if err := builder.SaveHTML(htmlPath, htmlContent); err != nil {
				return fmt.Errorf("save HTML for %s: %w", sourcePath, err)
			}

			log.Printf("Generated: %s", htmlPath)
		} else {
			log.Printf("Unchanged: %s", sourcePath)
		}

		newDocuments = append(newDocuments, doc)
	}

	// 4. 削除された Markdown に対応する HTML を削除する。
	for hash, oldDoc := range oldDocuments {
		if activeHashes[hash] {
			continue
		}

		if oldDoc.Paths.HTMLPath == "" {
			continue
		}

		// 出力ディレクトリ内のファイルだけを対象にする。
		oldHTMLPath := filepath.Join(
			outputDir,
			filepath.Base(filepath.FromSlash(oldDoc.Paths.HTMLPath)),
		)

		if err := os.Remove(oldHTMLPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove obsolete HTML: %w", err)
		}

		log.Printf("Removed obsolete document: %s", oldDoc.Paths.SourcePath)
	}

	// 5. 文書一覧とタグ辞書を更新する。
	if err := builder.SaveIndexes(outputDir, newDocuments); err != nil {
		return fmt.Errorf("save indexes: %w", err)
	}

	log.Printf("Build completed: %d documents", len(newDocuments))
	return nil
}

// fileExists はファイルの存在を確認する。
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func main() {
	if err := build(); err != nil {
		log.Fatal(err)
	}
}
