# 登录与两步验证接口约定

[English](auth-api.md) | 简体中文

当前由 Go + Gin + SQLite 实现，下述接口由真实后端提供。开发与生产均无模拟认证或默认账号；管理员通过本地 `make admin-create`
命令创建。Docker Compose 的前端通过同源代理连接后端。

## 页面与状态

- `/login`：账号密码登录；未创建管理员时提示本地初始化命令。
- `/login/verify`：验证器动态码或恢复码验证；可返回账号登录。
- `/settings/security`：验证器绑定、恢复码保存、恢复码重新生成、关闭两步验证、修改密码和退出其他会话。
- 其余控制台路由需有完整会话；两步验证尚未完成时始终返回验证页。
- 导航时向服务端读取会话；已登录会话在 15 秒内确认过时跳过这次读取，避免连续切换页面重复请求。刷新后始终从服务端恢复状态。认证服务不可用、返回
  HTML 或不符合契约的 JSON 时不会放行。
- 登录后的返回地址仅允许已知的控制台页面，保留其查询参数和锚点。外部地址和登录页循环跳转回退到 `/`。

前端路由守卫只控制界面，不是后端授权机制。未来所有管理接口仍需在服务端检查完整会话及权限。

## 传输与会话

接口基路径固定为同源 `/api/auth`。GET 无请求体；POST 使用 JSON。客户端携带 `credentials: same-origin`、
`Accept: application/json`、`X-Requested-With: Veilpost`，禁止跟随接口重定向，10 秒超时，不自动重试写操作。

后端使用 `vpn_session` Cookie 保存随机会话标识，属性为 `HttpOnly; SameSite=Strict; Path=/`；生产模式额外强制 `Secure` 并要求
HTTPS。仅显式 `VPN_ENV=development` 的本机 HTTP 开发允许非 Secure Cookie。未完成两步验证的 Cookie
不得授权任何管理接口。后端在密码登录和验证完成时轮换对应会话，退出时销毁完整会话及待验证状态，过期挑战返回匿名状态。

后端对写操作校验请求 `Origin` 和上述自定义请求头，只接受 JSON，禁止为不受信来源开启凭据 CORS；部署若使用其他 CSRF
方案，应同步调整客户端。所有认证响应设置 `Cache-Control: no-store`。前端不在 localStorage、sessionStorage、URL 或持久化 Pinia
store 中保存密码、验证码、恢复码、绑定密钥或会话令牌。

## 会话响应

`GET /session` 返回以下三种 JSON 之一：

```ts
type AuthSession =
    | { status: 'anonymous'; setupRequired?: boolean }
    | { status: 'mfa_required'; expiresAt: string }
    | {
          status: 'authenticated'
          user: { id: string; username: string; twoFactorEnabled: boolean }
      }
```

`expiresAt` 为带时区的 ISO 8601 时间。`401 { "code": "SESSION_EXPIRED" }` 也视为匿名。该接口必须能通过短期 Cookie
恢复待验证状态，不依赖前端保存挑战标识。完整会话过期后不得继续返回旧用户信息。

## 登录

| 方法与路径     | JSON 请求体                              | 成功响应                             |
| -------------- | ---------------------------------------- | ------------------------------------ |
| `POST /login`  | `{ username, password }`                 | 已认证的会话，或 `mfa_required` 会话 |
| `POST /verify` | `{ method: 'totp' \| 'recovery', code }` | 已认证的会话                         |
| `POST /logout` | `{}`                                     | `204`，并清除所有会话 Cookie         |

`username` 会去除首尾空格，密码保持原样。动态码为六位数字字符串，保留前导零；恢复码为字符串。前端仅去除验证码或恢复码中的空白字符。密码错误使用统一
`INVALID_CREDENTIALS`，不要暴露账号是否存在。

