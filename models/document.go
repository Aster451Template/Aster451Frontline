package models

import (
	"time"

	"github.com/google/uuid"
)

// DocumentHistory は Git のコミット履歴を表す。
type DocumentHistory struct {
	CommitSHA string    `json:"commit_sha"`
	Message   string    `json:"message"`
	Author    string    `json:"author"`
	Committer string    `json:"committer"`
	CreatedAt time.Time `json:"created_at"`
}

// DocumentPath は Markdown と HTML の対応関係を表す。
type DocumentPath struct {
	SourcePath string `json:"source_path"`
	HTMLPath   string `json:"html_path"`
}

// Document はドキュメント1件分の情報を表す。
type Document struct {
	ID         uuid.UUID `json:"id"`
	SourceHash string    `json:"source_hash"`

	Title  string   `json:"title"`
	Path   string   `json:"path"`
	Author string   `json:"author"`
	Tags   []string `json:"tags"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`

	Paths   DocumentPath      `json:"paths"`
	History []DocumentHistory `json:"history"`
}

// DocumentIndex は全ドキュメントのインデックスを表す。
type DocumentIndex struct {
	GeneratedAt time.Time  `json:"generated_at"`
	Documents   []Document `json:"documents"`
}

// TagDictionary は1つのタグに紐づくドキュメントを表す。
type TagDictionary struct {
	Tag       string      `json:"tag"`
	Documents []uuid.UUID `json:"documents"`
}

// TagIndex はタグ辞書全体を表す。
type TagIndex struct {
	GeneratedAt time.Time       `json:"generated_at"`
	Tags        []TagDictionary `json:"tags"`
}
