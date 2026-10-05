package service

import (
	"net/netip"
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/logger"
)

func TestParseConnectionLogExamples(t *testing.T) {
	tests := []struct {
		message     string
		resource    string
		protocol    string
		tag         string
		user        string
		remote      string
		destination string
		source      string
	}{
		{
			message:  "outbound/direct[direct]outbound connection to 149.154.175.211:443",
			resource: "outbound", protocol: "direct", tag: "direct",
			remote: "149.154.175.211:443", destination: "149.154.175.211:443",
		},
		{
			message:  "inbound/vless[vless-443][刘晓辰] inbound connection to 149.154.175.211:443",
			resource: "inbound", protocol: "vless", tag: "vless-443", user: "刘晓辰",
			remote: "149.154.175.211:443", destination: "149.154.175.211:443",
		},
		{
			message:  "inbound/vless[vless-443]inbound connection from 166.111.232.125:61748",
			resource: "inbound", protocol: "vless", tag: "vless-443",
			source: "166.111.232.125:61748",
		},
		{
			message:  "endpoint/wireguard[office] endpoint connection to 10.0.0.2:443",
			resource: "endpoint", protocol: "wireguard", tag: "office",
			remote: "10.0.0.2:443", destination: "10.0.0.2:443",
		},
	}

	for _, test := range tests {
		entry, ok := parseConnectionLog(logger.LogEntry{Timestamp: 1, Time: "2026/06/18 23:03:09", Message: test.message})
		if !ok {
			t.Fatalf("message was not parsed: %q", test.message)
		}
		if entry.Resource != test.resource || entry.Protocol != test.protocol || entry.Tag != test.tag || entry.User != test.user || entry.Remote != test.remote || entry.Destination != test.destination || entry.Source != test.source {
			t.Fatalf("unexpected parse result for %q: %#v", test.message, entry)
		}
	}
}

func TestAttachConnectionSources(t *testing.T) {
	items := []ConnectionEntry{
		{
			Timestamp:   100,
			Resource:    "inbound",
			Protocol:    "vless",
			Tag:         "vless-443",
			User:        "刘晓辰",
			Destination: "www.gstatic.com:80",
		},
		{
			Timestamp: 101,
			Resource:  "inbound",
			Protocol:  "vless",
			Tag:       "vless-443",
			Source:    "166.111.232.125:61748",
			SourceInfo: &ConnectionIPInfo{
				Address: "166.111.232.125:61748",
				Host:    "166.111.232.125",
				Port:    "61748",
				IP:      "166.111.232.125",
			},
		},
	}

	AttachConnectionSources(items)
	if items[0].Source != "166.111.232.125:61748" || items[0].SourceInfo == nil || items[0].SourceInfo.IP != "166.111.232.125" {
		t.Fatalf("source was not attached to user connection: %#v", items[0])
	}
}

func TestConnectionIPInfo(t *testing.T) {
	entry, ok := parseConnectionLog(logger.LogEntry{Timestamp: 1, Time: "2026/06/18 23:03:09", Message: "inbound/vless[vless-443]inbound connection from 10.0.0.2:61748"})
	if !ok {
		t.Fatalf("message was not parsed")
	}
	if entry.SourceInfo == nil || entry.SourceInfo.IP != "10.0.0.2" || entry.SourceInfo.Scope != "private" || entry.SourceInfo.Port != "61748" {
		t.Fatalf("source info missing: %#v", entry.SourceInfo)
	}
}

func TestConnectionDomainInfoUsesCachedResolution(t *testing.T) {
	const (
		host = "target.example"
		ip   = "203.0.113.10"
	)
	storeCachedDomainResolution(host, []netip.Addr{netip.MustParseAddr(ip)})
	storeCachedIPOwner(ip, cachedIPOwner{
		attribution: "AS64500 · Example Network · ZZ",
		isp:         "Example Network",
		asn:         "64500",
		country:     "ZZ",
		region:      "Example Region",
		city:        "Example City",
		network:     "203.0.113.0/24",
	}, positiveIPOwnerTTL)

	info := describeConnectionAddress(host + ":443")
	entry := &ConnectionEntry{DestinationInfo: info, RemoteInfo: info}
	EnrichConnectionEntriesOwners([]*ConnectionEntry{entry}, 0)

	if info.Host != host || info.IP != ip || info.Scope != "public" {
		t.Fatalf("domain resolution was not applied: %#v", info)
	}
	if info.Country != "ZZ" || info.Region != "Example Region" || info.City != "Example City" || info.ASN != "64500" || info.ISP != "Example Network" {
		t.Fatalf("resolved target ownership was not applied: %#v", info)
	}
}

func TestParseIPWhoOwner(t *testing.T) {
	addr := netip.MustParseAddr("104.21.10.20")
	body := []byte(`{
		"ip":"104.21.10.20",
		"success":true,
		"country":"United States",
		"country_code":"US",
		"region":"California",
		"city":"Los Angeles",
		"connection":{"asn":13335,"org":"Cloudflare, Inc.","isp":"Cloudflare, Inc."}
	}`)
	owner, ok := parseIPWhoOwner(body, addr)
	if !ok {
		t.Fatal("valid ownership response was rejected")
	}
	if owner.isp != "Cloudflare, Inc." || owner.asn != "13335" || owner.city != "Los Angeles" || owner.region != "California" || owner.country != "US" {
		t.Fatalf("unexpected ownership response: %#v", owner)
	}
	if owner.attribution != "Cloudflare, Inc. · Los Angeles" {
		t.Fatalf("unexpected attribution: %q", owner.attribution)
	}
}

func TestResolveConnectionAddressRejectsOversizedInput(t *testing.T) {
	if _, err := ResolveConnectionAddress(nil, strings.Repeat("a", maxConnectionAddressLength+1)); err == nil {
		t.Fatal("oversized address should be rejected")
	}
}

func TestConnectionDestinationFilterMatchesSummaryTag(t *testing.T) {
	item := ConnectionEntry{Destination: "target.example:443", Remote: "target.example:443"}
	if !connectionMatches(item, ConnectionFilter{Resource: "destination", Tag: "target.example"}) {
		t.Fatal("destination summary tag should match a connection with a port")
	}
	if connectionMatches(item, ConnectionFilter{Resource: "destination", Tag: "other.example"}) {
		t.Fatal("unrelated destination should not match")
	}
}

func TestConnectionSummaryIncludesEndpoints(t *testing.T) {
	summary := summarizeConnections([]ConnectionEntry{
		{Resource: "endpoint", Tag: "office", User: "alice", Destination: "10.0.0.2:443", Timestamp: 100},
		{Resource: "inbound", Protocol: "vless", Tag: "vless-443", Source: "166.111.232.125:61748", Timestamp: 101},
	})
	if len(summary["endpoints"]) != 1 || summary["endpoints"][0].Tag != "office" {
		t.Fatalf("endpoint summary missing: %#v", summary)
	}
	if len(summary["destinations"]) != 1 || summary["destinations"][0].Tag != "10.0.0.2" {
		t.Fatalf("destination summary should only include targets: %#v", summary)
	}
}
