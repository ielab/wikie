package wikie

import (
	"context"
	"encoding/json"
	"github.com/go-errors/errors"
	"gopkg.in/olivere/elastic.v5"
	"strings"
)

func NewPage(client *elastic.Client, path string, page Page) error {
	_, err := client.Index().Index("wikie").Id(path).BodyJson(page).Type("page").Do(context.Background())
	return err
}

func UpdatePage(client *elastic.Client, path string, page Page) error {
	_, err := client.Update().Index("wikie").Id(path).Doc(page).Type("page").Do(context.Background())
	return err
}

func GetPage(client *elastic.Client, pagePath string) (Page, error) {
	result, err := client.Get().Index("wikie").Id(pagePath).Do(context.Background())
	if err != nil {
		return Page{}, err
	}

	if !result.Found {
		return Page{}, errors.New("page not found")
	}

	b, err := result.Source.MarshalJSON()
	if err != nil {
		return Page{}, err

	}
	var i map[string]interface{}
	err = json.Unmarshal(b, &i)
	if err != nil {
		return Page{}, err
	}
	var page Page
	page.Body = i["delta"].(map[string]interface{})["ops"]
	page.Path = pagePath
	rel := strings.Split(pagePath, "/")[1:]
	for i := 0; i < len(rel); i++ {
		page.Relationships = append(page.Relationships, PageRelationship{
			URL:   strings.Join(rel[:i+1], "/"),
			Title: rel[i],
		})
	}
	page.LastUpdated = i["updated"].(string)
	page.EditedBy = i["edited"].(string)
	page.Ops = i["delta"]
	return page, nil
}
