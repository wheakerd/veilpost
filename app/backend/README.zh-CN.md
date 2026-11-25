# Veilpost 后端

[English](README.md) | 简体中文

Go 1.27.4 + Gin 1.12.0，使用纯 Go SQLite 驱动，支持无 CGO 构建。认证接口、节点注册与控制通道（节点设计第 1
阶段）、软件商店（内置目录与安装状态），以及证书与 DNS 服务商凭据（ACME / DNS-01）已经实现，不包含默认账号、Mock 或绕过登录的开发接口。

## 启动与创建账号

本地开发时，在仓库根目录执行 `make start`，通过 Compose 启动前后端。在本地终端执行 `make admin-create`
，输入账号和密码；非交互环境自动生成随机密码，账号名默认 `admin`。也可显式执行
`make admin-create GENERATE_PASSWORD=1 USERNAME=admin`。账号为 3–64 位 ASCII 字母、数字、点、下划线或连字符，密码至少 12
个字符、最多 256 个 UTF-8 字节。

管理员初始化使用 `scripts/admin-create.sh`，不依赖 Python。自动密码使用系统 `openssl rand -hex 32` 生成（64 位十六进制字符、256
位随机熵）。缺失 OpenSSL/coreutils 时识别 Linux 发行版，通过 apt-get、dnf/yum、apk、pacman 或 zypper 安装后继续；需要 root 或可用的
sudo。密码通过标准输入交给后端，保存到 Git 忽略的 `.runtime/admin-credentials/` 文本文件，目录权限 `700`、文件权限 `600`
；终端只输出文件位置。请在保存到密码管理器后删除明文文件。每次生成均使用独立文件，不覆盖已有凭据；创建失败或结果不明时保留文件，不自动重试或重置已有账号。

原生部署使用 `VPN_API_BIN=/path/veilpost-api VPN_DATA_DIR=/path/data GENERATE_PASSWORD=1 sh scripts/admin-create.sh`，
`VPN_DATA_DIR` 必须与运行中的服务一致。可用 `VPN_CREDENTIALS_DIR` 指定密码文件目录。生产发布包附带独立的 `admin-create.sh`
，可直接运行。

在 Linux 上，在 `app/backend/` 中运行 `make dev-start` 可构建后端并在后台启动。`make dev-status` 显示实际回环端口，`make dev-logs` 查看最近的日志，`make dev-restart` 和 `make dev-stop` 分别重启和停止后端。Go 启动器与仓库根目录的 `make native-*` 共用进程记录和 `.runtime/backend-data` 数据目录。前端开发启动器会自动识别运行中的后端。

如需在前台运行后端：

```bash
VPN_ENV=development VPN_DATA_DIR=../../.runtime/backend-data make run
```

原生创建账号时，通过标准输入把密码交给 `bin/veilpost-api admin-create --username <账号> --password-stdin`
；不要把密码写进命令行、环境变量或仓库文件。该命令仅能由有权访问数据目录的本机用户执行，不存在公开注册接口。

本地恢复命令同样只接受标准输入密码，并撤销该账号的全部会话：

- `veilpost-api admin-reset-password --username <账号> --password-stdin`：替换忘记的密码。
- `veilpost-api admin-reset-two-factor --username <账号>`：移除丢失的验证器和全部恢复码；账号未启用两步验证时报错。

根目录对应 `make admin-reset-password` 和 `make admin-reset-two-factor`（可加 `USERNAME=`、`GENERATE_PASSWORD=1`），内部调用
`scripts/admin-create.sh` 的 `ADMIN_ACTION=reset-password` / `reset-two-factor`，凭据处理与创建账号相同。

## 持久化与安全边界

- `auth.sqlite` 保存用户、Argon2id 密码哈希、会话哈希、待绑定信息、恢复码哈希和限流计数，以及节点（名称、状态、Ed25519
  公钥、代理程序版本、最后通信时间）、注册令牌的 SHA-256 哈希和代理程序请求随机数，以及已安装软件（`installed_apps`：目录版本、配置
  JSON、安装与修改时间）。节点私钥只保存在节点上；未安装的软件不保存任何配置。证书部分保存 DNS 服务商凭据、ACME 账户和证书（见下文）。
- 数据库版本记录在 `PRAGMA user_version`（当前为 5，v3 为软件商店，v4 为证书与 DNS 服务商，v5 为节点 Docker 安装）。启动时在写锁事务内按顺序执行未应用的迁移，遇到更新版本的数据库拒绝启动。
- `master.key` 是首次初始化时随机生成的 32 字节密钥，权限为 0600。TOTP 密钥和软件的敏感配置项使用 AES-GCM 加密保存；其他包通过
  `auth` 的 `Seal` / `Unseal` 使用该密钥，附加数据绑定用途（软件配置为 `apps:<id>:<配置项>`
  ），密文不能挪到其他记录或字段。数据库存在而密钥缺失时拒绝启动，不能自动重置密钥。
