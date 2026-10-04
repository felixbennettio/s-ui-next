package service

import (
	"encoding/hex"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/felixbennettio/s-ui-next/util/common"
	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/webauthn"
)

const maxPasskeyNameRunes = 80

// These stable authenticator model identifiers are exposed by WebAuthn in the
// attested credential data. Unknown identifiers deliberately fall back to the
// attachment and platform hints instead of being presented as a known vendor.
var passkeyProviderByAAGUID = map[string]string{
	"08987058-cadc-4b81-b6e1-30de50dcbe96": "Windows Hello",
	"0ea242b4-43c4-4a1b-8b17-dd6d0b6baec6": "Keeper",
	"50726f74-6f6e-5061-7373-50726f746f6e": "Proton Pass",
	"531126d6-e717-415c-9320-3d9aa6981239": "Dashlane",
	"53414d53-554e-4700-0000-000000000000": "Samsung Pass",
	"6028b017-b1d4-4c02-b4b3-afcdafc96bb2": "Windows Hello",
	"771b48fd-d3d4-4f74-9232-fc157ab0507a": "Microsoft Edge on macOS",
	"9ddd1817-af5a-4672-a2b9-3e3dd95000a9": "Windows Hello",
	"adce0002-35bc-c60a-648b-0b25f1f05503": "Chrome on macOS",
	"b5397666-4885-aa6b-cebf-e52262a439a2": "Chromium",
	"b84e4048-15dc-4dd0-8640-f4f60813c8af": "NordPass",
	"bada5566-a7aa-401f-bd96-45619a55120d": "1Password",
	"d548826e-79b4-db40-a3d8-11116f7e8349": "Bitwarden",
	"dd4ec289-e01d-41c9-bb89-70fa845d4bf2": "iCloud Keychain (Managed)",
	"ea9b8d66-4d01-1d21-3ce4-b6b48cb575d4": "Google Password Manager",
	"f3809540-7f14-49c1-a8b3-8f813b225541": "Enpass",
	"fbfc3007-154e-4ecc-8c0b-6e020557d7bd": "iCloud Keychain",
	"fdb141b2-5d84-443e-8a35-4698c205a502": "KeePassXC",
}

func resolvePasskeyName(credential *webauthn.Credential, requestedName, platform string) (string, error) {
	name, err := normalizePasskeyName(requestedName)
	if err != nil {
		return "", err
	}
	if name != "" {
		return name, nil
	}
	if credential != nil {
		if provider := passkeyProviderByAAGUID[formatAAGUID(credential.Authenticator.AAGUID)]; provider != "" {
			return provider, nil
		}
	}
	return fallbackPasskeyName(credential, platform), nil
}

func normalizePasskeyName(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", nil
	}
	if !utf8.ValidString(value) || utf8.RuneCountInString(value) > maxPasskeyNameRunes {
		return "", common.NewError("passkey name is too long")
	}
	for _, char := range value {
		if unicode.IsControl(char) {
			return "", common.NewError("passkey name contains invalid characters")
		}
	}
	return value, nil
}

func formatAAGUID(value []byte) string {
	if len(value) != 16 {
		return ""
	}
	raw := hex.EncodeToString(value)
	return raw[:8] + "-" + raw[8:12] + "-" + raw[12:16] + "-" + raw[16:20] + "-" + raw[20:]
}

func fallbackPasskeyName(credential *webauthn.Credential, platform string) string {
	if credential == nil {
		return "Passkey"
	}

	hasHybrid := false
	isPlatform := credential.Authenticator.Attachment == protocol.Platform
	for _, transport := range credential.Transport {
		switch transport {
		case protocol.USB, protocol.NFC, protocol.BLE, protocol.SmartCard:
			return "Security key"
		case protocol.Hybrid:
			hasHybrid = true
		case protocol.Internal:
			isPlatform = true
		}
	}
	if hasHybrid {
		return "Phone or tablet"
	}
	if credential.Authenticator.Attachment == protocol.CrossPlatform {
		return "Security key"
	}
	if !isPlatform {
		return "Passkey"
	}

	switch strings.ToLower(strings.TrimSpace(platform)) {
	case "windows":
		return "Windows Hello"
	case "macos", "ios":
		return "iCloud Keychain"
	case "android", "chromeos":
		return "Google Password Manager"
	default:
		return "Platform passkey"
	}
}
