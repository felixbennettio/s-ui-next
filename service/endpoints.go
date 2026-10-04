package service

import (
	"encoding/json"
	"strings"

	"github.com/felixbennettio/s-ui-next/database"
	"github.com/felixbennettio/s-ui-next/database/model"
	"github.com/felixbennettio/s-ui-next/util/common"

	"gorm.io/gorm"
)

type EndpointService struct {
	WarpService
}

func (o *EndpointService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	endpoints := []*model.Endpoint{}
	err := db.Model(model.Endpoint{}).Scan(&endpoints).Error
	if err != nil {
		return nil, err
	}
	var data []map[string]interface{}
	for _, endpoint := range endpoints {
		epData := map[string]interface{}{
			"id":   endpoint.Id,
			"type": endpoint.Type,
			"tag":  endpoint.Tag,
		}
		var ext interface{}
		if len(endpoint.Ext) > 0 && string(endpoint.Ext) != "null" {
			if err := json.Unmarshal(endpoint.Ext, &ext); err != nil {
				return nil, err
			}
		}
		epData["ext"] = ext
		if endpoint.Options != nil {
			var restFields map[string]interface{}
			if err := json.Unmarshal(endpoint.Options, &restFields); err != nil {
				return nil, err
			}
			for k, v := range restFields {
				epData[k] = v
			}
		}
		if endpoint.Type == "wireguard" {
			redactWireGuardSecrets(epData)
		} else if endpoint.Type == "warp" {
			redactWarpSecrets(epData)
		}
		data = append(data, epData)
	}
	return &data, nil
}

func (o *EndpointService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var endpointsJson []json.RawMessage
	var endpoints []*model.Endpoint
	err := db.Model(model.Endpoint{}).Scan(&endpoints).Error
	if err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		endpointJson, err := endpoint.MarshalJSON()
		if err != nil {
			return nil, err
		}
		endpointsJson = append(endpointsJson, endpointJson)
	}
	return endpointsJson, nil
}

func (s *EndpointService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	var err error

	switch act {
	case "new", "edit":
		var oldEndpoint *model.Endpoint
		if act == "edit" {
			oldEndpoint = &model.Endpoint{}
			var idHolder struct {
				Id uint `json:"id"`
			}
			_ = json.Unmarshal(data, &idHolder)
			if err = tx.First(oldEndpoint, idHolder.Id).Error; err != nil {
				return err
			}
			data, err = mergeWireGuardSecrets(data, oldEndpoint)
			if err != nil {
				return err
			}
			data, err = mergeWarpSecrets(data, oldEndpoint)
			if err != nil {
				return err
			}
		}
		data, err = normalizeAndValidateWireGuard(data)
		if err != nil {
			return err
		}
		var endpoint model.Endpoint
		err = endpoint.UnmarshalJSON(data)
		if err != nil {
			return err
		}
		endpoint.Tag = strings.TrimSpace(endpoint.Tag)
		if err = ensureEgressTagAvailable(tx, "endpoint", endpoint.Id, endpoint.Tag); err != nil {
			return err
		}

		if endpoint.Type == "warp" {
			if act == "new" {
				err = s.WarpService.RegisterWarp(&endpoint)
				if err != nil {
					return err
				}
			} else {
				err = s.WarpService.SetWarpLicense(warpLicense(oldEndpoint), &endpoint)
				if err != nil {
					return err
				}
			}
		}

		err = tx.Save(&endpoint).Error
		if err != nil {
			return err
		}
		if oldEndpoint != nil && oldEndpoint.Tag != endpoint.Tag {
			if err = tx.Where("endpoint_tag = ?", oldEndpoint.Tag).Delete(&model.ManagedRouteRule{}).Error; err != nil {
				return err
			}
		}
		if err = syncWireGuardManagedRoute(tx, &endpoint); err != nil {
			return err
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return err
		}
		if err = tx.Where("endpoint_tag = ?", tag).Delete(&model.ManagedRouteRule{}).Error; err != nil {
			return err
		}
		err = tx.Where("tag = ?", tag).Delete(model.Endpoint{}).Error
		if err != nil {
			return err
		}
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
	return nil
}
