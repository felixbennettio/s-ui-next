package util

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/util/common"
)

const maxExternalSubscriptionBytes int64 = 8 << 20

var externalSubscriptionClient = &http.Client{
	Timeout: 20 * time.Second,
	CheckRedirect: func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return common.NewError("too many subscription redirects")
		}
		if request.URL.Scheme != "http" && request.URL.Scheme != "https" {
			return common.NewError("subscription redirect must use HTTP or HTTPS")
		}
		return nil
	},
}

func GetExternalLink(rawURL string) string {
	parsed, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		logger.Warning("sub: invalid external subscription URL")
		return ""
	}
	response, err := externalSubscriptionClient.Get(parsed.String())
	if err != nil {
		logger.Warning("sub: Error making HTTP request:", err)
		return ""
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		logger.Warning("sub: external subscription returned HTTP ", response.StatusCode)
		return ""
	}

	body, err := io.ReadAll(io.LimitReader(response.Body, maxExternalSubscriptionBytes+1))
	if err != nil {
		logger.Warning("sub: Error reading response body:", err)
		return ""
	}
	if int64(len(body)) > maxExternalSubscriptionBytes {
		logger.Warning("sub: external subscription response is too large")
		return ""
	}

	data := StrOrBase64Encoded(string(body))
	return data
}

func GetExternalSub(url string) ([]map[string]interface{}, error) {
	var err error
	var result []map[string]interface{}

	if len(url) == 0 {
		return nil, common.NewError("no url")
	}

	data := strings.TrimSpace(GetExternalLink(url))
	if len(data) == 0 {
		return nil, common.NewError("no result")
	}

	// if the data is a JSON object
	if strings.HasPrefix(data, "{") && strings.HasSuffix(data, "}") {
		var jsonData map[string]interface{}
		err = json.Unmarshal([]byte(data), &jsonData)
		if err != nil {
			logger.Warning("sub: Error unmarshalling JSON:", err)
			return nil, err
		}
		outbounds, ok := jsonData["outbounds"].([]any)
		if !ok {
			err = common.NewError("subscription JSON does not contain an outbounds array")
			logger.Warning("sub: Error getting outbounds:", err)
			return nil, err
		}
		for _, outbound := range outbounds {
			outboundMap, ok := outbound.(map[string]interface{})
			if ok && len(outboundMap) > 0 {
				oType, _ := outboundMap["type"].(string)
				switch oType {
				case "urltest":
				case "direct":
				case "selector":
				case "block":
					continue
				default:
					result = append(result, outboundMap)
				}
			}
		}
		if len(result) == 0 {
			return nil, common.NewError("no result")
		}
		return result, nil
	} else {
		// if data is a text
		links := strings.Split(data, "\n")
		for _, link := range links {
			linkToJson, _, err := GetOutbound(link, 0)
			if err == nil {
				result = append(result, *linkToJson)
			}
		}
	}
	if len(result) == 0 {
		return nil, common.NewError("no result")
	}
	return result, nil
}
