package util

import (
	"fmt"
	"strings"

	"github.com/felixbennettio/s-ui-next/database/model"
)

type SubInfoOptions struct {
	Upload   bool
	Download bool
	Total    bool
	Expire   bool
}

func GetHeaders(client *model.Client, updateInterval int, options ...SubInfoOptions) []string {
	show := SubInfoOptions{Upload: true, Download: true, Total: true, Expire: true}
	if len(options) > 0 {
		show = options[0]
	}
	parts := make([]string, 0, 4)
	if show.Upload {
		parts = append(parts, fmt.Sprintf("upload=%d", client.Up))
	}
	if show.Download {
		parts = append(parts, fmt.Sprintf("download=%d", client.Down))
	}
	if show.Total {
		parts = append(parts, fmt.Sprintf("total=%d", client.Volume))
	}
	if show.Expire {
		parts = append(parts, fmt.Sprintf("expire=%d", client.Expiry))
	}
	var headers []string
	headers = append(headers, strings.Join(parts, "; "))
	headers = append(headers, fmt.Sprintf("%d", updateInterval))
	headers = append(headers, client.Name)
	return headers
}
