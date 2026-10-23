package builder

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"frontline/models"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
)

var markedHeadingPattern = regexp.MustCompile(`^( {0,3})(#{1,6})\*([ \t]+.*)$`)

type headingInfo struct {
	level   int
	title   string
	id      string
	exclude bool
}

type tocExtension struct{}
type tocNodeRenderer struct{ renderer.Config }

var kindTOC = ast.NewNodeKind("TableOfContents")

type tocNode struct {
	ast.BaseBlock
	html string
}
type tocBlockParser struct{}

func (tocBlockParser) Trigger() []byte { return []byte{'['} }
func (tocBlockParser) Open(parent ast.Node, reader text.Reader, pc parser.Context) (ast.Node, parser.State) {
	line, segment := reader.PeekLine()
	if strings.TrimSpace(string(line)) != "[[toc]]" {
		return nil, parser.NoChildren
	}
	reader.Advance(segment.Len() - 1)
	return &tocNode{}, parser.NoChildren
}
func (tocBlockParser) Continue(ast.Node, text.Reader, parser.Context) parser.State {
	return parser.Close
}
func (tocBlockParser) Close(ast.Node, text.Reader, parser.Context) {}
func (tocBlockParser) CanInterruptParagraph() bool                 { return true }
func (tocBlockParser) CanAcceptIndentedLine() bool                 { return false }

func (*tocNode) Kind() ast.NodeKind { return kindTOC }
func (*tocNode) Dump([]byte, int)   {}

func (tocNodeRenderer) RegisterFuncs(reg renderer.NodeRendererFuncRegisterer) {
	reg.Register(kindTOC, func(w util.BufWriter, source []byte, node ast.Node, entering bool) (ast.WalkStatus, error) {
		if entering {
			_, err := w.WriteString(node.(*tocNode).html)
			return ast.WalkSkipChildren, err
		}
		return ast.WalkContinue, nil
	})
}

func (tocExtension) Extend(md goldmark.Markdown) {
	md.Renderer().AddOptions(renderer.WithNodeRenderers(util.Prioritized(tocNodeRenderer{}, 500)))
}

type tocTransformer struct{ excluded map[int]bool }

func (t tocTransformer) Transform(doc *ast.Document, reader text.Reader, _ parser.Context) {
	source := reader.Source()
	var headings []headingInfo
	var tocNodes []*tocNode
	var walk func(ast.Node)
	walk = func(parent ast.Node) {
		for child := parent.FirstChild(); child != nil; child = child.NextSibling() {
			if heading, ok := child.(*ast.Heading); ok {
				title := strings.TrimSpace(string(heading.Text(source)))
				id := headingID(title, len(headings)+1)
				exclude := false
				if start := heading.Lines().At(0).Start; start >= 0 {
					line := bytes.Count(source[:start], []byte("\n"))
					exclude = t.excluded[line]
				}
				heading.SetAttributeString("id", id)
				headings = append(headings, headingInfo{level: heading.Level, title: title, id: id, exclude: exclude})
			} else if toc, ok := child.(*tocNode); ok {
				tocNodes = append(tocNodes, toc)
			} else if child.HasChildren() {
				walk(child)
			}
		}
	}
	walk(doc)
	tocHTML := buildTOC(headings)
	for _, node := range tocNodes {
		node.html = tocHTML
	}
}

func buildTOC(headings []headingInfo) string {
	var b strings.Builder
	b.WriteString(`<nav class="table-of-contents"><ul>`)
	for _, heading := range headings {
		if heading.exclude {
			continue
		}
		fmt.Fprintf(&b, `<li class="toc-level-%d"><a href="#%s">%s</a></li>`, heading.level, template.HTMLEscapeString(heading.id), template.HTMLEscapeString(heading.title))
	}
	b.WriteString(`</ul></nav>`)
	return b.String()
}

func headingID(title string, index int) string {
	var b strings.Builder
	separator := false
	for _, r := range strings.ToLower(title) {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0x80 {
			if separator && b.Len() > 0 {
				b.WriteByte('-')
			}
			b.WriteRune(r)
			separator = false
		} else {
			separator = true
		}
	}
	if b.Len() == 0 {
		return fmt.Sprintf("section-%d", index)
	}
	return b.String()
}

// PageData は HTML テンプレートに渡すデータ。
type PageData struct {
	Title, Author string
	Body          template.HTML
}

func markdownWithTOC(source []byte) ([]byte, error) {
	lines := bytes.Split(source, []byte("\n"))
	excluded := make(map[int]bool)
	for i, line := range lines {
		match := markedHeadingPattern.FindSubmatch(line)
		if match == nil {
			continue
		}
		lines[i] = bytes.Join([][]byte{match[1], match[2], match[3]}, nil)
		excluded[i] = true
	}
	source = bytes.Join(lines, []byte("\n"))
	md := goldmark.New(
		goldmark.WithExtensions(extension.Strikethrough, tocExtension{}),
		goldmark.WithParserOptions(
			parser.WithBlockParsers(util.Prioritized(tocBlockParser{}, 50)),
			parser.WithASTTransformers(util.Prioritized(tocTransformer{excluded: excluded}, 500)),
		),
	)
	var rendered bytes.Buffer
	if err := md.Convert(source, &rendered); err != nil {
		return nil, err
	}
	return rendered.Bytes(), nil
}

// RenderMarkdown は Markdown 本文を HTML に変換し、テンプレートを適用する。
func RenderMarkdown(doc models.Document, body []byte, templatePath string) ([]byte, error) {
	converted, err := markdownWithTOC(body)
	if err != nil {
		return nil, fmt.Errorf("convert markdown: %w", err)
	}
	tmpl, err := template.ParseFiles(templatePath)
	if err != nil {
		return nil, fmt.Errorf("parse HTML template: %w", err)
	}
	data := PageData{Title: doc.Title, Author: doc.Author, Body: template.HTML(converted)}
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
