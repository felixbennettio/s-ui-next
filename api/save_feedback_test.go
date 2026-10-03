package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ciallothu/s-ui-next/core"
	"github.com/ciallothu/s-ui-next/database"
	"github.com/ciallothu/s-ui-next/database/model"
	projectLogger "github.com/ciallothu/s-ui-next/logger"
	"github.com/ciallothu/s-ui-next/service"
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
