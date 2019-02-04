package wikie

import (
	"bytes"
	"github.com/gomarkdown/markdown"
	"github.com/jlubawy/go-boilerpipe"
	"html/template"
	"strings"
)

type PageRelationship struct {
	URL   string
	Title string
}

type Page struct {
	Path          string             `json:"path"`
	Body          string             `json:"body"`
	Relationships []PageRelationship `json:"relationships"`
	LastUpdated   string             `json:"updated"`
	EditedBy      string             `json:"edited"`
}

func (p Page) Render() template.HTML {
	return template.HTML(string(markdown.ToHTML([]byte(p.Body), nil, nil)))
}

func (p Page) Snippet() template.HTML {
	s := []string{"<html><body>", string(markdown.ToHTML([]byte(p.Body), nil, nil)), "</body></html>"}
	doc, err := boilerpipe.ParseDocument(bytes.NewBufferString(strings.Join(s, "")))

	if err != nil {
		panic(err)
	}
	var blocks []string
	for _, block := range doc.TextBlocks {
		blocks = append(blocks, block.Text)
	}
	content := strings.Join(blocks, "...")
	if len(content) > 250 {
		return template.HTML(content)[:250] + "..."
	}
	return template.HTML(content)
}
