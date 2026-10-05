package service

import (
	"net"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/felixbennettio/s-ui-next/logger"
)

type ConnectionFilter struct {
	Resource string
	Tag      string
	User     string
	Search   string
	Start    int64
	End      int64
	Offset   int
	Limit    int
}

type ConnectionEntry struct {
	Timestamp       int64             `json:"timestamp"`
	Time            string            `json:"time"`
	Resource        string            `json:"resource"`
	Protocol        string            `json:"protocol"`
	Tag             string            `json:"tag"`
	User            string            `json:"user,omitempty"`
	Event           string            `json:"event"`
	Remote          string            `json:"remote,omitempty"`
	RemoteInfo      *ConnectionIPInfo `json:"remoteInfo,omitempty"`
	Destination     string            `json:"destination,omitempty"`
	DestinationInfo *ConnectionIPInfo `json:"destinationInfo,omitempty"`
	Source          string            `json:"source,omitempty"`
	SourceInfo      *ConnectionIPInfo `json:"sourceInfo,omitempty"`
	Message         string            `json:"message"`
}

type ConnectionIPInfo struct {
	Address     string `json:"address,omitempty"`
	Host        string `json:"host,omitempty"`
	Port        string `json:"port,omitempty"`
	IP          string `json:"ip,omitempty"`
	Scope       string `json:"scope,omitempty"`
	Attribution string `json:"attribution,omitempty"`
	ISP         string `json:"isp,omitempty"`
	ASN         string `json:"asn,omitempty"`
	Country     string `json:"country,omitempty"`
	Region      string `json:"region,omitempty"`
	City        string `json:"city,omitempty"`
	Network     string `json:"network,omitempty"`
}

type ConnectionSummary struct {
	Resource string   `json:"resource"`
	Tag      string   `json:"tag"`
	Count    int      `json:"count"`
	LastSeen int64    `json:"lastSeen"`
	Users    []string `json:"users,omitempty"`
}

type ConnectionQueryResult struct {
	Items   []ConnectionEntry              `json:"items"`
	Total   int                            `json:"total"`
	Offset  int                            `json:"offset"`
	Limit   int                            `json:"limit"`
	Summary map[string][]ConnectionSummary `json:"summary"`
	Parsed  int                            `json:"parsed"`
	Scanned int                            `json:"scanned"`
}

var (
	connectionPrefixPattern = regexp.MustCompile(`^(inbound|outbound|endpoint)/([^\[]+)\[([^\]]+)\](?:\[([^\]]+)\])?\s*(.*)$`)
	connectionToPattern     = regexp.MustCompile(`\bconnection\s+to\s+([^,\s]+)`)
	connectionFromPattern   = regexp.MustCompile(`\bconnection\s+from\s+([^,\s]+)`)
)

const connectionSourceAttachWindow int64 = 10

func ParseConnectionLog(entry logger.LogEntry) (ConnectionEntry, bool) {
	message := strings.TrimSpace(entry.Message)
	matches := connectionPrefixPattern.FindStringSubmatch(message)
	if len(matches) == 0 {
		return ConnectionEntry{}, false
	}
	detail := strings.TrimSpace(matches[5])
	result := ConnectionEntry{
		Timestamp: entry.Timestamp,
		Time:      entry.Time,
		Resource:  matches[1],
		Protocol:  matches[2],
		Tag:       matches[3],
		User:      strings.TrimSpace(matches[4]),
		Event:     detail,
		Message:   message,
	}
	if to := connectionToPattern.FindStringSubmatch(detail); len(to) > 1 {
		result.Destination = strings.TrimSpace(to[1])
		result.Remote = result.Destination
		result.DestinationInfo = describeConnectionAddress(result.Destination)
		result.RemoteInfo = result.DestinationInfo
	}
	if from := connectionFromPattern.FindStringSubmatch(detail); len(from) > 1 {
		result.Source = strings.TrimSpace(from[1])
		result.SourceInfo = describeConnectionAddress(result.Source)
	}
	return result, true
}

func parseConnectionLog(entry logger.LogEntry) (ConnectionEntry, bool) {
	return ParseConnectionLog(entry)
}

func (s *StatsService) QueryConnections(filter ConnectionFilter) (*ConnectionQueryResult, error) {
	filter.Offset, filter.Limit = normalizePage(filter.Offset, filter.Limit, 2000)
	logs, scanned := logger.QueryLogs(logger.LogQuery{
		Level: "INFO",
		Start: filter.Start, End: filter.End, Limit: 5000,
	})

	parsed := make([]ConnectionEntry, 0, len(logs))
	for _, logEntry := range logs {
		item, ok := ParseConnectionLog(logEntry)
		if ok {
			parsed = append(parsed, item)
		}
	}
	AttachConnectionSources(parsed)

	items := make([]ConnectionEntry, 0, len(parsed))
	for _, item := range parsed {
		if !connectionMatches(item, filter) {
			continue
		}
		items = append(items, item)
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].Timestamp > items[j].Timestamp })
	summary := summarizeConnections(items)
	total := len(items)
	pageEnd := filter.Offset + filter.Limit
	if pageEnd > total {
		pageEnd = total
	}
	page := []ConnectionEntry{}
	if filter.Offset < total {
		page = append(page, items[filter.Offset:pageEnd]...)
	}
	pageEntries := make([]*ConnectionEntry, len(page))
	for index := range page {
		pageEntries[index] = &page[index]
	}
	EnrichConnectionEntriesOwners(pageEntries, 8)
	return &ConnectionQueryResult{
		Items: page, Total: total, Offset: filter.Offset, Limit: filter.Limit,
		Summary: summary, Parsed: len(items), Scanned: scanned,
	}, nil
}

