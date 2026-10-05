package service

import (
	"encoding/json"
	"fmt"
	"net"
	"net/netip"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	"github.com/felixbennettio/s-ui-next/util/common"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"gorm.io/gorm"
)

const wireGuardSchemaVersion = 4
const wireGuardRedactedSecret = "[redacted]"

type WireGuardExport struct {
	Name     string `json:"name"`
	Filename string `json:"filename"`
	Config   string `json:"config"`
}

func mapValue(value interface{}) map[string]interface{} {
	result, _ := value.(map[string]interface{})
	return result
}

func listValue(value interface{}) []interface{} {
	result, _ := value.([]interface{})
	return result
}

func stringValue(value interface{}) string {
	result, _ := value.(string)
	return strings.TrimSpace(result)
}

func boolValue(value interface{}, fallback bool) bool {
	result, ok := value.(bool)
	if !ok {
		return fallback
	}
	return result
}

func intValue(value interface{}) int {
	switch typed := value.(type) {
	case float64:
		return int(typed)
	case int:
		return typed
	case json.Number:
		result, _ := typed.Int64()
		return int(result)
	case string:
		result, _ := strconv.Atoi(typed)
		return result
	default:
		return 0
	}
}

func stringsValue(value interface{}) []string {
	values := listValue(value)
	result := make([]string, 0, len(values))
	for _, item := range values {
		if text := stringValue(item); text != "" {
			result = append(result, text)
		}
	}
	if len(result) == 0 {
		if text := stringValue(value); text != "" {
			for _, item := range strings.Split(text, ",") {
				if item = strings.TrimSpace(item); item != "" {
					result = append(result, item)
				}
			}
		}
	}
	return result
}

func interfaceStrings(values []string) []interface{} {
	result := make([]interface{}, 0, len(values))
	for _, value := range values {
		result = append(result, value)
	}
	return result
}

func jsonStringList(values []string) string {
	raw, _ := json.Marshal(values)
	return string(raw)
}

func isRedactedSecret(value string) bool {
	trimmed := strings.TrimSpace(value)
	return trimmed == wireGuardRedactedSecret || strings.Contains(trimmed, "•")
}

func setRedactedSecret(container map[string]interface{}, key, setKey string) {
	if stringValue(container[key]) != "" {
		container[setKey] = true
		container[key] = wireGuardRedactedSecret
	}
}

func parsePrefix(value, field string) (netip.Prefix, error) {
	prefix, err := netip.ParsePrefix(strings.TrimSpace(value))
	if err != nil {
		return netip.Prefix{}, common.NewErrorf("%s is not a valid CIDR: %s", field, value)
	}
	return prefix, nil
}

func validateTunnel(value string, ipv6 bool) (netip.Prefix, error) {
	prefix, err := parsePrefix(value, "WireGuard network prefix")
	if err != nil {
		return netip.Prefix{}, err
	}
	if prefix.Addr().Is6() != ipv6 || prefix != prefix.Masked() {
		return netip.Prefix{}, common.NewErrorf("WireGuard network prefix must be a canonical IPv%d network", map[bool]int{false: 4, true: 6}[ipv6])
	}
	if ipv6 {
		if prefix.Bits() < 48 || prefix.Bits() > 124 {
			return netip.Prefix{}, common.NewError("IPv6 WireGuard network prefix must be between /48 and /124")
		}
		if prefix.Addr().IsLinkLocalUnicast() {
			return netip.Prefix{}, common.NewError("link-local IPv6 ranges must not be used as the WireGuard virtual network")
		}
	} else if prefix.Bits() < 8 || prefix.Bits() > 30 {
		return netip.Prefix{}, common.NewError("IPv4 WireGuard network prefix must be between /8 and /30")
	}
	return prefix, nil
}

func normalizePeerRole(peer map[string]interface{}) (string, string) {
	role := stringValue(peer["peer_role"])
	mode := stringValue(peer["peer_mode"])
	if role == "" {
		switch mode {
		case "static_peer":
			role = "fixed_node"
		case "site_to_site":
			if len(stringsValue(peer["remote_site_cidrs"])) > 0 || len(stringsValue(peer["local_site_cidrs"])) > 0 {
				role = "site_gateway"
			} else {
				role = "fixed_node"
			}
		default:
			role = "client"
		}
	}
	switch role {
	case "roaming_client":
		role = "client"
	case "static_peer":
		role = "fixed_node"
	case "site_to_site":
		role = "site_gateway"
	}
	remoteMode := stringValue(peer["remote_endpoint_mode"])
	if remoteMode == "" {
		if stringValue(peer["static_remote_address"]) != "" || intValue(peer["static_remote_port"]) > 0 {
			remoteMode = "static"
		} else {
			remoteMode = "dynamic"
		}
	}
	peer["peer_role"] = role
	peer["remote_endpoint_mode"] = remoteMode
	peer["peer_mode"] = map[string]string{
		"client":       "roaming_client",
		"fixed_node":   "static_peer",
		"site_gateway": "site_to_site",
	}[role]
	return role, remoteMode
}