- Cookie 会话令牌为随机 256 位值，数据库只存 SHA-256；完整会话有效期八小时，待验证与绑定流程五分钟。
- 密码登录和完成验证都会轮换令牌；启用 / 关闭两步验证和修改密码会撤销其他会话。用户也可在账号安全页退出其他会话。
- TOTP 采用 SHA-1、六位、30 秒，允许相邻一个时间窗口，同一时间步不能重复使用；恢复码在事务内一次性消费，只在启用成功时返回明文。
- 登录按客户端地址、账号与地址组合、账号三级限流；动态码和恢复码验证按挑战和账号限流；安全设置按会话限流。计数持久化，进程重启不会清零。客户端地址只采信
  `VPN_TRUSTED_PROXIES` 所列代理转发的 `X-Forwarded-For`，未配置时使用 TCP 对端地址；部署在反向代理后面时必须配置，否则所有请求共享代理地址的额度。
- 节点注册令牌随机 256 位、15 分钟有效、只能使用一次，只在创建响应中返回明文。代理程序请求使用 Ed25519 签名（节点
  ID、时间戳、随机数、方法、路径和请求体哈希），时间偏差超过 60 秒拒绝，随机数在有效期内去重；吊销后签名立即失效。`/api/agent/*`
  按节点限流（每 5 分钟 120 次），注册按客户端地址限流（每 5 分钟 30 次），与登录额度互不影响。
- 软件目录用 `embed` 打包在二进制中（`internal/apps/catalog/*.json`），启动时严格校验，清单无效则拒绝启动；不支持远程软件源。已发布镜像按多架构索引摘要固定，AmneziaWG
  指向本仓库 `docker/node/amneziawg` 的本地构建。
- `/api/apps/*` 沿用会话认证和写请求规则；配置按清单严格校验（未知字段、越界值拒绝），敏感项只写不读，任何接口都不返回明文。卸载需要重新输入当前密码，与账号安全操作共用按会话的限流，并删除全部配置；已部署到节点时返回
  `409 APP_DEPLOYED`（节点第 2 阶段之前不会出现）。
- `/api/nodes/*` 沿用会话认证和写请求规则；创建、吊销、为未注册节点重新签发令牌以及删除已吊销节点都需要重新输入当前密码，与账号安全操作共用按会话的限流。
- 过期会话、待绑定状态、限流计数、注册令牌和代理程序随机数在启动时及之后每小时清理一次。
- `dns_credentials` 的全部字段作为一个 JSON 整体、`acme_accounts` 的账户私钥与临时 EAB、`certificates` 的证书私钥都用
  `master.key` 以 AES-256-GCM 加密（`auth.Service.Seal`），附加数据绑定用途与记录
  ID，密文不能挪用到其他记录。接口只返回凭据各字段“是否已设置”和修改时间，从不返回凭据、账户私钥或证书私钥；证书链（公开信息）以明文保存，可通过接口下载。EAB
  在 ACME 账户注册成功后删除。
- `/api/dns-credentials/*` 与 `/api/certificates/*` 沿用会话认证和写请求规则；新增、修改、删除凭据需要重新输入当前密码，被证书引用的凭据不能删除。验证凭据按会话限流（5
  分钟 10 次），申请与重新签发证书按会话限流（10 分钟 20 次）。
- 证书签发与续期由 `internal/certs` 的独立调度器在后台执行（最多 2
  个并发），不与每小时的认证清理循环合并：它需要可取消的任务、在申请后立即排队，并在关闭时等待任务退出。启动时与之后每小时检查到期与续期：到期前
  30 天续期（有效期不足 90 天的证书在剩余三分之一时），失败按 1、2、4、8、16、24 小时退避；首次签发失败不自动重试。优雅关闭或删除证书会取消进行中的任务（ACME
  请求绑定任务上下文），中断的任务不记录错误，下次启动时继续，关闭最多等待 `VPN_SHUTDOWN_TIMEOUT`。
- 签发错误只保存错误类别、时间，以及 ACME 问题文档的类型与简短说明（最多 300 字符）；DNS 服务商返回的错误文本从不保存或记录。lego
  的日志被丢弃，后台任务的日志只包含错误类别，不含域名、凭据或令牌。
- 浏览器写请求要求 JSON、同源 Origin 和 `X-Requested-With: Veilpost`；不启用凭据 CORS。代理程序接口不使用 Cookie，以签名认证。所有
  API 响应 `Cache-Control: no-store`。
- Cookie 始终使用 HttpOnly、SameSite=Strict、Path=/；生产模式额外强制 Secure，并要求 HTTPS Origin。
- 日志只记录路由模板、方法、状态和耗时，不记录密码、绑定密钥、恢复码、注册令牌、节点 ID、软件标识、配置值、公钥、签名、Cookie、请求体或查询参数。
- 代理程序长轮询最长保持 25 秒，该路由单独延长写超时；反向代理的读取超时需大于 25 秒。优雅关闭时等待中的长轮询会立即返回。

### 外部网络访问

