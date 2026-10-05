package service

import (
	"path/filepath"
	"testing"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
)

func TestGetEnabledBySubscriptionKeyKeepsLegacyNameLinks(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	client := model.Client{
		Enable: true,
		Name:   "alice",
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatal(err)
	}

	service := ClientService{}
	legacy, err := service.GetEnabledBySubscriptionKey(db, "alice")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Name != "alice" {
		t.Fatalf("legacy lookup returned %q", legacy.Name)
	}
	if legacy.SubId == "" {
		t.Fatal("legacy lookup should backfill a subscription id")
	}

	current, err := service.GetEnabledBySubscriptionKey(db, legacy.SubId)
	if err != nil {
		t.Fatal(err)
	}
	if current.Id != legacy.Id {
		t.Fatalf("sub id lookup returned client %d, want %d", current.Id, legacy.Id)
	}
}
