package wikie

import (
	"bytes"
	"fmt"
	"github.com/gomarkdown/markdown"
	"github.com/jlubawy/go-boilerpipe"
	"gopkg.in/neurosnap/sentences.v1"
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
	Public        bool               `json:"public"`
	Files         []string
}

func (p Page) Render() template.HTML {
	return template.HTML(string(markdown.ToHTML([]byte(p.Body), nil, nil)))
}

func (p Page) Snippet(query string) template.HTML {
	s := []string{"<html><body>", string(markdown.ToHTML([]byte(p.Body), nil, nil)), "</body></html>"}
	doc, err := boilerpipe.ParseDocument(bytes.NewBufferString(strings.Join(s, "")))

	tokeniser := sentences.NewWordTokenizer(&sentences.DefaultPunctStrings{})
	queryTerms := tokeniser.Tokenize(query, false)

	if err != nil {
		panic(err)
	}
	var blocks []string
	for _, block := range doc.TextBlocks {
		text := block.Text
		skip := true
		for _, queryTerm := range queryTerms {
			if strings.Contains(strings.ToLower(text), strings.ToLower(queryTerm.Tok)) {
				idx := strings.Index(strings.ToLower(text), strings.ToLower(queryTerm.Tok))
				size := 125
				if len(text) > size && idx > size && idx < len(text)-size {
					text = fmt.Sprintf("...%s...", text[idx-125:idx+125])
				} else if len(text) > size && idx > len(text)-size {
					text = fmt.Sprintf("...%s", text[idx-125:])
				}
				skip = false
				break
			}
		}
		if skip {
			continue
		}
		for _, queryTerm := range queryTerms {
			text = strings.Replace(text, queryTerm.Tok, fmt.Sprintf("<b>%s</b>", queryTerm.Tok), -1)
			text = strings.Replace(text, strings.Title(queryTerm.Tok), fmt.Sprintf("<b>%s</b>", strings.Title(queryTerm.Tok)), -1)
			text = strings.Replace(text, strings.ToUpper(queryTerm.Tok), fmt.Sprintf("<b>%s</b>", strings.ToUpper(queryTerm.Tok)), -1)
			text = strings.Replace(text, strings.ToLower(queryTerm.Tok), fmt.Sprintf("<b>%s</b>", strings.ToLower(queryTerm.Tok)), -1)
		}
		blocks = append(blocks, text)
	}
	content := strings.Join(blocks, "...")
	if len(content) > 250 {
		return template.HTML(content)[:250] + "..."
	}
	return template.HTML(content)
}