func AttachConnectionSources(items []ConnectionEntry) {
	index := buildConnectionSourceIndex(items)
	for itemIndex := range items {
		attachConnectionSource(&items[itemIndex], index)
	}
}

func AttachConnectionSourceFromCandidates(item *ConnectionEntry, candidates []ConnectionEntry) {
	if item == nil {
		return
	}
	attachConnectionSource(item, buildConnectionSourceIndex(candidates))
}

func buildConnectionSourceIndex(items []ConnectionEntry) map[string][]ConnectionEntry {
	index := make(map[string][]ConnectionEntry)
	for _, item := range items {
		if item.Source == "" {
			continue
		}
		key := connectionSourceKey(item)
		if key == "" {
			continue
		}
		index[key] = append(index[key], item)
	}
	return index
}

func attachConnectionSource(item *ConnectionEntry, index map[string][]ConnectionEntry) {
	if item.Source != "" || item.Destination == "" || item.Resource != "inbound" {
		return
	}
	candidates := index[connectionSourceKey(*item)]
	if len(candidates) == 0 {
		return
	}
	bestIndex := -1
	bestDelta := connectionSourceAttachWindow + 1
	for candidateIndex, candidate := range candidates {
		delta := absInt64(item.Timestamp - candidate.Timestamp)
		if delta > connectionSourceAttachWindow || delta >= bestDelta {
			continue
		}
		bestIndex = candidateIndex
		bestDelta = delta
	}
	if bestIndex < 0 {
		return
	}
	source := candidates[bestIndex]
	item.Source = source.Source
	item.SourceInfo = source.SourceInfo
}

func connectionSourceKey(item ConnectionEntry) string {
	if item.Resource == "" || item.Protocol == "" || item.Tag == "" {
		return ""
	}
	return strings.Join([]string{item.Resource, item.Protocol, item.Tag}, "\x00")
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func connectionMatches(item ConnectionEntry, filter ConnectionFilter) bool {
	resource := strings.ToLower(strings.TrimSpace(filter.Resource))
	tag := strings.TrimSpace(filter.Tag)
	user := strings.TrimSpace(filter.User)
	search := strings.ToLower(strings.TrimSpace(filter.Search))

	if user != "" && !strings.EqualFold(item.User, user) && !strings.Contains(strings.ToLower(item.Message), strings.ToLower(user)) {
		return false
	}
	switch resource {
	case "", "all":
	case "user":
		if tag != "" && !strings.EqualFold(item.User, tag) {
			return false
		}
	case "inbound", "outbound", "endpoint", "node":
		if resource == "node" {
			resource = "endpoint"
		}
		if item.Resource != resource {
			return false
		}
		if tag != "" && item.Tag != tag {
			return false
		}
	case "destination", "remote":
		if tag != "" && normalizeRemote(item.Remote) != tag && normalizeRemote(item.Destination) != tag && normalizeRemote(item.Source) != tag {
			return false
		}
	default:
		if tag != "" && item.Tag != tag {
			return false
		}
	}
	if search == "" {
		return true
	}
	haystack := strings.ToLower(strings.Join([]string{
		item.Resource, item.Protocol, item.Tag, item.User, item.Event, item.Remote, item.Destination, item.Source, item.Message,
		connectionIPInfoText(item.RemoteInfo), connectionIPInfoText(item.DestinationInfo), connectionIPInfoText(item.SourceInfo),
	}, " "))
	return strings.Contains(haystack, search)
}

func summarizeConnections(items []ConnectionEntry) map[string][]ConnectionSummary {
	type bucket struct {
		ConnectionSummary
		userSet map[string]bool
	}
	maps := map[string]map[string]*bucket{
		"users":        {},
		"inbounds":     {},
		"outbounds":    {},
		"endpoints":    {},
		"destinations": {},
	}
	add := func(kind, resource, tag, user string, timestamp int64) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		key := resource + "\x00" + tag
		item := maps[kind][key]
		if item == nil {
			item = &bucket{ConnectionSummary: ConnectionSummary{Resource: resource, Tag: tag}, userSet: map[string]bool{}}
			maps[kind][key] = item
		}
		item.Count++
		if timestamp > item.LastSeen {
			item.LastSeen = timestamp
		}
		if user != "" {
			item.userSet[user] = true
		}
	}
	for _, item := range items {
		add(item.Resource+"s", item.Resource, item.Tag, item.User, item.Timestamp)
		if item.User != "" {
			add("users", "user", item.User, item.User, item.Timestamp)
		}
		if item.Destination != "" {
			add("destinations", "destination", normalizeRemote(item.Destination), item.User, item.Timestamp)
		}
	}
	result := make(map[string][]ConnectionSummary, len(maps))
	for kind, source := range maps {
		values := make([]ConnectionSummary, 0, len(source))
		for _, item := range source {
			for user := range item.userSet {
				item.Users = append(item.Users, user)
			}
			sort.Strings(item.Users)
			values = append(values, item.ConnectionSummary)
		}
		sort.SliceStable(values, func(i, j int) bool {
			if values[i].Count == values[j].Count {
				return values[i].Tag < values[j].Tag
			}
			return values[i].Count > values[j].Count
		})
		if len(values) > 100 {
			values = values[:100]
		}
		result[kind] = values
	}
	return result
}

func connectionIPInfoText(info *ConnectionIPInfo) string {
	if info == nil {
		return ""
	}
	return strings.Join([]string{
		info.Address, info.Host, info.Port, info.IP, info.Scope, info.Attribution, info.ISP, info.ASN, info.Country, info.Region, info.City, info.Network,
	}, " ")
}

func normalizeRemote(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return host
	}
	if parsed, err := url.Parse("//" + value); err == nil && parsed.Hostname() != "" {
		return parsed.Hostname()
	}
	return value
}
