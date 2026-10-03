package api

import (
	"net"
	"net/http"
	"strings"

	"github.com/ciallothu/s-ui-next/logger"

	"github.com/gin-gonic/gin"
)

type Msg struct {
	Success bool        `json:"success"`
	Msg     string      `json:"msg"`
	Obj     interface{} `json:"obj"`
	Warning string      `json:"warning,omitempty"`
}

func getRemoteIp(c *gin.Context) string {
	value := c.GetHeader("X-Forwarded-For")
	if value != "" {
		for _, candidate := range strings.Split(value, ",") {
			candidate = strings.TrimSpace(candidate)
			if net.ParseIP(candidate) != nil {
				return candidate
			}
		}
	}
	return directPeerIP(c)
}

func getHostname(c *gin.Context) string {
	host := c.Request.Host
	if strings.Contains(host, ":") {
		host, _, _ = net.SplitHostPort(c.Request.Host)
		if strings.Contains(host, ":") {
			host = "[" + host + "]"
		}
	}
	return host
}

func jsonMsg(c *gin.Context, msg string, err error) {
	jsonMsgObj(c, msg, nil, err)
}

func jsonObj(c *gin.Context, obj interface{}, err error) {
	jsonMsgObj(c, "", obj, err)
}

func jsonMsgObj(c *gin.Context, msg string, obj interface{}, err error) {
	m := Msg{
		Obj: obj,
	}
	if err == nil {
		m.Success = true
		if msg != "" {
			m.Msg = msg
		}
	} else {
		m.Success = false
		m.Msg = msg + ": " + err.Error()
		logger.Warning("failed :", err)
	}
	c.JSON(http.StatusOK, m)
}

func pureJsonMsg(c *gin.Context, success bool, msg string) {
	if success {
		c.JSON(http.StatusOK, Msg{
			Success: true,
			Msg:     msg,
		})
	} else {
		c.JSON(http.StatusOK, Msg{
			Success: false,
			Msg:     msg,
		})
	}
}

func checkLogin(c *gin.Context) {
	if !IsLogin(c) {
		if c.GetHeader("X-Requested-With") == "XMLHttpRequest" {
			pureJsonMsg(c, false, "Invalid login")
		} else {
			c.Redirect(http.StatusTemporaryRedirect, "/login")
		}
		c.Abort()
	} else {
		c.Next()
	}
}
