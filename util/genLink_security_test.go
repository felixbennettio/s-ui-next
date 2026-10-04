package util

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/database/model"
)

func TestNormalizeLinkAddressesRejectsMalformedAndFormatsIPv6(t *testing.T) {
	addresses := normalizeLinkAddresses([]map[string]interface{}{
		{"server": "missing-port"},
		{"server": "bad host", "server_port": 443},
		{"server": "2001:db8::1", "server_port": 443},
	})
	if len(addresses) != 1 {
		t.Fatalf("got %d valid addresses, want 1", len(addresses))
	}
	if addresses[0]["server"] != "[2001:db8::1]" || addresses[0]["server_port"] != float64(443) {
		t.Fatalf("unexpected normalized address: %#v", addresses[0])
	}
}

func TestPrepareTLSHandlesMissingClientReality(t *testing.T) {
	tls := &model.Tls{
		Server: json.RawMessage(`{"enabled":true,"reality":{"enabled":true,"short_id":["",7,"abcd"]}}`),
		Client: json.RawMessage(`{}`),
	}
	result := prepareTls(tls)
	reality, ok := result["reality"].(map[string]interface{})
	if !ok || reality["enabled"] != true || reality["short_id"] != "abcd" {
		t.Fatalf("unexpected client reality configuration: %#v", result)
	}
}

func TestTLSAndTransportCollectionsIgnoreMalformedValues(t *testing.T) {
	transport := getTransportParams(map[string]interface{}{
		"type": "http",
		"host": []interface{}{"example.com", 42},
	})
	if len(transport) != 2 || transport[1].Value != "example.com" {
		t.Fatalf("unexpected transport parameters: %#v", transport)
	}
	var tlsParams []LinkParam
	getTlsParams(&tlsParams, map[string]interface{}{
		"reality": map[string]interface{}{"enabled": "not-a-bool"},
		"alpn":    []interface{}{"h2", 42},
	}, "allowInsecure")
	if len(tlsParams) != 2 || tlsParams[0].Value != "tls" || tlsParams[1].Value != "h2" {
		t.Fatalf("unexpected TLS parameters: %#v", tlsParams)
	}
}

func TestAddTLSHandlesMissingClientSections(t *testing.T) {
	outbound := map[string]interface{}{}
	addTls(&outbound, &model.Tls{
		Server: json.RawMessage(`{"reality":{"enabled":true,"short_id":["abcd"]},"ech":{"enabled":true}}`),
		Client: json.RawMessage(`{}`),
	})
	tlsConfig, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("TLS configuration missing: %#v", outbound)
	}
	if reality, ok := tlsConfig["reality"].(map[string]interface{}); !ok || reality["short_id"] != "abcd" {
		t.Fatalf("reality configuration missing: %#v", tlsConfig)
	}
	if ech, ok := tlsConfig["ech"].(map[string]interface{}); !ok || ech["enabled"] != true {
		t.Fatalf("ECH configuration missing: %#v", tlsConfig)
	}
}

func TestHTTPLinkDoesNotCarryTLSAcrossAddresses(t *testing.T) {
	addresses := normalizeLinkAddresses([]map[string]interface{}{
		{"server": "secure.example", "server_port": 443, "tls": map[string]interface{}{"enabled": true}},
		{"server": "plain.example", "server_port": 80},
	})
	links := httpLink(map[string]interface{}{"username": "a@b", "password": "p:q"}, addresses)
	if len(links) != 2 || !strings.HasPrefix(links[0], "https://") || !strings.HasPrefix(links[1], "http://") {
		t.Fatalf("unexpected HTTP links: %#v", links)
	}
	if !strings.Contains(links[0], "a%40b:p%3Aq@") {
		t.Fatalf("credentials were not URL encoded: %q", links[0])
	}
}
