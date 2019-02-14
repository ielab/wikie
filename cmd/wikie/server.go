package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/boltdb/bolt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/h2non/filetype"
	"github.com/ielab/wikie"
	"gopkg.in/olivere/elastic.v5"
	"io/ioutil"
	"log"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

//noinspection GoUnhandledErrorResult
func main() {
	config, err := wikie.ReadConfig("config.yml")
	if err != nil {
		panic(err)
	}

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
		session := sessions.Default(c)
		if session.Get("token") != nil {
			c.Redirect(http.StatusTemporaryRedirect, "/w/home")
			return
		}
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

			// Now we know the user is indeed not an admin.
			// So show the user permissions for what they have been granted.
			perms, err := wikie.GetUserPermissions(db, v.(string))
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
			c.HTML(http.StatusOK, "permissions.html", perms)
			return
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
			fmt.Println("add permission")
			if v := session.Get("username"); v != nil {
				for _, admin := range config.Admins {
					if admin == v {
						wikie.AddPermission(db, user, permPath, wikie.AccessType(access))
						c.Redirect(http.StatusFound, "/permissions")
						return
					}
				}

				if ok, err := wikie.HasPermission(db, v.(string), permPath, wikie.AccessType(access)); err == nil && ok {
					fmt.Println(v, user, permPath)
					err := wikie.AddPermission(db, user, permPath, wikie.AccessType(access))
					if err != nil {
						fmt.Println(err)
						c.Status(http.StatusInternalServerError)
						return
					}
					c.Redirect(http.StatusFound, "/permissions")
					return
				}
			}
		} else if v == "-" && ok {
			if v := session.Get("username"); v != nil {
				for _, admin := range config.Admins {
					if admin == v {
						wikie.RemovePermission(db, user, permPath, wikie.AccessType(access))
						c.Redirect(http.StatusFound, "/permissions")
						return
					}
				}
				if ok, err := wikie.HasPermission(db, v.(string), permPath, wikie.AccessType(access)); err == nil && ok {
					wikie.RemovePermission(db, user, permPath, wikie.AccessType(access))
					c.Redirect(http.StatusFound, "/permissions")
					return
				}
			}
		} else {
			c.Status(http.StatusInternalServerError)
			return
		}

		c.HTML(http.StatusForbidden, "forbidden.html", nil)
		return
	})

	g.GET("/storage", func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("token") == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
			return
		}

		permissions, err := wikie.GetPermissions(db)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}

		username := session.Get("username").(string)
		var files []string

		if p, ok := permissions[username]; ok {
			err := filepath.Walk("storage", func(path string, info os.FileInfo, err error) error {
				if info.IsDir() {
					return nil
				}
				for _, permission := range p {
					if strings.Contains(path, permission.Path) && permission.Access >= wikie.PermissionRead {
						files = append(files, path)
					}
				}
				return nil
			})
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
		}

		c.HTML(http.StatusOK, "storage.html", files)
	})
	g.GET("/storage/*file", func(c *gin.Context) {
		filePath := c.Param("file")

		session := sessions.Default(c)
		if session.Get("token") == nil {
			// Check to see if the referrer is public
			u, err := url.Parse(c.Request.Referer())
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
			p := strings.Split(u.Path, "/")
			if len(p) < 2 || p[1] != "public" {
				c.HTML(http.StatusForbidden, "forbidden.html", nil)
				return
			}

			// If the referrer is public, also check to see if the page is indeed public.
			if page, err := wikie.GetPage(esClient, path.Dir(filePath)); err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			} else if err == nil && !page.Public {
				c.HTML(http.StatusForbidden, "forbidden.html", nil)
				return
			}
		} else {
			// Check for permission to the page.
			if v := session.Get("username"); v != nil {
				if len(filePath) > 0 && filePath[len(filePath)-1] == '/' {
					c.Redirect(http.StatusTemporaryRedirect, path.Join("/w", filePath[:len(filePath)-1]))
					return
				}
				if ok, err := wikie.HasPermission(db, v.(string), filePath, wikie.PermissionRead); err == nil && !ok {
					c.HTML(http.StatusForbidden, "forbidden.html", nil)
					return
				} else if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}
			}
		}

		pathOnDisk := path.Join("storage", filePath)
		if _, err := os.Stat(pathOnDisk); err != nil {
			c.String(http.StatusForbidden, "forbidden")
			return
		}

		f, err := os.OpenFile(pathOnDisk, os.O_RDONLY, 0777)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		b, err := ioutil.ReadAll(f)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		t, err := filetype.MatchReader(f)
		if err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}

		c.Data(http.StatusOK, t.MIME.Value, b)
	})
	g.POST("/storage", func(c *gin.Context) {
		session := sessions.Default(c)
		if session.Get("token") == nil {
			c.Redirect(http.StatusTemporaryRedirect, "/login/rocket")
			return
		}

		filePath := "/" + strings.Join(strings.Split(c.PostForm("file"), "/")[1:], "/")
		fmt.Println(filePath)
		if v, ok := c.GetPostForm("action"); v == "Delete" && ok {
			if ok, err := wikie.HasPermission(db, session.Get("username").(string), filePath, wikie.PermissionWrite); err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			} else if !ok {
				fmt.Println(err)
				c.String(http.StatusForbidden, "forbidden")
				return
			}

			fileOnDisk := c.PostForm("file")
			fmt.Println(fileOnDisk)
			if _, err := os.Stat(fileOnDisk); err == nil || os.IsExist(err) {
				err = os.Remove(fileOnDisk)
				if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}
			}

		} else if v == "Upload" && ok {
			file, err := c.FormFile("uploadfile")
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}

			if ok, err := wikie.HasPermission(db, session.Get("username").(string), c.PostForm("namespace"), wikie.PermissionWrite); err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			} else if !ok {
				fmt.Println(err)
				c.String(http.StatusForbidden, "forbidden")
				return
			}

			uploadPath := path.Join("storage", c.PostForm("namespace"))

			err = os.MkdirAll(uploadPath, 0777)
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}

			filename := filepath.Base(file.Filename)
			err = c.SaveUploadedFile(file, path.Join(uploadPath, filename))
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}
		}

		c.Redirect(http.StatusFound, c.Request.Referer())
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

	g.GET("/public/*page", func(c *gin.Context) {
		pagePath := c.Param("page")
		if len(pagePath) > 0 && pagePath[len(pagePath)-1] == '/' {
			c.Redirect(http.StatusTemporaryRedirect, path.Join("/public", pagePath[:len(pagePath)-1]))
			return
		}

		page, err := wikie.GetPage(esClient, pagePath)
		if err != nil {
			fmt.Println(err)
			c.HTML(http.StatusForbidden, "forbidden.html", nil)
			return
		}

		if page.Public {
			c.HTML(http.StatusOK, "public.html", page)
			return
		}

		c.HTML(http.StatusForbidden, "forbidden.html", nil)
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
				if ok, err := wikie.HasPermission(db, v.(string), pagePath, wikie.PermissionRead); err == nil && !ok {
					c.HTML(http.StatusForbidden, "forbidden.html", nil)
					return
				} else if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}

				permissions, err := wikie.GetPermissions(db)
				if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}

				username := session.Get("username").(string)
				var files []string
				if p, ok := permissions[username]; ok {
					err := filepath.Walk(path.Join("storage", pagePath), func(path string, info os.FileInfo, err error) error {
						if err != nil {
							return nil
						}
						if info.IsDir() {
							return nil
						}
						for _, permission := range p {
							if strings.Contains(path, permission.Path) && permission.Access >= wikie.PermissionRead {
								files = append(files, path)
							}
						}
						return nil
					})
					if err != nil {
						fmt.Println(err)
						c.Status(http.StatusInternalServerError)
						return
					}
				}

				page.Files = files

				c.HTML(http.StatusOK, "notfound.html", wikie.Page{Path: pagePath, Files: files})
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

			permissions, err := wikie.GetPermissions(db)
			if err != nil {
				fmt.Println(err)
				c.Status(http.StatusInternalServerError)
				return
			}

			username := session.Get("username").(string)
			var files []string
			if p, ok := permissions[username]; ok {
				err := filepath.Walk(path.Join("storage", pagePath), func(path string, info os.FileInfo, err error) error {
					if err != nil || info.IsDir() {
						return nil
					}
					for _, permission := range p {
						if strings.Contains(path, permission.Path) && permission.Access >= wikie.PermissionRead {
							files = append(files, path)
						}
					}
					return nil
				})
				if err != nil {
					fmt.Println(err)
					c.Status(http.StatusInternalServerError)
					return
				}
			}

			page.Files = files

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
		var p wikie.Page
		err = json.Unmarshal(s, &p)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		
		p.Path = pagePath
		p.LastUpdated = time.Now().Format(time.RFC822)
		p.EditedBy = session.Get("username").(string)
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

		var p wikie.Page
		err = json.Unmarshal(s, &p)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}

		p.LastUpdated = time.Now().Format(time.RFC822)
		p.EditedBy = session.Get("username").(string)

		err = wikie.UpdatePage(esClient, pagePath, p)
		if err != nil {
			fmt.Println(err)
			c.Status(http.StatusInternalServerError)
			return
		}
		c.Status(http.StatusOK)
		return
	})

	fmt.Print(`
          _ _    _      
         (_) |  (_)     
__      ___| | ___  ___ 
\ \ /\ / / | |/ / |/ _ \
 \ V  V /| |   <| |  __/
  \_/\_/ |_|_|\_\_|\___|

author: h.scells@uq.net.au
version: 14.Feb.2019
`)

	log.Panic(http.ListenAndServe("0.0.0.0:"+config.Port, g))
}