func validateSiteCIDR(value string, field string) (netip.Prefix, error) {
	prefix, err := parsePrefix(value, field)
	if err != nil {
		return netip.Prefix{}, err
	}
	if prefix != prefix.Masked() {
		return netip.Prefix{}, common.NewErrorf("%s must be a canonical network prefix: %s", field, value)
	}
	if prefix.Bits() == 0 {
		return netip.Prefix{}, common.NewErrorf("%s must not be a default route: %s", field, value)
	}
	return prefix, nil
}

func normalizeAndValidateWireGuard(data json.RawMessage) (json.RawMessage, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil {
		return nil, err
	}
	if stringValue(root["type"]) != "wireguard" {
		return data, nil
	}
	strict := intValue(root["wireguard_schema"]) >= 2
	if !strict {
		return data, nil
	}
	root["wireguard_schema"] = wireGuardSchemaVersion
	if stringValue(root["tag"]) == "" {
		return nil, common.NewError("WireGuard tag is required")
	}
	var localPublicKey string
	if key := stringValue(root["private_key"]); key == "" {
		return nil, common.NewError("WireGuard server private key is required")
	} else if privateKey, err := wgtypes.ParseKey(key); err != nil {
		return nil, common.NewErrorf("invalid WireGuard server private key: %v", err)
	} else {
		localPublicKey = privateKey.PublicKey().String()
	}

	var tunnel4, tunnel6 netip.Prefix
	var err error
	if value := stringValue(root["tunnel_ipv4_cidr"]); value != "" {
		tunnel4, err = validateTunnel(value, false)
		if err != nil {
			return nil, err
		}
	}
	if value := stringValue(root["tunnel_ipv6_cidr"]); value != "" {
		tunnel6, err = validateTunnel(value, true)
		if err != nil {
			return nil, err
		}
	}

	addresses := stringsValue(root["address"])
	if len(addresses) == 0 {
		return nil, common.NewError("at least one WireGuard endpoint address is required")
	}
	endpointAddresses := make(map[netip.Addr]struct{}, len(addresses))
	for _, value := range addresses {
		prefix, prefixErr := parsePrefix(value, "WireGuard endpoint address")
		if prefixErr != nil {
			return nil, prefixErr
		}
		if prefix.Bits() != prefix.Addr().BitLen() {
			return nil, common.NewErrorf("WireGuard endpoint address %s must use /32 for IPv4 or /128 for IPv6", value)
		}
		if prefix.Addr().Is4() && tunnel4.IsValid() && !tunnel4.Contains(prefix.Addr()) {
			return nil, common.NewErrorf("WireGuard endpoint address %s is outside the IPv4 virtual network", value)
		}
		if prefix.Addr().Is6() && tunnel6.IsValid() && !tunnel6.Contains(prefix.Addr()) {
			return nil, common.NewErrorf("WireGuard endpoint address %s is outside the IPv6 virtual network", value)
		}
		if _, exists := endpointAddresses[prefix.Addr()]; exists {
			return nil, common.NewErrorf("WireGuard endpoint address %s is duplicated", value)
		}
		endpointAddresses[prefix.Addr()] = struct{}{}
	}
	listenPort := intValue(root["listen_port"])
	if listenPort < 0 || listenPort > 65535 {
		return nil, common.NewError("WireGuard listen port must be 0 (disabled) or between 1 and 65535")
	}

	clientExportEnabled := boolValue(
		root["client_export_enabled"],
		stringValue(root["advertised_endpoint_host"]) != "" || intValue(root["advertised_endpoint_port"]) > 0,
	)
	root["client_export_enabled"] = clientExportEnabled
	if clientExportEnabled {
		if stringValue(root["advertised_endpoint_host"]) == "" {
			return nil, common.NewError("client endpoint host is required while client configuration export is enabled")
		}
		if intValue(root["advertised_endpoint_port"]) < 1 || intValue(root["advertised_endpoint_port"]) > 65535 {
			return nil, common.NewError("client endpoint port must be between 1 and 65535 while client configuration export is enabled")
		}
	}
	if _, exists := root["hub_peer_forwarding_enabled"]; !exists {
		root["hub_peer_forwarding_enabled"] = boolValue(root["peer_to_peer_enabled"], false)
	}
	root["peer_to_peer_enabled"] = boolValue(root["hub_peer_forwarding_enabled"], boolValue(root["peer_to_peer_enabled"], false))

	peers := listValue(root["peers"])
	if len(peers) == 0 {
		return nil, common.NewError("at least one WireGuard peer is required")
	}
	seenAssignedPrefixes := make([]netip.Prefix, 0, len(peers)*2)
	type ownedPrefix struct {
		prefix    netip.Prefix
		peerIndex int
	}
	seenRuntimePrefixes := make([]ownedPrefix, 0, len(peers)*2)
	seenRemoteSites := make([]netip.Prefix, 0, len(peers))
	seenPeerKeys := make(map[string]int, len(peers))
	for index, rawPeer := range peers {
		peer := mapValue(rawPeer)
		if peer == nil {
			return nil, common.NewErrorf("WireGuard peer %d must be an object", index+1)
		}
		role, remoteMode := normalizePeerRole(peer)
		if role != "client" && role != "fixed_node" && role != "site_gateway" {
			return nil, common.NewErrorf("WireGuard peer %d has an unsupported role", index+1)
		}
		if remoteMode != "dynamic" && remoteMode != "static" {
			return nil, common.NewErrorf("WireGuard peer %d has an unsupported remote endpoint mode", index+1)
		}
		if remoteMode == "dynamic" {
			delete(peer, "address")
			delete(peer, "port")
		} else {
			if stringValue(peer["static_remote_address"]) == "" || intValue(peer["static_remote_port"]) < 1 {
				return nil, common.NewErrorf("WireGuard peer %d requires a static remote address and port", index+1)
			}
		}
		if intValue(peer["static_remote_port"]) > 65535 {
			return nil, common.NewErrorf("WireGuard peer %d requires a static remote address and port", index+1)
		}
		if key := stringValue(peer["public_key"]); key == "" {
			return nil, common.NewErrorf("WireGuard peer %d public key is required", index+1)
		} else if _, keyErr := wgtypes.ParseKey(key); keyErr != nil {
			return nil, common.NewErrorf("WireGuard peer %d has an invalid public key", index+1)
		} else if key == localPublicKey {
			return nil, common.NewErrorf("WireGuard peer %d uses this endpoint's own public key", index+1)
		} else if previous, exists := seenPeerKeys[key]; exists {
			return nil, common.NewErrorf("WireGuard peer %d duplicates the public key used by peer %d", index+1, previous+1)
		} else {
			seenPeerKeys[key] = index
		}
		if key := stringValue(peer["pre_shared_key"]); key != "" && !isRedactedSecret(key) {
			if _, keyErr := wgtypes.ParseKey(key); keyErr != nil {
				return nil, common.NewErrorf("WireGuard peer %d has an invalid pre-shared key", index+1)
			}
		}

		keyMode := stringValue(peer["peer_key_mode"])
		if keyMode == "" {
			if stringValue(peer["client_private_key"]) != "" || boolValue(peer["client_private_key_set"], false) || role == "client" {
				keyMode = "generated_client"
			} else {
				keyMode = "existing_peer"
			}
		}
		if keyMode != "generated_client" && keyMode != "existing_peer" {
			return nil, common.NewErrorf("WireGuard peer %d has an unsupported key ownership mode", index+1)
		}
		peer["peer_key_mode"] = keyMode
		if keyMode == "existing_peer" {
			delete(peer, "client_private_key")
			delete(peer, "client_private_key_set")
		}

		assigned := make([]string, 0, 2)
		assignedCandidates := []string{stringValue(peer["assigned_ipv4"]), stringValue(peer["assigned_ipv6"])}
		if assignedCandidates[0] == "" && assignedCandidates[1] == "" {
			for _, candidate := range append(stringsValue(peer["server_allowed_ips"]), stringsValue(peer["allowed_ips"])...) {
				prefix, prefixErr := netip.ParsePrefix(candidate)
				if prefixErr == nil && prefix.Bits() == prefix.Addr().BitLen() {
					if prefix.Addr().Is4() && assignedCandidates[0] == "" {
						assignedCandidates[0] = prefix.String()
					}
					if prefix.Addr().Is6() && assignedCandidates[1] == "" {
						assignedCandidates[1] = prefix.String()
					}
				}
			}
		}
		for _, candidate := range assignedCandidates {
			if candidate != "" {
				assigned = append(assigned, candidate)
			}
		}
		if keyMode == "generated_client" && len(assigned) == 0 {
			return nil, common.NewErrorf("WireGuard peer %d requires an assigned /32 or /128 tunnel address", index+1)
		}
		seenFamily := map[bool]bool{}
		normalizedAssigned := make([]string, 0, len(assigned))
		for _, value := range assigned {
			prefix, prefixErr := parsePrefix(value, fmt.Sprintf("WireGuard peer %d server allowed IP", index+1))
			if prefixErr != nil {
				return nil, prefixErr
			}
			if prefix.Bits() != prefix.Addr().BitLen() {
				return nil, common.NewErrorf("WireGuard peer %d server allowed IP %s must use a host mask (/32 or /128)", index+1, value)
			}
			if _, exists := endpointAddresses[prefix.Addr()]; exists {
				return nil, common.NewErrorf("WireGuard peer %d address %s is already assigned to the server endpoint", index+1, value)
			}
			family := prefix.Addr().Is6()
			if seenFamily[family] {
				return nil, common.NewErrorf("WireGuard peer %d has more than one assigned IPv%d address", index+1, map[bool]int{false: 4, true: 6}[family])
			}
			seenFamily[family] = true
			if prefix.Addr().Is4() {
				if tunnel4.IsValid() && !tunnel4.Contains(prefix.Addr()) {
					return nil, common.NewErrorf("WireGuard peer %d IPv4 address is outside the virtual network", index+1)
				}
				peer["assigned_ipv4"] = prefix.String()
			} else {
				if tunnel6.IsValid() && !tunnel6.Contains(prefix.Addr()) {
					return nil, common.NewErrorf("WireGuard peer %d IPv6 address is outside the virtual network", index+1)
				}
				peer["assigned_ipv6"] = prefix.String()
			}
			for _, existing := range seenAssignedPrefixes {
				if existing.Overlaps(prefix) {
					return nil, common.NewErrorf("WireGuard peer %d address %s overlaps another peer", index+1, value)
				}
			}
			seenAssignedPrefixes = append(seenAssignedPrefixes, prefix)
			normalizedAssigned = append(normalizedAssigned, prefix.String())
		}
		remoteSites := stringsValue(peer["remote_site_cidrs"])
		if role == "site_gateway" && len(remoteSites) == 0 {
			return nil, common.NewErrorf("WireGuard peer %d is a site gateway and requires at least one remote site CIDR", index+1)
		}
		if role != "site_gateway" && len(remoteSites) > 0 {
			return nil, common.NewErrorf("WireGuard peer %d must be a site gateway before it can advertise remote site CIDRs", index+1)
		}
		normalizedRemoteSites := make([]string, 0, len(remoteSites))
		for _, value := range remoteSites {
			prefix, prefixErr := validateSiteCIDR(value, fmt.Sprintf("WireGuard peer %d remote site CIDR", index+1))
			if prefixErr != nil {
				return nil, prefixErr
			}
			if (tunnel4.IsValid() && prefix.Overlaps(tunnel4)) || (tunnel6.IsValid() && prefix.Overlaps(tunnel6)) {
				return nil, common.NewErrorf("WireGuard peer %d remote site CIDR %s overlaps a WireGuard virtual network", index+1, value)
			}
			for _, existing := range seenRemoteSites {
				if existing.Overlaps(prefix) {
					return nil, common.NewErrorf("WireGuard peer %d remote site CIDR %s overlaps another site gateway", index+1, value)
				}
			}
			seenRemoteSites = append(seenRemoteSites, prefix)
			normalizedRemoteSites = append(normalizedRemoteSites, prefix.String())
		}
		if len(normalizedRemoteSites) > 0 {
			peer["remote_site_cidrs"] = interfaceStrings(normalizedRemoteSites)
		}
		localSites := stringsValue(peer["local_site_cidrs"])
		normalizedLocalSites := make([]string, 0, len(localSites))
		for _, value := range localSites {
			prefix, prefixErr := validateSiteCIDR(value, fmt.Sprintf("WireGuard peer %d local site CIDR", index+1))
			if prefixErr != nil {
				return nil, prefixErr
			}
			normalizedLocalSites = append(normalizedLocalSites, prefix.String())
		}
		if len(normalizedLocalSites) > 0 {
			peer["local_site_cidrs"] = interfaceStrings(normalizedLocalSites)
		}
		if role == "site_gateway" {
			routeInbounds := stringsValue(peer["route_inbounds"])
			if _, specified := peer["route_inbounds"]; !specified {
				routeInbounds = []string{stringValue(root["tag"])}
			}
			peer["route_inbounds"] = interfaceStrings(routeInbounds)
		} else {
			delete(peer, "route_inbounds")
			delete(peer, "local_site_cidrs")
		}

		runtimePreset := stringValue(peer["runtime_route_preset"])
		runtimeAllowed := stringsValue(peer["runtime_allowed_ips"])
		legacyRuntimeAllowed := append(stringsValue(peer["server_allowed_ips"]), stringsValue(peer["allowed_ips"])...)
		if runtimePreset == "" {
			candidates := runtimeAllowed
			if len(candidates) == 0 {
				candidates = legacyRuntimeAllowed
			}
			runtimePreset = "custom"
			for _, value := range candidates {
				if prefix, prefixErr := netip.ParsePrefix(value); prefixErr == nil && prefix.Bits() == 0 {
					runtimePreset = "full_tunnel"
					break
				}
			}
			if len(candidates) == 0 || sameStringSet(candidates, normalizedAssigned) {
				runtimePreset = "peer_addresses"
			} else if role == "site_gateway" && len(normalizedRemoteSites) > 0 {
				runtimePreset = "remote_networks"
			}
		}
		switch runtimePreset {
		case "peer_addresses":
			if role == "site_gateway" {
				return nil, common.NewErrorf("WireGuard peer %d must include its remote site networks in runtime AllowedIPs", index+1)
			}
			runtimeAllowed = append([]string{}, normalizedAssigned...)
		case "remote_networks":
			runtimeAllowed = append(append([]string{}, normalizedAssigned...), normalizedRemoteSites...)
		case "full_tunnel":
			if len(runtimeAllowed) == 0 {
				runtimeAllowed = []string{"0.0.0.0/0", "::/0"}
			}
		case "custom":
			if len(runtimeAllowed) == 0 {
				runtimeAllowed = legacyRuntimeAllowed
			}
		default:
			return nil, common.NewErrorf("WireGuard peer %d has an unsupported runtime route preset", index+1)
		}
		if len(runtimeAllowed) == 0 {
			return nil, common.NewErrorf("WireGuard peer %d requires at least one runtime AllowedIP", index+1)
		}
		normalizedRuntime := make([]string, 0, len(runtimeAllowed))
		seenRuntimeValues := map[string]bool{}
		for _, value := range runtimeAllowed {
			prefix, prefixErr := parsePrefix(value, fmt.Sprintf("WireGuard peer %d runtime AllowedIP", index+1))
			if prefixErr != nil {
				return nil, prefixErr
			}
			prefix = prefix.Masked()
			if prefix.Bits() == 0 && runtimePreset != "full_tunnel" {
				return nil, common.NewErrorf("WireGuard peer %d must explicitly select the full-tunnel runtime preset before using %s", index+1, value)
			}
			for _, existing := range seenRuntimePrefixes {
				if existing.peerIndex != index && existing.prefix.Overlaps(prefix) {
					return nil, common.NewErrorf("WireGuard peer %d runtime AllowedIP %s overlaps peer %d", index+1, value, existing.peerIndex+1)
				}
			}
			normalized := prefix.String()
			if !seenRuntimeValues[normalized] {
				normalizedRuntime = append(normalizedRuntime, normalized)
				seenRuntimeValues[normalized] = true
				seenRuntimePrefixes = append(seenRuntimePrefixes, ownedPrefix{prefix: prefix, peerIndex: index})
			}
		}
		if role == "site_gateway" {
			for _, remoteValue := range normalizedRemoteSites {
				remotePrefix, _ := netip.ParsePrefix(remoteValue)
				covered := false
				for _, runtimeValue := range normalizedRuntime {
					runtimePrefix, _ := netip.ParsePrefix(runtimeValue)
					if runtimePrefix.Bits() <= remotePrefix.Bits() && runtimePrefix.Contains(remotePrefix.Addr()) {
						covered = true
						break
					}
				}
				if !covered {
					return nil, common.NewErrorf("WireGuard peer %d runtime AllowedIPs do not include remote site %s", index+1, remoteValue)
				}
			}
		}
		peer["runtime_route_preset"] = runtimePreset
		peer["runtime_allowed_ips"] = interfaceStrings(normalizedRuntime)
		peer["server_allowed_ips"] = interfaceStrings(normalizedRuntime)
		peer["allowed_ips"] = interfaceStrings(normalizedRuntime)

		if keyMode != "generated_client" {
			peers[index] = peer
			continue
		}
		ownAddresses := make(map[netip.Addr]struct{}, len(normalizedAssigned))
		for _, value := range normalizedAssigned {
			if prefix, prefixErr := netip.ParsePrefix(value); prefixErr == nil {
				ownAddresses[prefix.Addr()] = struct{}{}
			}
		}

		include4 := boolValue(peer["include_ipv4"], stringValue(peer["assigned_ipv4"]) != "")
		include6 := boolValue(peer["include_ipv6"], stringValue(peer["assigned_ipv6"]) != "")
		if !include4 && !include6 {
			return nil, common.NewErrorf("WireGuard peer %d must include IPv4, IPv6, or both", index+1)
		}
		peer["include_ipv4"] = include4
		peer["include_ipv6"] = include6
		preset := stringValue(peer["client_route_preset"])
		if preset == "" {
			preset = "virtual_network"
			peer["client_route_preset"] = preset
		}
		switch preset {
		case "virtual_network", "single_peer", "custom", "full_tunnel":
		default:
			return nil, common.NewErrorf("WireGuard peer %d has an unsupported client route preset", index+1)
		}
		clientAllowed := stringsValue(peer["client_allowed_ips"])
		if len(clientAllowed) == 0 {
			clientAllowed = stringsValue(root["default_client_allowed_ips"])
		}
		if len(clientAllowed) == 0 {
			if include4 && tunnel4.IsValid() {
				clientAllowed = append(clientAllowed, tunnel4.String())
			}
			if include6 && tunnel6.IsValid() {
				clientAllowed = append(clientAllowed, tunnel6.String())
			}
		}
		if len(clientAllowed) == 0 {
			return nil, common.NewErrorf("WireGuard peer %d requires client AllowedIPs for configuration export", index+1)
		}
		if role == "site_gateway" {
			clientAllowed = append(clientAllowed, normalizedLocalSites...)
		}
		filteredAllowed := make([]string, 0, len(clientAllowed))
		for _, value := range clientAllowed {
			prefix, prefixErr := parsePrefix(value, fmt.Sprintf("WireGuard peer %d client AllowedIPs", index+1))
			if prefixErr != nil {
				return nil, prefixErr
			}
			if (prefix.Bits() == 0) && preset != "full_tunnel" {
				return nil, common.NewErrorf("WireGuard peer %d must explicitly select the full-tunnel preset before using %s", index+1, value)
			}
			if preset == "single_peer" {
				if prefix.Bits() != prefix.Addr().BitLen() {
					return nil, common.NewErrorf("WireGuard peer %d single-peer routes must use /32 or /128 host addresses", index+1)
				}
				if _, ownAddress := ownAddresses[prefix.Addr()]; ownAddress {
					return nil, common.NewErrorf("WireGuard peer %d single-peer route %s points back to the same client", index+1, value)
				}
			}
			if (prefix.Addr().Is4() && include4) || (prefix.Addr().Is6() && include6) {
				filteredAllowed = append(filteredAllowed, prefix.String())
			}
		}
		if len(filteredAllowed) == 0 {
			return nil, common.NewErrorf("WireGuard peer %d client AllowedIPs are empty after applying IP version choices", index+1)
		}
		peer["client_allowed_ips"] = interfaceStrings(filteredAllowed)
		peers[index] = peer
	}
	root["peers"] = peers
	return json.Marshal(root)
}

