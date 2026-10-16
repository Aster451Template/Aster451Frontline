package main

import (
	"time"

	"github.com/google/uuid"
)

type Paths struct {
	repository string
	Paths      []string
}

type FrontMatter struct {
	ID        uuid.UUID `yaml:"id" json:"id"`
	Title     string    `yaml:"title" json:"title"`
	Path      string    `yaml:"path" json:"path"`
	Author    string    `yaml:"author" json:"author"`
	Tags      []string  `yaml:"tags" json:"tags"`
	CreatedAt time.Time `yaml:"created_at" json:"created_at"`
	UpdatedAt time.Time `yaml:"updated_at" json:"updated_at"`
}

type TagDictionary struct {
	Tag       string   `json:"tag"`
	Documents []string `json:"documents"`
}
