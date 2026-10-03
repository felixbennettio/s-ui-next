package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ciallothu/s-ui-next/core"
	"github.com/ciallothu/s-ui-next/database"
	"github.com/ciallothu/s-ui-next/database/model"
	projectLogger "github.com/ciallothu/s-ui-next/logger"
	"github.com/op/go-logging"
)

func TestRewriteInboundRulesDoesNotBroadenMatches(t *testing.T) {
	tests := []struct{ name, input, want string }{
		{"sole condition", `[{"inbound":"deleted","outbound":"direct"}]`, `[]`},
		{"multi inbound", `[{"inbound":["deleted","keep"],"domain_suffix":"example.org","outbound":"direct"}]`, `[{"inbound":["keep"],"domain_suffix":"example.org","outbound":"direct"}]`},
		{"and", `[{"type":"logical","mode":"and","rules":[{"inbound":"deleted"},{"domain":"example.org"}],"outbound":"direct"}]`, `[]`},
		{"or", `[{"type":"logical","mode":"or","rules":[{"inbound":"deleted"},{"domain":"example.org"}],"outbound":"direct"}]`, `[{"type":"logical","mode":"or","rules":[{"domain":"example.org"}],"outbound":"direct"}]`},
		{"inverted or", `[{"type":"logical","mode":"or","invert":true,"rules":[{"inbound":"deleted"},{"domain":"example.org"}]}]`, `[]`},
		{"unrelated", `[{"action":"sniff"},{"inbound":"keep","outbound":"direct"}]`, `[{"action":"sniff"},{"inbound":"keep","outbound":"direct"}]`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var rules []interface{}
			if err := json.Unmarshal([]byte(tt.input), &rules); err != nil {
				t.Fatal(err)
			}
			result, _ := rewriteInboundRules(rules, func(tag string) string {
				if tag == "deleted" {
					return ""
				}
				return tag
			})
			got, _ := json.Marshal(result)
			var expected interface{}
			json.Unmarshal([]byte(tt.want), &expected)
			want, _ := json.Marshal(expected)
			if string(got) != string(want) {
				t.Fatalf("got %s, want %s", got, want)
			}
		})
	}
}

func TestDeleteInboundCleansDNSRouteAndStats(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	projectLogger.InitLogger(logging.ERROR)
	previous := corePtr
	defer func() { corePtr = previous }()
	s := NewConfigService(core.NewCore())
	db := database.GetDB()
	db.Create(&model.Inbound{Tag: "deleted", Type: "mixed", Options: json.RawMessage(`{"listen":"127.0.0.1","listen_port":0}`)})
	db.Create(&model.Inbound{Tag: "keep", Type: "mixed", Options: json.RawMessage(`{"listen":"127.0.0.1","listen_port":0}`)})
	db.Create(&model.Endpoint{Tag: "tunnel", Type: "wireguard", Options: json.RawMessage(`{}`)})
	config := `{"route":{"rules":[{"inbound":["deleted","keep"],"outbound":"direct"},{"inbound":"tunnel","outbound":"direct"}]},"dns":{"rules":[{"inbound":"deleted","server":"dns"}]},"experimental":{"v2ray_api":{"stats":{"inbounds":["deleted","keep"]}}}}`
	db.Create(&model.Setting{Key: "config", Value: config})
	objs, err := s.SaveWithApply("inbounds", "del", json.RawMessage(`"deleted"`), "", "tester", "", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(objs, ","), "config") {
		t.Fatal("frontend config not refreshed")
	}
	var setting model.Setting
	db.Where("key = ?", "config").First(&setting)
	if strings.Contains(setting.Value, "deleted") {
		t.Fatal("deleted inbound reference remains")
	}
	if !strings.Contains(setting.Value, "tunnel") || !strings.Contains(setting.Value, "keep") {
		t.Fatal("unrelated references removed")
	}
	if err := s.ReconcileInboundReferences(); err != nil {
		t.Fatal(err)
	}
	db.Where("key = ?", "config").First(&setting)
	if !strings.Contains(setting.Value, "tunnel") {
		t.Fatal("valid endpoint inbound reference removed")
	}
	changed, err := renameInboundReferences(db, "keep", "renamed")
	if err != nil || !changed {
		t.Fatalf("rename: %v, %v", changed, err)
	}
	db.Where("key = ?", "config").First(&setting)
	if strings.Contains(setting.Value, "keep") || !strings.Contains(setting.Value, "renamed") {
		t.Fatal("rename not propagated")
	}
}

func TestDeletedInboundCannotReturnThroughManagedWireGuardRoutes(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "panel.db")); err != nil {
		t.Fatal(err)
	}
	projectLogger.InitLogger(logging.ERROR)
	previous := corePtr
	defer func() { corePtr = previous }()
	s := NewConfigService(core.NewCore())
	db := database.GetDB()
	for _, tag := range []string{"deleted", "keep"} {
		if err := db.Create(&model.Inbound{Tag: tag, Type: "mixed", Options: json.RawMessage(`{}`)}).Error; err != nil {
			t.Fatal(err)
		}
	}
	data := testWireGuardData(t)
	data["hub_peer_forwarding_enabled"], data["peer_to_peer_enabled"] = false, false
	peer := mapValue(listValue(data["peers"])[0])
	peer["peer_role"], peer["peer_mode"] = "site_gateway", "site_to_site"
	peer["runtime_route_preset"] = "remote_networks"
	peer["remote_site_cidrs"] = []string{"192.168.50.0/24"}
	peer["route_inbounds"] = []string{"deleted", "keep"}
	raw, _ := json.Marshal(data)
	raw, err := normalizeAndValidateWireGuard(raw)
	if err != nil {
		t.Fatal(err)
	}
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	if err := syncWireGuardManagedRoute(db, &endpoint); err != nil {
		t.Fatal(err)
	}
	for _, tag := range []string{"deleted", "keep"} {
		tagData, _ := json.Marshal(tag)
		if _, err := s.SaveWithApply("inbounds", "del", tagData, "", "tester", "", false); err != nil {
			t.Fatal(err)
		}
		if err := db.First(&endpoint, endpoint.Id).Error; err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(endpoint.Options), `"`+tag+`"`) {
			t.Fatal("stale peer inbound reference")
		}
		var routes []model.ManagedRouteRule
		if err := db.Find(&routes).Error; err != nil {
			t.Fatal(err)
		}
		if tag == "deleted" {
			if len(routes) != 1 || routes[0].InboundTags != `["keep"]` {
				t.Fatal("remaining inbound route was not preserved")
			}
		} else if len(routes) != 0 {
			t.Fatal("deleted route became a fallback route")
		}
	}
	// Saving the tunnel again must preserve the explicit empty source list.
	var options map[string]interface{}
	if err := json.Unmarshal(endpoint.Options, &options); err != nil {
		t.Fatal(err)
	}
	options["type"], options["tag"] = endpoint.Type, endpoint.Tag
	raw, _ = json.Marshal(options)
	raw, err = normalizeAndValidateWireGuard(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := endpoint.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if err := syncWireGuardManagedRoute(db, &endpoint); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.Model(&model.ManagedRouteRule{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("saving the tunnel recreated a removed route")
	}
}
