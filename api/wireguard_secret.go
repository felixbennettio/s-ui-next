package api

import (
	"strconv"

	"github.com/ciallothu/s-ui-next/util/common"
	"github.com/gin-gonic/gin"
)

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
