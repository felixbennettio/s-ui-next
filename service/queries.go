package service

import (
	"strings"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"

	"gorm.io/gorm"
)

// StatsFilter is shared by the app API's chart and drill-down endpoints.
// Start and End are Unix seconds. A zero value leaves that boundary open.
type StatsFilter struct {
	Resource string
	Tag      string
	Search   string
	Start    int64
	End      int64
	Offset   int
	Limit    int
}

type StatsQueryResult struct {
	Items  []model.Stats `json:"items"`
	Total  int64         `json:"total"`
	Offset int           `json:"offset"`
	Limit  int           `json:"limit"`
}

type UsageFilter struct {
	User   string
	Search string
	Start  int64
	End    int64
	Offset int
	Limit  int
}

type UserUsage struct {
	User       string `json:"user" gorm:"column:user"`
	Upload     int64  `json:"upload" gorm:"column:upload"`
	Download   int64  `json:"download" gorm:"column:download"`
	Total      int64  `json:"total" gorm:"column:total"`
	Enabled    bool   `json:"enabled" gorm:"-"`
	Quota      int64  `json:"quota" gorm:"-"`
	Expiry     int64  `json:"expiry" gorm:"-"`
	Group      string `json:"group,omitempty" gorm:"-"`
	Online     bool   `json:"online" gorm:"-"`
	LifetimeUp int64  `json:"lifetimeUpload" gorm:"-"`
	LifetimeDn int64  `json:"lifetimeDownload" gorm:"-"`
}

type UsageQueryResult struct {
	Items    []UserUsage `json:"items"`
	Total    int64       `json:"total"`
	Upload   int64       `json:"upload"`
	Download int64       `json:"download"`
	Offset   int         `json:"offset"`
	Limit    int         `json:"limit"`
}

type ChangesFilter struct {
	Actor  string
	Key    string
	Search string
	Start  int64
	End    int64
	Offset int
	Limit  int
}

type ChangesQueryResult struct {
	Items  []model.Changes `json:"items"`
	Total  int64           `json:"total"`
	Offset int             `json:"offset"`
	Limit  int             `json:"limit"`
}

func normalizePage(offset, limit, max int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > max {
		limit = max
	}
	return offset, limit
}

func applyStatsFilter(query *gorm.DB, filter StatsFilter) *gorm.DB {
	resource := strings.TrimSpace(filter.Resource)
	switch resource {
	case "endpoint":
		query = query.Where("resource IN ?", []string{"inbound", "outbound"})
	case "", "all":
		// No resource filter.
	default:
		query = query.Where("resource = ?", resource)
	}
	if filter.Tag != "" {
		query = query.Where("tag = ?", filter.Tag)
	}
	if filter.Search != "" {
		like := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("tag LIKE ? OR resource LIKE ?", like, like)
	}
	if filter.Start > 0 {
		query = query.Where("date_time >= ?", filter.Start)
	}
	if filter.End > 0 {
		query = query.Where("date_time <= ?", filter.End)
	}
	return query
}

