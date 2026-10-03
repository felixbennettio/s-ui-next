package service

import (
	"encoding/json"

	"github.com/ciallothu/s-ui-next/database"
	"github.com/ciallothu/s-ui-next/database/model"
	"github.com/ciallothu/s-ui-next/util/common"
)

// WireGuardSecret is called only by an explicit authenticated copy action.
// Normal list/config responses continue to redact saved keys.
func (s *EndpointService) WireGuardSecret(id uint, field, publicKey string) (string, error) {
	if id == 0 {
		return "", common.NewError("save the WireGuard endpoint before copying a stored key")
	}
	if field != "private_key" && field != "client_private_key" && field != "pre_shared_key" {
		return "", common.NewError("unsupported WireGuard secret field")
	}
	var endpoint model.Endpoint
	if err := database.GetDB().Where("id = ? AND type = ?", id, "wireguard").First(&endpoint).Error; err != nil {
		return "", err
	}
	var root map[string]interface{}
	if err := json.Unmarshal(endpoint.Options, &root); err != nil {
		return "", err
	}
	secret := ""
	if field == "private_key" {
		secret = stringValue(root[field])
	} else {
		for _, raw := range listValue(root["peers"]) {
			peer := mapValue(raw)
			if publicKey != "" && stringValue(peer["public_key"]) == publicKey {
				secret = stringValue(peer[field])
				break
			}
		}
		if secret == "" && field == "client_private_key" && publicKey != "" {
			var ext map[string]interface{}
			if err := json.Unmarshal(endpoint.Ext, &ext); err == nil {
				for _, raw := range listValue(ext["keys"]) {
					key := mapValue(raw)
					if stringValue(key["public_key"]) == publicKey {
						secret = stringValue(key["private_key"])
						break
					}
				}
			}
		}
	}
	if secret == "" || secret == wireGuardRedactedSecret {
		return "", common.NewError("stored key is unavailable; save or reload this endpoint first")
	}
	return secret, nil
}
