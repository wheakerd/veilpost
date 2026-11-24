# Soft Router Docker 参考配置

[English](README.md) | 简体中文

这里收录 36 个 VPN 部署配置及容器配套文件，仅作参考。文件取自一套私有部署方案，已去除私人域名、私人主机路径、原项目路径和主机商标识；其余结构、镜像、端口和语义保持不变。每个文件的大小和 SHA-256 记录在 `source-manifest.json`。

| 子目录                       | 内容                                                                               |
| ---------------------------- | ---------------------------------------------------------------------------------- |
| `wire-guard/`                | 标准 Docker 的 AWG / WebSocket Compose、初始化覆盖配置、Caddy、环境示例及 nft 入口 |
| `wg-easy/`                   | 独立 wg-easy 的 Compose、Caddy、daemon、systemd 与 nft 入口                        |
| `amneziawg-3.1/`             | Compose / SELinux 覆盖、环境示例、Dockerfile、rootfs 入口脚本、Docker systemd 单元 |
| `awg31-reality-stack/`       | AWG / Xray Compose、daemon、配置模板、sysctl 与 systemd                            |
| `chain-reuse-v1/`            | 中继候选及生产覆盖 Compose                                                         |
| `reality-chain/`             | 历史 REALITY 出口 Compose                                                          |
| `xray-reality-shadowrocket/` | Xray Compose 与客户端 / 服务端模板                                                 |

## 占位符

复用前必须把占位符替换为目标环境的值。

| 占位符                                                                                                                                                            | 位置                                                   | 含义                                                                                                                                                         |
| ----------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `panel.example.com`                                                                                                                                               | `wg-easy/Caddyfile`                                    | wg-easy 管理面板的公开域名；Caddy 用它申请 TLS 证书                                                                                                          |
| `PANEL_DOMAIN`、`TUNNEL_DOMAIN`                                                                                                                                   | `wire-guard/.env.example`、`compose.yaml`、`Caddyfile` | 管理面板与 wstunnel 的公开域名，在 `.env` 中设置；Compose 以 `${PANEL_DOMAIN:?…}` 传入，Caddy 以 `{$PANEL_DOMAIN}` 读取；示例值 `*.example.invalid` 不可解析 |
| `${VAR:?…}`、`${VAR:-默认值}`                                                                                                                                     | 各 Compose 文件                                        | Compose 变量插值：`:?` 表示缺失时拒绝启动，`:-` 表示未设置时使用默认值                                                                                       |
| `AWG_ENDPOINT_HOST=CHANGE_ME`                                                                                                                                     | `amneziawg-3.1/config/deployment.env.example`          | 客户端连接的服务器公网地址或域名                                                                                                                             |
| `__SERVER_ADDRESS__`、`__SERVER_PORT__`、`__CLIENT_UUID__`、`__REALITY_PRIVATE_KEY__`、`__REALITY_PUBLIC_KEY__`、`__SHORT_ID__`、`__SOCKS_PORT__`、`__DNS_PORT__` | Xray 配置模板                                          | 部署流程渲染模板时写入的服务器地址、端口、客户端 UUID、REALITY 密钥对、shortId 及本地测试端口；仓库中不含真实值                                              |

以下约定值不是私人信息，按原样保留，复用时可按需调整：

- 上游镜像及摘要（`ghcr.io/wg-easy/wg-easy`、`ghcr.io/xtls/xray-core`、`caddy`、`golang`、`alpine` 等）与上游源码地址。
- `/var/lib/<组件>`、`/run/<组件>` 等容器数据和运行目录约定；`/opt/wg-easy/`、`/opt/soft-router/awg31-reality-stack/` 是独立 Docker 运行时的安装目录约定，systemd 单元和 `daemon.json` 引用它们。
- `soft-router` 前缀的本地镜像名、Compose 标签和 nft 规则注释，用于标识这些参考配置创建的资源。
- 私有网段（`10.42.42.0/24`、`10.79.31.0/24` 等）及 IPv6 ULA 前缀；正式部署建议按 RFC 4193 自行生成 ULA 前缀。
- REALITY 伪装目标 `dl.google.com` 与公共 DNS `1.1.1.1`。

## 边界

- 仅包含配置及其直接使用的容器脚本；不含 SSH 凭据、真实 `.env`、VPN 私钥、运行时状态、数据库、日志、离线镜像和二进制工具。
- 第三方依赖缓存和上游源码中的 Docker 示例不属于本项目部署配置，未收录。
- 本项目的 Compose 开发环境不会启动或修改这些配置；不能据此推断任何 VPN 服务当前是否在线。不要直接将这些 Compose 合并运行。

## 运行前仍需准备的材料

- `wire-guard`：Compose 提及的 `manage.py` 是原部署工具，未收录。私有 bootstrap 环境文件、wstunnel restrictions 与持久化目录需要另行准备。
- `wg-easy`：替换 Caddy 域名，并按目标主机调整 `/var/lib/wg-easy` 与 `/opt/wg-easy` 目录。
- `amneziawg-3.1`：Dockerfile 需要的两个 `artifacts/sources/*.tar.gz` 源码归档未收录，因此不能独立完成离线镜像构建；rootfs 脚本已保留。可改用 `docker/node/amneziawg/`，它从上游固定提交构建，不需要这两个归档。
- `awg31-reality-stack`：需要 AWG / Xray 镜像标识、私有 `service.env`、服务端运行配置和独立 Docker runtime。
- Xray / REALITY 相关 Compose：挂载的 `/var/lib/...` 服务端目录与真实密钥不在仓库内，配置模板需由部署流程填充。
- systemd 文件是文本参考，未写入系统服务目录。

## 校验

在项目根目录执行 `python3 scripts/verify-docker-copy.py`，确认清单中的每个文件存在、大小与 SHA-256 一致，且目录中没有未登记的文件（本说明、其英文版和清单本身除外）。

有意修改参考文件后，执行 `python3 scripts/verify-docker-copy.py --update` 重新生成清单，并在提交前检查清单差异。
