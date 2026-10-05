package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
)

func TestEgressTagsShareOneNamespace(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	db := database.GetDB()
	outbound := model.Outbound{Type: "direct", Tag: "shared-egress"}
	if err := db.Create(&outbound).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureEgressTagAvailable(db, "endpoint", 0, outbound.Tag); err == nil || !strings.Contains(err.Error(), "share one namespace") {
		t.Fatalf("expected endpoint/outbound collision error, got %v", err)
	}
	if err := ensureEgressTagAvailable(db, "outbound", outbound.Id, outbound.Tag); err != nil {
		t.Fatalf("editing an outbound without changing its tag must be allowed: %v", err)
	}

	if err := db.Delete(&outbound).Error; err != nil {
		t.Fatal(err)
	}
	endpoint := model.Endpoint{Type: "tailscale", Tag: "shared-egress"}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	if err := ensureEgressTagAvailable(db, "outbound", 0, endpoint.Tag); err == nil || !strings.Contains(err.Error(), "share one namespace") {
		t.Fatalf("expected outbound/endpoint collision error, got %v", err)
	}
}
