package service

import (
	"encoding/base64"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func testWireGuardData(t *testing.T) map[string]interface{} {
	t.Helper()
	serverKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	clientKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	psk, err := GenerateWireGuardPresharedKey()
	if err != nil {
		t.Fatal(err)
	}
	return map[string]interface{}{
		"id":                          0,
		"type":                        "wireguard",
		"tag":                         "wireguard-test",
		"wireguard_schema":            4,
		"address":                     []string{"10.66.66.1/32", "fd66:66:66::1/128"},
		"tunnel_ipv4_cidr":            "10.66.66.0/24",
		"tunnel_ipv6_cidr":            "fd66:66:66::/64",
		"private_key":                 serverKey.String(),
		"listen_port":                 20522,
		"advertised_endpoint_host":    "vpn.example.com",
		"advertised_endpoint_port":    20522,
		"client_export_enabled":       true,
		"peer_to_peer_enabled":        true,
		"hub_peer_forwarding_enabled": true,
		"default_client_allowed_ips":  []string{"10.66.66.0/24", "fd66:66:66::/64"},
		"default_client_dns":          []string{"10.66.66.1"},
		"default_client_mtu":          1420,
		"default_client_keepalive":    25,
		"peers": []interface{}{map[string]interface{}{
			"name":                 "Laptop",
			"peer_mode":            "roaming_client",
			"peer_role":            "client",
			"peer_key_mode":        "generated_client",
			"remote_endpoint_mode": "dynamic",
			"public_key":           clientKey.PublicKey().String(),
			"client_private_key":   clientKey.String(),
			"pre_shared_key":       psk,
			"assigned_ipv4":        "10.66.66.2/32",
			"assigned_ipv6":        "fd66:66:66::2/128",
			"runtime_route_preset": "peer_addresses",
			"runtime_allowed_ips":  []string{"10.66.66.2/32", "fd66:66:66::2/128"},
			"client_route_preset":  "virtual_network",
			"client_allowed_ips":   []string{"10.66.66.0/24", "fd66:66:66::/64"},
			"include_ipv4":         true,
			"include_ipv6":         true,
		}},
		"ext": map[string]interface{}{
			"public_key": serverKey.PublicKey().String(),
			"keys":       []interface{}{},
		},
	}
}

func TestGenerateWireGuardPresharedKeyIsRandomBase64(t *testing.T) {
	first, err := GenerateWireGuardPresharedKey()
	if err != nil {
		t.Fatal(err)
	}
	second, err := GenerateWireGuardPresharedKey()
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("generated PSKs should not repeat")
	}
	decoded, err := base64.StdEncoding.DecodeString(first)
	if err != nil {
		t.Fatal(err)
	}
	if len(decoded) != 32 {
		t.Fatalf("PSK must decode to 32 bytes, got %d", len(decoded))
	}
	if _, err := wgtypes.ParseKey(first); err != nil {
		t.Fatalf("PSK must be compatible with WireGuard keys: %v", err)
	}
}

func TestNormalizeWireGuardSeparatesServerAndClientAllowedIPs(t *testing.T) {
	data, err := json.Marshal(testWireGuardData(t))
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := normalizeAndValidateWireGuard(data)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err = json.Unmarshal(normalized, &root); err != nil {
		t.Fatal(err)
	}
	peer := mapValue(listValue(root["peers"])[0])
	if got := strings.Join(stringsValue(peer["server_allowed_ips"]), ","); got != "10.66.66.2/32,fd66:66:66::2/128" {
		t.Fatalf("unexpected server allowed IPs: %s", got)
	}
	if got := strings.Join(stringsValue(peer["client_allowed_ips"]), ","); got != "10.66.66.0/24,fd66:66:66::/64" {
		t.Fatalf("unexpected client AllowedIPs: %s", got)
	}
}

func TestNormalizeWireGuardRejectsEndpointNetworkMask(t *testing.T) {
	data := testWireGuardData(t)
	data["address"] = []string{"10.66.66.1/24", "fd66:66:66::1/128"}
	raw, _ := json.Marshal(data)
	if _, err := normalizeAndValidateWireGuard(raw); err == nil || !strings.Contains(err.Error(), "/32") {
		t.Fatalf("expected host-mask validation error, got %v", err)
	}
}

