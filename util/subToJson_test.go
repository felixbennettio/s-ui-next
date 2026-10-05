package util

import (
	"net/http"
	"net/http/httptest"
	"testing"

	projectLogger "github.com/felixbennettio/s-ui-next/logger"
	"github.com/op/go-logging"
)

func TestGetExternalSubRejectsJSONWithoutOutbounds(t *testing.T) {
	projectLogger.InitLogger(logging.ERROR)
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "application/json")
		_, _ = writer.Write([]byte(`{"dns":{}}`))
	}))
	defer server.Close()

	if _, err := GetExternalSub(server.URL); err == nil {
		t.Fatal("subscription JSON without outbounds should fail")
	}
}
