# S-UI Next

**面向 Web 和移动端的 sing-box 管理面板**

[简体中文](README.md) | [English](README.en.md)

[![最新版本](https://img.shields.io/github/v/release/felixbennettio/s-ui-next.svg)](https://github.com/felixbennettio/s-ui-next/releases/latest)
[![Go Report Card](https://goreportcard.com/badge/github.com/felixbennettio/s-ui-next)](https://goreportcard.com/report/github.com/felixbennettio/s-ui-next)
[![下载量](https://img.shields.io/github/downloads/felixbennettio/s-ui-next/total.svg)](https://github.com/felixbennettio/s-ui-next/releases)
[![许可证](https://img.shields.io/badge/license-GPLv3-blue.svg)](LICENSE)

S-UI Next 是基于 [alireza0/s-ui](https://github.com/alireza0/s-ui) 继续维护的下游项目。它保留原有面板和数据库模型，在此基础上加入版本化 API、Android 与 iPhone 管理 App、管理员多因素认证、可检索的流量与连接记录、更加稳妥的订阅链接、支持双 VPS 中转的 WireGuard，以及可直接创建的 Cloudflare WARP 出口。

当前内置核心使用 `sing-box v1.13.14`。原有 Web 管理方式、API v2、数据库和订阅接口继续保留，已有 S-UI 数据可以迁移，不需要重新录入整套配置。

> 请只在当地法律允许的范围内使用本项目。部署配置及其承载的流量由使用者自行负责。

## S-UI Next 增加了什么

### Web 面板与配置管理

- 在同一面板中管理用户、入站、出站、Endpoint、服务、TLS、DNS、路由规则和 sing-box 全局配置。
- 常用配置使用结构化表单，也可以随时切换到原始 JSON，处理表单尚未覆盖的 sing-box 字段。
- 支持单独或批量管理用户，包括流量上限、到期时间、分组、启用状态和订阅选项。
- 面板内可以查看系统状态、在线用户、资源流量、连接详情、系统日志、管理员变更记录和历史用量。
- 历史流量图保持稳定，不会自动改变查看区间；需要实时数据时可以单独开启实时模式。
- 连接详情和日志可以清晰显示长用户名、IPv6 地址、长目标地址和长消息，在桌面与小屏幕上都不会相互遮挡。
- 支持深色与浅色主题，以及英语、波斯语、越南语、简体中文、繁体中文、俄语、日语、法语和拉丁语。

### 手机 App

[`mobile/`](mobile/README.md) 中的 Flutter App 直接调用 `/apiv3`，不是 WebView 封装。

- 提供 Android arm64 APK 和未签名的 iPhone arm64 IPA。
- 首页、用户、资源、TLS、核心配置、统计、日志、管理员、设置、备份和工具等页面与 Web 面板采用同一套管理模型。
- 资源和配置同时支持可视化编辑与原始 JSON。端口、用户 ID、WireGuard reserved 等数字列表会保持正确的数据类型。
- 可以使用管理员账号密码登录并创建独立的移动端 API Token。Token、面板地址和自定义 Header 保存在 Android Keystore 或 iOS Keychain 支持的安全存储中。
- 连接配置支持任意请求 Header，并提供 Cloudflare Access Service Token 专用字段。
- 可以同时保存多个控制面板。切换入口位于侧边栏顶部当前面板名称旁，未登录时也可以从连接页选择其他面板。
- 切换面板后，首页、资源、配置、统计、管理和工具等数据会自动重新加载，不需要手动下拉刷新，也不会继续显示上一个面板的数据。
- 旧的单面板连接会自动迁移。普通退出会保留已保存面板；撤销 Token 并退出时会删除该面板在本机保存的凭据。

### API v3

`/apiv3` 是移动端使用的稳定 JSON 接口，也可以供其他客户端接入。

- 管理员账号密码登录，API Token 签发、查询与撤销，初始化数据、面板信息、运行状态和在线用户。
- 用户、入站、出站、Endpoint、服务、TLS、全局配置和设置的增删改查与批量操作。
- 用户用量、资源统计、连接记录、系统日志和管理员审计，支持服务端搜索、时间筛选和受限分页。
- 数据库备份导入导出、sing-box 配置导出、面板与核心重启、链接与订阅转换、密钥生成和出站检查。
- 统一的成功与错误响应，并使用对应的 HTTP 状态码。推荐使用 Bearer Token，同时兼容旧客户端使用的 Token Header。

接口路径、参数和响应格式见 [`docs/mobile-api.md`](docs/mobile-api.md)。

### 管理员身份认证

- OIDC 单点登录，可配置 Issuer、Client ID、Client Secret、Scopes、用户名 Claim 和允许登录的外部身份。
- TOTP 两步验证，并提供一次性恢复码。
- WebAuthn 通行密钥，支持注册和免密码登录；常见反向代理环境下可以自动识别 RP ID 和 Origin。
- 在认证器提供 AAGUID 时自动识别通行密钥名称。目前覆盖 Bitwarden、1Password、iCloud 钥匙串、Google Password Manager、Windows Hello、Dashlane、Keeper、NordPass、Proton Pass 和 KeePassXC 等常见提供方。
- 对隐藏 AAGUID 或尚未收录的认证器，会根据系统平台、认证器类型和传输方式生成合适的名称，也可以在添加后手动改名。
- 管理员密码使用 bcrypt 保存；旧数据库中的明文密码会在成功登录后自动迁移。
- Web 登录使用 HttpOnly Session Cookie，前端通过服务端会话状态判断是否已经登录。

### 流量统计、日志与连接归属

- 按用户汇总用量，并按资源、Tag 和时间范围查看统计。
- 按用户、入站、出站、Endpoint、目标地址、来源地址或消息内容搜索连接记录。
- 在可获得数据时，为来源、目标和远端地址补充 IP、网络类型、ASN、组织和地区信息；私有与保留网段会在本地识别。
- 系统日志和管理员配置变更记录支持用户、级别、日期与关键词筛选。
- Web 与手机 App 使用相同的统计和连接详情数据模型。

### 订阅与用户隐私

- 新用户使用随机且不可猜测的订阅 ID，生成的公开链接不会直接暴露用户名。旧版用户名订阅链接继续兼容，便于已有部署升级。
- 可分别控制订阅信息中的上传量、下载量、总量、到期时间，以及节点名称中的剩余额度。
- Link、JSON 和 Clash 订阅继续支持外部链接与外部订阅，同时对 URL、域名、响应大小和数据格式进行校验。
- 关闭订阅信息后不会残留不完整的 `Subscription-Userinfo`，缺失的订阅元数据也不会造成异常。

### 隧道与出口管理

- 可以创建由面板管理的 WireGuard 客户端，也可以连接已有 VPS 或设备，无需将对方私钥交给面板。
- 支持监听等待对端、主动连接固定对端、双 VPS 中转、全局出口和站点互联等使用方式。
- 本机监听、对端连接、流量范围和客户端配置导出可以分别设置，适应只有部分连接信息可用的组网场景。
- 客户端配置和二维码可以按需生成；不需要客户端配置时，也可以只维护服务器之间的连接。
- WireGuard 保存与应用会提供准确反馈，避免重复提交和错误的重名提示；应用失败时保留上一份可用配置。
- Cloudflare WARP 可以直接创建为出口，无需另行安装 WARP 客户端或转接 SOCKS。
- WireGuard 密钥、WARP 设备凭据和许可证在保存后以脱敏状态显示。

### 安全与数据保护

- 登录接口带有限流，认证会话数量受到约束，降低暴力尝试和资源耗尽风险。
- 配置写入前会先进行校验；应用失败时恢复上一份可运行状态，避免核心停在半更新状态。
- 数据库备份导入会先检查上传文件，再进行原子替换；新数据库无法启用时会恢复旧数据库。
- Token、订阅 ID、密钥和其他安全敏感值使用密码学安全随机源生成。
- 外部请求设置超时和响应大小上限，面板地址、域名、链接、订阅和生成配置在使用前会经过校验。
- 前端不会直接插入未经处理的动态 HTML，请求失败也不会静默覆盖已经有效的面板数据。

## 支持的协议

| 分类 | 协议与模式 |
| --- | --- |
| 通用 | Mixed、SOCKS、HTTP、HTTPS、Direct、Redirect、TProxy |
| 代理协议 | VLESS、VMess、Trojan、Shadowsocks、ShadowTLS |
| 现代传输 | Hysteria、Hysteria2、TUIC、Naive |
| 隧道与出口 | WireGuard、Tailscale、Cloudflare WARP |
| 路由与安全 | XTLS、Reality、uTLS、ACME、gVisor、PROXY Protocol、透明代理 |

具体可用字段仍以当前内置 sing-box 版本和对应平台的构建能力为准。

## 下载

| 目标 | 架构 | 格式 |
| --- | --- | --- |
| Linux 服务端 | amd64、arm64、armv7、armv6、armv5、386、s390x | `.tar.gz` |
| Windows 服务端 | amd64、arm64 | `.zip` |
| Android App | arm64 | `.apk` |
| iPhone App | arm64 | 未签名 `.ipa` |
| GHCR 镜像 | linux/amd64、linux/386、linux/arm64/v8、linux/arm/v7、linux/arm/v6 | OCI 镜像 |

安装包可以从 [GitHub Releases](https://github.com/felixbennettio/s-ui-next/releases/latest) 下载。iPhone 安装包没有签名，需要使用自己的 Apple Developer 身份签名后安装。

## 快速开始

### Docker Compose

```sh
mkdir s-ui-next && cd s-ui-next
curl -fsSLO https://raw.githubusercontent.com/felixbennettio/s-ui-next/main/docker-compose.yml
docker compose up -d
```

Compose 默认将数据库保存在 `./db`，证书保存在 `./cert`，并开放面板和订阅端口。

### Docker 命令

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

### Linux 安装包

1. 从最新 Release 下载适合当前架构的 Linux 压缩包。
2. 解压后将其中的 `s-ui-next` 目录放到 `/usr/local/`。
3. 将 `s-ui-next.sh` 安装为 `/usr/bin/s-ui-next`，并把 `s-ui-next.service` 复制到 `/etc/systemd/system/`。
4. 执行 `systemctl daemon-reload && systemctl enable --now s-ui-next`。
5. 以后可以运行 `s-ui-next` 进入管理菜单。

### Windows 安装包

1. 从最新 Release 下载对应架构的 Windows ZIP。
2. 解压后以管理员身份运行 `install-windows.bat`。
3. 使用 `s-ui-next-windows.bat` 管理服务。

### 手机安装包

- Android arm64 设备可以直接安装 APK。
- iPhone arm64 IPA 需要自行签名后安装。
- 在连接页填写面板地址、账号密码或 API Token，以及反向代理需要的自定义 Header。

## 默认设置

| 项目 | 默认值 |
| --- | --- |
| 面板地址 | `http://<服务器地址>:2095/app/` |
| 订阅地址 | `http://<服务器地址>:2096/sub/` |
| 初始数据库账号 | `admin` / `admin` |

首次部署后请立即修改初始账号密码，并通过 HTTPS 对外提供管理面板。端口、路径和管理员凭据可以通过管理菜单或 Web 面板修改。

## 身份认证配置

认证功能位于 **设置 → 登录与身份认证** 和 **管理员 → 登录安全**。

### OIDC / SSO

填写 Issuer URL、Client ID、Client Secret、Scopes、用户名 Claim 和允许登录的身份。默认 Web Path 对应的回调地址为：

```text
https://panel.example.com/app/api/oidc-callback
```

修改 Web Path 后，回调路径也必须同步修改。用户名 Claim 默认使用 `preferred_username`，取不到时依次回退到 `email` 和 `sub`。

### TOTP / 两步验证

在 **管理员 → 登录安全** 中为管理员启用 TOTP。生成恢复码后请立即妥善保存，每个恢复码只能使用一次。

### WebAuthn 通行密钥

先在设置中全局启用通行密钥，再为管理员添加。通常可以留空 RP ID 和允许的 Origin，S-UI Next 会根据浏览器 Origin 以及可信的 `Forwarded`、`X-Forwarded-Host`、`X-Forwarded-Proto` 自动识别反向代理后的地址。

特殊代理结构可以手动设置。RP ID 只填写域名，例如 `panel.example.com`；Origin 需要填写完整来源，例如 `https://panel.example.com`。除 localhost 开发环境外，WebAuthn 需要 HTTPS。

## WireGuard 配置要点

- **本机 Endpoint 地址**代表当前 S-UI Next 实例，通常使用 `10.66.66.1/32`、`fd66:66:66::1/128` 这样的主机路由。
- **虚拟网段**仅用于自动分配受管客户端地址。连接已有 VPS 时可以不填写，但本机仍需配置自己的 WireGuard 地址。
- **运行时 AllowedIPs**决定哪些地址或目标网段由该 Peer 承载。作为中转发起端时，可以为固定对端明确选择全局路由；作为接收端时，可以监听端口并让对端地址动态更新。
- **客户端 AllowedIPs**决定哪些目标流量进入隧道。新 Peer 默认仅包含 WireGuard 虚拟网段；只有选择全局隧道时才会导出 `0.0.0.0/0` 和 `::/0`。
- **客户端配置导出**是可选功能。启用后，连接地址和端口必须指向真正接收 WireGuard UDP 的公网入口。
- **动态对端**适合接收来自 NAT 后设备或另一台 VPS 的连接；**固定对端**由当前服务器主动连接指定地址和端口。
- **站点网关**会把远端 LAN 加入服务端 Peer，并把配置的本地 LAN 导出给该网关。两侧仍需要正确的返回路由，或者另行配置 NAT。

## 环境变量

| 变量 | 可用值 | 默认值 |
| --- | --- | --- |
| `SUI_LOG_LEVEL` | `debug`、`info`、`warn`、`error` | `info` |
| `SUI_DEBUG` | 布尔值 | `false` |
| `SUI_BIN_FOLDER` | 目录 | `bin` |
| `SUI_DB_FOLDER` | 目录 | `db` |
| `SINGBOX_API` | sing-box API 地址 | 未设置 |

## 开发

```sh
git clone --recurse-submodules https://github.com/felixbennettio/s-ui-next.git
cd s-ui-next
```

- 后端使用 Go，准确版本见 `go.mod`。
- Web 前端使用 Vue 和 TypeScript，位于 [`frontend`](https://github.com/felixbennettio/s-ui-next-frontend) 子模块。
- Flutter 移动端源码位于 `mobile/`。
- 完整开发与贡献说明见 [`CONTRIBUTING.md`](CONTRIBUTING.md)。

## 来源与许可证

S-UI Next 基于 [alireza0/s-ui](https://github.com/alireza0/s-ui) 和 [SagerNet/sing-box](https://github.com/SagerNet/sing-box) 开发。Web 前端由 [felixbennettio/s-ui-next-frontend](https://github.com/felixbennettio/s-ui-next-frontend) 单独维护。

本项目使用 [GNU General Public License v3.0](LICENSE) 发布。
