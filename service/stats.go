package service

import (
	"sort"
	"sync"
	"time"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"

	"gorm.io/gorm"
)

type onlines struct {
	Inbound  []string `json:"inbound,omitempty"`
	User     []string `json:"user,omitempty"`
	Outbound []string `json:"outbound,omitempty"`
}

var onlineResources = &onlines{}
var onlineResourcesM sync.RWMutex

type StatsService struct {
}

func (s *StatsService) SaveStats(enableTraffic bool) error {
	if corePtr == nil || !corePtr.IsRunning() {
		return nil
	}
	box := corePtr.GetInstance()
	if box == nil {
		return nil
	}
	st := box.StatsTracker()
	if st == nil {
		return nil
	}
	stats := st.GetStats()

	currentOnline := onlines{}

	if len(*stats) == 0 {
		onlineResourcesM.Lock()
		*onlineResources = currentOnline
		onlineResourcesM.Unlock()
		return nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	defer tx.Rollback()

	for _, stat := range *stats {
		if stat.Resource == "user" {
			var err error
			if stat.Direction {
				err = tx.Model(model.Client{}).Where("name = ?", stat.Tag).
					UpdateColumn("up", gorm.Expr("up + ?", stat.Traffic)).Error
			} else {
				err = tx.Model(model.Client{}).Where("name = ?", stat.Tag).
					UpdateColumn("down", gorm.Expr("down + ?", stat.Traffic)).Error
			}
			if err != nil {
				return err
			}
		}
		if stat.Direction {
			switch stat.Resource {
			case "inbound":
				currentOnline.Inbound = append(currentOnline.Inbound, stat.Tag)
			case "outbound":
				currentOnline.Outbound = append(currentOnline.Outbound, stat.Tag)
			case "user":
				currentOnline.User = append(currentOnline.User, stat.Tag)
			}
		}
	}
	if enableTraffic {
		if err := tx.Create(&stats).Error; err != nil {
			return err
		}
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	onlineResourcesM.Lock()
	*onlineResources = currentOnline
	onlineResourcesM.Unlock()
	return nil
}

func (s *StatsService) GetStats(resource string, tag string, limit int) ([]model.Stats, error) {
	var err error
	var result []model.Stats

	currentTime := time.Now().Unix()
	timeDiff := currentTime - (int64(limit) * 3600)

	db := database.GetDB()
	resources := []string{resource}
	if resource == "endpoint" {
		resources = []string{"inbound", "outbound"}
	}
	err = db.Model(model.Stats{}).Where("resource in ? AND tag = ? AND date_time > ?", resources, tag, timeDiff).Scan(&result).Error
	if err != nil {
		return nil, err
	}

	result = s.downsampleStats(result, 60) // 60 rows for 30 buckets
	return result, nil
}

// downsampleStats reduces stats to maxRows rows.
// Each bucket outputs two rows (direction false and true) with average Traffic.
func (s *StatsService) downsampleStats(stats []model.Stats, maxRows int) []model.Stats {
	if len(stats) <= maxRows {
		return stats
	}
	numBuckets := int(maxRows / 2)
	sort.Slice(stats, func(i, j int) bool { return stats[i].DateTime < stats[j].DateTime })
	timeMin, timeMax := stats[0].DateTime, stats[len(stats)-1].DateTime
	bucketSpan := (timeMax - timeMin) / int64(numBuckets)
	if bucketSpan == 0 {
		bucketSpan = 1
	}
	downsampled := make([]model.Stats, 0, maxRows)
	for i := 0; i < numBuckets; i++ {
		bucketStart := timeMin + int64(i)*bucketSpan
		bucketEnd := timeMin + int64(i+1)*bucketSpan
		if i == numBuckets-1 {
			bucketEnd = timeMax + 1
		}
		for _, dir := range []bool{false, true} {
			var sum int64
			var count int
			for _, r := range stats {
				if r.DateTime >= bucketStart && r.DateTime < bucketEnd && r.Direction == dir {
					sum += r.Traffic
					count++
				}
			}
			avg := int64(0)
			if count > 0 {
				avg = sum / int64(count)
			}
			downsampled = append(downsampled, model.Stats{
				DateTime:  bucketStart,
				Resource:  stats[0].Resource,
				Tag:       stats[0].Tag,
				Direction: dir,
				Traffic:   avg,
			})
		}
	}
	return downsampled
}

func (s *StatsService) GetOnlines() (onlines, error) {
	onlineResourcesM.RLock()
	defer onlineResourcesM.RUnlock()
	return onlines{
		Inbound:  append([]string(nil), onlineResources.Inbound...),
		User:     append([]string(nil), onlineResources.User...),
		Outbound: append([]string(nil), onlineResources.Outbound...),
	}, nil
}
func (s *StatsService) DelOldStats(days int) error {
	oldTime := time.Now().AddDate(0, 0, -(days)).Unix()
	db := database.GetDB()
	return db.Where("date_time < ?", oldTime).Delete(model.Stats{}).Error
}