认证与节点功能不主动访问外部网络。证书功能会由后端发起出站 HTTPS 与 DNS 请求：ACME 目录（默认
`acme-v02.api.letsencrypt.org`，测试环境 `acme-staging-v02.api.letsencrypt.org`，或管理员填写的自定义 `https://`
目录，后端会访问该地址，部署时可在出口防火墙限制）；所选 DNS 服务商的 API（`api.cloudflare.com`、阿里云
`alidns.cn-hangzhou.aliyuncs.com`、`dnspod.tencentcloudapi.com`、`route53.amazonaws.com`）；以及系统解析器（
`/etc/resolv.conf`）和各域名的权威 DNS 服务器，用于检查验证记录是否生效（等待最长 5 分钟）。ACME 请求遵循 `HTTPS_PROXY`
等代理环境变量。服务商客户端只使用界面保存的凭据，不读取主机上的 AWS、阿里云或腾讯云环境变量、配置文件或实例角色。没有证书和凭据时不会发起任何出站请求。

当前按单实例控制台设计，SQLite 通过 WAL 和事务提供一致性；不要让多个副本共享同一数据文件横向扩容。备份必须同时保存数据库、WAL（若存在）和
`master.key`；推荐停止后端后备份整个数据目录或命名卷。完整卷与密钥同时泄漏不在数据库加密的保护范围内。

## 配置

程序读取进程环境变量，`.env.example` 不会自动加载。

| 变量                   | 默认值           | 含义                                           |
| ---------------------- | ---------------- | ---------------------------------------------- |
| `VPN_ENV`              | `production`     | `development` 才允许本机 HTTP Cookie           |
| `VPN_DATA_DIR`         | `data`           | SQLite 与密钥目录；Compose 使用 `/data` 命名卷 |
| `VPN_HTTP_ADDR`        | `127.0.0.1:8080` | 监听地址，端口 0 由系统分配                    |
| `VPN_PUBLIC_ORIGIN`    | 空               | 可设置精确外部 Origin；生产必须是 HTTPS        |
| `VPN_TRUSTED_PROXIES`  | 空               | 逗号分隔的反向代理 IP 或 CIDR，用于识别客户端  |
| `VPN_LOG_LEVEL`        | `info`           | slog 级别                                      |
| `VPN_SHUTDOWN_TIMEOUT` | `10s`            | 优雅关闭期限                                   |

原生开发启动器使用 `--listen-file /绝对路径/listen.json` 获取真实 IP、端口及 PID；Compose 使用服务名与固定内部端口，宿主机映射端口仍动态分配。

## 接口与验证

认证契约见 [auth-api.md](../frontend/docs/auth-api.zh-CN.md)
，节点与代理程序接口见 [nodes-api.md](../frontend/docs/nodes-api.zh-CN.md)
，软件商店接口见 [apps-api.md](../frontend/docs/apps-api.zh-CN.md)，证书与 DNS
服务商接口见 [certs-api.md](../frontend/docs/certs-api.zh-CN.md)。`GET /api/health` 是存活检查，`GET /api/ready`
检查认证存储连接；健康检查不代表已创建管理员或已有 VPN 节点。`veilpost-api healthcheck` 按 `VPN_HTTP_ADDR` 的端口访问
`/api/ready`，监听全部地址时使用 `127.0.0.1`。

```bash
make check  # 格式、go vet、真实 SQLite/HTTP/TOTP 测试及竞态检测、构建
make build
./bin/veilpost-api --version
```

测试不与源码放在一起，位于 `tests/<包名>/`，以外部包 `<包名>_test` 的形式只调用公开接口；测试需要的时钟、协作对象注入点与纯函数在各包的 `export.go` 中显式导出。测试覆盖会话持久化、过期与撤销、真实 TOTP、防重放、并发恢复码消费、密码错误、限流、Cookie 与 Origin
校验、日志边界和优雅关闭；节点部分覆盖签名已知向量、重放、时钟偏差、篡改、吊销与长轮询唤醒、注册令牌一次性核销、过期与重新签发、已吊销节点删除、按节点限流、清理和数据库迁移；软件商店部分覆盖目录清单加载与校验、迁移
v2→v3、安装 / 卸载 /
配置接口与权限、配置校验、敏感项加密且不回显，以及日志边界；证书部分覆盖凭据加密与不回显、密文与记录绑定、密码确认、删除限制、域名 /
ACME 目录 / 邮箱 / EAB 校验、退避与续期时间，并用进程内的 Let's Encrypt Pebble 测试 CA 与进程内 DNS
服务器完成真实的签发、续期（新私钥、复用账户）、EAB 注册、错误分类和后台任务取消，不访问公网，不调用真实服务商。测试数据库使用临时目录，不接触开发数据卷。

ACME 客户端为 `github.com/go-acme/lego/v4`，只导入 Cloudflare、阿里云 DNS、腾讯云 DNSPod 与 AWS Route 53 四个 DNS
子包；Pebble（`github.com/letsencrypt/pebble/v2`）只用于测试，不进入构建产物。Go 模块路径为
`github.com/wheakerd/veilpost/app/backend`；版本与依赖由 `.go-version`、`go.mod`、`go.sum` 管理。Makefile
优先使用仓库 `.runtime/toolchains/` 中的 Go，也支持 `GO=/path/to/go`。构建和模块缓存位于仓库 `.runtime/`，不进入 Git。