启用两步验证的账号通过密码验证后，只能创建短期挑战，不能提前创建完整会话。挑战有效期五分钟；服务器负责过期、尝试次数限制、动态码防重放，以及恢复码的一次性消费。前端显示倒计时，服务端时间及校验才是最终依据。恢复码登录不会自动关闭两步验证。

## 启用两步验证

1. `POST /two-factor/setup`，请求 `{ password }`。要求完整登录及当前密码。返回：

    ```ts
    {
      setupId: string
      secret: string
      expiresAt: string
      qrCodeDataUrl?: string
    }
    ```

    `secret` 为大写 Base32（A–Z、2–7，16–128 字符）。后端生成全新的随机密钥并保存待绑定状态；这一步不能启用两步验证。采用
    TOTP、SHA-1、6 位、30 秒参数，便于手动添加。

    当前后端始终返回 `qrCodeDataUrl`，仅接受 `data:image/png;base64,...`。二维码由 Go TOTP 库在后端本地生成，PNG
    内包含白色静区，内容为对应的 `otpauth://totp/` 配置，issuer 为 `Veilpost`，账号为用户名，参数与手动配置一致。不接受外部二维码
    URL，不向第三方发送密钥。界面优先显示扫码绑定，手动密钥默认折叠；客户端兼容旧后端省略二维码或图片加载失败时的手动添加。取消、离开页面、过期或绑定成功时同时清除二维码和临时密钥。

    `setupId` 绑定当前用户、会话和有效期，有效期五分钟；新的 setup 请求替换旧的待绑定状态。用户取消、导航离开或刷新时，前端丢弃临时密钥；服务端让未完成的绑定到期失效，不得自动启用。

2. `POST /two-factor/enable`，请求 `{ setupId, code }`。服务器验证绑定状态和首枚动态码，成功后原子地启用两步验证，返回：

    ```ts
    {
      status: 'authenticated'
      user: { id: string; username: string; twoFactorEnabled: true }
      recoveryCodes: string[]
    }
    ```

    `recoveryCodes`
    为非空列表，服务端生成高熵随机恢复码，只保存其不可逆哈希。仅本次响应返回明文，前端在当前页面显示，并要求用户确认保存后结束设置。未保存时离开页面会提示；取消页面或关闭浏览器无法恢复这些明文。前端不生成密钥或恢复码，不校验
    TOTP 算法。

## 关闭两步验证

`POST /two-factor/disable`，请求 `{ password, code }`，同时要求当前密码和验证器动态码；成功返回 `status: 'authenticated'` 且
`user.twoFactorEnabled: false` 的会话。服务器原子地移除 TOTP 配置、未使用恢复码和待绑定状态，按服务策略撤销其他会话。页面只在成功响应后更新状态。

## 恢复码与安全状态

`GET /security` 要求完整会话，返回：

```ts
{
    recoveryCodesRemaining: number
    otherSessions: number
}
```

两者均为非负整数。`otherSessions` 只统计同一账号除当前会话外未过期的完整会话。

`POST /two-factor/recovery-codes`，请求 `{ password, code }`，要求已启用两步验证、当前密码和验证器动态码（同样防重放）。成功返回
`{ recoveryCodes: string[] }`，原子地替换全部未使用恢复码；明文只在本次响应中返回，前端沿用启用时的保存确认流程。

`POST /sessions/revoke-others`，请求 `{}`，成功返回 `204`，销毁同一账号除当前会话外的所有会话（包括待验证挑战）。

## 修改密码

`POST /password`，请求 `{ password, newPassword }`，要求完整会话和当前密码。新密码沿用创建规则（至少 12 个字符、最多 256 个
UTF-8 字节），且不能与当前密码相同。成功返回已认证的会话，服务端轮换当前会话令牌并撤销该账号的其他会话。前端要求两次输入新密码一致，提交后清空所有密码字段。

