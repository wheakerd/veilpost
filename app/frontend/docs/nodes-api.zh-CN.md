# 节点与代理程序接口约定

[English](nodes-api.md) | 简体中文

对应 [节点设计](../../../docs/nodes-design.zh-CN.md) 的第 1 阶段：节点注册与控制通道。控制台只负责登记节点、签发一次性注册令牌、验证代理程序的签名请求并记录在线状态；本阶段不下发
VPN 配置，不管理任何容器。

接口由 Go 后端（`app/backend/internal/nodes`）实现，代理程序为 `app/agent` 中的 `veilpost-agent`。两部分共用下文的签名格式，并在各自的
Go 测试中固定同一组已知签名向量。

## 浏览器接口 `/api/nodes`

沿用 [认证契约](auth-api.zh-CN.md) 的传输规则：同源、`credentials: same-origin`、`X-Requested-With: Veilpost`，写请求只接受
JSON、校验 `Origin`，禁止跟随重定向，10 秒超时，不自动重试写操作；所有响应 `Cache-Control: no-store`。所有接口都要求完成两步验证的完整会话，否则返回
`401 SESSION_EXPIRED`。

### 节点对象

```ts
interface VpnNode {
    id: string // 22 位 base64url 随机标识
    name: string
    status: 'pending' | 'active' | 'revoked'
    online: boolean
    agentVersion: string | null
    createdAt: string
    enrolledAt: string | null
    revokedAt: string | null
    lastSeenAt: string | null
    enrollmentExpiresAt: string | null
}
```

- `pending`：已创建、尚未注册。`enrollmentExpiresAt` 为仍有效的注册令牌过期时间；令牌过期或被清理后为 `null`
  ，界面显示“令牌已过期”，可以重新签发令牌。
- `active`：代理程序已用注册令牌绑定公钥。
- `revoked`：已吊销，不能恢复；重新接入需要创建新节点。已吊销节点可以删除。
- `online` 由服务端计算：仅当 `status` 为 `active` 且最后一次通过验证的签名请求（`lastSeenAt`）在 90 秒以内时为 `true`
  。代理程序每次长轮询结束后立即发起下一次，因此 90 秒覆盖多个轮询周期。
- 时间均为带时区的 ISO 8601 字符串。数据缺失时为 `null`，界面显示“未知”，不补默认值。

### 接口

| 方法与路径                       | JSON 请求体          | 成功响应                                        |
| -------------------------------- | -------------------- | ----------------------------------------------- |
| `GET /api/nodes`                 | —                    | `200 { nodes: VpnNode[] }`                      |
| `POST /api/nodes`                | `{ name, password }` | `201 { node: VpnNode, enrollment: Enrollment }` |
| `POST /api/nodes/:id/revoke`     | `{ password }`       | `200 { node: VpnNode }`                         |
| `POST /api/nodes/:id/enrollment` | `{ password }`       | `200 { node: VpnNode, enrollment: Enrollment }` |
| `POST /api/nodes/:id/delete`     | `{ password }`       | `204`（无响应体）                               |

```ts
interface Enrollment {
    token: string // 43 位 base64url，256 位随机值
    expiresAt: string // 创建后 15 分钟
}
```

- 列表按“未吊销在前、创建时间倒序”排列，不包含任何令牌或公钥。
- 创建：`name` 去除首尾空白后为 1–64 个字符，不能含控制字符，否则 `400 INVALID_NAME`。需要重新输入当前密码，错误时
  `403 INVALID_PASSWORD`。注册令牌只在本次响应中出现；数据库只保存其 SHA-256 哈希。
- 吊销：需要当前密码。吊销立即生效：该节点的签名请求被拒绝，未使用的注册令牌和已记录的随机数被删除，正在等待的长轮询立即以
  `NODE_REVOKED` 结束。对已吊销节点重复吊销返回该节点（幂等）。节点不存在返回 `404 NODE_NOT_FOUND`。
- 重新签发：只适用于 `pending` 节点，需要当前密码。该节点之前所有未使用的注册令牌立即作废，新令牌 15
  分钟有效，只在本次响应中出现。节点已注册或已吊销返回 `409 NODE_NOT_PENDING`。
