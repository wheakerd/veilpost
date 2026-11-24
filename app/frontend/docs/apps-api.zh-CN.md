# 软件商店接口约定

[English](apps-api.md) | 简体中文

对应 [平台设计](../../../docs/platform-design.zh-CN.md)“二、软件商店”。在软件商店中“安装”一个 VPN
软件，只是在控制台中启用它：侧栏出现它的配置与管理页面，配置保存在控制台数据库中。部署到节点依赖[节点设计](../../../docs/nodes-design.zh-CN.md)
第 2 阶段，尚未实现。

接口由 Go 后端 `app/backend/internal/apps` 实现，客户端为 `src/api/apps.ts`。

## 内置目录

- 目录用 Go `embed` 打包在后端二进制中（`internal/apps/catalog/<id>.json`
  ），随控制台版本发布；不支持远程软件源或上传自定义包。后端启动时严格校验全部清单（未知字段、无效默认值、未固定摘要的镜像等都会让启动失败）。
- 清单中的名称、描述、配置项标签、说明和枚举选项都是语言包键，必须位于 `apps.catalog.<id>.` 之下；后端不返回界面文案。
  `tests/dev/locales.test.ts` 检查清单引用的每个键都存在于语言包中。
- 第一版目录：

| id          | 版本           | 协议（传输）                                                    | 镜像                                                                                                         |
| ----------- | -------------- | --------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| `xray`      | `26.11.9`      | VLESS + REALITY + Vision（TCP）、VLESS + XHTTP + REALITY（TCP） | `ghcr.io/xtls/xray-core:26.11.9@sha256:9812e382e229e9ed63fd179bf1a4e3fe244c12ff3ae4fd8128745d79c87575ab`     |
| `hysteria2` | `2.14.0`       | Hysteria 2（UDP）                                               | `docker.io/tobyxdd/hysteria:v2.14.0@sha256:b15cedc3de200ae7a4f177908a83ef23ed3de2935458b22b7d46a7a8697038a9` |
| `amneziawg` | `3.1.20261116` | AmneziaWG 2.0（UDP）                                            | 由本仓库 `docker/node/amneziawg` 构建，尚无发布地址（`image.build`，没有 `reference` 和 `digest`）           |

已发布镜像按多架构索引（OCI image index）摘要固定；摘要通过镜像仓库的 `Docker-Content-Digest` 响应头与清单内容的 SHA-256
双重核对（2026-11-23）。Xray 镜像由上游仓库 `XTLS/Xray-core` 的发布工作流推送到 `ghcr.io/xtls/xray-core`；Hysteria 2 的官方镜像由
`apernet/hysteria` 的发布工作流推送到 Docker Hub 的 `tobyxdd/hysteria`。升级版本时，版本、标签和摘要必须一起修改。

## 传输规则

沿用 [认证契约](auth-api.zh-CN.md) 的传输规则：同源、`credentials: same-origin`、`X-Requested-With: Veilpost`，写请求只接受 JSON
并校验 `Origin`，禁止跟随重定向，10 秒超时，不自动重试写操作；所有响应 `Cache-Control: no-store`。所有接口都要求完成两步验证的完整会话，否则返回
`401 SESSION_EXPIRED`。写请求全部使用 `POST`，与认证和节点接口一致。

## 对象

```ts
interface StoreApp {
    id: string // ^[a-z][a-z0-9-]{1,31}$
    version: string // 目录中的版本
    name: string // 语言包键
    description: string // 语言包键
    protocols: { id: string; name: string; transport: 'tcp' | 'udp' }[] // name 为专有名词，不翻译
    ports: {
        setting: string // 对应的端口配置项
        protocol: string
        transport: 'tcp' | 'udp'
        port: number | null // 已安装：当前配置；未安装：默认值；null 表示部署时按节点随机
    }[]
    settings: AppSetting[]
    image: {
        reference: string // repository:tag@digest；本地构建时为空字符串
        repository?: string
        tag?: string
        digest?: string // sha256:<64 位十六进制>，多架构索引摘要
        platforms?: string[] // 如 linux/amd64
        build?: string // 本仓库中的构建目录，例如 docker/node/amneziawg
        source: string // 上游源码仓库
    }
    installed: null | {
        version: string // 安装时的目录版本
        installedAt: string
        updatedAt: string
        configured: boolean // 所有必填项都有值
    }
}

interface AppSetting {
    key: string // ^[a-z][A-Za-z0-9]{0,31}$
    type:
        | 'port'
        | 'integer'
        | 'string'
        | 'enum'
        | 'boolean'
        | 'hostname'
        | 'hostport'
        | 'url'
    label: string // 语言包键
    hint?: string // 语言包键
    required?: true
    requiredWhen?: { setting: string; equals: string } // 另一枚举项取该值时必填
    default?: number | string | boolean
    min?: number // integer
    max?: number // integer
    minLength?: number // string
    maxLength?: number // string
    pattern?: string // string，锚定的正则
    options?: { value: string; label: string }[] // enum
    sensitive?: true // 只写不读，只能用于没有默认值的 string
}
```

配置值校验（服务器端为准）：

