# Veilpost

自托管的多协议 VPN 节点控制台。

[English](README.md) | 简体中文

Vue 前端、Go + Gin 后端与 Docker Compose 开发环境。登录、会话恢复、TOTP 两步验证、恢复码和账号安全设置均调用真实后端；不提供默认账号或模拟认证。界面采用参照 iOS 26 的液态玻璃风格，提供浅色 / 深色两种模式和八组主题色；登录页是缓慢旋转的线框地球，控制台有可折叠分组、可拖动调宽的侧栏、工具栏和已打开页面标签栏；“偏好设置”抽屉提供主题色、视觉模式（含色弱友好与灰阶）、液态玻璃、页面切换动画、页签栏、加载进度条和页面状态缓存等选项。界面支持简体中文、English、繁體中文和 Русский：默认跟随浏览器语言，可在“偏好设置”中切换，选择只保存在当前浏览器。软件商店从随控制台发布的内置目录（Xray-core、Hysteria 2、AmneziaWG）启用 VPN 软件并在控制台中配置，部署到节点将在节点第 2 阶段提供。证书通过 ACME（默认 Let's Encrypt，可自定义目录与 EAB）和 DNS-01 申请并自动续期，支持 Cloudflare、阿里云 DNS、腾讯云 DNSPod 与 AWS Route 53；凭据和私钥用 `master.key` 加密保存，后端需要能访问 ACME 服务和所选 DNS 服务商的 API。节点第 1 阶段已实现：在“VPN 节点”页创建节点并获得一次性注册令牌，节点上的 `veilpost-agent` 注册后通过签名长轮询上报在线状态。

## 使用 Docker 继续开发

```bash
make start          # 构建并启动前后端，等待健康检查，打开默认浏览器
make status         # 查看容器状态及实际访问地址
make admin-create   # 终端内交互式创建；非交互环境自动生成随机密码
make admin-reset-password USERNAME=admin   # 忘记密码时重置，并撤销该账号全部会话
make admin-reset-two-factor USERNAME=admin # 验证器和恢复码都丢失时移除两步验证
make logs
make restart        # 重新构建并重建容器，保留账号和会话数据
make stop           # 停止本项目容器，保留数据卷
```

默认入口为 <http://127.0.0.1:3000>。使用 `FRONTEND_PORT=0 make start` 可让 Docker 分配前端宿主机端口；`OPEN_BROWSER=false make start` 关闭自动打开浏览器。前端源码通过绑定挂载提供 HMR；修改后端源码后执行 `make restart`。

在终端中也可以运行 `make admin-create GENERATE_PASSWORD=1 USERNAME=admin`，生成 64 位随机密码并创建账号。这条命令直接运行 POSIX Shell，不依赖 Python；也可执行 `GENERATE_PASSWORD=1 sh scripts/admin-create.sh`。使用系统 `openssl rand -hex 32` 生成密码；缺失工具时读取 Linux 的 `os-release`，通过 apt-get、dnf/yum、apk、pacman 或 zypper 安装所需的 OpenSSL/coreutils，再检查工具并继续。安装需要 root 或可用的 sudo；非交互执行使用 `sudo -n`，权限不足或安装失败时明确退出。

自动生成模式的账号名默认为 `admin`；不会预置账号，也不会覆盖已有账号或重置密码。密码通过标准输入交给后端，只保存到命令输出所指向的 `.runtime/admin-credentials/` 文本文件（目录权限 `700`，文件权限 `600`，Git 已忽略），不输出到终端或日志。`VPN_CREDENTIALS_DIR` 可指定其他私有目录。请将其保存到密码管理器后删除本地明文文件。若创建结果未确认，文件会保留，命令不会自动重试。重置密码沿用相同的密码输入与保存方式。

生产环境直接运行二进制时，设置 `VPN_API_BIN` 和与服务一致的 `VPN_DATA_DIR`；发布包自带同一 Shell 脚本，无需安装 Python、Node 或 Go SDK。示例见 [发布文档](docs/releasing.zh-CN.md)。其余开发和打包命令仍使用 Python。

容器间通过 `backend:8080` 通信；后端宿主机映射端口由 Docker 分配，只绑定 `127.0.0.1`。项目名默认为 `veilpost-dev`，不会启动或修改 `docker/soft-router/` 中的参考部署。

Make 启动器会利用项目已有的 Go 模块缓存加速镜像构建，并在镜像中重新校验；直接使用 Docker Compose 时默认从空缓存联网获取锁定依赖。

认证数据保存在独立的 `auth-data` 命名卷中，包括 SQLite 数据库和加密密钥。重建容器不会清空用户数据。不要删除该卷；备份时应停止后端并同时保存数据库及 `master.key`。

## 检查、构建和发布

```bash
make install
PLAYWRIGHT_CHANNEL=chrome make check
make build
make release VERSION=v0.1.0
```

本地检查需要 Go、Node、Bun 和 Python，版本见各子项目配置。浏览器测试启动真实 Go + SQLite 服务，使用独立临时数据库和随机测试账号；没有 MSW 或 Playwright 接口拦截。

GitHub Actions 在分支 push / PR 时检查；推送版本标签后，检查通过才会发布前端包、Linux amd64 / arm64 后端包和校验和。详细说明见 [开发与发布](docs/releasing.zh-CN.md)。

## 原生开发

需要原生调试时可使用 `make native-start`、`make native-status`、`make native-restart`、`make native-stop`。后端使用系统分配的随机端口，再通过命令行传给前端；与 Compose 使用独立数据目录。两种模式默认都使用前端 3000 端口，切换前先停止另一种模式。

## 结构与边界

```text
app/frontend/     Vue 界面、真实 API 客户端、浏览器测试
app/backend/      Gin API、SQLite 认证与节点存储、管理员命令与测试
app/agent/        veilpost-agent 节点代理程序（Go，无 CGO）
compose.yaml      前后端开发编排，独立网络和命名卷
docker/frontend/  前端开发镜像与 Nginx 参考
docker/backend/   Go 构建及后端运行镜像
docker/soft-router/  已脱敏的 VPN 部署参考配置
docker/node/      节点镜像（AmneziaWG 从上游固定版本源码构建，开发环境不启动）
scripts/          启停、测试、发布脚本
.github/          CI 与标签发布工作流
```

当前真实后端覆盖认证、健康检查以及节点注册与在线状态；VPN 配置下发、入口协议和监控尚未实现。没有节点时节点页保持真实的未接入状态，不显示虚构数据。设计见 [节点功能设计](docs/nodes-design.zh-CN.md)。

参考文档：[前端](app/frontend/README.zh-CN.md)、[后端](app/backend/README.zh-CN.md)、[认证契约](app/frontend/docs/auth-api.zh-CN.md)、[节点契约](app/frontend/docs/nodes-api.zh-CN.md)、[代理程序](app/agent/README.zh-CN.md)。VPN 部署参考配置的占位符说明见 [docker/soft-router](docker/soft-router/README.zh-CN.md)，文件完整性可用 `python3 scripts/verify-docker-copy.py` 校验。
