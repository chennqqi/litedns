package main

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sirupsen/logrus"
)

type Web struct {
	WebConf
	app *App
	svr *http.Server
}

type CR struct {
	Code    int    `json:"code"`
	Message string `json:"msg"`
}

func NewWeb(conf *WebConf, app *App) *Web {
	w := &Web{}
	w.WebConf = *conf
	w.app = app

	r := gin.Default()
	r.GET("/health", w.health)
	r.GET("/resolve/:name", w.authHandler, w.getResolve)
	r.POST("/resolve/:name", w.authHandler, w.readOnly, w.setResolve)
	w.svr = &http.Server{
		Addr:    conf.Addr,
		Handler: r,
	}
	return w
}

// middleware: check token
func (self *Web) authHandler(c *gin.Context) {
	token := c.GetHeader("Authorization")
	if token != self.Token {
		c.JSON(http.StatusUnauthorized, CR{
			Code:    1,
			Message: "token not match",
		})
		logrus.Infof("[web.go::authHandler] unexpect token: %v", token)
		c.Abort()
		return
	}
}

// GET /health
// 200
func (self *Web) health(c *gin.Context) {
	c.JSON(http.StatusOK, CR{
		Code: 0, Message: "OK",
	})
}

// middleware: check readonly
func (self *Web) readOnly(c *gin.Context) {
	if self.Readonly {
		c.JSON(http.StatusForbidden, CR{
			Code: 1, Message: "webconf set readonly",
		})
		c.Abort()
		return
	}
}

// GET /resolve/:name
// 200
// Content-Type: application/json
// {
// 	"name": ""
// 	"A":[
// 	],
// 	"AAAA": [
// 	],
// 	"expire" : "240s"
// 	"ttl" : 10
// }
func (self *Web) getResolve(c *gin.Context) {
	name := c.Param("name")
	app := self.app

	r, err := app.Get(name)
	if err != nil {
		c.JSON(http.StatusNotFound, CR{
			Code:    1,
			Message: "not found",
		})
		return
	}
	c.JSON(http.StatusOK, r)
}

// POST /server/:name
// Content-Type: application/json
// {
// 	"name": "name"
// 	"A":[
// 	],
// 	"AAAA": [
// 	]
// 	"expire" : "120s"
// 	"ttl" : 10
// }
// 200
func (self *Web) setResolve(c *gin.Context) {
	app := self.app

	var r Resolve
	err := c.BindJSON(&r)
	if err != nil {
		logrus.Errorf("[web.go::setResolve] error: %v", err)
		c.JSON(http.StatusBadRequest, CR{
			Code: 1, Message: err.Error(),
		})
		return
	}
	r.Fix()
	err = app.Set(r.Name, r)
	if err != nil {
		logrus.Errorf("[web.go::setResolve] app.Set error: %v", err)
		c.JSON(http.StatusBadRequest, CR{
			Code: 1, Message: err.Error(),
		})
		return
	}
	c.JSON(http.StatusOK, CR{
		Code: 0, Message: "OK",
	})
}

// POST /server/{name}
// set a server
func (self *Web) Run() error {
	return self.svr.ListenAndServe()
}

func (self *Web) Shutdown(ctx context.Context) error {
	return self.svr.Shutdown(ctx)
}
