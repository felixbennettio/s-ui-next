package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/felixbennettio/s-ui-next/config"
	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/service"
	"github.com/felixbennettio/s-ui-next/util"
	"github.com/felixbennettio/s-ui-next/util/common"

	"github.com/gin-gonic/gin"
)

// APIv3Handler is the stable, JSON-first API consumed by the mobile app. It
// intentionally sits beside the legacy web APIs so existing installations and
// third-party scripts keep working.
type APIv3Handler struct {
	ApiService
	apiv2 *APIv2Handler
}

type apiV3Response struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

type resourceMutation struct {
	Action    string          `json:"action"`
	Data      json.RawMessage `json:"data"`
	InitUsers []uint          `json:"initUsers,omitempty"`
	Apply     *bool           `json:"apply,omitempty"`
}

func NewAPIv3Handler(g *gin.RouterGroup, apiv2 *APIv2Handler) *APIv3Handler {
	a := &APIv3Handler{apiv2: apiv2}
	a.initRouter(g)
	return a
}

func (a *APIv3Handler) initRouter(g *gin.RouterGroup) {
	g.POST("/auth/login", a.login)
	g.GET("/auth/methods", a.authMethods)

	protected := g.Group("")
	protected.Use(a.checkToken)
	protected.GET("/meta", a.meta)
	protected.GET("/me", a.me)
	protected.DELETE("/auth/token", a.logout)
	protected.GET("/auth/security", a.securitySummary)
	protected.POST("/auth/totp/begin", a.totpBegin)
	protected.POST("/auth/totp/enable", a.totpEnable)
	protected.POST("/auth/totp/disable", a.totpDisable)
	protected.POST("/auth/passkeys/register/begin", a.passkeyRegisterBegin)
	protected.POST("/auth/passkeys/register/finish", a.passkeyRegisterFinish)
	protected.PATCH("/auth/passkeys/:id", a.passkeyRename)
	protected.DELETE("/auth/passkeys/:id", a.passkeyDelete)
	protected.GET("/bootstrap", a.bootstrap)

	protected.GET("/resources/:resource", a.getResource)
	protected.POST("/resources/:resource", a.saveResource)
	protected.POST("/wireguard/export", a.exportWireGuard)
	protected.POST("/wireguard/secret", a.copyWireGuardSecret)
	protected.POST("/wireguard/generate-psk", a.generateWireGuardPsk)

	protected.GET("/status", a.status)
	protected.GET("/onlines", a.onlines)
	protected.GET("/analytics/stats", a.stats)
	protected.GET("/analytics/usage", a.usage)
	protected.GET("/analytics/connections", a.connections)
	protected.GET("/analytics/address-info", a.addressInfo)
	protected.GET("/logs", a.logs)
	protected.GET("/changes", a.changes)

	protected.GET("/users", a.users)
	protected.PATCH("/users/:id", a.changeCredentials)
	protected.GET("/tokens", a.tokens)
	protected.POST("/tokens", a.addToken)
	protected.DELETE("/tokens/:id", a.deleteToken)

	protected.GET("/backup/database", a.downloadDatabase)
	protected.POST("/backup/database", a.importDatabase)
	protected.GET("/backup/singbox", a.downloadSingboxConfig)

	protected.POST("/actions/:action", a.action)
	protected.POST("/tools/link-convert", a.linkConvert)
	protected.POST("/tools/sub-convert", a.subConvert)
	protected.POST("/tools/keypair", a.keypair)
	protected.GET("/tools/check-outbound", a.checkOutbound)
}

func v3OK(c *gin.Context, data interface{}) {
	c.JSON(http.StatusOK, apiV3Response{Success: true, Data: data})
}

func v3Error(c *gin.Context, status int, err error) {
	if status < 400 {
		status = http.StatusBadRequest
	}
	c.JSON(status, apiV3Response{Success: false, Error: err.Error()})
}

func queryInt(c *gin.Context, key string, fallback int) int {
	value, err := strconv.Atoi(c.Query(key))
	if err != nil {
		return fallback
	}
	return value
}

func queryInt64(c *gin.Context, key string) int64 {
	value, _ := strconv.ParseInt(c.Query(key), 10, 64)
	return value
}

func (a *APIv3Handler) checkToken(c *gin.Context) {
	username := a.apiv2.findUsername(c)
	if username == "" {
		v3Error(c, http.StatusUnauthorized, common.NewError("invalid or expired API token"))
		c.Abort()
		return
	}
	c.Set("apiUsername", username)
	c.Next()
}

