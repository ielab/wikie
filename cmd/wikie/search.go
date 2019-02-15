package main

import (
	"context"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"github.com/ielab/wikie"
	"gopkg.in/olivere/elastic.v5"
	"net/http"
)

func (s server) search(c *gin.Context) {
	session := sessions.Default(c)
	token := session.Get("token")
	if token == nil {
		c.Redirect(http.StatusTemporaryRedirect, "/")
		return
	}
	if _, ok := s.sessions[token.(string)]; !ok {
		c.Redirect(http.StatusTemporaryRedirect, "/")
		return
	}

	if q := c.Query("q"); len(q) > 0 {
		result, err := s.esClient.Search("wikie").Query(elastic.NewSimpleQueryStringQuery(q)).Do(context.Background())
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		var pages []wikie.Page
		for _, hit := range result.Hits.Hits {
			page, err := wikie.GetPage(s.esClient, hit.Id)
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
			if ok, err := wikie.HasPermission(s.permissionDB, session.Get("username").(string), page.Path, wikie.PermissionRead); err == nil && ok {
				pages = append(pages, page)
			}
		}
		c.HTML(http.StatusOK, "search.html", struct {
			Pages []wikie.Page
			Query string
		}{pages, q})
		return
	}
	c.HTML(http.StatusOK, "search.html", nil)
}
