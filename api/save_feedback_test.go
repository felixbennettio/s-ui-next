package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/core"
	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	projectLogger "github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/service"
	"github.com/gin-gonic/gin"
	"github.com/op/go-logging"
)

func TestCommittedSaveIsSuccessfulWhenRefreshFails(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	projectLogger.InitLogger(logging.ERROR)
	service.NewConfigService(core.NewCore())
	// A malformed pre-existing row fails the post-save list refresh only.
	if err := database.GetDB().Create(&model.Outbound{Tag: "legacy-broken", Type: "direct", Options: json.RawMessage(`{`)}).Error; err != nil {
		t.Fatal(err)
	}
	form := url.Values{"object": {"outbounds"}, "action": {"new"}, "data": {`{"type":"direct","tag":"new-outbound"}`}, "apply": {"false"}}
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/api/save", strings.NewReader(form.Encode()))
	c.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	(&ApiService{}).Save(c, "tester")
	var msg Msg
	if err := json.Unmarshal(w.Body.Bytes(), &msg); err != nil {
		t.Fatal(err)
	}
	if !msg.Success || msg.Warning != "savedButRefreshFailed" {
		t.Fatalf("wrong save outcome: success=%v warning=%q", msg.Success, msg.Warning)
	}
	var count int64
	database.GetDB().Model(&model.Outbound{}).Where("tag = ?", "new-outbound").Count(&count)
	if count != 1 {
		t.Fatal("save was not committed exactly once")
	}
}

func TestV3CommittedSaveSurvivesRefreshFailure(t *testing.T) {
	for _, action := range []string{"new", "addbulk"} {
		t.Run(action, func(t *testing.T) {
			if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
				t.Fatal(err)
			}
			projectLogger.InitLogger(logging.ERROR)
			service.NewConfigService(core.NewCore())
			if err := database.GetDB().Create(&model.Outbound{Tag: "legacy-broken", Type: "direct", Options: json.RawMessage(`{`)}).Error; err != nil {
				t.Fatal(err)
			}
			data := `{"type":"direct","tag":"mobile-outbound"}`
			if action == "addbulk" {
				data = "[" + data + "]"
			}
			body := `{"action":"` + action + `","data":` + data + `,"apply":false}`
			router := gin.New()
			NewAPIv3Handler(router.Group("/apiv3"), &APIv2Handler{tokens: []TokenInMemory{{Token: "test-token", Username: "tester"}}})
			req := httptest.NewRequest(http.MethodPost, "/apiv3/resources/outbounds", strings.NewReader(body))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("Authorization", "Bearer test-token")
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)
			var result apiV3Response
			if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			dataMap, ok := result.Data.(map[string]interface{})
			if w.Code != http.StatusOK || !result.Success || !ok || dataMap["warning"] != "savedButRefreshFailed" {
				t.Fatalf("committed save returned failure: %s", w.Body.String())
			}
			var count int64
			database.GetDB().Model(&model.Outbound{}).Where("tag = ?", "mobile-outbound").Count(&count)
			if count != 1 {
				t.Fatal("expected exactly one committed resource")
			}
		})
	}
}
