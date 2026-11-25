# 多语言、软件商店与证书管理设计

[English](platform-design.md) | 简体中文

状态：设计已确认（2026-11-22）。多语言、软件商店与证书管理已实现（2026-11-23）。多语言：未选择语言时不写入存储，已显示的提示消息不会随语言切换重新渲染。软件商店：配置写入用 `POST /api/apps/:id/settings`（与现有写请求一致），部署到节点依赖节点第 2 阶段；Hysteria 2 官方镜像为 `tobyxdd/hysteria`；AmneziaWG 端口默认按节点随机，UDP 443 留给 Hysteria 2。证书：lego v4.35.2；续期失败按 1、2、4、8、16 小时退避后每天重试，首次签发失败不自动重试；续期中状态保持 `valid`。接口契约见 `app/frontend/docs/apps-api.md` 与 `certs-api.md`。节点功能设计见 [nodes-design.md](nodes-design.zh-CN.md)。

## 已定事项

1. 软件商店里“安装”一个 VPN 软件 = 在控制台启用它：侧栏出现它的配置和管理页面。部署到哪些节点在管理页中另行选择，由节点代理程序实际拉取镜像并启动（依赖节点第 2 阶段）。已部署到节点的软件不能直接卸载，需要先从所有节点移除。
2. 商店只使用随控制台版本发布的内置目录；镜像按摘要固定。新的 VPN 软件随控制台升级加入，不支持远程软件源或上传自定义包。
3. 界面语言：简体中文（`zh-CN`）、English（`en`）、繁體中文（`zh-TW`）、Русский（`ru`）。
4. 证书使用 ACME 协议，默认 Let's Encrypt，允许自定义 ACME 目录地址（如 ZeroSSL、Google Trust Services，需要时填写 EAB）。域名验证使用 DNS-01，界面先提供 Cloudflare、阿里云 DNS、腾讯云 DNSPod、AWS Route 53 四家。

## 一、多语言

- 采用 `vue-i18n`（Composition API 模式），语言包按功能拆分：`src/locales/<语言>/<功能>.ts`，避免多人同时修改同一文件。
- 默认语言：首次访问按浏览器语言匹配，匹配不到用简体中文；用户在“偏好设置”中切换，选择保存在 `src/stores/preferences.ts`（非敏感偏好，可持久化）。
- `<html lang>` 随语言切换；日期和数字用 `Intl` 按当前语言格式化。
- 后端只返回错误码，不返回文案；所有错误码在各语言包中都有对应文本，缺失时回退到简体中文并在开发构建中警告。
- Element Plus 组件同步切换对应语言。
- 测试：类型检查保证各语言包的键与简体中文完全一致；浏览器测试覆盖切换语言、刷新后保持、`<html lang>`，以及移动端布局不溢出（俄语文本较长）。

## 二、软件商店

### 目录

- 内置目录用 Go `embed` 打包在后端，每个软件一份清单：`id`、版本、名称与描述的翻译键、协议类别、所需端口、配置项定义（类型、默认值、取值范围、是否敏感），以及按摘要固定的镜像。
- 第一版目录（与节点设计的入口协议一致）：

| id          | 软件                                 | 说明                                                       |
| ----------- | ------------------------------------ | ---------------------------------------------------------- |
| `xray`      | Xray-core                            | VLESS + REALITY + Vision（TCP）与 XHTTP + REALITY 两类入口 |
| `hysteria2` | Hysteria 2（官方服务端）             | UDP，伪装或 Salamander 混淆                                |
| `amneziawg` | AmneziaWG（`docker/node/amneziawg`） | UDP，2.0 混淆参数按节点随机生成                            |

### 安装状态与接口

- 新表 `installed_apps(app_id, version, settings, installed_at, updated_at)`；未安装的软件不在库中保存任何配置。
- `GET /api/apps`：目录加安装状态。`POST /api/apps/:id/install`、`POST /api/apps/:id/uninstall`、`GET/PUT /api/apps/:id/settings`。安装、卸载和修改配置需要会话与同源规则；卸载需要重新输入密码。
- 配置按清单校验（未知字段、越界值拒绝）。敏感配置项用 `master.key` 加密保存，读取时不返回明文。
- 卸载会删除该软件的配置；已部署到节点时返回 `409 APP_DEPLOYED`（节点第 2 阶段之前不会出现部署）。

