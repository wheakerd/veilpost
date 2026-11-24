# Veilpost 前端

[English](README.md) | 简体中文

Vue 3 + TypeScript + Rsbuild 控制台，提供真实后端登录、两步验证、账号安全、网络概览、节点注册与在线状态、软件商店与已安装软件的配置、证书与 DNS 服务商凭据，以及偏好设置。

## Docker 开发

在仓库根目录运行 `make start`。前端在 Compose 容器中运行，源码绑定挂载支持 HMR，`/api` 通过 `backend:8080` 转发到 Go 服务。首次使用执行 `make admin-create` 创建管理员，没有演示账号。

默认入口为 <http://127.0.0.1:3000>。`make status` 显示实际端口；`OPEN_BROWSER=false make start` 关闭宿主机自动打开浏览器。容器自身不会尝试启动浏览器。

## 原生开发与命令行

使用 `.nvmrc` 的 Node 26.13.1 和 Bun 1.4.4：

```bash
nvm use
bun install --frozen-lockfile
bun run dev --backend-host 127.0.0.1 --backend-port 49152
```

后端地址必须成对提供，支持 IP 或服务主机名。开发配置通过私有进程变量读取启动器参数，不会把地址编译到浏览器资源中。没有后端地址时不会生成模拟数据，登录页会提示服务不可用。

原生 `bun run dev` 默认打开系统浏览器，`--no-open` 或 `OPEN_BROWSER=false` 可关闭；CI 和生产预览不自动打开浏览器。根目录 `make native-start` 可以自动启动原生后端、发现随机端口并传给前端；不要与 Compose 同时占用前端 3000 端口。

```bash
bun run typecheck
bun run build
bun run format:check
bun run lint
bun run test:dev
PLAYWRIGHT_CHANNEL=chrome bun run test:e2e --workers=4
```

## 真实接口与测试

节点页调用同源 `/api/nodes` 显示真实节点列表与在线 / 离线状态（以最后一次通过验证的代理程序签名请求为准），页面可见时每 15 秒刷新。创建、吊销、重新签发注册令牌（仅未注册节点）和删除节点（仅已吊销节点）都需要重新输入当前密码；创建后显示 `veilpost-agent enroll --server <当前 Origin> --token-stdin` 注册命令和一次性注册令牌，令牌只在页面内存中保留，隐藏、过期、离开页面或会话失效时立即清除。没有节点时保持“尚未接入”的空状态，不显示示例数据。概览页读取同一列表，显示未吊销节点中的在线数量。当前不下发 VPN 配置。节点页不再列出计划接入的协议，改为链接到软件商店。接口详情见 [节点契约](docs/nodes-api.zh-CN.md)。

软件商店页（`/store`）调用同源 `/api/apps`，列出随控制台发布的内置目录（Xray-core、Hysteria 2、AmneziaWG）及其版本、协议、端口、按摘要固定的镜像和安装状态。安装即在控制台中启用该软件，侧栏“已安装软件”分组随即出现它的页面（`/apps/:id`）；卸载需要重新输入当前密码，并删除全部配置。软件页按清单生成配置表单，由服务器严格校验并返回字段级错误；敏感配置只写不读，只显示“已设置 / 未设置”，输入值只在页面内存中保留。“部署到节点”在节点第 2 阶段之前显示真实的未开放状态。未安装的软件直接访问其地址时显示“尚未安装”并引导到商店。窄屏底部导航保持四项，软件商店、证书、DNS 服务商与已安装软件的入口显示在品牌下方。接口详情见 [软件商店契约](docs/apps-api.zh-CN.md)。

