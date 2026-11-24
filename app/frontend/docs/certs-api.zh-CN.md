# 证书与 DNS 服务商接口约定

[English](certs-api.md) | 简体中文

对应 [平台设计](../../../docs/platform-design.zh-CN.md) 的“三、证书与 DNS 服务商”。控制台保存 DNS 服务商凭据，通过 ACME 协议与
DNS-01 验证申请证书，并在后台自动续期。接口由 Go 后端 `app/backend/internal/certs` 实现，前端客户端为 `src/api/certs.ts`。

## 传输规则

沿用 [认证契约](auth-api.zh-CN.md)：同源、`credentials: same-origin`、`X-Requested-With: Veilpost`；写请求只接受 JSON 并校验
`Origin`（开发模式 http/https 同源，生产必须 HTTPS 且与 `VPN_PUBLIC_ORIGIN` 一致）；禁止跟随重定向，10 秒超时，不自动重试写操作；所有响应
`Cache-Control: no-store`。所有接口都要求完成两步验证的完整会话，否则返回 `401 SESSION_EXPIRED`。请求体只接受一个 JSON
对象，含未知字段返回 `400 REQUEST_FAILED`。

## DNS 服务商凭据 `/api/dns-credentials`

### 凭据对象

```ts
interface DnsCredential {
    id: string // 22 位 base64url 随机标识
    name: string
    provider: 'cloudflare' | 'alidns' | 'tencentcloud' | 'route53'
    fields: Record<string, boolean> // 每个字段是否已保存；从不返回值
    certificates: number // 引用该凭据的证书数
    createdAt: string
    updatedAt: string
}
```

各服务商的字段（`fields` 的键恰好是该服务商的全部字段）：

| provider       | 字段                                                                       |
| -------------- | -------------------------------------------------------------------------- |
| `cloudflare`   | `apiToken`                                                                 |
| `alidns`       | `accessKeyId`、`accessKeySecret`                                           |
| `tencentcloud` | `secretId`、`secretKey`                                                    |
| `route53`      | `accessKeyId`、`secretAccessKey`、`region`、`hostedZoneId`（唯一可选字段） |

值为不含空白的可打印 ASCII，最多 512 字节；`region` 形如 `us-east-1`；`hostedZoneId` 为大写字母与数字（可带 `/hostedzone/`
前缀）。全部字段（包括区域与 Hosted Zone ID）作为一个整体用 `master.key` 加密保存，接口和日志都不返回明文。

### 接口

| 方法与路径                             | JSON 请求体                             | 成功响应                               |
| -------------------------------------- | --------------------------------------- | -------------------------------------- |
| `GET /api/dns-credentials`             | —                                       | `200 { credentials: DnsCredential[] }` |
| `POST /api/dns-credentials`            | `{ name, provider, secrets, password }` | `201 { credential }`                   |
| `POST /api/dns-credentials/:id/update` | `{ name, secrets, password }`           | `200 { credential }`                   |
| `POST /api/dns-credentials/:id/delete` | `{ password }`                          | `204`                                  |
| `POST /api/dns-credentials/:id/verify` | `{}`                                    | `200 { status: 'ok' }`                 |

- 新增、修改、删除都需要当前密码，与账号安全操作共用按会话的限流（5 分钟 15 次）。密码错误返回 `403 INVALID_PASSWORD`，不改变任何数据。
- 修改时服务商不能改变。`secrets` 中非空的值替换该字段，空字符串清除可选字段（必填字段为空返回 `INVALID_SECRETS`
  ），未出现的字段保持原值。界面只发送用户填写过的字段，从不读取已保存的值。
- 被证书引用的凭据不能删除：`409 CREDENTIAL_IN_USE`。
- 验证凭据调用服务商的一个只读接口：Cloudflare `GET /zones?per_page=1`，阿里云 `DescribeDomains`（每页 1 条），腾讯云
  `DescribeDomainList`（Limit 1），Route 53 `GetHostedZone`（已填 Hosted Zone ID）或 `ListHostedZones`（MaxItems 1）。不创建、不修改任何
  DNS 记录。超时 8 秒；按会话限流 5 分钟 10 次。服务商拒绝返回 `422 CREDENTIAL_REJECTED`，网络错误或服务商故障返回
  `424 PROVIDER_UNREACHABLE`。

