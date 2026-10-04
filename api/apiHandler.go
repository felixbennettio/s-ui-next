package api

import (
	"github.com/felixbennettio/s-ui-next/util/common"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	ApiService
	apiv2 *APIv2Handler
}

func NewAPIHandler(g *gin.RouterGroup, a2 *APIv2Handler) {
	a := &APIHandler{
		apiv2: a2,
	}
	a.initRouter(g)
}

func (a *APIHandler) initRouter(g *gin.RouterGroup) {
	g.Use(func(c *gin.Context) {
		action := c.Param("postAction")
		if action == "" {
			action = c.Param("getAction")
		}
		public := map[string]bool{
			"login": true, "logout": true, "auth-meta": true, "auth-check": true, "oidc-start": true, "oidc-callback": true,
			"passkey-login-begin": true, "passkey-login-finish": true,
		}
		if !public[action] {
			checkLogin(c)
		}
	})
	g.POST("/:postAction", a.postHandler)
	g.GET("/:getAction", a.getHandler)
}

func (a *APIHandler) postHandler(c *gin.Context) {
	loginUser := GetLoginUser(c)
	action := c.Param("postAction")

	switch action {
	case "login":
		a.ApiService.Login(c)
	case "totp-enable":
		a.ApiService.TOTPEnable(c)
	case "totp-disable":
		a.ApiService.TOTPDisable(c)
	case "passkey-register-finish":
		a.ApiService.PasskeyRegistrationFinish(c)
	case "passkey-login-finish":
		a.ApiService.PasskeyLoginFinish(c)
	case "passkey-delete":
		a.ApiService.PasskeyDelete(c)
	case "passkey-rename":
		a.ApiService.PasskeyRename(c)
	case "changePass":
		a.ApiService.ChangePass(c)
		a.apiv2.ReloadTokens()
	case "save":
		a.ApiService.Save(c, loginUser)
	case "wireguardSecret":
		a.ApiService.CopyWireGuardSecret(c)
	case "wireguardExport":
		a.ApiService.ExportWireGuard(c)
	case "restartApp":
		a.ApiService.RestartApp(c)
	case "restartSb":
		a.ApiService.RestartSb(c)
	case "linkConvert":
		a.ApiService.LinkConvert(c)
	case "subConvert":
		a.ApiService.SubConvert(c)
	case "importdb":
		a.ApiService.ImportDb(c)
	case "addToken":
		a.ApiService.AddToken(c)
		a.apiv2.ReloadTokens()
	case "deleteToken":
		a.ApiService.DeleteToken(c)
		a.apiv2.ReloadTokens()
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}

func (a *APIHandler) getHandler(c *gin.Context) {
	action := c.Param("getAction")

	switch action {
	case "logout":
		a.ApiService.Logout(c)
	case "auth-meta":
		a.ApiService.AuthMeta(c)
	case "auth-check":
		a.ApiService.AuthCheck(c)
	case "oidc-start":
		a.ApiService.OIDCStart(c)
	case "oidc-callback":
		a.ApiService.OIDCCallback(c)
	case "security":
		a.ApiService.SecuritySummary(c)
	case "totp-begin":
		a.ApiService.TOTPBegin(c)
	case "passkey-register-begin":
		a.ApiService.PasskeyRegistrationBegin(c)
	case "passkey-login-begin":
		a.ApiService.PasskeyLoginBegin(c)
	case "load":
		a.ApiService.LoadData(c)
	case "inbounds", "outbounds", "endpoints", "services", "tls", "clients", "config":
		err := a.ApiService.LoadPartialData(c, []string{action})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "users":
		a.ApiService.GetUsers(c)
	case "settings":
		a.ApiService.GetSettings(c)
	case "stats":
		a.ApiService.GetStats(c)
	case "status":
		a.ApiService.GetStatus(c)
	case "onlines":
		a.ApiService.GetOnlines(c)
	case "logs":
		a.ApiService.GetLogs(c)
	case "structured-logs":
		a.ApiService.GetStructuredLogs(c)
	case "analytics-usage":
		a.ApiService.GetFilteredUsage(c)
	case "analytics-stats":
		a.ApiService.GetFilteredStats(c)
	case "analytics-connections":
		a.ApiService.GetConnectionAnalytics(c)
	case "analytics-address-info":
		a.ApiService.GetConnectionAddressInfo(c)
	case "changes":
		a.ApiService.CheckChanges(c)
	case "keypairs":
		a.ApiService.GetKeypairs(c)
	case "getdb":
		a.ApiService.GetDb(c)
	case "tokens":
		a.ApiService.GetTokens(c)
	case "singbox-config":
		a.ApiService.GetSingboxConfig(c)
	case "checkOutbound":
		a.ApiService.GetCheckOutbound(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}
