package builder

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"frontline/models"
)

// GetDocumentHistory は指定した Markdown ファイルの Git 履歴を取得する。
// repoPath は Git リポジトリのルートディレクトリ。
// sourcePath はリポジトリルートからの相対パス。
func GetDocumentHistory(
	repoPath string,
	sourcePath string,
) ([]models.DocumentHistory, error) {
	sourcePath = filepath.ToSlash(sourcePath)
	sourcePath = strings.TrimPrefix(sourcePath, "./")

	if sourcePath == "" || filepath.IsAbs(sourcePath) ||
		sourcePath == ".." || strings.HasPrefix(sourcePath, "../") {
		return nil, fmt.Errorf("invalid source path: %q", sourcePath)
	}

	// --follow により、Git が改名と判定できる場合は
	// 改名前の履歴も追跡する。
	format := `%H%x1f%s%x1f%an%x1f%cn%x1f%cI`

	cmd := exec.Command(
		"git",
		"-C", repoPath,
		"log",
		"--follow",
		"--format="+format,
		"--",
		sourcePath,
	)

	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf(
				"git log failed for %q: %s",
				sourcePath,
				strings.TrimSpace(string(exitErr.Stderr)),
			)
		}

		return nil, fmt.Errorf("execute git log: %w", err)
	}

	history := make([]models.DocumentHistory, 0)

	for _, line := range strings.Split(
		strings.TrimSpace(string(output)),
		"\n",
	) {
		if strings.TrimSpace(line) == "" {
			continue
		}

		fields := strings.Split(line, "\x1f")
		if len(fields) != 5 {
			return nil, fmt.Errorf(
				"invalid git log record for %q",
				sourcePath,
			)
		}

		committedAt, err := time.Parse(time.RFC3339, fields[4])
		if err != nil {
			return nil, fmt.Errorf(
				"parse commit time %q: %w",
				fields[4],
				err,
			)
		}

		history = append(history, models.DocumentHistory{
			CommitSHA: fields[0],
			Message:   fields[1],
			Author:    fields[2],
			Committer: fields[3],
			CreatedAt: committedAt.UTC(),
		})
	}

	// Git log は通常、新しいコミットから順に返す。
	return history, nil
}
