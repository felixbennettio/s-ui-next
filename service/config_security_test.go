package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/felixbennettio/s-ui-next/core"
	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	projectLogger "github.com/felixbennettio/s-ui-next/logger"
	"github.com/op/go-logging"
)

func TestCheckChangesRejectsInvalidCursorAndUsesLatestChange(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	LastUpdate.Store(0)
	defer LastUpdate.Store(0)
	service := ConfigService{}
	if _, err := service.CheckChanges("0 OR 1=1"); err == nil {
		t.Fatal("invalid cursor should be rejected")
	}
	if err := database.GetDB().Create(&model.Changes{DateTime: 10, Actor: "test", Key: "config", Action: "set", Obj: []byte(`{}`)}).Error; err != nil {
		t.Fatal(err)
	}
	changed, err := service.CheckChanges("9")
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("latest database change was not detected")
	}
}

func TestValidateTokenOptions(t *testing.T) {
	if err := ValidateTokenOptions(30, "mobile"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTokenOptions(-1, "mobile"); err == nil {
		t.Fatal("negative token expiry should be rejected")
	}
	if err := ValidateTokenOptions(MaxTokenExpiryDays+1, "mobile"); err == nil {
		t.Fatal("excessive token expiry should be rejected")
	}
}

func TestSaveConfigRejectsInvalidRuntimeConfigBeforeCommit(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	if err := db.Create(&model.Setting{Key: "config", Value: defaultConfig}).Error; err != nil {
		t.Fatal(err)
	}
	previousCore := corePtr
	defer func() { corePtr = previousCore }()
	projectLogger.InitLogger(logging.ERROR)
	configService := NewConfigService(core.NewCore())
	invalid := json.RawMessage(`{"log":{"level":42}}`)
	if _, err := configService.SaveWithApply("config", "set", invalid, "", "tester", "", false); err == nil {
		t.Fatal("invalid sing-box configuration should be rejected")
	}
	var stored string
	if err := db.Model(&model.Setting{}).Select("value").Where("key = ?", "config").Scan(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored != defaultConfig {
		t.Fatal("invalid configuration was committed")
	}
	var changes int64
	if err := db.Model(&model.Changes{}).Count(&changes).Error; err != nil {
		t.Fatal(err)
	}
	if changes != 0 {
		t.Fatal("rejected configuration created an audit change")
	}
}

func TestSaveConfigCreatesMissingSetting(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatal(tx.Error)
	}
	if err := (&SettingService{}).SaveConfig(tx, json.RawMessage(`{}`)); err != nil {
		_ = tx.Rollback().Error
		t.Fatal(err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatal(err)
	}
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatal(err)
	}
	if setting.Value != "{}" {
		t.Fatalf("stored config = %q, want {}", setting.Value)
	}
}
