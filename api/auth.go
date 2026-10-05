package api

import (
	"net/http"
	"strings"

	"github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/util/common"
	"github.com/gin-gonic/gin"
)

func (a *ApiService) AuthMeta(c *gin.Context) {
	jsonObj(c, a.AuthService.PublicAuthMethods(), nil)
}

func (a *ApiService) AuthCheck(c *gin.Context) {
	username := GetLoginUser(c)
	result := gin.H{"authenticated": username != ""}
	if username != "" {
		result["username"] = username
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) OIDCStart(c *gin.Context) {
	result, err := a.AuthService.BeginOIDC(c.Request.Context())
	jsonObj(c, result, err)
}

func (a *ApiService) OIDCCallback(c *gin.Context) {
	if providerError := c.Query("error"); providerError != "" {
		c.String(http.StatusBadRequest, "OIDC error: %s", providerError)
		return
	}
	username, err := a.AuthService.FinishOIDC(c.Request.Context(), c.Query("state"), c.Query("code"))
	if err != nil {
		logger.Warning("OIDC login failed: ", err)
		c.String(http.StatusUnauthorized, "OIDC login failed")
		return
	}
	maxAge, _ := a.SettingService.GetSessionMaxAge()
	if err := SetLoginUser(c, username, maxAge); err != nil {
		c.String(http.StatusInternalServerError, "Unable to create session")
		return
	}
	a.UserService.RecordLogin(username, getRemoteIp(c))
	logger.Audit(username, "OIDC login")
	path, _ := a.SettingService.GetWebPath()
	c.Redirect(http.StatusTemporaryRedirect, path)
}

func (a *ApiService) SecuritySummary(c *gin.Context) {
	result, err := a.AuthService.SecuritySummary(GetLoginUser(c))
	jsonObj(c, result, err)
}

func (a *ApiService) TOTPBegin(c *gin.Context) {
	result, err := a.UserService.BeginTOTP(GetLoginUser(c), "S-UI Next")
	jsonObj(c, result, err)
}

func (a *ApiService) TOTPEnable(c *gin.Context) {
	codes, err := a.UserService.EnableTOTP(GetLoginUser(c), c.Request.FormValue("code"))
	if err == nil {
		logger.Audit(GetLoginUser(c), "enabled TOTP two-factor authentication")
	}
	jsonObj(c, gin.H{"recoveryCodes": codes}, err)
}

func (a *ApiService) TOTPDisable(c *gin.Context) {
	err := a.UserService.DisableTOTP(GetLoginUser(c), c.Request.FormValue("password"), c.Request.FormValue("code"), getRemoteIp(c))
	if err == nil {
		logger.Audit(GetLoginUser(c), "disabled TOTP two-factor authentication")
	}
	jsonMsg(c, "save", err)
}

func (a *ApiService) PasskeyRegistrationBegin(c *gin.Context) {
	options, sessionID, err := a.AuthService.BeginPasskeyRegistration(GetLoginUser(c), c.Request)
	jsonObj(c, gin.H{"options": options, "sessionId": sessionID}, err)
}

func (a *ApiService) PasskeyRegistrationFinish(c *gin.Context) {
	sessionID := strings.TrimSpace(c.GetHeader("X-WebAuthn-Session"))
	if sessionID == "" {
		sessionID = c.Query("sessionId")
	}
	name, err := a.AuthService.FinishPasskeyRegistration(GetLoginUser(c), sessionID, c.Query("name"), c.Request)
	if err == nil {
		logger.Audit(GetLoginUser(c), "registered a passkey")
	}
	jsonMsgObj(c, "save", gin.H{"name": name}, err)
}

func (a *ApiService) PasskeyLoginBegin(c *gin.Context) {
	username := strings.TrimSpace(c.Query("username"))
	if username == "" {
		username = strings.TrimSpace(c.Request.FormValue("username"))
	}
	options, sessionID, err := a.AuthService.BeginPasskeyLogin(username, c.Request)
	jsonObj(c, gin.H{"options": options, "sessionId": sessionID}, err)
}

func (a *ApiService) PasskeyLoginFinish(c *gin.Context) {
	sessionID := strings.TrimSpace(c.GetHeader("X-WebAuthn-Session"))
	if sessionID == "" {
		sessionID = c.Query("sessionId")
	}
	if sessionID == "" {
		jsonMsg(c, "", common.NewError("missing passkey session"))
		return
	}
	username, err := a.AuthService.FinishPasskeyLogin(sessionID, c.Request)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	maxAge, _ := a.SettingService.GetSessionMaxAge()
	if err := SetLoginUser(c, username, maxAge); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.UserService.RecordLogin(username, getRemoteIp(c))
	logger.Audit(username, "passkey login")
	jsonObj(c, gin.H{"username": username}, nil)
}

func (a *ApiService) PasskeyDelete(c *gin.Context) {
	err := a.AuthService.DeletePasskey(GetLoginUser(c), c.Request.FormValue("id"))
	if err == nil {
		logger.Audit(GetLoginUser(c), "deleted a passkey")
	}
	jsonMsg(c, "save", err)
}

func (a *ApiService) PasskeyRename(c *gin.Context) {
	err := a.AuthService.RenamePasskey(GetLoginUser(c), c.Request.FormValue("id"), c.Request.FormValue("name"))
	if err == nil {
		logger.Audit(GetLoginUser(c), "renamed a passkey")
	}
	jsonMsg(c, "save", err)
}
