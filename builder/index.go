package builder

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"frontline/models"

	"github.com/google/uuid"
)

// BuildDocumentIndex は文書一覧からインデックスを構築する。
func BuildDocumentIndex(documents []models.Document) models.DocumentIndex {
	if documents == nil {
		documents = []models.Document{}
	}

	return models.DocumentIndex{
		GeneratedAt: time.Now().UTC(),
		Documents:   documents,
	}
}

// BuildTagIndex は文書一覧からタグ辞書を構築する。
func BuildTagIndex(documents []models.Document) models.TagIndex {
	tagMap := make(map[string]map[uuid.UUID]struct{})

	for _, doc := range documents {
		seen := make(map[string]struct{})

		for _, rawTag := range doc.Tags {
			tag := strings.TrimSpace(rawTag)
			if tag == "" {
				continue
			}

			// 同じ文書内のタグ重複を防ぐ。
			if _, exists := seen[tag]; exists {
				continue
			}
			seen[tag] = struct{}{}

			if tagMap[tag] == nil {
				tagMap[tag] = make(map[uuid.UUID]struct{})
			}

			tagMap[tag][doc.ID] = struct{}{}
		}
	}

	tags := make([]string, 0, len(tagMap))
	for tag := range tagMap {
		tags = append(tags, tag)
	}
	sort.Strings(tags)

	result := models.TagIndex{
		GeneratedAt: time.Now().UTC(),
		Tags:        make([]models.TagDictionary, 0, len(tags)),
	}

	for _, tag := range tags {
		ids := make([]uuid.UUID, 0, len(tagMap[tag]))

		for id := range tagMap[tag] {
			ids = append(ids, id)
		}

		// JSON の出力順を安定させる。
		sort.Slice(ids, func(i, j int) bool {
			return ids[i].String() < ids[j].String()
		})

		result.Tags = append(result.Tags, models.TagDictionary{
			Tag:       tag,
			Documents: ids,
		})
	}

	return result
}

// SaveJSON はデータを整形した JSON として保存する。
func SaveJSON(path string, data any) error {
	dir := filepath.Dir(path)

	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create JSON directory: %w", err)
	}

	content, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal JSON: %w", err)
	}

	content = append(content, '\n')

	if err := os.WriteFile(path, content, 0644); err != nil {
		return fmt.Errorf("write JSON: %w", err)
	}

	return nil
}

// SaveIndexes は文書一覧とタグ辞書を JSON に出力する。
func SaveIndexes(
	outputDir string,
	documents []models.Document,
) error {
	index := BuildDocumentIndex(documents)
	tagIndex := BuildTagIndex(documents)

	if err := SaveJSON(
		filepath.Join(outputDir, "index.json"),
		index,
	); err != nil {
		return err
	}

	if err := SaveJSON(
		filepath.Join(outputDir, "tags.json"),
		tagIndex,
	); err != nil {
		return err
	}

	return nil
}