func sameStringSet(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	values := make(map[string]int, len(left))
	for _, value := range left {
		values[strings.TrimSpace(value)]++
	}
	for _, value := range right {
		key := strings.TrimSpace(value)
		if values[key] == 0 {
			return false
		}
		values[key]--
	}
	return true
}

func mergeWireGuardSecrets(data json.RawMessage, oldEndpoint *model.Endpoint) (json.RawMessage, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil || stringValue(root["type"]) != "wireguard" || oldEndpoint == nil {
		return data, err
	}
	var oldRoot map[string]interface{}
	if err := json.Unmarshal(oldEndpoint.Options, &oldRoot); err != nil {
		return nil, err
	}
	if key := stringValue(root["private_key"]); (key == "" || isRedactedSecret(key)) && stringValue(oldRoot["private_key"]) != "" {
		root["private_key"] = stringValue(oldRoot["private_key"])
	}
	var oldExt map[string]interface{}
	_ = json.Unmarshal(oldEndpoint.Ext, &oldExt)
	legacySecrets := map[string]string{}
	for _, raw := range listValue(oldExt["keys"]) {
		key := mapValue(raw)
		if key != nil {
			legacySecrets[stringValue(key["public_key"])] = stringValue(key["private_key"])
		}
	}
	oldPeers := listValue(oldRoot["peers"])
	oldPeerByPublic := map[string]map[string]interface{}{}
	for _, raw := range listValue(oldRoot["peers"]) {
		peer := mapValue(raw)
		if peer != nil && stringValue(peer["client_private_key"]) != "" {
			legacySecrets[stringValue(peer["public_key"])] = stringValue(peer["client_private_key"])
		}
		if peer != nil && stringValue(peer["public_key"]) != "" {
			oldPeerByPublic[stringValue(peer["public_key"])] = peer
		}
	}
	for index, raw := range listValue(root["peers"]) {
		peer := mapValue(raw)
		if peer == nil {
			continue
		}
		oldPeer := oldPeerByPublic[stringValue(peer["public_key"])]
		if oldPeer == nil && index < len(oldPeers) {
			oldPeer = mapValue(oldPeers[index])
		}
		if key := stringValue(peer["client_private_key"]); key == "" || isRedactedSecret(key) {
			if secret := legacySecrets[stringValue(peer["public_key"])]; secret != "" {
				peer["client_private_key"] = secret
			} else if oldPeer != nil && stringValue(oldPeer["client_private_key"]) != "" {
				peer["client_private_key"] = stringValue(oldPeer["client_private_key"])
			}
		}
		clearPSK := boolValue(peer["pre_shared_key_clear"], false) ||
			(stringValue(peer["pre_shared_key"]) == "" && valueExplicitlyFalse(peer["pre_shared_key_set"]))
		if clearPSK {
			delete(peer, "pre_shared_key")
		} else if key := stringValue(peer["pre_shared_key"]); key == "" || isRedactedSecret(key) {
			if oldPeer != nil && stringValue(oldPeer["pre_shared_key"]) != "" {
				peer["pre_shared_key"] = stringValue(oldPeer["pre_shared_key"])
			}
		}
		delete(peer, "pre_shared_key_clear")
	}
	return json.Marshal(root)
}