func TestNormalizeWireGuardRequiresExplicitFullTunnel(t *testing.T) {
	data := testWireGuardData(t)
	peer := mapValue(listValue(data["peers"])[0])
	peer["client_allowed_ips"] = []string{"0.0.0.0/0", "::/0"}
	raw, _ := json.Marshal(data)
	if _, err := normalizeAndValidateWireGuard(raw); err == nil || !strings.Contains(err.Error(), "full-tunnel") {
		t.Fatalf("expected full-tunnel validation error, got %v", err)
	}
	peer["client_route_preset"] = "full_tunnel"
	raw, _ = json.Marshal(data)
	if _, err := normalizeAndValidateWireGuard(raw); err != nil {
		t.Fatalf("explicit full tunnel should be accepted: %v", err)
	}
}

func TestNormalizeWireGuardRejectsPeerAddressOwnedByEndpoint(t *testing.T) {
	data := testWireGuardData(t)
	peer := mapValue(listValue(data["peers"])[0])
	peer["assigned_ipv4"] = "10.66.66.1/32"
	peer["assigned_ipv6"] = ""
	peer["client_allowed_ips"] = []string{"10.66.66.0/24"}
	peer["include_ipv6"] = false
	raw, _ := json.Marshal(data)
	if _, err := normalizeAndValidateWireGuard(raw); err == nil || !strings.Contains(err.Error(), "server endpoint") {
		t.Fatalf("expected endpoint ownership validation error, got %v", err)
	}
}

func TestNormalizeWireGuardSinglePeerRouteCannotPointToSelf(t *testing.T) {
	data := testWireGuardData(t)
	peer := mapValue(listValue(data["peers"])[0])
	peer["client_route_preset"] = "single_peer"
	peer["client_allowed_ips"] = []string{"10.66.66.2/32"}
	raw, _ := json.Marshal(data)
	if _, err := normalizeAndValidateWireGuard(raw); err == nil || !strings.Contains(err.Error(), "same client") {
		t.Fatalf("expected single-peer self-route validation error, got %v", err)
	}
}

func TestWireGuardRuntimeJSONStripsEditorMetadata(t *testing.T) {
	raw, _ := json.Marshal(testWireGuardData(t))
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	runtimeJSON, err := endpoint.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	text := string(runtimeJSON)
	for _, forbidden := range []string{"client_allowed_ips", "client_private_key", "runtime_allowed_ips", "advertised_endpoint_host", "peer_to_peer_enabled"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("runtime JSON leaked editor metadata %q: %s", forbidden, text)
		}
	}
	if strings.Contains(text, `"address":"`) {
		t.Fatalf("roaming peer must not contain a static remote address: %s", text)
	}
}

func TestWireGuardRuntimeJSONStripsDynamicSiteGatewayEndpoint(t *testing.T) {
	data := testWireGuardData(t)
	peer := mapValue(listValue(data["peers"])[0])
	peer["peer_role"] = "site_gateway"
	peer["peer_mode"] = "site_to_site"
	peer["remote_endpoint_mode"] = "dynamic"
	peer["runtime_route_preset"] = "remote_networks"
	peer["remote_site_cidrs"] = []string{"192.168.50.0/24"}
	peer["static_remote_address"] = "198.51.100.10"
	peer["static_remote_port"] = 51820
	peer["address"] = "198.51.100.10"
	peer["port"] = 51820
	raw, _ := json.Marshal(data)
	normalized, err := normalizeAndValidateWireGuard(raw)
	if err != nil {
		t.Fatal(err)
	}
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(normalized); err != nil {
		t.Fatal(err)
	}
	runtimeJSON, err := endpoint.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(runtimeJSON, &root); err != nil {
		t.Fatal(err)
	}
	runtimePeer := mapValue(listValue(root["peers"])[0])
	if _, exists := runtimePeer["address"]; exists {
		t.Fatalf("dynamic site gateway must not contain static address: %s", runtimeJSON)
	}
	if _, exists := runtimePeer["port"]; exists {
		t.Fatalf("dynamic site gateway must not contain static port: %s", runtimeJSON)
	}
	if _, exists := runtimePeer["remote_site_cidrs"]; exists {
		t.Fatalf("runtime JSON leaked site-gateway metadata: %s", runtimeJSON)
	}
}