当前版本未包含自助密码找回、账号注册、短信或邮件发送。验证器和恢复码都丢失，或忘记密码时，由能访问服务器数据目录的管理员执行
`make admin-reset-two-factor` 或 `make admin-reset-password`（发布包中为 `ADMIN_ACTION=reset-two-factor` /
`reset-password` 的 `admin-create.sh`），两者都会撤销该账号的全部会话。

## 错误格式

错误使用非 2xx HTTP 状态与 `{ "code": "..." }`。后端只返回错误码；前端按当前界面语言显示 `src/locales/<语言>/errors.ts`
中同名的固定文案（认证与节点错误码共用），不直接显示后端错误文本。语言包中没有的错误码显示为 `REQUEST_FAILED`
的文案。新增错误码时，四种语言的 `errors.ts` 必须同时添加。

| code                                     | 建议 HTTP 状态 | 前端行为                           |
| ---------------------------------------- | -------------- | ---------------------------------- |
| `INVALID_CREDENTIALS`                    | 401            | 保留账号，清空密码，可重试         |
| `INVALID_PASSWORD`                       | 403            | 保留安全设置步骤，重新输入密码     |
| `WEAK_PASSWORD` / `PASSWORD_REUSED`      | 400            | 保留修改密码表单，重新输入新密码   |
| `INVALID_CODE` / `INVALID_RECOVERY_CODE` | 400            | 保留验证步骤，清空输入             |
| `CHALLENGE_EXPIRED`                      | 401            | 返回账号登录                       |
| `SESSION_EXPIRED`                        | 401            | 清除前端会话，重新登录             |
| `SETUP_EXPIRED`                          | 410            | 清除待绑定密钥，允许重新设置       |
| `TOO_MANY_ATTEMPTS`                      | 429            | 提示稍后再试；限流由服务端强制执行 |
| 其他错误                                 | 4xx / 5xx      | 提示失败，不假定操作成功           |

`404`、5xx、网络失败、超时、HTML fallback 和不符合契约的成功响应均显示认证服务不可用。退出失败时保留当前界面并显示错误，不声称服务端会话已销毁。

## 接入和验证

根目录 `make start` 使用 Compose 启动真实前后端。容器之间使用 `backend:8080`；前端代理保留原始 Host，浏览器请求仍为同源请求。
`VPN_PUBLIC_ORIGIN` 可进一步绑定精确外部 Origin；生产部署必须配置 HTTPS。

完整会话有效期八小时；待验证和待绑定五分钟。密码采用 Argon2id（64 MiB、3 次、并行度 2），TOTP 密钥使用 AES-GCM
加密，恢复码和会话令牌仅保存哈希。TOTP 允许相邻一个时间步并记录已使用步数，启用和关闭两步验证、修改密码都会撤销旧会话。

登录按客户端地址（每 5 分钟 240 次）、账号与地址组合（10 次）和账号（100 次）限流，单一来源无法通过输错密码锁定账号。动态码和恢复码验证按挑战（每
5 分钟 10 次）和账号（每小时 30 次）限流，重新登录不会重置账号预算。安全设置操作按会话限流（每 5 分钟 15 次）。客户端地址只采信
`VPN_TRUSTED_PROXIES` 中反向代理转发的 `X-Forwarded-For`。

未创建用户时，匿名会话附带 `setupRequired: true`。此标记只引导本地初始化，不开放注册或认证绕过接口。账号名为 3–64 位 ASCII
字母、数字、点、下划线或连字符；创建密码至少 12 个字符，最多 256 个 UTF-8 字节。

浏览器测试位于 `tests/e2e/`，使用真实 Go 后端及临时 SQLite 数据库。`real-auth.ts` 仅负责创建隔离测试账号、调用真实接口和生成真实
TOTP，不拦截或伪造 HTTP 响应。Go 测试另覆盖服务端并发恢复码消费、防重放、限流、Origin 和 Cookie 边界。测试不访问开发数据卷。

参考：[RFC 6238](https://datatracker.ietf.org/doc/html/rfc6238)、[OWASP 密码存储](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html)。
