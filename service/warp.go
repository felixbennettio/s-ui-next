package service

import (
	"bytes"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/felixbennettio/s-ui-next/database/model"
	"github.com/felixbennettio/s-ui-next/util/common"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

type WarpService struct{}

const (
	maxWarpResponseBytes int64 = 1 << 20
	warpAPIVersion             = "v0a1922"
	warpClientVersion          = "a-6.3-1922"
	warpTermsURL               = "https://www.cloudflare.com/application/terms/"
)

var (
	warpAPIBaseURL = "https://api.cloudflareclient.com"
	warpHTTPClient = newWarpHTTPClient()
)

func newWarpHTTPClient() *http.Client {
	return &http.Client{
		Timeout: 30 * time.Second,
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{
				MinVersion: tls.VersionTLS12,
				MaxVersion: tls.VersionTLS12,
			},
			ForceAttemptHTTP2:     false,
			MaxIdleConns:          100,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: time.Second,
		},
	}
}

func warpAPIURL(path string) string {
	return strings.TrimRight(warpAPIBaseURL, "/") + "/" + warpAPIVersion + "/" + strings.TrimLeft(path, "/")
}

func newWarpRequest(method string, path string, accessToken string, body []byte) (*http.Request, error) {
	req, err := http.NewRequest(method, warpAPIURL(path), bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "okhttp/3.12.1")
	req.Header.Set("CF-Client-Version", warpClientVersion)
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return req, nil
}

func (s *WarpService) getWarpInfo(deviceID string, accessToken string) ([]byte, error) {
	req, err := newWarpRequest(http.MethodGet, "reg/"+url.PathEscape(deviceID), accessToken, nil)
	if err != nil {
		return nil, err
	}
	resp, err := warpHTTPClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, common.NewErrorf("WARP API returned HTTP %d: %s", resp.StatusCode, readWarpResponse(resp))
	}
	return readWarpBody(resp)
}

func (s *WarpService) RegisterWarp(endpoint *model.Endpoint) error {
	var requested map[string]interface{}
	if err := json.Unmarshal(endpoint.Options, &requested); err != nil {
		return err
	}
	if !boolValue(requested["warp_terms_accepted"], false) {
		return common.NewErrorf("Cloudflare terms must be accepted before creating a WARP tunnel: %s", warpTermsURL)
	}

	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return common.NewErrorf("failed to generate WARP private key: %v", err)
	}
	hostName, err := os.Hostname()
	if err != nil || strings.TrimSpace(hostName) == "" {
		hostName = "s-ui-next"
	}
	payload := struct {
		FCMToken string `json:"fcm_token"`
		Install  string `json:"install_id"`
		Key      string `json:"key"`
		Locale   string `json:"locale"`
		Model    string `json:"model"`
		Terms    string `json:"tos"`
		Type     string `json:"type"`
	}{
		Key:    privateKey.PublicKey().String(),
		Locale: "en_US",
		Model:  "s-ui-next-" + hostName,
		Terms:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		Type:   "Android",
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := newWarpRequest(http.MethodPost, "reg", "", body)
	if err != nil {
		return err
	}
	resp, err := warpHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return common.NewErrorf("WARP registration returned HTTP %d: %s", resp.StatusCode, readWarpResponse(resp))
	}
	responseBody, err := readWarpBody(resp)
	if err != nil {
		return err
	}

	var registration struct {
		DeviceID string `json:"id"`
		Token    string `json:"token"`
		Account  struct {
			License string `json:"license"`
		} `json:"account"`
	}
	if err = json.Unmarshal(responseBody, &registration); err != nil {
		return err
	}
	if registration.DeviceID == "" || registration.Token == "" || registration.Account.License == "" {
		return common.NewError("WARP registration response is missing device credentials")
	}

	warpInfo, err := s.getWarpInfo(registration.DeviceID, registration.Token)
	if err != nil {
		return err
	}
	var details struct {
		Config struct {
			ClientID  string `json:"client_id"`
			Interface struct {
				Addresses struct {
					IPv4 string `json:"v4"`
					IPv6 string `json:"v6"`
				} `json:"addresses"`
			} `json:"interface"`
			Peers []struct {
				Endpoint struct {
					Host string `json:"host"`
				} `json:"endpoint"`
				PublicKey string `json:"public_key"`
			} `json:"peers"`
		} `json:"config"`
	}
	if err = json.Unmarshal(warpInfo, &details); err != nil {
		return err
	}
	if len(details.Config.Peers) == 0 {
		return common.NewError("WARP device response is missing peers")
	}
	reserved, err := decodeWarpReserved(details.Config.ClientID)
	if err != nil {
		return err
	}
	ipv4, err := parseWarpAddress(details.Config.Interface.Addresses.IPv4, false)
	if err != nil {
		return err
	}
	ipv6, err := parseWarpAddress(details.Config.Interface.Addresses.IPv6, true)
	if err != nil {
		return err
	}
	peerAddress, peerPortText, err := net.SplitHostPort(details.Config.Peers[0].Endpoint.Host)
	if err != nil {
		return common.NewErrorf("invalid WARP peer endpoint: %v", err)
	}
	peerPort, err := strconv.Atoi(peerPortText)
	if err != nil || peerPort < 1 || peerPort > 65535 || strings.TrimSpace(peerAddress) == "" {
		return common.NewError("WARP device response contains an invalid peer endpoint")
	}
	if _, err = wgtypes.ParseKey(details.Config.Peers[0].PublicKey); err != nil {
		return common.NewErrorf("WARP device response contains an invalid peer public key: %v", err)
	}

	credentials := map[string]interface{}{
		"access_token": registration.Token,
		"device_id":    registration.DeviceID,
		"license_key":  registration.Account.License,
	}
	endpoint.Ext, err = json.MarshalIndent(credentials, "", "  ")
	if err != nil {
		return err
	}
	requested["private_key"] = privateKey.String()
	requested["address"] = []string{ipv4.String() + "/32", ipv6.String() + "/128"}
	requested["listen_port"] = 0
	requested["mtu"] = 1280
	requested["system"] = false
	requested["peers"] = []map[string]interface{}{{
		"address":     peerAddress,
		"port":        peerPort,
		"public_key":  details.Config.Peers[0].PublicKey,
		"allowed_ips": []string{"0.0.0.0/0", "::/0"},
		"reserved":    reserved,
	}}
	endpoint.Options, err = json.MarshalIndent(requested, "", "  ")
	return err
}