func TestWireGuardRelayAllowsIndependentInitiatorAndReceiverSettings(t *testing.T) {
	receiver := testWireGuardData(t)
	receiver["wireguard_schema"] = 4
	receiver["address"] = []string{"10.77.0.2/32"}
	receiver["listen_port"] = 51820
	receiver["client_export_enabled"] = false
	delete(receiver, "tunnel_ipv4_cidr")
	delete(receiver, "tunnel_ipv6_cidr")
	delete(receiver, "advertised_endpoint_host")
	delete(receiver, "advertised_endpoint_port")
	receiverPeer := mapValue(listValue(receiver["peers"])[0])
	receiverPeer["peer_mode"] = "static_peer"
	receiverPeer["peer_role"] = "fixed_node"
	receiverPeer["peer_key_mode"] = "existing_peer"
	receiverPeer["remote_endpoint_mode"] = "dynamic"
	receiverPeer["runtime_route_preset"] = "custom"
	receiverPeer["runtime_allowed_ips"] = []string{"10.77.0.1/32"}
	delete(receiverPeer, "client_private_key")
	delete(receiverPeer, "assigned_ipv4")
	delete(receiverPeer, "assigned_ipv6")
	delete(receiverPeer, "server_allowed_ips")
	delete(receiverPeer, "allowed_ips")

	raw, _ := json.Marshal(receiver)
	normalized, err := normalizeAndValidateWireGuard(raw)
	if err != nil {
		t.Fatalf("receiver configuration should allow a dynamic peer without client export fields: %v", err)
	}
	var receiverEndpoint model.Endpoint
	if err = receiverEndpoint.UnmarshalJSON(normalized); err != nil {
		t.Fatal(err)
	}
	runtime, err := receiverEndpoint.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var receiverRuntime map[string]interface{}
	if err = json.Unmarshal(runtime, &receiverRuntime); err != nil {
		t.Fatal(err)
	}
	runtimePeer := mapValue(listValue(receiverRuntime["peers"])[0])
	if _, exists := runtimePeer["address"]; exists {
		t.Fatalf("receiver peer must learn its remote endpoint dynamically: %s", runtime)
	}
	if got := strings.Join(stringsValue(runtimePeer["allowed_ips"]), ","); got != "10.77.0.1/32" {
		t.Fatalf("unexpected receiver runtime AllowedIPs: %s", got)
	}

	initiator := testWireGuardData(t)
	initiator["wireguard_schema"] = 4
	initiator["address"] = []string{"10.77.0.1/32"}
	initiator["listen_port"] = 0
	initiator["client_export_enabled"] = false
	delete(initiator, "tunnel_ipv4_cidr")
	delete(initiator, "tunnel_ipv6_cidr")
	delete(initiator, "advertised_endpoint_host")
	delete(initiator, "advertised_endpoint_port")
	initiatorPeer := mapValue(listValue(initiator["peers"])[0])
	initiatorPeer["peer_mode"] = "static_peer"
	initiatorPeer["peer_role"] = "fixed_node"
	initiatorPeer["peer_key_mode"] = "existing_peer"
	initiatorPeer["remote_endpoint_mode"] = "static"
	initiatorPeer["static_remote_address"] = "relay.example.com"
	initiatorPeer["static_remote_port"] = 51820
	initiatorPeer["runtime_route_preset"] = "full_tunnel"
	initiatorPeer["runtime_allowed_ips"] = []string{"0.0.0.0/0", "::/0"}
	delete(initiatorPeer, "client_private_key")
	delete(initiatorPeer, "assigned_ipv4")
	delete(initiatorPeer, "assigned_ipv6")
	delete(initiatorPeer, "server_allowed_ips")
	delete(initiatorPeer, "allowed_ips")

	raw, _ = json.Marshal(initiator)
	normalized, err = normalizeAndValidateWireGuard(raw)
	if err != nil {
		t.Fatalf("initiator configuration should allow a static full-tunnel peer without listening: %v", err)
	}
	var initiatorEndpoint model.Endpoint
	if err = initiatorEndpoint.UnmarshalJSON(normalized); err != nil {
		t.Fatal(err)
	}
	runtime, err = initiatorEndpoint.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	var initiatorRuntime map[string]interface{}
	if err = json.Unmarshal(runtime, &initiatorRuntime); err != nil {
		t.Fatal(err)
	}
	runtimePeer = mapValue(listValue(initiatorRuntime["peers"])[0])
	if runtimePeer["address"] != "relay.example.com" || intValue(runtimePeer["port"]) != 51820 {
		t.Fatalf("initiator static endpoint was not preserved: %s", runtime)
	}
	if got := strings.Join(stringsValue(runtimePeer["allowed_ips"]), ","); got != "0.0.0.0/0,::/0" {
		t.Fatalf("unexpected initiator runtime AllowedIPs: %s", got)
	}
}

