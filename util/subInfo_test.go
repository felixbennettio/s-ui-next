package util

import (
	"strings"
	"testing"

	"github.com/felixbennettio/s-ui-next/database/model"
)

func TestGetHeadersRespectsGranularUserInfoOptions(t *testing.T) {
	client := &model.Client{Name: "alice", Up: 10, Down: 20, Volume: 100, Expiry: 1234}
	headers := GetHeaders(client, 12, SubInfoOptions{Upload: true, Expire: true})
	if len(headers) != 3 {
		t.Fatalf("expected three subscription headers, got %d", len(headers))
	}
	if headers[0] != "upload=10; expire=1234" {
		t.Fatalf("unexpected user info header: %q", headers[0])
	}
	for _, hidden := range []string{"download=", "total="} {
		if strings.Contains(headers[0], hidden) {
			t.Fatalf("disabled field %q leaked into header %q", hidden, headers[0])
		}
	}
}
