# AmneziaWG 节点镜像

[English](README.md) | 简体中文

从上游固定提交的源码构建 AmneziaWG 服务端镜像：用户态守护进程 `amneziawg-go`（Go）和管理工具 `awg`、`awg-quick`（C，来自 `amneziawg-tools`）。容器在首次启动时于节点本地随机生成服务端密钥、首个客户端配置和混淆参数；镜像和仓库中不含任何密钥或客户端配置。

本目录只是节点侧镜像，不属于开发用 Compose 环境，`make start` 不会构建或启动它。`docker/soft-router/amneziawg-3.1/` 是它的参考来源（缺少源码归档，无法独立构建），容器脚本由那里复制后修改。

## 上游版本

| 组件              | 版本            | 提交                                       | 归档 SHA-256                                                       |
| ----------------- | --------------- | ------------------------------------------ | ------------------------------------------------------------------ |
| `amneziawg-go`    | `v3.1.20261116` | `08fe3d33bbb116be09a6183d02609311b8b0c9e8` | `e33d8a62928222592df23b75356d79e0ea6b590943cbe3a4cfcdf9c357bb416e` |
| `amneziawg-tools` | `v3.1.20260812` | `ee0f0a9aa34ff0a0da4b3433b9512781cfe02843` | `22438f231d39ea27e4bdc69707ec3103357f3cecc9ad3eecba18a61e5b39c9b9` |

- 源码取自 `https://github.com/amnezia-vpn/<仓库>/archive/<提交>.tar.gz`，由 `ADD --checksum` 在使用前校验；哈希不符时构建失败。
- 基础镜像（`golang:1.27.4-alpine3.24`、`alpine:3.24.4`）与 Dockerfile 前端（`docker/dockerfile:1.29.0`）均按摘要固定。
- 构建时运行上游全部 Go 测试；测试需要 cgo（上游 `outline` 包的依赖），守护进程本身以 `CGO_ENABLED=0` 静态编译。`awg` 以 `-static` 链接。
- 版本信息写入镜像标签（`amneziawg-go.version`、`amneziawg-go.revision`、`amneziawg-tools.version`、`amneziawg-tools.revision`）和容器内 `/usr/local/share/amneziawg/versions`，初始化时复制到状态目录的 `meta/versions`。

升级上游时，Dockerfile 顶部的版本、提交和 SHA-256 三项必须一起修改。计算新哈希：

```sh
curl -fsSL https://github.com/amnezia-vpn/amneziawg-go/archive/<提交>.tar.gz | sha256sum
```

升级前确认 `amneziawg-tools` 与 `amneziawg-go` 的 UAPI 键一致，并重新运行下文的检查。

## 构建

在本目录执行：

```sh
docker build -t veilpost-amneziawg:dev .
```

需要 BuildKit（Docker 23 及以上默认启用）和访问 GitHub、Go 模块代理与 Docker Hub 的网络。可选构建参数：`IMAGE_VERSION`、`IMAGE_REVISION`、`IMAGE_SOURCE`、`BUILD_DATE`，写入对应的 `org.opencontainers.image.*` 标签。

不需要任何权限的冒烟检查：

```sh
docker run --rm --network none --entrypoint awg veilpost-amneziawg:dev --version
docker run --rm --network none --entrypoint amneziawg-go veilpost-amneziawg:dev --version
```

## 运行要求

- 内核：任意支持 TUN 的 Linux 内核，不需要 AmneziaWG 内核模块（如果主机已加载 `amneziawg` 模块，`awg-quick` 会优先使用它）。需要 `/dev/net/tun`。
- 网络：`network_mode: host`。容器在主机网络命名空间中创建接口、开放 UDP 监听端口，并为隧道子网安装转发和 NAT 规则。
- 容器能力：`NET_ADMIN`（创建接口、路由、iptables/nftables）、`NET_RAW`（iptables）、`NET_BIND_SERVICE`（监听 1024 以下端口，例如 443）。其余能力全部丢弃；不需要 `--privileged`。
- 主机 sysctl：容器不修改主机 sysctl，启动前由主机设置：
  - `net.ipv4.ip_forward=1`；
  - 启用 IPv6（`AWG_REQUIRE_IPV6=true`）时还需 `net.ipv6.conf.all.forwarding=1`；如果 IPv6 默认路由来自路由通告，出口接口需 `accept_ra=2`。