func TestWireGuardExportUsesAdvertisedEndpointAndSplitTunnel(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	fixture := testWireGuardData(t)
	storedPeer := mapValue(listValue(fixture["peers"])[0])
	raw, _ := json.Marshal(fixture)
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	result, err := (&EndpointService{}).ExportWireGuardPeer(endpoint.Tag, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result.Config, "Endpoint = vpn.example.com:20522") {
		t.Fatalf("export did not use the advertised endpoint: %s", result.Config)
	}
	if !strings.Contains(result.Config, "AllowedIPs = 10.66.66.0/24, fd66:66:66::/64") {
		t.Fatalf("export did not use split-tunnel routes: %s", result.Config)
	}
	if strings.Contains(result.Config, "0.0.0.0/0") {
		t.Fatalf("split-tunnel export unexpectedly contains a default route: %s", result.Config)
	}
	if !strings.Contains(result.Config, "PresharedKey = "+stringValue(storedPeer["pre_shared_key"])) {
		t.Fatalf("export did not include the stored PSK: %s", result.Config)
	}
}

func TestWireGuardGetAllRedactsSecrets(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(testWireGuardData(t))
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	result, err := (&EndpointService{}).GetAll()
	if err != nil {
		t.Fatal(err)
	}
	item := (*result)[0]
	if stringValue(item["private_key"]) != wireGuardRedactedSecret || !boolValue(item["private_key_set"], false) {
		t.Fatalf("server private key was not redacted: %#v", item)
	}
	peer := mapValue(listValue(item["peers"])[0])
	for _, key := range []string{"client_private_key", "pre_shared_key"} {
		if stringValue(peer[key]) != wireGuardRedactedSecret {
			t.Fatalf("%s was not redacted: %#v", key, peer)
		}
	}
}

func TestMergeWireGuardSecretsPreservesRedactedPSK(t *testing.T) {
	data := testWireGuardData(t)
	raw, _ := json.Marshal(data)
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(raw); err != nil {
		t.Fatal(err)
	}
	next := testWireGuardData(t)
	next["private_key"] = wireGuardRedactedSecret
	peer := mapValue(listValue(next["peers"])[0])
	peer["public_key"] = mapValue(listValue(data["peers"])[0])["public_key"]
	peer["client_private_key"] = wireGuardRedactedSecret
	peer["pre_shared_key"] = wireGuardRedactedSecret
	nextRaw, _ := json.Marshal(next)
	merged, err := mergeWireGuardSecrets(nextRaw, &endpoint)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(merged, &root); err != nil {
		t.Fatal(err)
	}
	mergedPeer := mapValue(listValue(root["peers"])[0])
	oldPeer := mapValue(listValue(data["peers"])[0])
	if stringValue(mergedPeer["pre_shared_key"]) != stringValue(oldPeer["pre_shared_key"]) {
		t.Fatalf("PSK was not preserved: %#v", mergedPeer)
	}
	if stringValue(mergedPeer["client_private_key"]) != stringValue(oldPeer["client_private_key"]) {
		t.Fatalf("client private key was not preserved: %#v", mergedPeer)
	}
}

