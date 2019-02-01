package wikie

import (
	"bytes"
	"encoding/json"
	"github.com/dchenk/go-render-quill"
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
	Body          interface{}        `json:"delta"`
	Ops           interface{}        `json:"ops"`
	Relationships []PageRelationship `json:"relationships"`
	LastUpdated   string             `json:"updated"`
	EditedBy      string             `json:"edited"`
}

func (p Page) Delta() template.JS {
	b, err := json.Marshal(p.Ops)
	if err != nil {
		panic(err)
	}
	return template.JS(b)
}

func (p Page) delta() ([]byte, error) {
	b, err := json.Marshal(p.Body)
	if err != nil {
		return nil, err
	}
	return b, nil
}

func (p Page) Render() template.HTML {
	d, err := p.delta()
	if err != nil {
		panic(err)
	}
	html, err := quill.Render(d)
	if err != nil {
		panic(err)
	}
	return template.HTML(string(html))
}

func (p Page) Snippet() template.HTML {
	d, err := p.delta()
	if err != nil {
		panic(err)
	}
	b, err := quill.Render(d)
	if err != nil {
		panic(err)
	}
	s := []string{"<html><body>", string(b), "</body></html>"}
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