- 防火墙：脚本插入 `AWG_IN`、`AWG_FWD`、`AWG_NAT`（IPv6 为 `AWG_IN6`、`AWG_FWD6`、`AWG_NAT6`）链，容器停止时删除。镜像中的 iptables 使用 nf_tables 后端；主机仍在使用 iptables-legacy 时规则会落在另一套表里，需要自行确认。主机存在 nftables 原生 `input`/`forward` 链（例如自定义的 `inet filter` 表）时，`AWG_NATIVE_NFT_MODE=auto` 会在链首插入带 `amneziawg-node-*` 注释的放行规则，并在停止时按注释删除，避免这些链的丢弃策略挡住隧道流量。
- 根文件系统只读；`/run` 和 `/tmp` 使用 tmpfs。容器以 root 运行（创建接口需要）。

## 状态目录

容器内 `/var/lib/amneziawg`，示例中绑定到主机上的 `AWG_STATE_DIR`。该目录需要预先创建（例如 `install -d -m 0700 -o root -g root <目录>`），并纳入备份。目录内容：

| 路径                      | 内容                                                       |
| ------------------------- | ---------------------------------------------------------- |
| `server/<接口>.conf`      | 服务端配置，含服务端私钥、预共享密钥和全部 `[Peer]`        |
| `clients/<名称>.conf`     | 客户端配置，含客户端私钥；可直接导入 AmneziaVPN 等客户端   |
| `meta/`                   | 服务端公钥、接口名、首个客户端名、对外地址、版本、初始化时间 |
| `rollbacks/`              | `awg-add-client` 每次修改前保存的服务端配置                |

- 所有文件权限 `0600`，目录 `0700`。这些文件都是凭据，不要提交到仓库或发送到日志。
- 状态只在首次启动时生成，之后每次启动只校验：接口名、首个客户端名或对外地址与已有状态不一致，或者目录残缺时，容器拒绝启动，不会重新生成密钥。需要换地址时先备份并清空状态目录，再重新分发客户端配置。
- 镜像不含恢复主机 sysctl 的脚本（参考配置中的 `awg-restore-sysctls` 依赖原部署工具，未复制）。

## 环境变量

| 变量                                | 默认值                          | 说明                                                                                          |
| ----------------------------------- | ------------------------------- | --------------------------------------------------------------------------------------------- |
| `AWG_ENDPOINT_HOST`                 | 无（必填）                      | 客户端连接的公网域名或 IP，写入客户端配置的 `Endpoint`                                        |
| `AWG_LISTEN_PORT`                   | `443`                           | UDP 监听端口                                                                                  |
| `AWG_INTERFACE`                     | `awg0`                          | 接口名（最多 15 个字符）                                                                      |
| `AWG_MTU`                           | `1280`                          | 隧道 MTU，1280–1420                                                                           |
| `AWG_SERVER_IPV4`、`AWG_SERVER_IPV6` | `10.77.0.1/24`、`fd77:77:77::1/64` | 服务端隧道地址                                                                               |
| `AWG_CLIENT_IPV4`、`AWG_CLIENT_IPV6` | `10.77.0.2/32`、`fd77:77:77::2/128` | 首个客户端地址                                                                              |
| `AWG_IPV4_SUBNET`、`AWG_IPV6_SUBNET` | `10.77.0.0/24`、`fd77:77:77::/64` | 转发与 NAT 规则匹配的隧道子网，应与服务端地址一致                                            |
| `AWG_CLIENT_NAME`                   | `client1`                       | 首个客户端配置的文件名                                                                        |
| `AWG_CLIENT_DNS`                    | `1.1.1.1, 2606:4700:4700::1111` | 写入客户端配置的 DNS，只接受 IP                                                               |
| `AWG_CLIENT_I1` … `AWG_CLIENT_I5`   | 空                              | 客户端握手前签名包，见下文                                                                    |
| `AWG_REQUIRE_IPV6`                  | `false`                         | `true` 时要求主机有 IPv6 默认路由和全局地址，并安装 IPv6 转发与 NAT 规则                      |
| `AWG_NATIVE_NFT_MODE`               | `auto`                          | `auto`：主机有对应 nftables 链时插入放行规则；`required`：链必须存在；`off`：不处理            |
| `AWG_NATIVE_NFT_FAMILY`、`_TABLE`、`_INPUT_CHAIN`、`_FORWARD_CHAIN` | `inet`、`filter`、`input`、`forward` | 主机 nftables 原生链的位置                                                  |
| `LOG_LEVEL`                         | 未设置                          | `amneziawg-go` 日志级别：`error`、`verbose`（或 `debug`）、`silent`；示例 Compose 设为 `error` |