func TestWireGuardSiteGatewayRoutesRemoteSite(t *testing.T) {
	data := testWireGuardData(t)
	peer := mapValue(listValue(data["peers"])[0])
	peer["peer_role"] = "site_gateway"
	peer["peer_mode"] = "site_to_site"
	peer["remote_endpoint_mode"] = "dynamic"
	peer["runtime_route_preset"] = "remote_networks"
	peer["remote_site_cidrs"] = []string{"192.168.50.0/24", "fd50:50:50::/64"}
	peer["local_site_cidrs"] = []string{"192.168.100.0/24"}
	raw, _ := json.Marshal(data)
	normalized, err := normalizeAndValidateWireGuard(raw)
	if err != nil {
		t.Fatal(err)
	}
	var root map[string]interface{}
	if err := json.Unmarshal(normalized, &root); err != nil {
		t.Fatal(err)
	}
	normalizedPeer := mapValue(listValue(root["peers"])[0])
	serverAllowed := strings.Join(stringsValue(normalizedPeer["server_allowed_ips"]), ",")
	if !strings.Contains(serverAllowed, "192.168.50.0/24") || !strings.Contains(serverAllowed, "fd50:50:50::/64") {
		t.Fatalf("site gateway remote CIDRs were not added to server allowed IPs: %s", serverAllowed)
	}

	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	var endpoint model.Endpoint
	if err := endpoint.UnmarshalJSON(normalized); err != nil {
		t.Fatal(err)
	}
	if err := database.GetDB().Create(&endpoint).Error; err != nil {
		t.Fatal(err)
	}
	if err := syncWireGuardManagedRoute(database.GetDB(), &endpoint); err != nil {
		t.Fatal(err)
	}
	var routes []model.ManagedRouteRule
	if err := database.GetDB().Where("managed_key like ?", "wireguard-site-gateway:%").Find(&routes).Error; err != nil {
		t.Fatal(err)
	}
	if len(routes) != 1 || !strings.Contains(routes[0].CIDRs, "192.168.50.0/24") {
		t.Fatalf("site gateway managed route was not created: %#v", routes)
	}
	exported, err := (&EndpointService{}).ExportWireGuardPeer(endpoint.Tag, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(exported.Config, "192.168.100.0/24") {
		t.Fatalf("site gateway export did not include local site CIDR: %s", exported.Config)
	}
	if strings.Contains(exported.Config, "192.168.50.0/24") {
		t.Fatalf("site gateway export must not route the remote site's own LAN back into the tunnel: %s", exported.Config)
	}
}

func TestManagedWireGuardRouteDoesNotDuplicateUserRule(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "s-ui-next.db")); err != nil {
		t.Fatal(err)
	}
	rule := model.ManagedRouteRule{
		ManagedKey:  "wireguard-peer-to-peer:wireguard-test",
		EndpointTag: "wireguard-test",
		IPv4CIDR:    "10.66.66.0/24",
		IPv6CIDR:    "fd66:66:66::/64",
	}
	if err := database.GetDB().Create(&rule).Error; err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(`{"rules":[{"inbound":["wireguard-test"],"ip_cidr":["10.66.66.0/24","fd66:66:66::/64"],"action":"route","outbound":"wireguard-test"}]}`)
	result, err := injectManagedRoutes(database.GetDB(), raw)
	if err != nil {
		t.Fatal(err)
	}
	var route map[string]interface{}
	if err = json.Unmarshal(result, &route); err != nil {
		t.Fatal(err)
	}
	if got := len(listValue(route["rules"])); got != 1 {
		t.Fatalf("managed route duplicated an equivalent user rule: %d", got)
	}
}

func TestAuditRedactionHidesWireGuardSecrets(t *testing.T) {
	redacted := redactChangeData(json.RawMessage(`{"private_key":"private","pre_shared_key":"psk","token":"token","public_key":"public"}`))
	var value map[string]interface{}
	if err := json.Unmarshal(redacted, &value); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"private_key", "pre_shared_key", "token"} {
		if value[key] != "[redacted]" {
			t.Fatalf("audit data did not redact %s: %s", key, string(redacted))
		}
	}
	if value["public_key"] != "public" {
		t.Fatalf("non-secret public key was unexpectedly redacted: %s", string(redacted))
	}
}