证书页调用同源 `/api/certificates`：列出证书的域名、颁发机构、状态（签发中、有效、签发失败、已过期，续期时另显示“续期中”）、到期时间与最近错误；申请证书时填写域名、DNS 服务商凭据、ACME 服务（Let's Encrypt 默认、测试环境，或 `https://` 自定义目录与可选 EAB）和联系邮箱。签发在后端后台任务中进行，接口立即返回；有进行中的任务时每 3 秒刷新，否则每 30 秒。可以重新签发、删除和下载证书链（PEM），私钥不提供浏览器下载。DNS 服务商页调用 `/api/dns-credentials`，支持 Cloudflare、阿里云 DNS、腾讯云 DNSPod 和 AWS Route 53：密钥只写不读，列表只显示各字段“已设置 / 未设置”，修改时留空的字段保持原值；新增、修改、删除需要当前密码，被证书使用的凭据不能删除；“验证凭据”只做只读查询。密钥、EAB HMAC Key 和密码只在页面内存中保留，表单关闭、离开页面或会话失效时清除。接口详情见 [证书契约](docs/certs-api.zh-CN.md)。

账号安全页提供两步验证绑定与关闭、恢复码重新生成和剩余数量、修改密码，以及退出其他会话。客户端始终调用同源 `/api/auth`，验证响应后才接受认证状态。密码、验证码、恢复码和会话令牌不存入 localStorage、sessionStorage 或持久化 Pinia。实际认证、授权、TOTP、防重放、恢复码消费和限流由 Go 后端执行。

浏览器测试由 `scripts/e2e-server.py` 启动真实 Go 服务和临时 SQLite 数据库；每个测试通过本地管理员命令创建随机独立账号，再通过真实 HTTP 登录。验证码根据真实绑定密钥计算。没有 MSW、`page.route` 接口拦截或生产测试入口，测试结束会销毁临时数据。

测试端口为 4173，后端端口由系统分配。测试覆盖桌面与移动端的登录、刷新、保护路由、真实两步验证、恢复码一次性使用、过期、网络故障、主题偏好、键盘操作、放大文字，以及界面语言（按浏览器语言选择默认语言、切换后刷新保持、`<html lang>`、四种语言在窄屏下不横向溢出）。Playwright 项目固定 `locale: 'zh-CN'`，其余测试看到简体中文界面；语言测试用 `test.use({ locale })` 模拟其他浏览器语言。证书测试只使用指向本机关闭端口的自定义 ACME 目录，后台任务真实失败为“无法连接 ACME 服务”，不访问公网 ACME 或 DNS 服务商，也不点击“验证凭据”。节点测试通过真实接口创建与吊销节点，并在测试进程中用 Ed25519 调用真实的注册与签名轮询接口来验证在线状态；不运行 `veilpost-agent` 程序。测试共享同一个临时后端，节点列表可能包含其他并行测试创建的节点，因此测试只检查自己创建的节点。软件的安装状态是全局的，桌面与移动项目会同时运行同一测试：`tests/e2e/apps.spec.ts` 中每个测试独占一个软件（在本次运行的临时数据目录中创建锁目录），开始时通过真实接口把该软件重置为所需状态，只断言该软件；其他测试文件不安装软件。首次使用 Playwright 自带浏览器时，运行 `bunx playwright install chromium`。

## 加载与交互

- 初次会话恢复显示加载卡片，不提前展示受保护内容。
- 全局请求条跟随真实请求状态；登录与验证按钮显示进行中的动作并防止重复提交。
- 服务连接失败提供重试入口，错误与成功反馈使用短距离、无回弹的过渡。
- 安全设置阶段切换与页面导航采用可中断的 Motion 动画；用户或系统开启减少动态效果时取消位移。
- 内联表单与确认操作从当前尺寸展开和收起（`src/components/Reveal.vue`；表格行改为淡入淡出）；抽屉和窄屏底部面板使用 iOS 面板曲线；顶栏只在内容滚到下方时显示分隔线；节点刚变为在线时，状态圆点会向外扩散一圈。
- 登录时密码标签旁会提示大写锁定；账号密码或验证码被拒时表单抖动一次；验证码显示为六个格子，通过时依次抬起，并短暂显示“验证通过，正在继续…”；随后卡片消散、地球收拢，显示“正在进入控制台…”（`src/composables/useSignInExit.ts`）。
- 节点、DNS 凭据、证书和修改密码表单有未保存的修改时，取消或按 Esc 会就地询问是否放弃（`src/composables/useDiscardGuard.ts`）；无法使用的控件在悬停和聚焦时说明原因（`src/components/DisabledReason.vue`）；加载状态在 180 毫秒后才显示。
- 仍支持减少透明度、高对比度、系统字体、200% 文字与移动端底部导航。