ULA 前缀 `fd77:77:77::/64` 只是示例；正式部署建议按 RFC 4193 随机生成自己的前缀。

## 混淆参数

所有取值在首次初始化时由 `/dev/urandom` 按节点随机生成，不同节点不共用同一组特征值。参数含义以上游 `amneziawg-go` README 为准。

| 参数                                 | 生成方式                                                                                                                         | 位置               |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------- | ------------------ |
| `S1`、`S2`、`S3`、`S4`               | 分别取 48–96、96–144、48–96、12–32；保证 `S1 + 56 ≠ S2`，使握手发起与响应包长不同；头部保护要求每项至少 12                       | 服务端与客户端一致 |
| `H1`–`H4`                            | 把 5–4294967295 分成四段并随机打乱分配，每项在所属段内取一个宽度 2^16–2^24 的随机区间，四个区间互不重叠，也不与标准 WireGuard 类型值 1–4 重叠 | 服务端与客户端一致 |
| `HeaderProtectionKey`                | `awg genkey`                                                                                                                     | 服务端与客户端一致 |
| `RandomTrailers`、`DisableCookies`   | 固定 `on`、`off`；健康检查会核对                                                                                                 | 服务端与客户端一致 |
| `Jc`、`Jmin`、`Jmax`                 | 每个客户端各自随机：4–8、32–64、256–512                                                                                          | 仅客户端           |
| `I1`–`I5`                            | 取自 `AWG_CLIENT_I1`…`I5`，原样写入新生成的客户端配置；留空则不发送                                                              | 仅客户端           |

`I1`–`I5` 只接受上游文档列出的标签：`<b 0x十六进制>`（偶数位）、`<r N>`、`<rd N>`、`<rc N>`、`<t>`，标签之间不能有其他字符，总长不超过 4096。例如模拟某种协议首包时可以写 `<b 0x…固定头…><r 32><t>`；具体模板（QUIC、DNS 等外观）不随镜像提供，由使用者或控制台按上游文档生成。生成后的包长不要超过路径 MTU，否则会被分片。

这些变量只影响之后新生成的客户端配置；已有客户端配置不会被改写。

## 添加客户端

容器运行且健康时，在容器内执行 `awg-add-client`：

```sh
docker exec \
  -e AWG_NEW_CLIENT_NAME=phone \
  -e AWG_NEW_CLIENT_IPV4=10.77.0.3/32 \
  -e AWG_NEW_CLIENT_IPV6=fd77:77:77::3/128 \
  amneziawg awg-add-client
```

- `AWG_NEW_CLIENT_NAME` 最多 15 个字符；地址必须是未被占用的 `/32` 和 `/128`。`AWG_NEW_CLIENT_DNS` 默认 `1.1.1.1`，设为空字符串则不写 DNS。`AWG_CLIENT_I1`…`I5` 同样生效。
- 脚本先用 `awg syncconf` 重放当前配置以验证回滚路径，再应用新增的 `[Peer]`；失败时恢复原配置。客户端配置写入 `clients/<名称>.conf`。

导出客户端配置时直接从状态目录复制文件，不要用 `docker logs` 或命令行参数传递内容。

## 示例 Compose

`compose.example.yaml` 是示例，复制到节点后按需修改，在同目录的 `.env` 中至少设置 `AWG_STATE_DIR` 和 `AWG_ENDPOINT_HOST`：

```sh
docker compose -f compose.example.yaml up -d --build
docker compose -f compose.example.yaml logs -f
```

SELinux 主机可能需要在 `security_opt` 中加入 `label=disable`，或为状态目录设置合适的标签。

## 边界

- 本镜像目前独立运行：服务端密钥和首个客户端在容器内生成。`docs/nodes-design.md` 中由控制台生成客户端私钥、`veilpost-agent` 下发配置的接入方式尚未实现，届时需要为脚本增加“由外部提供服务端配置”的启动方式。
- 本目录的检查只覆盖镜像构建、二进制版本和离线的状态生成；没有在真实节点上启动隧道或连接客户端，不能据此推断任何节点可用。
