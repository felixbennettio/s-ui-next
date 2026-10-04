package service

import (
	"encoding/json"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	"gorm.io/gorm"
)

// rewriteInboundRules removes rules whose inbound condition no longer has any
// possible matches. Removing only the condition would turn them into catch-all
// rules. For AND or inverted logical rules, discard the containing rule if a
// child disappears, rather than broadening its match.
func rewriteInboundRules(rules []interface{}, rewrite func(string) string) ([]interface{}, bool) {
	result := make([]interface{}, 0, len(rules))
	changed := false
	for _, raw := range rules {
		rule, ok := raw.(map[string]interface{})
		if !ok {
			result = append(result, raw)
			continue
		}
		drop := false
		if value, exists := rule["inbound"]; exists {
			tags := stringsValue(value)
			next := make([]string, 0, len(tags))
			edited := false
			for _, tag := range tags {
				replacement := rewrite(tag)
				if replacement != tag {
					edited = true
				}
				if replacement != "" {
					next = append(next, replacement)
				}
			}
			if edited {
				changed = true
				if len(next) == 0 {
					drop = true
				} else {
					rule["inbound"] = next
				}
			}
		}
		if children, ok := rule["rules"].([]interface{}); ok && !drop {
			next, edited := rewriteInboundRules(children, rewrite)
			if edited {
				changed = true
				if len(next) == 0 || (len(next) < len(children) && (rule["mode"] != "or" || rule["invert"] == true)) {
					drop = true
				} else {
					rule["rules"] = next
				}
			}
		}
		if !drop {
			result = append(result, rule)
		}
	}
	return result, changed
}

func rewriteConfigInboundReferences(tx *gorm.DB, rewrite func(string) string) (bool, error) {
	var setting model.Setting
	if err := tx.Where("key = ?", "config").First(&setting).Error; err != nil {
		if database.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(setting.Value), &config); err != nil {
		return false, err
	}
	changed := false
	for _, section := range []string{"route", "dns"} {
		if obj, ok := config[section].(map[string]interface{}); ok {
			if rules, ok := obj["rules"].([]interface{}); ok {
				next, edited := rewriteInboundRules(rules, rewrite)
				if edited {
					obj["rules"] = next
					changed = true
				}
			}
		}
	}
	if experimental := mapValue(config["experimental"]); experimental != nil {
		stats := mapValue(mapValue(experimental["v2ray_api"])["stats"])
		if stats != nil {
			if tags := stringsValue(stats["inbounds"]); len(tags) > 0 {
				next := make([]string, 0, len(tags))
				for _, tag := range tags {
					replacement := rewrite(tag)
					if replacement != tag {
						changed = true
					}
					if replacement != "" {
						next = append(next, replacement)
					}
				}
				stats["inbounds"] = next
			}
		}
	}
	if !changed {
		return false, nil
	}
	raw, err := json.Marshal(config)
	if err != nil {
		return false, err
	}
	return true, tx.Model(&model.Setting{}).Where("key = ?", "config").Update("value", string(raw)).Error
}

// Keep persisted peer settings and generated routes in sync with user rules.
func rewriteInboundReferences(tx *gorm.DB, rewrite func(string) string) (bool, error) {
	changed, err := rewriteConfigInboundReferences(tx, rewrite)
	if err != nil {
		return false, err
	}
	rewriteTags := func(tags []string) ([]string, bool) {
		next := make([]string, 0, len(tags))
		edited := false
		for _, tag := range tags {
			replacement := rewrite(tag)
			edited = edited || replacement != tag
			if replacement != "" {
				next = append(next, replacement)
			}
		}
		return next, edited
	}
	var endpoints []model.Endpoint
	if err := tx.Where("type = ?", "wireguard").Find(&endpoints).Error; err != nil {
		return false, err
	}
	for _, endpoint := range endpoints {
		var options map[string]interface{}
		if err := json.Unmarshal(endpoint.Options, &options); err != nil {
			return false, err
		}
		edited := false
		for _, raw := range listValue(options["peers"]) {
			peer := mapValue(raw)
			if next, rewriteNeeded := rewriteTags(stringsValue(peer["route_inbounds"])); rewriteNeeded {
				peer["route_inbounds"] = next
				edited = true
			}
		}
		if !edited {
			continue
		}
		endpoint.Options, err = json.Marshal(options)
		if err != nil {
			return false, err
		}
		if err = tx.Model(&endpoint).Update("options", endpoint.Options).Error; err != nil {
			return false, err
		}
		if err = syncWireGuardManagedRoute(tx, &endpoint); err != nil {
			return false, err
		}
		changed = true
	}
	var managed []model.ManagedRouteRule
	if err := tx.Find(&managed).Error; err != nil {
		return false, err
	}
	for _, item := range managed {
		if item.InboundTags == "" {
			continue
		}
		var tags []string
		if err := json.Unmarshal([]byte(item.InboundTags), &tags); err != nil {
			return false, err
		}
		if next, edited := rewriteTags(tags); edited {
			if len(next) == 0 {
				err = tx.Delete(&item).Error
			} else {
				err = tx.Model(&item).Update("inbound_tags", jsonStringList(next)).Error
			}
			if err != nil {
				return false, err
			}
			changed = true
		}
	}
	return changed, nil
}

func renameInboundReferences(tx *gorm.DB, oldTag, newTag string) (bool, error) {
	if oldTag == "" || oldTag == newTag {
		return false, nil
	}
	return rewriteInboundReferences(tx, func(tag string) string {
		if tag == oldTag {
			return newTag
		}
		return tag
	})
}

func reconcileInboundReferences(tx *gorm.DB) (bool, error) {
	var tags, endpoints []string
	if err := tx.Model(&model.Inbound{}).Pluck("tag", &tags).Error; err != nil {
		return false, err
	}
	if err := tx.Model(&model.Endpoint{}).Pluck("tag", &endpoints).Error; err != nil {
		return false, err
	}
	valid := map[string]bool{}
	for _, tag := range append(tags, endpoints...) {
		valid[tag] = true
	}
	return rewriteInboundReferences(tx, func(tag string) string {
		if valid[tag] {
			return tag
		}
		return ""
	})
}

// Repair references left by older panel versions when loading an existing DB.
func (s *ConfigService) ReconcileInboundReferences() error {
	configSaveMu.Lock()
	defer configSaveMu.Unlock()
	return database.GetDB().Transaction(func(tx *gorm.DB) error {
		_, err := reconcileInboundReferences(tx)
		return err
	})
}
