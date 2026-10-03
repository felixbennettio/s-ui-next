package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestWireGuardSecretRequiresSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(sessions.Sessions("test", cookie.NewStore([]byte("test-only-cookie-signing-key"))))
	NewAPIHandler(r.Group("/api"), nil)
	req := httptest.NewRequest("POST", "/api/wireguardSecret", strings.NewReader("id=1&field=private_key"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("X-Requested-With", "XMLHttpRequest")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	var msg Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if msg.Success || msg.Obj != nil || msg.Msg != "Invalid login" {
		t.Fatal("unauthenticated secret request not rejected")
	}
}
