package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/database/model"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

func TestRegisterWarpBuildsDirectWireGuardEndpoint(t *testing.T) {
	peerKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("User-Agent") != "okhttp/3.12.1" || request.Header.Get("CF-Client-Version") != warpClientVersion {
			t.Errorf("unexpected WARP compatibility headers: %#v", request.Header)
		}
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/"+warpAPIVersion+"/reg":
			var payload map[string]interface{}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil {
				t.Error(err)
			}
			if payload["type"] != "Android" || payload["tos"] == "" {
				t.Errorf("unexpected registration payload: %#v", payload)
			}
			if _, err := wgtypes.ParseKey(stringValue(payload["key"])); err != nil {
				t.Errorf("registration public key is invalid: %v", err)
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"id":"device-1","token":"token-1","account":{"license":"license-1"}}`))
		case request.Method == http.MethodGet && request.URL.Path == "/"+warpAPIVersion+"/reg/device-1":
			if request.Header.Get("Authorization") != "Bearer token-1" {
				t.Errorf("missing device authorization header")
			}
			writer.Header().Set("Content-Type", "application/json")
			_, _ = writer.Write([]byte(`{"config":{"client_id":"AQID","interface":{"addresses":{"v4":"172.16.0.2","v6":"2606:4700:110:8765::2"}},"peers":[{"endpoint":{"host":"engage.cloudflareclient.com:2408"},"public_key":"` + peerKey.PublicKey().String() + `"}]}}`))
		default:
			http.NotFound(writer, request)
		}
	}))
	defer server.Close()
	oldURL, oldClient := warpAPIBaseURL, warpHTTPClient
	warpAPIBaseURL, warpHTTPClient = server.URL, server.Client()
	defer func() {
		warpAPIBaseURL, warpHTTPClient = oldURL, oldClient
	}()

	endpoint := model.Endpoint{
		Type:    "warp",
		Tag:     "warp-direct",
		Options: json.RawMessage(`{"warp_terms_accepted":true}`),
	}
	if err := (&WarpService{}).RegisterWarp(&endpoint); err != nil {
		t.Fatal(err)
	}
	var options map[string]interface{}
	if err := json.Unmarshal(endpoint.Options, &options); err != nil {
		t.Fatal(err)
	}
	if intValue(options["mtu"]) != 1280 || boolValue(options["system"], true) {
		t.Fatalf("unexpected WARP interface settings: %#v", options)
	}
	peer := mapValue(listValue(options["peers"])[0])
	if got := strings.Join(stringsValue(peer["allowed_ips"]), ","); got != "0.0.0.0/0,::/0" {
		t.Fatalf("WARP is not configured as a direct full-tunnel egress: %s", got)
	}
	runtime, err := endpoint.MarshalJSON()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(runtime), "warp_terms_accepted") || !strings.Contains(string(runtime), `"type":"wireguard"`) {
		t.Fatalf("unexpected WARP runtime JSON: %s", runtime)
	}
	var runtimeOptions map[string]interface{}
	if err := json.Unmarshal(runtime, &runtimeOptions); err != nil {
		t.Fatal(err)
	}
	runtimePeer := mapValue(listValue(runtimeOptions["peers"])[0])
	if runtimePeer["address"] != "engage.cloudflareclient.com" || intValue(runtimePeer["port"]) != 2408 {
		t.Fatalf("WARP runtime lost its Cloudflare peer endpoint: %s", runtime)
	}
}

func TestRegisterWarpRequiresTermsAcceptance(t *testing.T) {
	endpoint := model.Endpoint{Type: "warp", Tag: "warp-direct", Options: json.RawMessage(`{}`)}
	err := (&WarpService{}).RegisterWarp(&endpoint)
	if err == nil || !strings.Contains(err.Error(), warpTermsURL) {
		t.Fatalf("expected WARP terms error, got %v", err)
	}
}

func TestWarpSecretsAreRedactedAndMerged(t *testing.T) {
	oldEndpoint := model.Endpoint{
		Type:    "warp",
		Tag:     "warp-direct",
		Options: json.RawMessage(`{"private_key":"private-secret","address":["172.16.0.2/32"]}`),
		Ext:     json.RawMessage(`{"device_id":"device-1","access_token":"token-1","license_key":"license-1"}`),
	}
	incoming := json.RawMessage(`{"id":1,"type":"warp","tag":"warp-direct","private_key":"[redacted]","ext":{"device_id":"device-1","access_token":"[redacted]","license_key":"[redacted]"}}`)
	merged, err := mergeWarpSecrets(incoming, &oldEndpoint)
	if err != nil {
		t.Fatal(err)
	}
	var value map[string]interface{}
	if err := json.Unmarshal(merged, &value); err != nil {
		t.Fatal(err)
	}
	if value["private_key"] != "private-secret" || mapValue(value["ext"])["access_token"] != "token-1" {
		t.Fatalf("WARP secrets were not restored before save: %#v", value)
	}
	redactWarpSecrets(value)
	if value["private_key"] != wireGuardRedactedSecret || mapValue(value["ext"])["license_key"] != wireGuardRedactedSecret {
		t.Fatalf("WARP secrets were not redacted for the API: %#v", value)
	}
}
