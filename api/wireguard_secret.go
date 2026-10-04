package api

import (
	"net/http"
	"strconv"

	"github.com/felixbennettio/s-ui-next/util/common"
	"github.com/gin-gonic/gin"
)

func (a *APIv3Handler) copyWireGuardSecret(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	var body struct {
		ID        uint   `json:"id"`
		Field     string `json:"field"`
		PublicKey string `json:"publicKey"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	secret, err := a.EndpointService.WireGuardSecret(body.ID, body.Field, body.PublicKey)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, secret)
}

func (a *ApiService) CopyWireGuardSecret(c *gin.Context) {
	c.Header("Cache-Control", "no-store")
	id, err := strconv.ParseUint(c.PostForm("id"), 10, 32)
	if err != nil || id == 0 {
		jsonObj(c, nil, common.NewError("invalid WireGuard endpoint ID"))
		return
	}
	secret, err := a.EndpointService.WireGuardSecret(uint(id), c.PostForm("field"), c.PostForm("publicKey"))
	jsonObj(c, secret, err)
}