### 界面

- 新增“软件商店”页：列出目录中的软件、版本、协议、安装状态，提供安装和卸载。
- 侧栏新增“已安装软件”分组，只列出已安装的软件；每个软件有自己的配置页和管理页。未安装的软件没有入口，直接访问其地址显示“尚未安装”并引导到商店。
- 管理页中的“部署到节点”在节点第 2 阶段完成前显示真实的未开放状态，不显示虚构数据。
- 节点页不再列出“计划接入的协议”，改为链接到软件商店。

## 三、证书与 DNS 服务商

### 依赖

- 使用 `github.com/go-acme/lego/v4`（成熟的 Go ACME 客户端，纯 Go，可无 CGO 构建），只导入四家 DNS 服务商的子包，避免引入其全部依赖。

### DNS 服务商凭据

- 新表 `dns_credentials(id, name, provider, secrets, created_at, updated_at)`。`secrets` 整体用 `master.key` 加密；接口和日志都不返回明文，界面只显示是否已设置和最后修改时间。
- `name` 为可选备注（界面标签“备注（可选）”），去除首尾空白后最多 64 个 Unicode 字符。已有名称作为备注保留并可清空。凭据列表、证书选择框和证书列表采用统一显示规则：有备注显示备注，空备注显示本地化服务商名称。空备注在存储中保持为空，凭据列表省略重复的服务商副标题。证书响应包含当前备注和服务商，后续查询会反映修改。
- 各家需要的字段：Cloudflare（API Token）、阿里云（AccessKey ID / AccessKey Secret）、腾讯云 DNSPod（SecretId / SecretKey）、AWS Route 53（Access Key ID / Secret Access Key / 区域，可选 Hosted Zone ID）。
- 新增、修改、删除凭据需要重新输入密码；被证书引用的凭据不能删除（`409 CREDENTIAL_IN_USE`）。
- 提供“验证凭据”操作：调用服务商的只读接口确认可用，不创建任何 DNS 记录。

### ACME 账户与证书

- ACME 服务：内置 Let's Encrypt 正式环境（默认）与测试环境；自定义时填写目录地址（只接受 `https://`），可选 EAB Key ID / HMAC。
- ACME 账户私钥在控制台生成，`master.key` 加密保存；每个 ACME 服务加联系邮箱对应一个账户。
- 新表 `certificates(id, domains, ca_directory, dns_credential_id, status, not_before, not_after, certificate_pem, private_key, last_error, renew_at, created_at)`。证书私钥加密保存；证书链可下载，私钥不通过浏览器下载（后续由节点代理程序按需拉取）。
- 状态：`pending`（排队/签发中）、`valid`、`failed`、`expired`。签发在后端后台任务中执行，接口立即返回；界面轮询状态。
- 自动续期：到期前 30 天开始续期，失败按指数退避重试并在界面显示错误；控制台启动时与每小时检查一次。
- 域名校验：只接受合法的 DNS 名称，支持 `*.example.com` 通配符；同一张证书最多 100 个名称（ACME 上限）。
- 日志只记录路由模板、方法、状态和耗时；签发错误只保存 ACME/DNS 返回的错误类别和简短说明，不保存凭据或令牌。

### 测试

- Go 测试使用 Let's Encrypt 的 Pebble（测试用 ACME 服务器）与进程内的测试 DNS 服务商，不访问公网、不调用真实服务商。
- 凭据加密、接口权限、密码确认、删除限制与续期调度都有单元测试；浏览器测试覆盖凭据表单不回显明文、证书列表与状态显示。

## 实施顺序

1. 多语言（其他两部分的新文案直接写进语言包）。
2. 软件商店与证书管理可以并行：数据库迁移版本按顺序分配（软件商店 v3，证书 v4），语言包按功能分文件。
3. 每部分各自附带测试与文档更新，由主会话统一运行 `make check` 后合并。
