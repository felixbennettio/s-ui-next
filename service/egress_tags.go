package service

import (
	"strings"

	"github.com/felixbennettio/s-ui-next/database/model"
	"github.com/felixbennettio/s-ui-next/util/common"

	"gorm.io/gorm"
)

func ensureEgressTagAvailable(tx *gorm.DB, kind string, id uint, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return common.NewError("tag is required")
	}

	var count int64
	switch kind {
	case "endpoint":
		query := tx.Model(&model.Endpoint{}).Where("tag = ?", tag)
		if id > 0 {
			query = query.Where("id <> ?", id)
		}
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.NewErrorf("tag %q is already used by another tunnel or endpoint", tag)
		}
		if err := tx.Model(&model.Outbound{}).Where("tag = ?", tag).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.NewErrorf("tag %q is already used by an outbound; tunnel and outbound tags share one namespace", tag)
		}
	case "outbound":
		query := tx.Model(&model.Outbound{}).Where("tag = ?", tag)
		if id > 0 {
			query = query.Where("id <> ?", id)
		}
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.NewErrorf("tag %q is already used by another outbound", tag)
		}
		if err := tx.Model(&model.Endpoint{}).Where("tag = ?", tag).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.NewErrorf("tag %q is already used by a tunnel or endpoint; tunnel and outbound tags share one namespace", tag)
		}
	default:
		return common.NewErrorf("unknown egress resource type: %s", kind)
	}
	return nil
}