- 删除：只适用于 `revoked` 节点，需要当前密码。节点记录及其注册令牌、随机数记录一并删除，无法恢复。未吊销的节点返回
  `409 NODE_NOT_REVOKED`，节点不存在返回 `404 NODE_NOT_FOUND`。
- 创建、吊销、重新签发和删除与账号安全操作共用“每个会话每 5 分钟 15 次”的密码确认限流，超出返回 `429 TOO_MANY_ATTEMPTS`。
- 未知字段、多个 JSON 值或超过 8 KiB 的请求体返回 `400 REQUEST_FAILED`；缺少 `Origin`、自定义请求头或 JSON 类型返回
  `403 REQUEST_FAILED`。

### 前端要求

- 节点页显示真实列表；没有节点时保持“尚未接入”的空状态，不显示示例节点。页面可见时每 15
  秒在后台刷新一次（不显示全局请求条）；刷新失败时保留上次真实结果并提示。
- 注册令牌只保存在页面组件内存中：显示注册命令与令牌，提供复制按钮；点击“完成，隐藏令牌”、令牌过期、离开页面或会话过期时立即清除。令牌和密码不得进入
  localStorage、sessionStorage、URL、日志或持久化 Pinia store。
- 注册命令使用当前页面的 Origin：`veilpost-agent enroll --server <origin> --token-stdin`
  。令牌由使用者在命令提示后粘贴，不写进命令行参数。若代理程序访问控制台的地址与浏览器不同，使用者需要替换 `--server`。
- 前端只做展示控制；会话、密码、限流和吊销都由后端强制执行。

## 代理程序接口 `/api/agent`

代理程序不使用 Cookie，也不检查 `Origin`。请求体为 JSON（`Content-Type: application/json`，最大 64 KiB，否则 `415` /
`413 REQUEST_FAILED`）。为兼容不同版本，代理程序接口忽略请求体中的未知字段。

### 注册 `POST /api/agent/enroll`

```ts
// 请求
{
    token: string // 一次性注册令牌
    publicKey: string // Ed25519 公钥，32 字节，base64url（无填充）
    proof: string // 用对应私钥对注册消息的签名，64 字节，base64url
    agentVersion: string // ^[A-Za-z0-9][A-Za-z0-9._+-]{0,63}$
}
// 200 响应
{
    nodeId: string
    name: string
}
```

注册消息（`\n` 连接，无结尾换行）：

```text
vpn-agent-enroll-v1
<令牌的 SHA-256 十六进制>
<publicKey 原文>
```

服务端在一个事务内：确认令牌哈希存在、未使用、未过期且节点仍为 `pending`；标记令牌已使用并删除该节点其他未用令牌；写入公钥、代理程序版本和注册时间，把节点设为
`active`。同一令牌只能成功一次（并发请求也只有一个成功）。

| 情况                                   | 响应                     |
| -------------------------------------- | ------------------------ |
| 字段格式错误或 `proof` 校验失败        | `400 REQUEST_FAILED`     |
| 令牌不存在、已使用、已过期或节点已吊销 | `401 INVALID_ENROLLMENT` |
| 公钥已被其他节点使用                   | `409 KEY_IN_USE`         |
| 同一客户端地址每 5 分钟超过 30 次注册  | `429 TOO_MANY_ATTEMPTS`  |

客户端地址的识别规则与登录相同，只采信 `VPN_TRUSTED_PROXIES` 所列代理的 `X-Forwarded-For`。

### 签名请求

除注册外，每个代理程序请求携带：

| 请求头            | 内容                                                    |
| ----------------- | ------------------------------------------------------- |
| `X-VPN-Node`      | 节点 ID                                                 |
| `X-VPN-Timestamp` | Unix 秒，十进制                                         |
| `X-VPN-Nonce`     | 22–64 位 base64url 随机数（代理程序使用 24 字节随机值） |
| `X-VPN-Signature` | Ed25519 签名，64 字节，base64url                        |

签名消息（`\n` 连接，无结尾换行）：

```text
vpn-agent-v1
<HTTP 方法，如 POST>
<接口路径，如 /api/agent/poll>
<节点 ID>
<时间戳>
<随机数>
<请求体原始字节的 SHA-256 十六进制>
```

