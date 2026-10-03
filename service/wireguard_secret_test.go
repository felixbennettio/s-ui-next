package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/ciallothu/s-ui-next/database"
	"github.com/ciallothu/s-ui-next/database/model"
)

func TestWireGuardCopyDoesNotExposeSecretsInLists(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	e := model.Endpoint{Tag: "test-tunnel", Type: "wireguard", Options: json.RawMessage(`{"private_key":"server-test-key","peers":[{"public_key":"peer-public","client_private_key":"client-test-key","pre_shared_key":"psk-test-key"}]}`)}
	if err := database.GetDB().Create(&e).Error; err != nil {
		t.Fatal(err)
	}
	s := EndpointService{}
	for field, want := range map[string]string{"private_key": "server-test-key", "client_private_key": "client-test-key", "pre_shared_key": "psk-test-key"} {
		value, err := s.WireGuardSecret(e.Id, field, "peer-public")
		if err != nil || value != want {
			t.Fatalf("copy %s failed", field)
		}
	}
	if _, err := s.WireGuardSecret(e.Id, "client_private_key", "other-peer"); err == nil {
		t.Fatal("wrong peer accepted")
	}
	if _, err := s.WireGuardSecret(e.Id, "options", ""); err == nil {
		t.Fatal("arbitrary field accepted")
	}
	list, err := s.GetAll()
	if err != nil {
		t.Fatal(err)
	}
	row := (*list)[0]
	if row["private_key"] != wireGuardRedactedSecret {
		t.Fatal("private key exposed in list")
	}
	peer := mapValue(listValue(row["peers"])[0])
	if peer["client_private_key"] != wireGuardRedactedSecret || peer["pre_shared_key"] != wireGuardRedactedSecret {
		t.Fatal("peer secret exposed in list")
	}
}