| 类型       | 接受的值                                                                    |
| ---------- | --------------------------------------------------------------------------- |
| `port`     | 1–65535 的整数                                                              |
| `integer`  | `min`–`max` 的整数                                                          |
| `string`   | 字符数在 `minLength`–`maxLength` 之间、无控制字符，且匹配 `pattern`（若有） |
| `enum`     | `options` 中的某个 `value`                                                  |
| `boolean`  | `true` / `false`                                                            |
| `hostname` | 至少两段的 DNS 名称，转为小写；不接受 IP 地址                               |
| `hostport` | `hostname` 或 `hostname:端口`，保存为 `hostname:端口`（省略端口时为 443）   |
| `url`      | `https://` 加 DNS 名称，可带端口和路径；不接受账号、查询参数或片段          |

## 接口

| 方法与路径                     | JSON 请求体             | 成功响应                   |
| ------------------------------ | ----------------------- | -------------------------- |
| `GET /api/apps`                | —                       | `200 { apps: StoreApp[] }` |
| `POST /api/apps/:id/install`   | `{}`                    | `200 { app: StoreApp }`    |
| `POST /api/apps/:id/uninstall` | `{ password }`          | `204`（无响应体）          |
| `GET /api/apps/:id/settings`   | —                       | `200 AppSettings`          |
| `POST /api/apps/:id/settings`  | `{ values?, secrets? }` | `200 AppSettings`          |

```ts
interface AppSettings {
    values: Record<string, number | string | boolean | null> // 每个非敏感项，未设置为 null
    secrets: Record<string, boolean> // 每个敏感项是否已设置；永不包含值
    configured: boolean
    updatedAt: string
}
```

- 列表按目录顺序返回全部软件及其安装状态。读取列表不会写入任何数据。
- 安装：以清单默认值创建配置；没有默认值的必填项（例如 REALITY 借用站点）保持未设置，`configured` 为 `false`。目录中不存在返回
  `404 APP_NOT_FOUND`，已安装返回 `409 APP_ALREADY_INSTALLED`。
- 卸载：需要重新输入当前密码（与账号安全、节点操作共用“每个会话每 5 分钟 15 次”的密码确认限流），错误时
  `403 INVALID_PASSWORD`，超出限流 `429 TOO_MANY_ATTEMPTS`。成功后删除该软件的全部配置，包括已加密的敏感值；重新安装从默认值开始。未安装返回
  `404 APP_NOT_INSTALLED`。已部署到节点时返回 `409 APP_DEPLOYED`；节点第 2 阶段之前没有部署，因此目前不会出现，检查位置保留在卸载事务内。
- 读取配置：未安装返回 `404 APP_NOT_INSTALLED`。敏感项只报告是否已设置。
- 修改配置：
    - `values` 整体替换非敏感项：省略或为 `null` 的项回到默认值，没有默认值则为未设置。
    - `secrets` 只写：省略的键保留已保存的值，字符串替换，`null` 清除。
    - 敏感项出现在 `values` 中、非敏感项出现在 `secrets` 中，或键不在清单内：`400 { code: 'UNKNOWN_SETTING' }`。
    - 值不合法：`400 { code: 'INVALID_SETTING', field }`；保存后仍有必填项（含 `requiredWhen` 生效的项）没有值：
      `400 { code: 'SETTING_REQUIRED', field }`。`field` 是清单中的配置项键，不含值。
    - 未安装返回 `404 APP_NOT_INSTALLED`。
- 未知字段、多个 JSON 值或超过 16 KiB 的请求体返回 `400 REQUEST_FAILED`；缺少 `Origin`、自定义请求头或 JSON 类型返回
  `403 REQUEST_FAILED`。

## 存储与安全

- 表 `installed_apps(app_id PRIMARY KEY, version, settings, installed_at, updated_at)`（数据库迁移 v3）。未安装的软件不保存任何配置。
- `settings` 为 JSON：`{ values: {...}, secrets: { key: <密文> } }`。敏感值用 `master.key`（AES-256-GCM，随机
  nonce）加密，附加数据绑定 `apps:<id>:<key>`，密文不能挪到其他软件或配置项；任何接口都不返回明文。
- 读取时按当前清单重新校验已保存的值；控制台升级后清单不再接受的值按未设置处理。
- 日志只记录路由模板、方法、状态和耗时，不记录软件标识、配置值或密码。

## 前端要求

- 软件商店页（`/store`）列出目录中的软件、版本、协议、端口、镜像和安装状态，提供安装和卸载（卸载需要当前密码）。
- 侧栏“已安装软件”分组只列出已安装的软件，每个软件一个页面（`/apps/:id`）。窄屏底部导航保持四项，软件商店入口与已安装软件显示在品牌下方的一行中。
- 软件页按清单生成配置表单，提交后显示服务器的字段级错误。敏感字段只写不读：显示“已设置 /
  未设置”，输入框留空表示保留，可以选择清除；输入值只保存在组件内存中，保存成功、离开页面或会话失效时清除，不进入浏览器存储、URL、日志或持久化
  Pinia store。
- 管理区的“部署到节点”在节点第 2 阶段之前显示真实的未开放状态，不显示虚构的节点或部署数据。
- 未安装或目录中不存在的软件，直接访问其地址时显示“尚未安装”或“软件目录中没有这个软件”，并引导到软件商店。
- 目录与安装状态保存在内存中的 `src/stores/apps.ts`，不持久化，退出登录时清空。
- 前端只做展示控制；会话、写请求规则、配置校验、密码确认和限流都由后端执行。