路径使用接口本身的路径（不含反向代理前缀和查询参数），因此控制台可以部署在带路径前缀的代理之后。签名包含方法和路径，防止把一个接口的签名挪用到另一个接口。

服务端检查顺序：

1. 请求头格式；时间戳与服务端时间相差超过 60 秒返回 `401 CLOCK_SKEW`。
2. 节点存在且已绑定公钥，签名有效；否则 `401 AGENT_UNAUTHORIZED`（不区分节点不存在、未注册或签名错误）。
3. 节点已吊销返回 `403 NODE_REVOKED`。只有签名有效的请求才能得知吊销状态。
4. 随机数去重：同一节点的随机数在其时间戳有效期内（时间戳 + 61 秒）只能使用一次，重放返回 `401 AGENT_UNAUTHORIZED`。随机数保存在
   `agent_nonces` 表，过期后由定期清理删除。
5. 按节点限流：每个节点每 5 分钟 120 次通过验证的请求，超出返回 `429 TOO_MANY_ATTEMPTS`。该额度与登录、账号安全的限流相互独立；签名无效的请求不消耗节点额度。

### 长轮询 `POST /api/agent/poll`（签名）

```ts
// 请求
{
    agentVersion: string
    configVersion: number
}
// 200 响应
{
    configVersion: number
}
```

服务端先记录 `lastSeenAt`（即“最后一次签名请求时间”）和 `agentVersion`。若请求中的 `configVersion` 与控制台不同，立即返回当前版本；否则最多等待
25 秒，期间节点被吊销则返回 `403 NODE_REVOKED`，服务停止时提前返回。第 1 阶段配置版本始终为 `0`，代理程序收到不同版本时只记录日志，不应用任何配置。

长轮询请求单独延长了服务端写超时；部署在反向代理或 CDN 之后时，读取超时需大于 25 秒（Nginx 默认 60 秒）。

## 数据与日志

- `nodes`：名称、状态、Ed25519 公钥（唯一）、代理程序版本、配置版本、创建 / 注册 / 吊销 / 最后通信时间。
- `node_enrollments`：令牌 SHA-256 哈希、节点、过期时间、使用时间。过期记录（无论是否使用）由后端启动时及每小时的清理任务删除。
- `agent_nonces`：节点、随机数、过期时间，同样由清理任务删除。
- 节点私钥只在节点上；控制台不保存、不生成节点私钥。
- 日志只记录路由模板（例如 `/api/nodes/:id/revoke`）、方法、状态和耗时，不记录节点 ID、令牌、公钥、签名、请求体或密码。

## 代理程序

- `veilpost-agent enroll --server <url> --token-stdin [--state-dir DIR]`：从标准输入读取令牌，在状态目录（默认
  `/var/lib/veilpost-agent`，权限 `0700`）生成 `identity.key`（PKCS#8 PEM，`0600`），注册成功后写入 `node.json`（服务器地址、节点
  ID、名称，`0600`）。已有身份时拒绝覆盖；注册失败时删除新生成的私钥。服务器地址必须为 HTTPS，仅回环地址允许 HTTP。
- `veilpost-agent run [--state-dir DIR]`：拒绝组或其他用户可访问的私钥和状态目录；持续长轮询并上报版本。失败时 5 秒起指数退避至
  5 分钟（±20% 抖动），时钟偏差会提示同步时间；节点被吊销时以退出码 3 结束，便于服务管理器停止重启。
- 代理程序不跟随重定向，不记录令牌或私钥。

## 验证

- Go 测试（`app/backend/tests/nodes`、`app/agent/tests/agent`
  ）覆盖签名已知向量、重放、时钟偏差、请求篡改、未知节点、吊销与长轮询唤醒、令牌一次性核销与过期、并发核销、公钥复用、按节点限流、清理、HTTP
  写请求校验和日志边界。
- 浏览器测试 `tests/e2e/nodes.spec.ts` 使用真实后端创建与吊销节点，并在测试中用 Node 的 Ed25519
  实现调用真实的注册和签名轮询接口，验证在线状态与吊销效果；它不运行 `veilpost-agent` 程序。代理程序与控制台的端到端联调测试按已定事项暂不实施。
