package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
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

func TestV3WireGuardSecretCopy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	endpoint := model.Endpoint{Tag: "mobile-tunnel", Type: "wireguard", Options: json.RawMessage(`{"private_key":"server-test-key","peers":[{"public_key":"peer-public","client_private_key":"client-test-key","pre_shared_key":"psk-test-key"}]}`)}
	if err := database.GetDB().Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	router := gin.New()
	NewAPIv3Handler(router.Group("/apiv3"), &APIv2Handler{tokens: []TokenInMemory{{Token: "test-token", Username: "tester"}}})
	for _, test := range []struct {
		name, token, field, publicKey, want string
	}{
		{"unauthenticated", "", "private_key", "", ""},
		{"invalid token", "wrong-token", "private_key", "", ""},
		{"server key", "test-token", "private_key", "", "server-test-key"},
		{"client key", "test-token", "client_private_key", "peer-public", "client-test-key"},
		{"preshared key", "test-token", "pre_shared_key", "peer-public", "psk-test-key"},
		{"wrong peer", "test-token", "client_private_key", "wrong-peer", ""},
		{"invalid field", "test-token", "options", "", ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := fmt.Sprintf(`{"id":%d,"field":%q,"publicKey":%q}`, endpoint.Id, test.field, test.publicKey)
			req := httptest.NewRequest(http.MethodPost, "/apiv3/wireguard/secret", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer "+test.token)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			wantStatus := http.StatusOK
			if test.token != "test-token" {
				wantStatus = http.StatusUnauthorized
			} else if test.want == "" {
				wantStatus = http.StatusBadRequest
			}
			if w.Code != wantStatus {
				t.Fatalf("unexpected status: %d", w.Code)
			}
			var result apiV3Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if test.want != "" {
				if !result.Success || result.Data != test.want || w.Header().Get("Cache-Control") != "no-store" {
					t.Fatal("secret copy failed or was cacheable")
				}
			} else if result.Success || result.Data != nil {
				t.Fatal("unauthorized secret returned")
			}
		})
	}
}
