# S-UI Next

**A sing-box management panel for web and mobile**

[简体中文](README.md) | [English](README.en.md)

[![Latest release](https://img.shields.io/github/v/release/felixbennettio/s-ui-next.svg)](https://github.com/felixbennettio/s-ui-next/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/felixbennettio/s-ui-next)](https://goreportcard.com/report/github.com/felixbennettio/s-ui-next)
[![Downloads](https://img.shields.io/github/downloads/felixbennettio/s-ui-next/total.svg)](https://github.com/felixbennettio/s-ui-next/releases)
[![License](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)

S-UI Next is a downstream project based on [alireza0/s-ui](https://github.com/alireza0/s-ui). It keeps the original panel and database model while adding a versioned API, Android and iPhone management apps, stronger administrator authentication, searchable traffic and connection records, safer subscription links, WireGuard relays between VPS hosts, and directly managed Cloudflare WARP egress.

The embedded core currently follows `sing-box v1.13.14`. Existing Web management, API v2, database, and subscription interfaces remain available, so an existing S-UI installation can be migrated without rebuilding its configuration from scratch.

> Use this project only where it is legal to do so. You are responsible for the configuration you deploy and the traffic carried by it.

## What S-UI Next Adds

### Web panel and configuration

- Manage clients, inbounds, outbounds, endpoints, services, TLS, DNS, routing rules, and global sing-box settings from one panel.
- Use structured editors for day-to-day configuration or switch to raw JSON when a sing-box field is not represented by the form.
- Manage users individually or in bulk, including traffic quota, expiry, group, enable/disable state, and subscription options.
- View system state, online users, resource traffic, connection details, logs, administrator changes, and historical usage without leaving the panel.
- Keep historical charts stable until they are refreshed, or enable the separate real-time mode when live traffic is needed.
- Read long usernames, IPv6 addresses, targets, and log messages clearly on desktop and small screens without overlapping content.
- Use dark or light themes in English, Farsi, Vietnamese, Simplified Chinese, Traditional Chinese, Russian, Japanese, French, or Latin.

### Mobile app

The Flutter app in [`mobile/`](mobile/README.md) talks directly to `/apiv3`; it is not a WebView wrapper.

- Android arm64 APK and unsigned iPhone arm64 IPA are available from the Releases page.
- The dashboard, users, resources, TLS, core configuration, analytics, logs, administrators, settings, backup, and tools pages follow the Web panel's management model.
- Resource and configuration screens provide both visual editing and raw JSON. Numeric lists such as ports, user IDs, and WireGuard reserved values retain their JSON types.
- A username/password login can create a dedicated mobile API token. Tokens, panel addresses, and custom headers are kept in Android Keystore or iOS Keychain backed secure storage.
- Connection profiles accept arbitrary request headers and include dedicated fields for Cloudflare Access Service Tokens.
- Multiple panels can be saved at the same time. The panel switcher sits beside the current panel name at the top of the navigation drawer and is also available before login.
- Switching panels rebuilds the active view and reloads dashboard, resource, configuration, analytics, administration, and tools data automatically. No manual pull-to-refresh is required.
- Existing single-panel profiles are migrated automatically. A normal logout keeps the saved profile; revoking the token removes that panel's local credentials.

### API v3

`/apiv3` is the stable JSON interface used by the mobile app and is also suitable for third-party clients.

- Password login, API token issue/list/revoke, authenticated bootstrap, panel metadata, status, and online users.
- CRUD and bulk operations for clients, inbounds, outbounds, endpoints, services, TLS, global configuration, and settings.
- User usage, resource statistics, parsed connections, system logs, and administrator audit history with server-side search, time filters, and bounded pagination.
- Database backup import/export, sing-box configuration export, panel/core restart, link and subscription conversion, key generation, and outbound checks.
- Consistent success/error envelopes and standard HTTP status codes. Bearer tokens are preferred; legacy token headers remain supported for existing clients.

See [`docs/mobile-api.md`](docs/mobile-api.md) for routes, parameters, and response conventions.

### Administrator authentication

- OIDC single sign-on with configurable issuer, client credentials, scopes, username claim, and an allow-list for external identities.
- TOTP two-factor authentication with one-time recovery codes.
- WebAuthn passkeys for registration and passwordless login, with automatic RP ID and origin detection behind common reverse proxies.
- Passkey names derived from the authenticator AAGUID when available. Known providers include Bitwarden, 1Password, iCloud Keychain, Google Password Manager, Windows Hello, Dashlane, Keeper, NordPass, Proton Pass, and KeePassXC.
- Privacy-preserving or unknown authenticators fall back to a name based on platform, attachment type, and transports. Names can still be edited manually.
- Passwords are stored with bcrypt. Legacy plaintext administrator records are upgraded after a successful login.
- Web sessions use HttpOnly cookies, and the login router verifies the server-side session rather than depending on JavaScript access to the cookie.

### Analytics, logs, and connection attribution

- Aggregate usage by user and inspect resource statistics by tag and date range.
- Search parsed connections by user, inbound, outbound, endpoint, target, source, or message text.
- Enrich source, destination, and remote addresses with IP, network type, ASN, organization, and location data where available. Private and reserved networks are identified without unnecessary external lookups.
- Browse structured system logs and administrator change history with user, level, date, and text filters.
- Use the same analytics and connection-detail model in Web and mobile views.

### Subscriptions and client privacy

- New clients receive random opaque subscription IDs, so generated public URLs do not expose a username. Legacy username links remain readable for upgrades and older clients.
- Subscription user information can independently expose upload, download, quota, expiry, and remaining quota in the node name.
- Link, JSON, and Clash subscriptions continue to support external links and subscriptions while applying stricter URL, domain, size, and data validation.
- Disabled subscription information no longer leaks a partial `Subscription-Userinfo` header, and incomplete metadata is handled without panics.

### Tunnels and egress

- Create panel-managed WireGuard clients or connect an existing VPS or device without giving its private key to the panel.
- Support listening for a peer, connecting to a fixed peer, relaying between two VPS hosts, full egress, and site-to-site connections.
- Configure local listening, peer connection details, traffic scope, and client profile export independently for different network arrangements.
- Generate client profiles and QR codes only when needed, or manage a server-to-server connection without a client profile.
- Receive accurate feedback when saving or applying WireGuard changes, without duplicate submissions or misleading duplicate-name errors. A failed apply keeps the previous working configuration.
- Create Cloudflare WARP directly as an egress without installing a separate WARP client or bridging through SOCKS.
- View saved WireGuard keys, WARP device credentials, and licenses only in a redacted form.

### Security and data safety

- Login rate limiting and bounded authentication sessions reduce brute-force and resource exhaustion risk.
- Configuration changes are validated before they reach sing-box. Applying a configuration checks the restarted core and restores the previous working state on failure.
- Database backup import validates the uploaded database, replaces it atomically, and restores the previous database if activation fails.
- Security-sensitive identifiers and keys use cryptographically secure randomness.
- External requests have timeouts and response-size limits; panel addresses, domains, links, subscriptions, and generated configuration are validated before use.
- Frontend content is rendered without unsafe dynamic HTML, and failed requests do not silently replace valid panel data.

## Supported Protocols

| Category | Protocols and modes |
| --- | --- |
| General | Mixed, SOCKS, HTTP, HTTPS, Direct, Redirect, TProxy |
| Proxy | VLESS, VMess, Trojan, Shadowsocks, ShadowTLS |
| Modern transports | Hysteria, Hysteria2, TUIC, Naive |
| Tunnels and egress | WireGuard, Tailscale, Cloudflare WARP |
| Routing and security | XTLS, Reality, uTLS, ACME, gVisor, PROXY Protocol, transparent proxying |

Support ultimately follows the embedded sing-box version and the build tags used by each release target.

## Downloads

| Target | Architectures | Artifact |
| --- | --- | --- |
| Linux server | amd64, arm64, armv7, armv6, armv5, 386, s390x | `.tar.gz` |
| Windows server | amd64, arm64 | `.zip` |
| Android app | arm64 | `.apk` |
| iPhone app | arm64 | unsigned `.ipa` |
| GHCR image | linux/amd64, linux/386, linux/arm64/v8, linux/arm/v7, linux/arm/v6 | OCI image |

Download the current packages from [GitHub Releases](https://github.com/felixbennettio/s-ui-next/releases/latest). The iPhone package is not signed and must be signed with your own Apple Developer identity before installation.

## Quick Start

### Docker Compose

```sh
mkdir s-ui-next && cd s-ui-next
curl -fsSLO https://raw.githubusercontent.com/felixbennettio/s-ui-next/main/docker-compose.yml
docker compose up -d
```

The compose file stores the database in `./db`, certificates in `./cert`, and exposes the default panel and subscription ports.

### Docker CLI

```sh
mkdir -p s-ui-next/db s-ui-next/cert
cd s-ui-next
docker run -d \
  --name s-ui-next \
  --restart unless-stopped \
  -p 2095:2095 \
  -p 2096:2096 \
  -v "$PWD/db:/app/db" \
  -v "$PWD/cert:/app/cert" \
  ghcr.io/felixbennettio/s-ui-next:latest
```

### Linux packages

1. Download `s-ui-next-<tag>-linux-<arch>.tar.gz` from the latest release.
2. Extract it and place the `s-ui-next` directory under `/usr/local/`.
3. Install `s-ui-next.sh` as `/usr/bin/s-ui-next` and copy `s-ui-next.service` to `/etc/systemd/system/`.
4. Run `systemctl daemon-reload && systemctl enable --now s-ui-next`.
5. Use `s-ui-next` for the interactive management menu.

### Windows packages

1. Download the matching Windows ZIP from the latest release.
2. Extract it and run `install-windows.bat` as Administrator.
3. Use `s-ui-next-windows.bat` for service management.

### Mobile packages

- Install the Android arm64 APK directly on a compatible device.
- Sign the unsigned iPhone arm64 IPA with your own certificate before installing it.
- Add the panel URL, credentials or API token, and any reverse-proxy headers on the connection screen.

## Default Settings

| Setting | Default |
| --- | --- |
| Panel URL | `http://<host>:2095/app/` |
| Subscription URL | `http://<host>:2096/sub/` |
| Initial database account | `admin` / `admin` |

Change the initial credentials and publish the panel through HTTPS before exposing it to an untrusted network. Ports, paths, and administrator credentials can be changed from the management menu or the Web panel.

## Authentication Setup

Authentication options are under **Settings → Login & identity** and **Admins → Login security**.

### OIDC / SSO

Configure the issuer URL, client ID, client secret, scopes, username claim, and allowed identities. For the default Web Path, register this callback with the identity provider:

```text
https://panel.example.com/app/api/oidc-callback
```

If the Web Path changes, the callback path must change with it. The username claim defaults to `preferred_username`, then falls back to `email` and `sub`.

### TOTP / 2FA

Enable TOTP for an administrator from **Admins → Login security**. Save the generated recovery codes immediately; each code is valid once.

### WebAuthn passkeys

Enable passkeys globally, then register them for each administrator. RP ID and allowed origins can normally remain empty: S-UI Next derives them from the browser origin and trusted `Forwarded`, `X-Forwarded-Host`, and `X-Forwarded-Proto` headers.

For unusual proxy layouts, set the RP ID to a domain such as `panel.example.com` and allowed origins to complete origins such as `https://panel.example.com`. WebAuthn requires HTTPS except on localhost-style development origins.

## WireGuard Configuration Notes

- **Local endpoint addresses** identify this S-UI Next instance and are normally host routes such as `10.66.66.1/32` and `fd66:66:66::1/128`.
- **Virtual network prefixes** are only allocation ranges for managed clients. They may be omitted for an existing VPS peer, but the local WireGuard address is still required.
- **Runtime AllowedIPs** select the addresses or destination networks carried by a peer. A relay initiator can explicitly route all traffic to a fixed peer, while a receiver can listen and learn the peer endpoint dynamically.
- **Client AllowedIPs** choose destination traffic sent through the tunnel. New peers default to the WireGuard virtual networks; `0.0.0.0/0` and `::/0` are emitted only by the full-tunnel preset.
- **Client profile export** is optional. When enabled, its host and port must point to the real public UDP listener.
- **Dynamic peers** suit NATed devices or a receiving relay VPS. **Fixed peers** are contacted by this server at an explicit address and port.
- **Site gateways** add the remote LANs to the server-side peer while exporting the configured local LANs to that gateway. Both sides still need a valid return route or separately configured NAT.

## Environment Variables

| Variable | Values | Default |
| --- | --- | --- |
| `SUI_LOG_LEVEL` | `debug`, `info`, `warn`, `error` | `info` |
| `SUI_DEBUG` | boolean | `false` |
| `SUI_BIN_FOLDER` | directory | `bin` |
| `SUI_DB_FOLDER` | directory | `db` |
| `SINGBOX_API` | sing-box API address | unset |

## Development

```sh
git clone --recurse-submodules https://github.com/felixbennettio/s-ui-next.git
cd s-ui-next
```

- Backend: Go `1.26.5`; the exact version is declared in `go.mod`.
- Frontend: Vue and TypeScript in the [`frontend`](https://github.com/felixbennettio/s-ui-next-frontend) submodule. Use `npm ci`, then `npm run build`.
- Mobile: Flutter source lives in `mobile/`.
- Full development and contribution instructions are in [`CONTRIBUTING.md`](CONTRIBUTING.md).

## Credits and License

S-UI Next builds on [alireza0/s-ui](https://github.com/alireza0/s-ui) and [SagerNet/sing-box](https://github.com/SagerNet/sing-box). The Web frontend is maintained in [felixbennettio/s-ui-next-frontend](https://github.com/felixbennettio/s-ui-next-frontend).

This project is distributed under the [GNU General Public License v3.0](LICENSE).