## 多语言

界面支持简体中文（`zh-CN`）、English（`en`）、繁體中文（`zh-TW`）和 Русский（`ru`），使用 `vue-i18n` 的 Composition API 模式。

- 默认按浏览器语言（`navigator.languages`）匹配：`zh-TW`、`zh-HK`、`zh-MO`、`zh-Hant` 为繁體中文，其他 `zh` 为简体中文，`ru` 为俄语，`en` 为英语，都不匹配时用简体中文。
- “偏好设置”可以选择语言或恢复“跟随浏览器”。只有明确选择的语言写入 `src/stores/preferences.ts` 的持久化偏好（`locale` 字段）；跟随浏览器时不写入任何内容，“恢复默认”也会回到跟随浏览器。
- `<html lang>`、页面标题、页面描述和 Element Plus 的语言随界面语言切换；日期、相对时间和数字用 `Intl` 按当前语言格式化（`src/i18n/format.ts`）。
- 语言包按功能拆分在 `src/locales/<语言>/<功能>.ts`（`common`、`errors`、`auth`、`overview`、`nodes`、`security`、`appearance`、`apps`、`certs`）。软件目录清单引用的名称、描述和配置项文案位于 `apps.catalog.<id>`，`tests/dev/locales.test.ts` 检查清单中的每个键都存在。简体中文是类型来源，其他语言用 `satisfies` 校验，缺键或多键都会让 `bun run typecheck` 失败；运行时缺失的文案回退到简体中文，开发构建在控制台警告。
- `src/locales/<语言>/errors.ts` 以后端错误码为键；新增错误码需要同时补齐四种语言。
- 需要复数的文案用 `|` 分隔：英语两种形式（one | other），俄语三种（one | few | many），中文不分。文案中的代码片段（命令、路径）通过 `I18nT` 插槽渲染，不写进语言包。
- `bun run test:dev` 中的 `tests/dev/locales.test.ts` 检查各语言的文件与键一致、占位符一致、源码中引用的键存在，以及语言包之外没有中日文或俄文界面文字。

## 主题与结构

界面参照 iOS 26 与 macOS 26 的液态玻璃（`src/styles/_glass.scss`、`src/theme/liquidGlass.ts`）。工作区立在一张由主题主色与辅助色构成的静止背景上；侧栏、顶栏、页签栏和页面面板是彼此相隔半个 rem 的玻璃面板，内容滚到顶栏下方时顶栏会略微抬起。页面面板上的面板与卡片是带角标的细线框，数值用等宽字体，黑黄警示条只出现在需要留意的地方。菜单、弹出框、抽屉、对话框、消息和登录卡片是更通透的玻璃，带边缘高光，在 Chromium 内核浏览器中边缘还会折射背后的内容；它们从触发位置展开，玻璃随之变厚。所有按钮都是同一种主题主色的实心胶囊（危险操作为红色），带随指针移动的光和弹簧式按压；单行输入框为胶囊形。开启降低透明度或提高对比度时玻璃改为实色，开启减少动态效果时取消按压与折射。窄屏底部标签栏的选中指示是一块玻璃透镜：按下标签时浮起，拖动时随手指在标签间移动，松手后按拖动方向吸附到最近的标签并打开该页面。登录页用 Canvas 绘制缓慢旋转的线框地球（`src/components/LabGlobe.vue`），页面在后台或开启减少动态效果时停止。文字大小使用 rem，根字号随可视窗口宽度变化（`src/styles/_type.scss`）：约 1000px 宽以下为 15px，1440px 为 16px，2560px 起最大 18px；Element Plus 的文字和控件尺寸随之缩放，用户的默认字号和页面缩放依然生效。宽屏下窗口本身不滚动：顶栏和侧栏固定，只有页面面板内部滚动，离开再回到某个页面时恢复其滚动位置（`src/router/pageCache.ts`）；窄屏仍滚动整个窗口。控制台外壳包括通栏顶栏（左侧是品牌和收起侧栏按钮，右侧是刷新、明暗、语言、偏好设置、时钟和个人中心入口）、基于 Element Plus 菜单的侧栏（分组可折叠，可收起为图标栏），以及只存在于内存中的胶囊式“已打开页面”标签栏（`src/stores/tabs.ts`，仅桌面宽度），标签栏可用自身的关闭按钮隐藏。个人中心入口打开个人中心弹窗（`src/components/AccountCenter.vue`），其中显示当前账号及其两步验证状态，并提供账号安全、偏好设置和退出登录。明暗切换时，新外观从点击位置以圆形扩散或收拢，浏览器支持时使用 View Transitions API（`src/theme/index.ts`、`src/styles/_theme-transition.scss`）；不支持或开启减少动态效果时直接切换。