func apiUsername(c *gin.Context) string {
	username, _ := c.Get("apiUsername")
	value, _ := username.(string)
	return value
}

func requestToken(c *gin.Context) string {
	authorization := strings.TrimSpace(c.GetHeader("Authorization"))
	if strings.HasPrefix(strings.ToLower(authorization), "bearer ") {
		return strings.TrimSpace(authorization[7:])
	}
	if token := strings.TrimSpace(c.GetHeader("Token")); token != "" {
		return token
	}
	return strings.TrimSpace(c.GetHeader("X-API-Token"))
}

func (a *APIv3Handler) login(c *gin.Context) {
	var body struct {
		Username   string `json:"username" form:"username"`
		Password   string `json:"password" form:"password"`
		Code       string `json:"code" form:"code"`
		ExpiryDays int64  `json:"expiryDays" form:"expiryDays"`
	}
	if err := c.ShouldBind(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	body.Username = strings.TrimSpace(body.Username)
	if body.Username == "" || body.Password == "" {
		v3Error(c, http.StatusBadRequest, common.NewError("username and password are required"))
		return
	}
	if err := service.ValidateTokenOptions(body.ExpiryDays, "S-UI Next Mobile"); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	attemptKey := loginAttemptKey(c, body.Username)
	if allowed, retryAfter := loginAllowed(attemptKey); !allowed {
		v3Error(c, http.StatusTooManyRequests, common.NewErrorf("too many login attempts; try again in %s", retryAfter))
		return
	}
	user, err := a.UserService.CheckPassword(body.Username, body.Password, getRemoteIp(c))
	if err != nil {
		recordLoginFailure(attemptKey)
		v3Error(c, http.StatusUnauthorized, err)
		return
	}
	if user.TOTPEnabled && strings.TrimSpace(body.Code) == "" {
		v3OK(c, gin.H{"requires2FA": true})
		return
	}
	if user.TOTPEnabled && !a.UserService.VerifySecondFactor(user, body.Code) {
		recordLoginFailure(attemptKey)
		v3Error(c, http.StatusUnauthorized, common.NewError("invalid TOTP or recovery code"))
		return
	}
	username := user.Username
	clearLoginFailures(attemptKey)
	a.UserService.RecordLogin(username, getRemoteIp(c))
	token, err := a.UserService.AddToken(username, body.ExpiryDays, "S-UI Next Mobile")
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	a.apiv2.ReloadTokens()
	logger.Audit(username, "mobile API login")
	v3OK(c, gin.H{"token": token, "username": username, "expiryDays": body.ExpiryDays})
}

func (a *APIv3Handler) logout(c *gin.Context) {
	username := apiUsername(c)
	if err := a.UserService.DeleteTokenValue(username, requestToken(c)); err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	a.apiv2.ReloadTokens()
	logger.Audit(username, "mobile API token revoked")
	v3OK(c, gin.H{"revoked": true})
}

func (a *APIv3Handler) meta(c *gin.Context) {
	v3OK(c, gin.H{
		"apiVersion": "3", "panelVersion": config.GetVersion(), "panelName": config.GetName(),
		"features": []string{"resources", "usage-filter", "stats-filter", "structured-logs", "audit", "backup", "totp", "oidc", "passkey", "wireguard-export", "wireguard-relay", "wireguard-secret-copy", "warp-egress", "transactional-apply"},
	})
}

func (a *APIv3Handler) authMethods(c *gin.Context) { v3OK(c, a.AuthService.PublicAuthMethods()) }

func (a *APIv3Handler) securitySummary(c *gin.Context) {
	value, err := a.AuthService.SecuritySummary(apiUsername(c))
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) totpBegin(c *gin.Context) {
	value, err := a.UserService.BeginTOTP(apiUsername(c), "S-UI Next")
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) totpEnable(c *gin.Context) {
	var body struct {
		Code string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	codes, err := a.UserService.EnableTOTP(apiUsername(c), body.Code)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	logger.Audit(apiUsername(c), "enabled TOTP two-factor authentication")
	v3OK(c, gin.H{"recoveryCodes": codes})
}

func (a *APIv3Handler) totpDisable(c *gin.Context) {
	var body struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	if err := a.UserService.DisableTOTP(apiUsername(c), body.Password, body.Code, getRemoteIp(c)); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	logger.Audit(apiUsername(c), "disabled TOTP two-factor authentication")
	v3OK(c, gin.H{"disabled": true})
}

func (a *APIv3Handler) passkeyRegisterBegin(c *gin.Context) {
	options, sessionID, err := a.AuthService.BeginPasskeyRegistration(apiUsername(c), c.Request)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, gin.H{"options": options, "sessionId": sessionID})
}

func (a *APIv3Handler) passkeyRegisterFinish(c *gin.Context) {
	sessionID := strings.TrimSpace(c.GetHeader("X-WebAuthn-Session"))
	if sessionID == "" {
		sessionID = strings.TrimSpace(c.Query("sessionId"))
	}
	name, err := a.AuthService.FinishPasskeyRegistration(apiUsername(c), sessionID, c.Query("name"), c.Request)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	logger.Audit(apiUsername(c), "registered a passkey")
	v3OK(c, gin.H{"registered": true, "name": name})
}

func (a *APIv3Handler) passkeyRename(c *gin.Context) {
	var body struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	if err := a.AuthService.RenamePasskey(apiUsername(c), c.Param("id"), body.Name); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	logger.Audit(apiUsername(c), "renamed a passkey")
	v3OK(c, gin.H{"renamed": true})
}

func (a *APIv3Handler) passkeyDelete(c *gin.Context) {
	if err := a.AuthService.DeletePasskey(apiUsername(c), c.Param("id")); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	logger.Audit(apiUsername(c), "deleted a passkey")
	v3OK(c, gin.H{"deleted": true})
}

func (a *APIv3Handler) me(c *gin.Context) {
	v3OK(c, gin.H{"username": apiUsername(c)})
}

func (a *APIv3Handler) bootstrap(c *gin.Context) {
	data, err := a.getData(c)
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	users, _ := a.UserService.GetUsers()
	settings, _ := a.SettingService.GetAllSetting()
	status := a.ServerService.GetStatus("cpu,mem,dsk,swp,net,sys,sbd,db")
	v3OK(c, gin.H{"panel": data, "users": users, "settings": settings, "status": status, "username": apiUsername(c)})
}

func (a *APIv3Handler) resourceValue(resource, id string) (interface{}, error) {
	switch resource {
	case "inbounds":
		return a.InboundService.Get(id)
	case "clients":
		return a.ClientService.Get(id)
	case "outbounds":
		return a.OutboundService.GetAll()
	case "endpoints":
		return a.EndpointService.GetAll()
	case "services":
		return a.ServicesService.GetAll()
	case "tls":
		return a.TlsService.GetAll()
	case "settings":
		return a.SettingService.GetAllSetting()
	case "config":
		value, err := a.SettingService.GetConfig()
		if err != nil {
			return nil, err
		}
		var decoded interface{}
		if err := json.Unmarshal([]byte(value), &decoded); err != nil {
			return nil, err
		}
		return decoded, nil
	default:
		return nil, common.NewError("unknown resource: ", resource)
	}
}

func (a *APIv3Handler) getResource(c *gin.Context) {
	value, err := a.resourceValue(c.Param("resource"), c.Query("id"))
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) saveResource(c *gin.Context) {
	resource := c.Param("resource")
	var body resourceMutation
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	allowed := map[string]bool{"new": true, "edit": true, "del": true, "set": true, "addbulk": true, "editbulk": true, "delbulk": true}
	if !allowed[body.Action] {
		v3Error(c, http.StatusBadRequest, common.NewError("unsupported action: ", body.Action))
		return
	}
	if resource != "clients" && strings.HasSuffix(body.Action, "bulk") {
		a.saveResourceBulk(c, resource, body)
		return
	}
	initUsers := make([]string, 0, len(body.InitUsers))
	for _, id := range body.InitUsers {
		initUsers = append(initUsers, strconv.FormatUint(uint64(id), 10))
	}
	apply := true
	if body.Apply != nil {
		apply = *body.Apply
	}
	changed, err := a.ConfigService.SaveWithApply(resource, body.Action, body.Data, strings.Join(initUsers, ","), apiUsername(c), getHostname(c), apply)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	a.resourceSaveResult(c, changed, nil)
}

// A failed list refresh must not turn a committed mutation into a save error.
func (a *APIv3Handler) resourceSaveResult(c *gin.Context, changed []string, processed *int) {
	result := make(map[string]interface{}, len(changed))
	warning := ""
	for _, name := range changed {
		value, valueErr := a.resourceValue(name, "")
		if valueErr == nil {
			result[name] = value
		} else {
			warning = "savedButRefreshFailed"
		}
	}
	data := gin.H{"changed": changed, "resources": result, "saved": true}
	if warning != "" {
		data["warning"] = warning
	}
	if processed != nil {
		data["processed"] = *processed
	}
	v3OK(c, data)
}

func (a *APIv3Handler) saveResourceBulk(c *gin.Context, resource string, body resourceMutation) {
	var items []json.RawMessage
	if err := json.Unmarshal(body.Data, &items); err != nil {
		v3Error(c, http.StatusBadRequest, common.NewError("bulk data must be a JSON array: ", err))
		return
	}
	action := strings.TrimSuffix(body.Action, "bulk")
	if action == "add" {
		action = "new"
	}
	if action != "new" && action != "edit" && action != "del" {
		v3Error(c, http.StatusBadRequest, common.NewError("unsupported bulk action: ", body.Action))
		return
	}
	changed := []string{resource}
	seen := map[string]bool{resource: true}
	for index, item := range items {
		apply := true
		if body.Apply != nil {
			apply = *body.Apply
		}
		updated, err := a.ConfigService.SaveWithApply(resource, action, item, "", apiUsername(c), getHostname(c), apply)
		if err != nil {
			v3Error(c, http.StatusBadRequest, common.NewErrorf("bulk item %d failed: %v", index+1, err))
			return
		}
		for _, name := range updated {
			if !seen[name] {
				seen[name] = true
				changed = append(changed, name)
			}
		}
	}
	processed := len(items)
	a.resourceSaveResult(c, changed, &processed)
}

func (a *APIv3Handler) exportWireGuard(c *gin.Context) {
	var body struct {
		Tag       string `json:"tag"`
		PeerIndex int    `json:"peerIndex"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	result, err := a.EndpointService.ExportWireGuardPeer(strings.TrimSpace(body.Tag), body.PeerIndex)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) generateWireGuardPsk(c *gin.Context) {
	key, err := service.GenerateWireGuardPresharedKey()
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, gin.H{"pre_shared_key": key})
}

func (a *APIv3Handler) status(c *gin.Context) {
	request := c.Query("request")
	if request == "" {
		request = "cpu,mem,dsk,swp,net,sys,sbd,db"
	}
	v3OK(c, a.ServerService.GetStatus(request))
}

func (a *APIv3Handler) onlines(c *gin.Context) {
	value, err := a.StatsService.GetOnlines()
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) stats(c *gin.Context) {
	result, err := a.StatsService.QueryStats(service.StatsFilter{
		Resource: c.Query("resource"), Tag: c.Query("tag"), Search: c.Query("search"),
		Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 500),
	})
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) usage(c *gin.Context) {
	result, err := a.StatsService.QueryUsage(service.UsageFilter{
		User: c.Query("user"), Search: c.Query("search"), Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 100),
	})
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) connections(c *gin.Context) {
	result, err := a.StatsService.QueryConnections(service.ConnectionFilter{
		Resource: c.Query("resource"), Tag: c.Query("tag"), User: c.Query("user"), Search: c.Query("search"),
		Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 500),
	})
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) addressInfo(c *gin.Context) {
	result, err := service.ResolveConnectionAddress(c.Request.Context(), c.Query("address"))
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) changes(c *gin.Context) {
	result, err := a.ConfigService.QueryChanges(service.ChangesFilter{
		Actor: c.Query("user"), Key: c.Query("key"), Search: c.Query("search"),
		Start: queryInt64(c, "start"), End: queryInt64(c, "end"),
		Offset: queryInt(c, "offset", 0), Limit: queryInt(c, "limit", 100),
	})
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) logs(c *gin.Context) {
	result, err := a.ApiService.queryStructuredLogs(
		c.Query("level"), c.Query("user"), c.Query("search"),
		queryInt64(c, "start"), queryInt64(c, "end"),
		queryInt(c, "offset", 0), queryInt(c, "limit", 100),
	)
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, result)
}

func (a *APIv3Handler) users(c *gin.Context) {
	value, err := a.UserService.GetUsers()
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) changeCredentials(c *gin.Context) {
	var body struct {
		OldPassword string `json:"oldPassword"`
		Username    string `json:"username"`
		Password    string `json:"password"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	if strings.TrimSpace(body.Username) == "" || body.Password == "" {
		v3Error(c, http.StatusBadRequest, common.NewError("new username and password are required"))
		return
	}
	if err := a.UserService.ChangePass(c.Param("id"), body.OldPassword, strings.TrimSpace(body.Username), body.Password); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	a.apiv2.ReloadTokens()
	logger.Audit(apiUsername(c), "changed administrator credentials for user id ", c.Param("id"))
	v3OK(c, gin.H{"updated": true})
}

func (a *APIv3Handler) tokens(c *gin.Context) {
	value, err := a.UserService.GetUserTokens(apiUsername(c))
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) addToken(c *gin.Context) {
	var body struct {
		Description string `json:"description"`
		ExpiryDays  int64  `json:"expiryDays"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	if err := service.ValidateTokenOptions(body.ExpiryDays, body.Description); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	token, err := a.UserService.AddToken(apiUsername(c), body.ExpiryDays, body.Description)
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	a.apiv2.ReloadTokens()
	v3OK(c, gin.H{"token": token})
}

func (a *APIv3Handler) deleteToken(c *gin.Context) {
	if err := a.UserService.DeleteUserToken(apiUsername(c), c.Param("id")); err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	a.apiv2.ReloadTokens()
	v3OK(c, gin.H{"deleted": true})
}

func (a *APIv3Handler) downloadDatabase(c *gin.Context) {
	value, err := database.GetDb(c.Query("exclude"))
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=s-ui-next_"+time.Now().Format("20060102-150405")+".db")
	c.Data(http.StatusOK, "application/octet-stream", value)
}

func (a *APIv3Handler) importDatabase(c *gin.Context) {
	if err := limitDatabaseImport(c); err != nil {
		v3Error(c, http.StatusRequestEntityTooLarge, err)
		return
	}
	file, _, err := c.Request.FormFile("db")
	if err != nil {
		var sizeError *http.MaxBytesError
		if errors.As(err, &sizeError) {
			v3Error(c, http.StatusRequestEntityTooLarge, err)
		} else {
			v3Error(c, http.StatusBadRequest, err)
		}
		return
	}
	defer file.Close()
	if err := database.ImportDB(file); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	a.apiv2.ReloadTokens()
	logger.Audit(apiUsername(c), "restored database backup")
	v3OK(c, gin.H{"imported": true})
}

func (a *APIv3Handler) downloadSingboxConfig(c *gin.Context) {
	value, err := a.ConfigService.GetConfig("")
	if err != nil {
		v3Error(c, http.StatusInternalServerError, err)
		return
	}
	c.Header("Content-Disposition", "attachment; filename=config_"+time.Now().Format("20060102-150405")+".json")
	c.Data(http.StatusOK, "application/json", *value)
}

func (a *APIv3Handler) action(c *gin.Context) {
	username := apiUsername(c)
	switch c.Param("action") {
	case "restart-core":
		if err := a.ConfigService.RestartCore(); err != nil {
			v3Error(c, http.StatusInternalServerError, err)
			return
		}
		logger.Audit(username, "restarted sing-box")
	case "restart-panel":
		if err := a.PanelService.RestartPanel(3 * time.Second); err != nil {
			v3Error(c, http.StatusInternalServerError, err)
			return
		}
		logger.Audit(username, "restarted S-UI Next panel")
	default:
		v3Error(c, http.StatusNotFound, common.NewError("unknown action"))
		return
	}
	v3OK(c, gin.H{"accepted": true})
}

func (a *APIv3Handler) linkConvert(c *gin.Context) {
	var body struct {
		Link string `json:"link"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	value, _, err := util.GetOutbound(body.Link, 0)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) subConvert(c *gin.Context) {
	var body struct {
		Link string `json:"link"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	value, err := util.GetExternalSub(body.Link)
	if err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, value)
}

func (a *APIv3Handler) keypair(c *gin.Context) {
	var body struct {
		Type    string `json:"type"`
		Options string `json:"options"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		v3Error(c, http.StatusBadRequest, err)
		return
	}
	v3OK(c, a.ServerService.GenKeypair(body.Type, body.Options))
}

func (a *APIv3Handler) checkOutbound(c *gin.Context) {
	v3OK(c, a.ConfigService.CheckOutbound(c.Query("tag"), c.Query("link")))
}
