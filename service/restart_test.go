package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/felixbennettio/s-ui-next/core"
	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	projectLogger "github.com/felixbennettio/s-ui-next/logger"
	"github.com/op/go-logging"
)

func TestExplicitRestartBypassesAutomaticCooldown(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	projectLogger.InitLogger(logging.ERROR)
	previous, previousFailure := corePtr, lastStartFailTime
	c := core.NewCore()
	s := NewConfigService(c)
	defer func() { c.Stop(); corePtr = previous; lastStartFailTime = previousFailure }()
	lastStartFailTime = time.Now()
	for range 2 {
		if err := s.RestartCore(); err != nil {
			t.Fatal(err)
		}
		if !c.IsRunning() {
			t.Fatal("restart returned success with the core stopped")
		}
	}
	database.GetDB().Create(&model.Setting{Key: "config", Value: `{"log":{"level":42}}`})
	if err := s.RestartCore(); err == nil {
		t.Fatal("invalid config accepted")
	}
	if !c.IsRunning() {
		t.Fatal("validation failure stopped the working core")
	}
}
