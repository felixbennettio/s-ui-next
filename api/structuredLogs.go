package api

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/felixbennettio/s-ui-next/logger"
	"github.com/felixbennettio/s-ui-next/service"
	"github.com/gin-gonic/gin"
)

type structuredLogEntry struct {
	logger.LogEntry
	Connection *service.ConnectionEntry `json:"connection,omitempty"`
}

func (a *ApiService) queryStructuredLogs(level, user, search string, start, end int64, offset, limit int) (gin.H, error) {
	level = strings.ToUpper(strings.TrimSpace(level))
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 || limit > 1000 {
		limit = 100
	}

	systemEntries, _ := logger.QueryLogs(logger.LogQuery{
		Level: level, User: user, Search: search, Start: start, End: end, Limit: 5000,
	})
	entries := append([]logger.LogEntry{}, systemEntries...)
	if level == "" || level == "ALL" || level == "INFO" {
		auditRows, err := a.ConfigService.QueryChanges(service.ChangesFilter{
			Actor: user, Search: search, Start: start, End: end, Limit: 5000,
		})
		if err != nil {
			return nil, err
		}
		for _, row := range auditRows.Items {
			entries = append(entries, logger.LogEntry{
				Timestamp: row.DateTime,
				Time:      time.Unix(row.DateTime, 0).Format("2006/01/02 15:04:05"),
				Level:     "INFO",
				Message:   fmt.Sprintf("%s %s: %s", row.Action, row.Key, strings.TrimSpace(string(row.Obj))),
				User:      row.Actor,
				Source:    "audit",
			})
		}
	}

	sort.SliceStable(entries, func(i, j int) bool { return entries[i].Timestamp > entries[j].Timestamp })
	connectionCandidates := []service.ConnectionEntry{}
	if level == "" || level == "ALL" || level == "INFO" {
		candidateLogs, _ := logger.QueryLogs(logger.LogQuery{
			Level: "INFO", Start: start, End: end, Limit: 5000,
		})
		for _, entry := range candidateLogs {
			if connection, ok := service.ParseConnectionLog(entry); ok {
				connectionCandidates = append(connectionCandidates, connection)
			}
		}
		service.AttachConnectionSources(connectionCandidates)
	}
	total := len(entries)
	if offset > total {
		offset = total
	}
	pageEnd := offset + limit
	if pageEnd > total {
		pageEnd = total
	}
	items := make([]structuredLogEntry, 0, pageEnd-offset)
	connections := make([]*service.ConnectionEntry, 0, pageEnd-offset)
	for _, entry := range entries[offset:pageEnd] {
		item := structuredLogEntry{LogEntry: entry}
		if connection, ok := service.ParseConnectionLog(entry); ok {
			service.AttachConnectionSourceFromCandidates(&connection, connectionCandidates)
			if item.User == "" {
				item.User = connection.User
			}
			item.Connection = &connection
			connections = append(connections, item.Connection)
		}
		items = append(items, item)
	}
	service.EnrichConnectionEntriesOwners(connections, 8)
	return gin.H{"items": items, "total": total, "offset": offset, "limit": limit}, nil
}

func (a *ApiService) GetStructuredLogs(c *gin.Context) {
	result, err := a.queryStructuredLogs(
		c.Query("level"), c.Query("user"), c.Query("search"),
		queryInt64(c, "start"), queryInt64(c, "end"),
		queryInt(c, "offset", 0), queryInt(c, "limit", 100),
	)
	jsonObj(c, result, err)
}

func (a *ApiService) GetConnectionAddressInfo(c *gin.Context) {
	result, err := service.ResolveConnectionAddress(c.Request.Context(), c.Query("address"))
	jsonObj(c, result, err)
}
