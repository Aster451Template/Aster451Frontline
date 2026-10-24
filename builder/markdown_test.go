package builder

import (
	"strings"
	"testing"

	"frontline/models"

	"github.com/google/uuid"
)

func TestMarkdownWithTOCRendersHeadingsAndExcludesMarkedHeading(t *testing.T) {
	input := []byte("[[toc]]\n\n# 見出し A\n\n##* 対象外\n\n本文")
	got, err := markdownWithTOC(input)
	if err != nil {
		t.Fatalf("markdownWithTOC() error = %v", err)
	}
	html := string(got)
	if !strings.Contains(html, `<a href="#見出し-a">見出し A</a>`) {
		t.Errorf("TOC does not contain normal heading: %s", html)
	}
	if strings.Contains(html, `>対象外</a>`) {
		t.Errorf("TOC contains excluded heading: %s", html)
	}
	if !strings.Contains(html, `<h2 id="対象外">対象外</h2>`) {
		t.Errorf("excluded heading was not rendered: %s", html)
	}
}

func TestMarkdownWithTOCRendersAster451InlineElementsAndTaskList(t *testing.T) {
	input := []byte("`inline code`\n\n> 引用\n\n- [ ] 未完了\n- [x] 完了\n\n**強調**")
	got, err := markdownWithTOC(input)
	if err != nil {
		t.Fatalf("markdownWithTOC() error = %v", err)
	}
	html := string(got)
	for _, want := range []string{
		`<code class="blockquote-code">inline code</code>`,
		`<blockquote class="blockquote">`,
		`<input disabled="" type="checkbox">`,
		`<input checked="" disabled="" type="checkbox">`,
		`<b>強調</b>`,
	} {
		if !strings.Contains(html, want) {
			t.Errorf("rendered HTML does not contain %q: %s", want, html)
		}
	}
}

func TestBuildTagIndexTrimsDeduplicatesAndSortsTags(t *testing.T) {
	id := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	index := BuildTagIndex([]models.Document{{ID: id, Tags: []string{" zeta ", "alpha", "zeta", " "}}})
	if len(index.Tags) != 2 || index.Tags[0].Tag != "alpha" || index.Tags[1].Tag != "zeta" {
		t.Fatalf("BuildTagIndex() tags = %#v, want sorted alpha and zeta", index.Tags)
	}
	if len(index.Tags[1].Documents) != 1 || index.Tags[1].Documents[0] != id {
		t.Errorf("duplicate tags produced document entries %#v", index.Tags[1].Documents)
	}
}
