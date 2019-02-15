package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"io/ioutil"
	"math/rand"
	"net/http"
	"net/url"
)

func randState() string {
	b := make([]byte, 32)
	rand.Read(b)
	return base64.StdEncoding.EncodeToString(b)
}

func (s server) logout(c *gin.Context) {
	session := sessions.Default(c)
	delete(s.sessions, session.Get("token").(string))
	session.Clear()
	session.Save()
	c.Redirect(http.StatusTemporaryRedirect, "/")
}

func (s server) loginRocketView(c *gin.Context) {
	c.HTML(http.StatusOK, "login.html", nil)
}

func (s server) loginRocket(c *gin.Context) {
	email, _ := c.GetPostForm("email")
	password, _ := c.GetPostForm("password")
	client := &http.Client{}
	resp, err := client.PostForm(s.config.RocketChatConfig.URL+"/api/v1/login", url.Values{
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
	token := randState()
	session.Set("token", token)
	session.Set("username", f["username"])
	session.Save()
	s.sessions[token] = true
	c.Request.Method = "GET"
	c.Redirect(http.StatusFound, "/w/home")
	return
}

func (s server) loginOAuth2(c *gin.Context) {
	state := randState()
	session := sessions.Default(c)
	session.Set("state", state)
	session.Save()
	c.Redirect(http.StatusFound, s.oAuthConf.AuthCodeURL(state))
}

func (s server) loginOAuth2Callback(c *gin.Context) {
	session := sessions.Default(c)
	state := session.Get("state")
	if state != c.Query("state") {
		c.AbortWithError(http.StatusUnauthorized, fmt.Errorf("invalid session sate"))
		return
	}
	tok, err := s.oAuthConf.Exchange(context.Background(), c.Query("code"))
	if err != nil {
		c.AbortWithError(http.StatusBadRequest, err)
		return
	}

	if s.config.OAuth2Config.Provider == "Google" {
		client := s.oAuthConf.Client(context.Background(), tok)
		email, err := client.Get("https://www.googleapis.com/oauth2/v3/userinfo")
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}
		defer email.Body.Close()

		data, _ := ioutil.ReadAll(email.Body)

		var userInfo map[string]interface{}
		err = json.Unmarshal(data, &userInfo)
		if err != nil {
			c.AbortWithError(http.StatusBadRequest, err)
			return
		}

		session.Clear()

		token := randState()
		session.Set("token", token)
		session.Set("username", userInfo["email"])
		session.Save()
		s.sessions[token] = true
		c.Redirect(http.StatusFound, "/w/home")
		return
	}

	c.Redirect(http.StatusFound, "/")
	return
}
