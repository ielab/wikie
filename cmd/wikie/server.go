package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/ielab/wikie"
	"golang.org/x/oauth2"
	"gopkg.in/olivere/elastic.v5"
	"log"
	"net/http"
	"net/url"
	"path"
	"strconv"
	"time"
)

var (
	oAuth2Config *oauth2.Config
	esClient     elastic.Client
)

//noinspection GoUnhandledErrorResult
func main() {
	config, err := wikie.ReadConfig("config.yml")
	if err != nil {
		panic(err)
	}

	fmt.Println(config)

	esClient, err := elastic.NewClient(elastic.SetURL(config.ElasticsearchConfig.Hosts...))
	if err != nil {
		panic(err)
	}

	db, err := bolt.Open("perms.db", 0600, nil)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	wikie.Init(db, config.Admins)

	store := cookie.NewStore([]byte(config.CookieSecret))

	g := gin.Default()
	// Session middleware.
	g.Use(sessions.Sessions("wikie", store))

	g.LoadHTMLGlob("web/*.html")
	g.Static("/static/", "web/static")

	g.GET("/login/rocket", func(c *gin.Context) {
		c.HTML(http.StatusOK, "login.html", nil)
	})
	g.GET("/logout/rocket", func(c *gin.Context) {
		session := sessions.Default(c)
		session.Clear()
		session.Save()
		c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
	})
	g.GET("/", func(c *gin.Context) {
		c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
		return
	})
	g.POST("/login/rocket", func(c *gin.Context) {
		email, _ := c.GetPostForm("email")
		password, _ := c.GetPostForm("password")
		client := &http.Client{}
		resp, err := client.PostForm(config.RocketChat+"/api/v1/login", url.Values{
			"user":     []string{email},
			"password": []string{password},
		})
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		var i map[string]interface{}
		err = json.NewDecoder(resp.Body).Decode(&i)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}

		d := i["data"].(map[string]interface{})
		f := d["me"].(map[string]interface{})

		session := sessions.Default(c)
		session.Set("token", d["authToken"])
		session.Set("username", f["username"])
		session.Save()
		c.Request.Method = "GET"
		c.Redirect(http.StatusFound, "/w/home")
		return
	})
	g.GET("/permissions", func(c *gin.Context) {
		session := sessions.Default(c)
		v := session.Get("token")
		if v == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
			return
		}

		// Check for permission to the page.
		if v := session.Get("username"); v != nil {
			fmt.Println(config.Admins, v)
			for _, admin := range config.Admins {
				if admin == v {
					perms, err := wikie.GetPermissions(db)
					if err != nil {
						fmt.Println(err)
						c.Status(http.StatusInternalServerError)
						return
					}
					c.HTML(http.StatusOK, "permissions.html", perms)
					return
				}
			}
		}
		c.HTML(http.StatusForbidden, "forbidden.html", nil)
		return
	})
	g.POST("/permissions", func(c *gin.Context) {
		user := c.PostForm("user")
		permPath := c.PostForm("path")
		access, err := strconv.Atoi(c.PostForm("access"))
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}

		session := sessions.Default(c)
		v := session.Get("token")
		if v == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
			return
		}

		if v, ok := c.GetPostForm("action"); v == "+" && ok {
			wikie.AddPermission(db, user, permPath, wikie.AccessType(access))
			fmt.Println("add permission")
			if v := session.Get("username"); v != nil {
				fmt.Println(config.Admins, v)
				for _, admin := range config.Admins {
					if admin == v {
						perms, err := wikie.GetPermissions(db)
						if err != nil {
							fmt.Println(err)
							c.Status(http.StatusInternalServerError)
							return
						}
						c.HTML(http.StatusOK, "permissions.html", perms)
						return
					}
				}
			}
		} else if v == "-" && ok {
			wikie.RemovePermission(db, user, permPath, wikie.AccessType(access))
			fmt.Println("remove permission")
			if v := session.Get("username"); v != nil {
				fmt.Println(config.Admins, v)
				for _, admin := range config.Admins {
					if admin == v {
						perms, err := wikie.GetPermissions(db)
						if err != nil {
							fmt.Println(err)
							c.Status(http.StatusInternalServerError)
							return
						}
						c.HTML(http.StatusOK, "permissions.html", perms)
						return
					}
				}
			}
		} else {
			c.Status(http.StatusInternalServerError)
			return
		}
	})

	g.GET("/search", func(c *gin.Context) {
		session := sessions.Default(c)
		v := session.Get("token")
		if v == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
			return
		}

		if q := c.Query("q"); len(q) > 0 {
			result, err := esClient.Search("wikie").Query(elastic.NewSimpleQueryStringQuery(q)).Do(context.Background())
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
			var pages []wikie.Page
			for _, hit := range result.Hits.Hits {
				page, err := wikie.GetPage(esClient, hit.Id)
				if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}
				if ok, err := wikie.HasPermission(db, session.Get("username").(string), page.Path, wikie.PermissionRead); err == nil && ok {
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
	})

	wiki := g.Group("/w")
	// Permission middleware.
	wiki.GET("/*page", func(c *gin.Context) {
		session := sessions.Default(c)
		v := session.Get("token")
		if v == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
			return
		}

		// Check for permission to the page.
		if v := session.Get("username"); v != nil {
			pagePath := c.Param("page")
			if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
				c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
				return
			}
			if ok, err := wikie.HasPermission(db, v.(string), pagePath, wikie.PermissionRead); err == nil && !ok {
				c.HTML(http.StatusForbidden, "forbidden.html", nil)
				return
			} else if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
		}

		pagePath := c.Param("page")
		if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
			c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
			return
		}

		page, err := wikie.GetPage(esClient, pagePath)
		if err != nil {
			// Check for permission to the page.
			if v := session.Get("username"); v != nil {
				pagePath := c.Param("page")
				if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
					c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
					return
				}
				if ok, err := wikie.HasPermission(db, v.(string), pagePath, wikie.PermissionWrite); err == nil && !ok {
					c.HTML(http.StatusForbidden, "forbidden.html", nil)
					return
				} else if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}
				c.HTML(http.StatusOK, "notfound.html", wikie.Page{Path: pagePath})
				return
			} else if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
			c.HTML(http.StatusForbidden, "forbidden.html", nil)
			return
		}

		if _, ok := c.GetQuery("edit"); ok {
			if ok, err := wikie.HasPermission(db, session.Get("username").(string), pagePath, wikie.PermissionWrite); err == nil && !ok {
				c.HTML(http.StatusForbidden, "forbidden.html", nil)
				return
			} else if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
			c.HTML(http.StatusOK, "edit.html", page)
			return
		}

		c.HTML(http.StatusOK, "page.html", page)
		return
	})
	wiki.PUT("/*page", func(c *gin.Context) {
		session := sessions.Default(c)
		v := session.Get("token")
		if v == nil {
			c.HTML(http.StatusForbidden, "forbidden.html", nil)
			return
		}

		// Check for permission to the page.
		if v := session.Get("username"); v != nil {
			pagePath := c.Param("page")
			if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
				c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
				return
			}
			if ok, err := wikie.HasPermission(db, v.(string), pagePath, wikie.PermissionWrite); err == nil && !ok {
				c.HTML(http.StatusForbidden, "forbidden.html", nil)
				return
			} else if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
		}

		pagePath := c.Param("page")
		if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
			c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
		}
		s, err := c.GetRawData()
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		var i map[string]interface{}
		err = json.Unmarshal(s, &i)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		p := wikie.Page{
			Path:        pagePath,
			Body:        i,
			LastUpdated: time.Now().Format(time.RFC822),
			EditedBy:    session.Get("username").(string),
		}
		err = wikie.NewPage(esClient, pagePath, p)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
		return
	})
	wiki.POST("/*page", func(c *gin.Context) {
		session := sessions.Default(c)
		v := session.Get("token")
		if v == nil {
			c.HTML(http.StatusForbidden, "forbidden.html", nil)
			return
		}

		pagePath := c.Param("page")
		if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
			c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
		}

		// Check for permission to the page.
		if v := session.Get("username"); v != nil {
			pagePath := c.Param("page")
			if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
				c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", pagePath[:len(pagePath)-1]))
				return
			}
			if ok, err := wikie.HasPermission(db, v.(string), pagePath, wikie.PermissionWrite); err == nil && !ok {
				c.HTML(http.StatusForbidden, "forbidden.html", nil)
				return
			} else if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
		}

		s, err := c.GetRawData()
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		var i map[string]interface{}
		err = json.Unmarshal(s, &i)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		p := wikie.Page{
			Path:        pagePath,
			Body:        i,
			LastUpdated: time.Now().Format(time.RFC822),
			EditedBy:    session.Get("username").(string),
		}
		err = wikie.UpdatePage(esClient, pagePath, p)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
		return
	})
	log.Panic(http.ListenAndServe("0.0.0.0:"+config.Port, g))
}