func valueExplicitlyFalse(value interface{}) bool {
	result, ok := value.(bool)
	return ok && !result
}

func redactWireGuardSecrets(endpoint map[string]interface{}) {
	setRedactedSecret(endpoint, "private_key", "private_key_set")
	for _, raw := range listValue(endpoint["peers"]) {
		peer := mapValue(raw)
		if peer == nil {
			continue
		}
		setRedactedSecret(peer, "client_private_key", "client_private_key_set")
		setRedactedSecret(peer, "pre_shared_key", "pre_shared_key_set")
	}
	ext := mapValue(endpoint["ext"])
	for _, raw := range listValue(ext["keys"]) {
		key := mapValue(raw)
		if key != nil {
			delete(key, "private_key")
		}
	}
}

func syncWireGuardManagedRoute(tx *gorm.DB, endpoint *model.Endpoint) error {
	if endpoint == nil || endpoint.Type != "wireguard" {
		if endpoint != nil {
			return tx.Where("endpoint_tag = ?", endpoint.Tag).Delete(&model.ManagedRouteRule{}).Error
		}
		return nil
	}
	var options map[string]interface{}
	if err := json.Unmarshal(endpoint.Options, &options); err != nil {
		return err
	}
	if err := tx.Where("endpoint_tag = ?", endpoint.Tag).Delete(&model.ManagedRouteRule{}).Error; err != nil {
		return err
	}
	if boolValue(options["hub_peer_forwarding_enabled"], boolValue(options["peer_to_peer_enabled"], false)) {
		key := "wireguard-hub-forwarding:" + endpoint.Tag
		rule := model.ManagedRouteRule{
			ManagedKey: key, EndpointTag: endpoint.Tag,
			IPv4CIDR: stringValue(options["tunnel_ipv4_cidr"]),
			IPv6CIDR: stringValue(options["tunnel_ipv6_cidr"]),
		}
		if err := tx.Where("managed_key = ?", key).Assign(rule).FirstOrCreate(&rule).Error; err != nil {
			return err
		}
	}
	for index, raw := range listValue(options["peers"]) {
		peer := mapValue(raw)
		if peer == nil || stringValue(peer["peer_role"]) != "site_gateway" {
			continue
		}
		cidrs := stringsValue(peer["remote_site_cidrs"])
		if len(cidrs) == 0 {
			continue
		}
		inbounds := stringsValue(peer["route_inbounds"])
		if _, specified := peer["route_inbounds"]; !specified {
			inbounds = []string{endpoint.Tag}
		}
		// An explicit empty list keeps a deleted inbound from turning into a
		// route for a different source when the endpoint is saved again.
		if len(inbounds) == 0 {
			continue
		}
		key := fmt.Sprintf("wireguard-site-gateway:%s:%d", endpoint.Tag, index)
		rule := model.ManagedRouteRule{
			ManagedKey: key, EndpointTag: endpoint.Tag,
			CIDRs: jsonStringList(cidrs), InboundTags: jsonStringList(inbounds),
		}
		if err := tx.Where("managed_key = ?", key).Assign(rule).FirstOrCreate(&rule).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *EndpointService) ExportWireGuardPeer(tag string, peerIndex int) (*WireGuardExport, error) {
	var endpoint model.Endpoint
	if err := database.GetDB().Where("tag = ? AND type = ?", tag, "wireguard").First(&endpoint).Error; err != nil {
		return nil, err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(endpoint.Options, &root); err != nil {
		return nil, err
	}
	peers := listValue(root["peers"])
	if peerIndex < 0 || peerIndex >= len(peers) {
		return nil, common.NewError("WireGuard peer was not found")
	}
	peer := mapValue(peers[peerIndex])
	if peer == nil {
		return nil, common.NewError("WireGuard peer is invalid")
	}
	var ext map[string]interface{}
	_ = json.Unmarshal(endpoint.Ext, &ext)
	clientPrivateKey := stringValue(peer["client_private_key"])
	if clientPrivateKey == "" {
		for _, raw := range listValue(ext["keys"]) {
			key := mapValue(raw)
			if key != nil && stringValue(key["public_key"]) == stringValue(peer["public_key"]) {
				clientPrivateKey = stringValue(key["private_key"])
				break
			}
		}
	}
	if clientPrivateKey == "" {
		return nil, common.NewError("client private key is unavailable; generate a new key for this peer")
	}
	if !boolValue(root["client_export_enabled"], stringValue(root["advertised_endpoint_host"]) != "") {
		return nil, common.NewError("enable client configuration export before exporting this peer")
	}
	serverPublicKey := stringValue(ext["public_key"])
	if serverPublicKey == "" {
		privateKey, err := wgtypes.ParseKey(stringValue(root["private_key"]))
		if err != nil {
			return nil, common.NewError("WireGuard server public key is unavailable")
		}
		serverPublicKey = privateKey.PublicKey().String()
	}
	host := stringValue(root["advertised_endpoint_host"])
	port := intValue(root["advertised_endpoint_port"])
	if host == "" || port < 1 || port > 65535 {
		return nil, common.NewError("configure the client endpoint host and port before exporting")
	}

	include4 := boolValue(peer["include_ipv4"], true)
	include6 := boolValue(peer["include_ipv6"], true)
	addresses := make([]string, 0, 2)
	for _, value := range []string{stringValue(peer["assigned_ipv4"]), stringValue(peer["assigned_ipv6"])} {
		if value == "" {
			continue
		}
		prefix, err := netip.ParsePrefix(value)
		if err == nil && ((prefix.Addr().Is4() && include4) || (prefix.Addr().Is6() && include6)) {
			addresses = append(addresses, prefix.String())
		}
	}
	if len(addresses) == 0 {
		for _, value := range append(stringsValue(peer["runtime_allowed_ips"]), stringsValue(peer["allowed_ips"])...) {
			prefix, err := netip.ParsePrefix(value)
			if err == nil && prefix.Bits() == prefix.Addr().BitLen() && ((prefix.Addr().Is4() && include4) || (prefix.Addr().Is6() && include6)) {
				addresses = append(addresses, prefix.String())
			}
		}
	}
	allowed := stringsValue(peer["client_allowed_ips"])
	if len(allowed) == 0 {
		allowed = stringsValue(root["default_client_allowed_ips"])
	}
	if len(allowed) == 0 {
		for _, value := range []string{stringValue(root["tunnel_ipv4_cidr"]), stringValue(root["tunnel_ipv6_cidr"])} {
			if value != "" {
				allowed = append(allowed, value)
			}
		}
	}
	if stringValue(peer["peer_role"]) == "site_gateway" {
		allowed = append(allowed, stringsValue(peer["local_site_cidrs"])...)
	}
	filteredAllowed := make([]string, 0, len(allowed))
	seenAllowed := map[string]bool{}
	for _, value := range allowed {
		prefix, err := netip.ParsePrefix(value)
		if err == nil && ((prefix.Addr().Is4() && include4) || (prefix.Addr().Is6() && include6)) {
			normalized := prefix.String()
			if !seenAllowed[normalized] {
				filteredAllowed = append(filteredAllowed, normalized)
				seenAllowed[normalized] = true
			}
		}
	}
	if len(addresses) == 0 || len(filteredAllowed) == 0 {
		return nil, common.NewError("client addresses and AllowedIPs must be configured before exporting")
	}
	dns := stringsValue(peer["client_dns"])
	if len(dns) == 0 {
		dns = stringsValue(root["default_client_dns"])
	}
	if len(dns) == 0 {
		dns = stringsValue(ext["dns"])
	}
	mtu := intValue(peer["client_mtu"])
	if mtu == 0 {
		mtu = intValue(root["default_client_mtu"])
	}
	keepalive := intValue(peer["client_keepalive"])
	if keepalive == 0 {
		keepalive = intValue(root["default_client_keepalive"])
	}

	name := stringValue(peer["name"])
	if name == "" {
		name = fmt.Sprintf("Peer %d", peerIndex+1)
	}
	var config strings.Builder
	config.WriteString("[Interface]\nPrivateKey = " + clientPrivateKey + "\n")
	config.WriteString("Address = " + strings.Join(addresses, ", ") + "\n")
	if len(dns) > 0 {
		config.WriteString("DNS = " + strings.Join(dns, ", ") + "\n")
	}
	if mtu > 0 {
		config.WriteString(fmt.Sprintf("MTU = %d\n", mtu))
	}
	config.WriteString("\n[Peer]\nPublicKey = " + serverPublicKey + "\n")
	if psk := stringValue(peer["pre_shared_key"]); psk != "" {
		config.WriteString("PresharedKey = " + psk + "\n")
	}
	config.WriteString("AllowedIPs = " + strings.Join(filteredAllowed, ", ") + "\n")
	config.WriteString("Endpoint = " + net.JoinHostPort(strings.Trim(host, "[]"), strconv.Itoa(port)) + "\n")
	if keepalive > 0 {
		config.WriteString(fmt.Sprintf("PersistentKeepalive = %d\n", keepalive))
	}
	filename := filepath.Base(endpoint.Tag + "_" + strings.NewReplacer("/", "_", "\\", "_", " ", "_").Replace(name) + ".conf")
	return &WireGuardExport{Name: name, Filename: filename, Config: config.String()}, nil
}