“偏好设置”既是一个页面，也可以从顶栏右侧的按钮打开为抽屉（窄屏上是从底部弹出的面板，向下拖动或轻扫标题栏即可关闭）：八组主题色（默认深海蓝，另有晴空蓝、翡翠绿、雾海青、琥珀橙、珊瑚橙、玫瑰红、电光紫），按钮文字颜色按配色选取以保证 4.5:1 的对比度；浅色、深色、跟随系统；视觉模式（标准、色弱友好、灰阶）；液态玻璃的色调与强度；四种页面切换动画；侧栏宽度（拖动侧栏右边缘调整）；侧栏分组同时展开或一次只展开一个；页签栏、加载进度条、页面状态缓存与减少动态效果的开关；界面语言。仅持久化这些非敏感 UI 设置（收起的侧栏和自定义宽度只在设置时写入）。页面状态缓存只保留已打开页签的页面，离开页面时仍会清除密码、令牌和密钥；账号安全页不缓存。Element Plus 通过 `unplugin-vue-components` 与 `unplugin-auto-import` 按需引入，组件样式随组件自动加载。主题 token 在 `src/styles/_themes.scss`，配色元数据在 `src/theme/palettes.ts`，配色名称在各语言的 `appearance.ts`。图标来自 `@iconify-json/lucide`，由 `unplugin-icons` 在构建时编译为组件（`~icons/lucide/<名称>`），运行时不拉取网络，也不写入浏览器存储。

只使用 TypeScript 与 Vue，不新增 `.js` / `.mjs` 文件；启动器 `scripts/dev.ts` 由 Node 直接运行。`tsconfig.json` 开启严格模式与 `noUncheckedIndexedAccess`；`eslint.config.ts`（typescript-eslint 严格类型检查规则、eslint-plugin-vue、eslint-config-prettier）与 Prettier 分工：`bun run format` 先执行 `eslint --fix` 再执行 Prettier，`bun run lint` 只检查。构建产物输出到仓库根目录 `dist/frontend/`。

```text
src/api/          真实认证、节点、软件商店与证书客户端、响应验证、请求活动状态
src/stores/       内存认证状态、内存软件目录及非敏感偏好
src/components/   导航、加载、反馈、认证布局
src/views/        登录、验证、账号安全、概览、节点、软件商店、软件页、证书、DNS 服务商与偏好设置
src/i18n/         vue-i18n 实例、浏览器语言匹配与 Intl 格式化
src/locales/      按语言与功能拆分的语言包
config/           开发代理及浏览器开关
scripts/          Rsbuild 命令行入口
tests/e2e/        真实后端浏览器测试
tests/dev/        开发参数与语言包验证
```

开发服务器关闭了按需编译，启动时编译全部路由分块，避免首次访问时分块请求落到 History 回退页面。部署 `dist/frontend/` 静态资源时需配置 History fallback 与 `/api` 反向代理。`bun run preview` 不会使用开发后端地址；它只用于静态产物预览。接口详情见 [认证契约](docs/auth-api.zh-CN.md)、[节点契约](docs/nodes-api.zh-CN.md)、[软件商店契约](docs/apps-api.zh-CN.md) 与 [证书契约](docs/certs-api.zh-CN.md)，发布步骤见 [开发与发布](../../docs/releasing.zh-CN.md)。