func (s *StatsService) QueryStats(filter StatsFilter) (*StatsQueryResult, error) {
	filter.Offset, filter.Limit = normalizePage(filter.Offset, filter.Limit, 5000)
	db := database.GetDB()

	countQuery := applyStatsFilter(db.Model(&model.Stats{}), filter)
	var total int64
	if err := countQuery.Count(&total).Error; err != nil {
		return nil, err
	}

	var items []model.Stats
	dataQuery := applyStatsFilter(db.Model(&model.Stats{}), filter)
	if err := dataQuery.Order("date_time ASC, id ASC").Offset(filter.Offset).Limit(filter.Limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return &StatsQueryResult{Items: items, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}

func applyUsageFilter(query *gorm.DB, filter UsageFilter) *gorm.DB {
	query = query.Where("resource = ?", "user")
	if filter.User != "" {
		query = query.Where("tag = ?", filter.User)
	}
	if filter.Search != "" {
		query = query.Where("tag LIKE ?", "%"+strings.TrimSpace(filter.Search)+"%")
	}
	if filter.Start > 0 {
		query = query.Where("date_time >= ?", filter.Start)
	}
	if filter.End > 0 {
		query = query.Where("date_time <= ?", filter.End)
	}
	return query
}

func (s *StatsService) QueryUsage(filter UsageFilter) (*UsageQueryResult, error) {
	filter.Offset, filter.Limit = normalizePage(filter.Offset, filter.Limit, 1000)
	db := database.GetDB()

	var total int64
	if err := applyUsageFilter(db.Model(&model.Stats{}), filter).Distinct("tag").Count(&total).Error; err != nil {
		return nil, err
	}

	var totals struct {
		Upload   int64
		Download int64
	}
	if err := applyUsageFilter(db.Model(&model.Stats{}), filter).
		Select("COALESCE(SUM(CASE WHEN direction = 1 THEN traffic ELSE 0 END), 0) AS upload, COALESCE(SUM(CASE WHEN direction = 0 THEN traffic ELSE 0 END), 0) AS download").
		Scan(&totals).Error; err != nil {
		return nil, err
	}

	var items []UserUsage
	if err := applyUsageFilter(db.Model(&model.Stats{}), filter).
		Select("tag AS user, SUM(CASE WHEN direction = 1 THEN traffic ELSE 0 END) AS upload, SUM(CASE WHEN direction = 0 THEN traffic ELSE 0 END) AS download, SUM(traffic) AS total").
		Group("tag").Order("total DESC, tag ASC").Offset(filter.Offset).Limit(filter.Limit).Scan(&items).Error; err != nil {
		return nil, err
	}

	if len(items) > 0 {
		names := make([]string, 0, len(items))
		for _, item := range items {
			names = append(names, item.User)
		}
		var clients []model.Client
		if err := db.Model(&model.Client{}).Where("name IN ?", names).Find(&clients).Error; err != nil {
			return nil, err
		}
		clientMap := make(map[string]model.Client, len(clients))
		for _, client := range clients {
			clientMap[client.Name] = client
		}
		online, _ := s.GetOnlines()
		onlineMap := make(map[string]bool, len(online.User))
		for _, user := range online.User {
			onlineMap[user] = true
		}
		for index := range items {
			client, ok := clientMap[items[index].User]
			if ok {
				items[index].Enabled = client.Enable
				items[index].Quota = client.Volume
				items[index].Expiry = client.Expiry
				items[index].Group = client.Group
				items[index].LifetimeUp = client.Up + client.TotalUp
				items[index].LifetimeDn = client.Down + client.TotalDown
			}
			items[index].Online = onlineMap[items[index].User]
		}
	}

	return &UsageQueryResult{
		Items: items, Total: total, Upload: totals.Upload, Download: totals.Download,
		Offset: filter.Offset, Limit: filter.Limit,
	}, nil
}

func applyChangesFilter(query *gorm.DB, filter ChangesFilter) *gorm.DB {
	if filter.Actor != "" {
		query = query.Where("actor = ?", filter.Actor)
	}
	if filter.Key != "" {
		query = query.Where("key = ?", filter.Key)
	}
	if filter.Search != "" {
		like := "%" + strings.TrimSpace(filter.Search) + "%"
		query = query.Where("actor LIKE ? OR key LIKE ? OR action LIKE ? OR CAST(obj AS TEXT) LIKE ?", like, like, like, like)
	}
	if filter.Start > 0 {
		query = query.Where("date_time >= ?", filter.Start)
	}
	if filter.End > 0 {
		query = query.Where("date_time <= ?", filter.End)
	}
	return query
}

func (s *ConfigService) QueryChanges(filter ChangesFilter) (*ChangesQueryResult, error) {
	filter.Offset, filter.Limit = normalizePage(filter.Offset, filter.Limit, 5000)
	db := database.GetDB()
	var total int64
	if err := applyChangesFilter(db.Model(&model.Changes{}), filter).Count(&total).Error; err != nil {
		return nil, err
	}
	var items []model.Changes
	if err := applyChangesFilter(db.Model(&model.Changes{}), filter).
		Order("date_time DESC, id DESC").Offset(filter.Offset).Limit(filter.Limit).Find(&items).Error; err != nil {
		return nil, err
	}
	return &ChangesQueryResult{Items: items, Total: total, Offset: filter.Offset, Limit: filter.Limit}, nil
}