## 证书 `/api/certificates`

### 证书对象

```ts
interface Certificate {
    id: string
    domains: string[] // 规范化后的名称（小写、Punycode），第一个用于下载文件名
    directory: 'letsencrypt' | 'letsencrypt-staging' | 'custom'
    directoryUrl: string // https:// ACME 目录地址
    email: string // ACME 账户联系邮箱，可为空
    dnsCredentialId: string
    dnsCredentialName: string
    status: 'pending' | 'valid' | 'failed' | 'expired'
    issuing: boolean // 本进程中有签发或续期任务排队或运行
    issuer: string | null // 叶证书签发者的组织名（无则 CN）
    notBefore: string | null
    notAfter: string | null
    renewAt: string | null // 下一次自动签发或重试的时间
    lastError: { code: string; detail: string | null; at: string } | null
    createdAt: string
    updatedAt: string
}
```

- `pending`：首次签发排队或进行中。`valid`：已签发且未过期。`failed`：首次签发失败，等待手动重新签发。`expired`
  ：证书已过期（续期一直失败时会出现）。
- 续期期间状态保持 `valid`，`issuing` 为 `true`；界面显示“续期中”。
- `lastError.code` 为签发错误类别（见下表）；`detail` 只在 ACME 服务返回问题文档时存在，内容为问题类型与简短说明（来自
  CA，未翻译，最多 300 字符），DNS 服务商错误从不保存原文。签发成功后清空。
- 证书私钥从不出现在任何接口响应中。

| 签发错误类别              | 含义                                        |
| ------------------------- | ------------------------------------------- |
| `CREDENTIAL_UNAVAILABLE`  | 凭据无法解密或无法构造服务商客户端          |
| `ACME_UNREACHABLE`        | 无法连接 ACME 目录或网络错误                |
| `ACME_ACCOUNT_FAILED`     | ACME 账户注册失败（包括需要 EAB 而未提供）  |
| `ACME_RATE_LIMITED`       | ACME 返回 `rateLimited`                     |
| `ACME_REJECTED`           | ACME 返回其他问题（验证失败、拒绝的名称等） |
| `DNS_PROVIDER_FAILED`     | 通过服务商创建验证 TXT 记录失败             |
| `DNS_PROPAGATION_TIMEOUT` | 验证记录未在 5 分钟内出现在权威 DNS 上      |
| `ISSUANCE_FAILED`         | 其他错误                                    |

### 接口

| 方法与路径                           | JSON 请求体                                                                             | 成功响应                              |
| ------------------------------------ | --------------------------------------------------------------------------------------- | ------------------------------------- |
| `GET /api/certificates`              | —                                                                                       | `200 { certificates: Certificate[] }` |
| `POST /api/certificates`             | `{ domains, dnsCredentialId, directory, directoryUrl, email, eab: { keyId, hmacKey } }` | `202 { certificate }`                 |
| `POST /api/certificates/:id/reissue` | `{}`                                                                                    | `202 { certificate }`                 |
| `POST /api/certificates/:id/delete`  | `{}`                                                                                    | `204`                                 |
| `GET /api/certificates/:id/chain`    | —                                                                                       | `200` PEM 证书链                      |

- 申请立即返回 `pending`，签发在后台进行；界面在有 `pending` 或 `issuing` 的证书时每 3 秒刷新，否则每 30
  秒。申请与重新签发共用按会话限流（10 分钟 20 次）。
- `domains`：1–100 个合法 DNS 名称，每个至少两级、标签 1–63 个字母数字或连字符，总长不超过 253；可以用 `*.`
  开头表示通配符（只能出现在最左一级）；Unicode 名称转为 Punycode；重复项去除。不合法返回 `400 INVALID_DOMAIN`，超过 100 个返回
  `400 TOO_MANY_DOMAINS`。
- `directory`：`letsencrypt`（默认，也可为空字符串）为 Let's Encrypt 正式环境，`letsencrypt-staging` 为测试环境，此时
  `directoryUrl` 必须为空；`custom` 时 `directoryUrl` 必须是不含用户信息和片段的 `https://` 地址，否则
  `400 INVALID_DIRECTORY`。
- `email` 可为空，否则必须是普通邮箱地址（`400 INVALID_EMAIL`）。同一目录地址与邮箱共用一个 ACME 账户；账户私钥（ECDSA
  P-256）在控制台生成，用 `master.key` 加密保存。