func parseWarpAddress(value string, ipv6 bool) (netip.Addr, error) {
	address, err := netip.ParseAddr(strings.TrimSpace(value))
	if err != nil || address.Is6() != ipv6 {
		return netip.Addr{}, common.NewErrorf("WARP device response contains an invalid IPv%d address", map[bool]int{false: 4, true: 6}[ipv6])
	}
	return address, nil
}

func decodeWarpReserved(clientID string) ([]int, error) {
	decoded, err := base64.StdEncoding.DecodeString(clientID)
	if err != nil || len(decoded) != 3 {
		return nil, common.NewError("WARP device response contains an invalid reserved value")
	}
	return []int{int(decoded[0]), int(decoded[1]), int(decoded[2])}, nil
}

func (s *WarpService) SetWarpLicense(oldLicense string, endpoint *model.Endpoint) error {
	var warpData map[string]interface{}
	if err := json.Unmarshal(endpoint.Ext, &warpData); err != nil {
		return err
	}
	deviceID := stringValue(warpData["device_id"])
	accessToken := stringValue(warpData["access_token"])
	license := stringValue(warpData["license_key"])
	if deviceID == "" || accessToken == "" {
		return common.NewError("WARP device credentials are missing")
	}
	if license == "" {
		return common.NewError("WARP license key must not be empty")
	}
	if license == oldLicense {
		return nil
	}

	body, err := json.Marshal(map[string]string{"license": license})
	if err != nil {
		return err
	}
	req, err := newWarpRequest(http.MethodPut, "reg/"+url.PathEscape(deviceID)+"/account", accessToken, body)
	if err != nil {
		return err
	}
	resp, err := warpHTTPClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return common.NewErrorf("WARP license update returned HTTP %d: %s", resp.StatusCode, readWarpResponse(resp))
	}
	responseBody, err := readWarpBody(resp)
	if err != nil {
		return err
	}
	if len(bytes.TrimSpace(responseBody)) == 0 {
		return nil
	}
	var response map[string]interface{}
	if err = json.Unmarshal(responseBody, &response); err != nil {
		return err
	}
	if success, ok := response["success"].(bool); ok && !success {
		errorArr, _ := response["errors"].([]interface{})
		if len(errorArr) > 0 {
			if errorObj, ok := errorArr[0].(map[string]interface{}); ok {
				return common.NewError(errorObj["code"], errorObj["message"])
			}
		}
		return common.NewError("WARP license update failed")
	}
	return nil
}

func mergeWarpSecrets(data json.RawMessage, oldEndpoint *model.Endpoint) (json.RawMessage, error) {
	var root map[string]interface{}
	if err := json.Unmarshal(data, &root); err != nil || stringValue(root["type"]) != "warp" || oldEndpoint == nil {
		return data, err
	}
	var oldOptions map[string]interface{}
	if err := json.Unmarshal(oldEndpoint.Options, &oldOptions); err != nil {
		return nil, err
	}
	if key := stringValue(root["private_key"]); (key == "" || isRedactedSecret(key)) && stringValue(oldOptions["private_key"]) != "" {
		root["private_key"] = stringValue(oldOptions["private_key"])
	}
	var oldExt map[string]interface{}
	if err := json.Unmarshal(oldEndpoint.Ext, &oldExt); err != nil {
		return nil, err
	}
	ext := mapValue(root["ext"])
	if ext == nil {
		ext = map[string]interface{}{}
		root["ext"] = ext
	}
	for _, key := range []string{"device_id", "access_token", "license_key"} {
		if value := stringValue(ext[key]); value == "" || isRedactedSecret(value) {
			ext[key] = oldExt[key]
		}
	}
	delete(ext, "access_token_set")
	delete(ext, "license_key_set")
	return json.Marshal(root)
}

func warpLicense(endpoint *model.Endpoint) string {
	if endpoint == nil {
		return ""
	}
	var ext map[string]interface{}
	if json.Unmarshal(endpoint.Ext, &ext) != nil {
		return ""
	}
	return stringValue(ext["license_key"])
}

func redactWarpSecrets(endpoint map[string]interface{}) {
	setRedactedSecret(endpoint, "private_key", "private_key_set")
	ext := mapValue(endpoint["ext"])
	if ext != nil {
		setRedactedSecret(ext, "access_token", "access_token_set")
		setRedactedSecret(ext, "license_key", "license_key_set")
	}
	for _, raw := range listValue(endpoint["peers"]) {
		peer := mapValue(raw)
		if peer != nil {
			setRedactedSecret(peer, "pre_shared_key", "pre_shared_key_set")
		}
	}
}

func readWarpResponse(resp *http.Response) string {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	return string(body)
}

func readWarpBody(resp *http.Response) ([]byte, error) {
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxWarpResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > maxWarpResponseBytes {
		return nil, common.NewError("WARP API response is too large")
	}
	return body, nil
}