- `eab` 只能用于自定义目录，Key ID 与 HMAC Key（base64url）必须同时填写，否则 `400 INVALID_EAB`。EAB
  加密保存到该账户注册成功为止，注册后删除；已注册的账户忽略新提交的 EAB。
- 提交申请即表示同意所选 ACME 服务的服务条款（账户注册时 `termsOfServiceAgreed: true`）。
- 凭据不存在：`404 CREDENTIAL_NOT_FOUND`。证书不存在：`404 CERTIFICATE_NOT_FOUND`。
- 重新签发：任务已在进行时直接返回当前状态；`failed` 的证书变回 `pending` 并清除错误；已有证书的保持 `valid`，签发成功后替换证书与私钥。
- 删除会移除证书及其加密私钥，并取消正在进行的签发；ACME 账户保留供以后复用。
- 证书链：`Content-Type: application/pem-certificate-chain`，`Content-Disposition: attachment; filename=<第一个名称>.pem`（
  `*.` 写作 `wildcard.`），内容为叶证书在前的 PEM 链。尚未签发返回 `409 CERTIFICATE_NOT_ISSUED`。客户端校验响应只含
  `CERTIFICATE` 块后通过临时 Blob 下载。私钥不提供浏览器下载，后续由节点代理程序按需获取。

## 后台任务

- 签发、续期在后端任务中进行，最多 2 个并发；控制台启动时与之后每小时检查一次：把 `notAfter` 已过的 `valid` 证书标记为
  `expired`，并为 `renewAt` 已到的证书排队。
- 签发成功后 `renewAt` 为到期前 30 天；有效期不足 90 天的证书在剩余三分之一有效期时续期。每次签发都生成新的证书私钥（ECDSA
  P-256）。
- 续期失败保留现有证书，按 1、2、4、8、16 小时退避，之后每 24 小时重试；首次签发失败不自动重试，需要手动重新签发。
- 控制台关闭或证书被删除时，正在进行的任务立即取消：ACME 请求绑定任务上下文，DNS 传播等待在下一次检查时退出。被中断的任务不记录错误，下次启动时继续。

## 错误码

| 错误码                    | 状态 | 含义                                 |
| ------------------------- | ---- | ------------------------------------ |
| `INVALID_CREDENTIAL_NAME` | 400  | 名称为空、超过 64 字符或含控制字符   |
| `INVALID_PROVIDER`        | 400  | 不支持的服务商                       |
| `INVALID_SECRETS`         | 400  | 字段缺失、未知或格式不正确           |
| `CREDENTIAL_NOT_FOUND`    | 404  | 凭据不存在                           |
| `CREDENTIAL_IN_USE`       | 409  | 凭据被证书引用，不能删除             |
| `CREDENTIAL_UNREADABLE`   | 409  | 已保存的凭据无法解密（需要重新填写） |
| `CREDENTIAL_REJECTED`     | 422  | 验证凭据时服务商拒绝                 |
| `PROVIDER_UNREACHABLE`    | 424  | 验证凭据时无法连接服务商             |
| `INVALID_DOMAIN`          | 400  | 域名不合法                           |
| `TOO_MANY_DOMAINS`        | 400  | 超过 100 个名称                      |
| `INVALID_DIRECTORY`       | 400  | ACME 服务或目录地址不合法            |
| `INVALID_EMAIL`           | 400  | 联系邮箱不合法                       |
| `INVALID_EAB`             | 400  | EAB 不完整、格式错误或用于内置目录   |
| `CERTIFICATE_NOT_FOUND`   | 404  | 证书不存在                           |
| `CERTIFICATE_NOT_ISSUED`  | 409  | 证书尚未签发，无法下载证书链         |

另有通用的 `SESSION_EXPIRED`、`INVALID_PASSWORD`、`TOO_MANY_ATTEMPTS`、`REQUEST_FAILED`、`SERVICE_UNAVAILABLE`。所有错误码在四种语言的
`src/locales/<语言>/errors.ts` 中都有文本；签发错误类别的文本在 `certs.ts` 的 `certificates.issueErrors`。客户端把带
`_NOT_FOUND` 结尾错误码的 404 视为资源不存在，其他 404 视为服务不可用。
