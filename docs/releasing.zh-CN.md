# 开发命令与版本发布

[English](releasing.md) | 简体中文

仓库根目录的 Makefile 是本地开发和 GitHub Actions 的统一入口。前端继续使用 Node 26.13.1、Bun 1.4.4；后端使用 Go 1.27.4。具体版本分别从 `app/frontend/.nvmrc`、`app/frontend/package.json` 和 `app/backend/.go-version` 读取。

## 开发与检查

`make start` / `stop` / `restart` / `status` / `logs` 管理当前项目的 Docker Compose 服务。Compose 中后端信任私有网段内的前端代理转发的客户端地址（`VPN_TRUSTED_PROXIES`），仅用于开发。前端默认映射到本机 3000，后端宿主机端口动态分配；容器间使用服务名 `backend:8080`。`FRONTEND_PORT=0` 可为前端选择随机映射，`OPEN_BROWSER=false` 关闭宿主机浏览器自动打开。

首次使用运行 `make admin-create`：终端内交互式输入账号和密码，非交互环境自动生成随机密码、默认账号 `admin`。显式生成可用 `make admin-create GENERATE_PASSWORD=1 USERNAME=admin`。这条命令直接使用 Shell，不依赖 Python；所有凭据通过标准输入传给后端，不出现在命令行中。真实账号和会话保存在 `auth-data` 命名卷，重建 / 停止容器不删除数据。备份需包含 SQLite 文件和 `master.key`；不要使用 `docker compose down -v` 清除开发数据。

前端源码挂载到容器并支持 HMR；后端修改后运行 `make restart` 重新构建。`docker/soft-router/` 中的 VPN 部署参考配置不参与此编排。

## 在 Linux 宿主机安装

选择与宿主机架构（`amd64` 或 `arm64`）匹配的发布包，解压前用 `SHA256SUMS` 校验已下载的归档。

1. 通过宿主机的服务管理器，以专用账号运行 `veilpost-api`。数据放在 `/var/lib/veilpost` 等持久化目录。[后端配置](../app/backend/README.zh-CN.md#配置)
2. 通过 HTTPS 提供前端静态资源并配置 History fallback。把 `/api/` 代理到后端，按部署环境设置 `VPN_PUBLIC_ORIGIN` 和 `VPN_TRUSTED_PROXIES`，代理读取超时需大于长轮询的 25 秒。
3. 在各节点宿主机直接运行 `veilpost-agent`。启用 Docker Engine 安装时，登记该节点的签名 APT 软件源。[节点代理配置](../app/agent/README.zh-CN.md)

## 生产环境创建管理员

Linux 后端发布包附带 `admin-create.sh`，与 `veilpost-api` 放在同一目录即可执行。使用后端服务的数据目录和运行用户：

```bash
VPN_DATA_DIR=/var/lib/veilpost GENERATE_PASSWORD=1 USERNAME=admin sh ./admin-create.sh
```

如后端二进制放在其他位置，加上 `VPN_API_BIN=/path/veilpost-api`。脚本不需要 Python、Node、Go SDK、Docker 或 Make（原生二进制模式）；默认凭据保存在脚本目录的 `.runtime/admin-credentials/`，也可用 `VPN_CREDENTIALS_DIR` 指定可写的私有目录。账号创建不会重置已有密码。

忘记密码或丢失验证器及全部恢复码时，用同一脚本执行本地恢复，两者都会撤销该账号的全部会话：

```bash
VPN_DATA_DIR=/var/lib/veilpost ADMIN_ACTION=reset-password GENERATE_PASSWORD=1 USERNAME=admin sh ./admin-create.sh
VPN_DATA_DIR=/var/lib/veilpost ADMIN_ACTION=reset-two-factor USERNAME=admin sh ./admin-create.sh
```

后端部署在反向代理后面时，设置 `VPN_TRUSTED_PROXIES` 为代理地址（IP 或 CIDR），登录限流才能按真实客户端生效。

随机密码由系统 [OpenSSL rand](https://docs.openssl.org/master/man1/openssl-rand/) 生成，读取 32 个随机字节，输出 64 位十六进制字符。凭据文件权限为 `600`、目录为 `700`，明文不输出到终端；请保存到密码管理器后删除该文件。

缺少所需命令时读取 `/etc/os-release`（或 `/usr/lib/os-release`），安装 OpenSSL/coreutils 后重新检查并继续：Debian/Ubuntu 使用 apt-get，Fedora/RHEL 系使用 dnf/yum，Alpine 使用 apk，Arch 系使用 pacman，openSUSE/SLES 使用 zypper。只安装缺失工具对应的软件包，不进行全系统升级或更换软件源；Arch 使用已有同步数据库，过期时需要管理员先维护系统。安装使用 root 或 sudo，非交互模式要求 `sudo -n` 可用；不支持的发行版、权限不足、工具仍缺失或安装失败都会明确退出，不会继续创建账号。

## 检查与打包

```bash
make install
PLAYWRIGHT_CHANNEL=chrome make check
make build
make release VERSION=v0.1.0
```

原生检查需要固定版本 Go、Node、Bun、GNU Make、Python 3.11+ 和 C 编译器。浏览器测试默认四个 worker，使用 4173 及随机后端端口；通过真实 Go/SQLite 服务验证，不使用接口 Mock。临时测试账号、数据库和密钥在测试结束后销毁。首次安装浏览器可在 app/frontend 中运行 `bunx playwright install chromium`。`make build` 将前端产物输出到仓库根目录 `dist/frontend/`，并产出 `app/backend/bin/veilpost-api` 与 `app/agent/bin/veilpost-agent`。

可通过 `make native-start` / `native-stop` / `native-restart` / `native-status` 使用原生开发模式。与 Compose 的数据库相互独立，切换前先停止另一种模式，避免占用相同前端端口。

## 发布产物

支持 `vMAJOR.MINOR.PATCH` 和带预发布标识的版本，例如 `v0.1.0-rc.1`；不接受前导零、路径、空白或构建元数据 `+...`。

```text
dist/releases/v0.1.0/
  veilpost-v0.1.0-frontend.tar.gz
  veilpost-v0.1.0-linux-amd64.tar.gz
  veilpost-v0.1.0-linux-arm64.tar.gz
  veilpost-v0.1.0-agent-linux-amd64.tar.gz
  veilpost-v0.1.0-agent-linux-arm64.tar.gz
  manifest.json
  SHA256SUMS
```

- 前端包仅包含生产构建的 `frontend/` 静态目录，不包含源码目录或 node_modules。
- 两个后端包包含无 CGO 依赖的 Linux `veilpost-api`、独立管理员初始化脚本 `admin-create.sh`、非敏感配置示例和本说明。Go 使用 `-trimpath`，并通过链接参数写入版本、提交号；运行 `./veilpost-api --version` 查看。
- 两个代理程序包包含无 CGO 依赖的 Linux `veilpost-agent` 及其说明，在节点服务器上使用；同样写入版本、提交号，运行 `./veilpost-agent version` 查看。
- `manifest.json` 记录版本、提交、提交时间、工作区状态与各包的 SHA-256；`SHA256SUMS` 同时校验五个归档和 manifest。
- 归档使用排序后的文件、固定权限和提交时间，不携带机器路径、文件所有者或生成时间；拒绝打包符号链接。相同版本、提交、源文件和工具链可生成一致的归档。
- 只打包明确列出的构建文件，不包含 `.env`、运行记录、VPN 配置副本或密钥。`backend.env.example` 仅作参考，程序不会自动加载它。

本地允许从未提交的工作区生成预览包，但 manifest 会标记 `dirty: true`，发布脚本拒绝上传这种包。CI 打包要求工作区干净，且版本标签指向当前 HEAD。已有输出目录不会覆盖；需要重复验证同版本时可指定另一个被忽略的目录，例如：

```bash
RELEASE_DIR=.runtime/release-preview make release VERSION=v0.1.0-rc.1
```

后端包解压后可直接运行：

```bash
sha256sum --ignore-missing --check SHA256SUMS
tar -xzf veilpost-v0.1.0-linux-amd64.tar.gz
./veilpost-api --version
VPN_HTTP_ADDR=127.0.0.1:8080 VPN_DATA_DIR=/var/lib/veilpost VPN_PUBLIC_ORIGIN=https://console.example ./veilpost-api
```

发布工作流提供用于 Linux 宿主机安装的归档。[宿主机安装说明](#在-linux-宿主机安装)

## GitHub Actions

### CI

`.github/workflows/ci.yml` 在分支 push、Pull Request、手动触发时执行，也供发布流程复用。使用 Ubuntu 24.04、仓库固定版本工具链和 Playwright Chromium，执行：

1. `make install` 安装锁定依赖。
2. 安装 Chromium 及运行器上的浏览器系统依赖。
3. 使用固定版本 actionlint 检查工作流。
4. `make check` 和 `make build`。
5. 失败时保留浏览器诊断产物 7 天。
6. 使用一次性 Compose 环境检查认证和数据持久化。

GitHub Actions 从 `.github/workflows/` 发现工作流 YAML。适配其他 CI 平台时，在运行器上安装仓库指定版本的工具链和 Playwright Chromium，再从仓库根目录运行 `make install`、`make check` 和 `make build`。支持 Docker 的运行器还可执行一次性 Compose 冒烟检查。

### 自动发布

`.github/workflows/release.yml` 由推送 `v*` 标签触发；不符合上述版本格式的标签会失败，不会发布。

1. 复用完整 CI，通过后才进入打包。
2. 从标签对应的干净 checkout 构建五个归档（前端、两个架构的后端和两个架构的节点代理），上传临时 Actions artifact。
3. 独立发布 job 下载产物，重新检查清单、SHA-256、版本与标签提交一致性。
4. GitHub CLI 创建 Release 并附加全部七个文件（五个归档、`manifest.json` 与 `SHA256SUMS`），生成发布说明。带 `-rc.1` 等后缀的版本发布为 prerelease，不标记 Latest。

所有外部 Actions 固定到完整 commit SHA，并在注释记录版本。检查与打包 job 只有 `contents: read`；只有发布 job 获得 `contents: write`，且仅发布步骤设置 `GH_TOKEN`。使用 GitHub 自动提供的 `GITHUB_TOKEN`，无需添加个人访问令牌。仓库或组织必须允许这些 Actions 以及所请求的发布权限。

先提交代码与工作流，并把对应提交推送到仓库，再创建和推送版本标签：

```bash
git tag -a v0.1.0 -m "Release v0.1.0"
git push origin refs/tags/v0.1.0
```

自动化不会创建或移动 Git 标签，不覆盖已有 Release。GitHub CLI 先创建草稿、上传资产，成功后公开；若上传中断留下草稿，需要人工核对并清理该失败草稿后再重跑，避免覆盖已有版本。

## 本地发布前校验

在干净、已打版本标签的提交上生成发布包后，可以进行完全本地的发布前检查：

```bash
VERSION=v0.1.0 GITHUB_REPOSITORY=wheakerd/veilpost \
  python3 scripts/publish.py --dry-run
```

该命令不联系 GitHub。实际手动发布使用 `GITHUB_REPOSITORY=wheakerd/veilpost make publish VERSION=v0.1.0`，需要已登录的 GitHub CLI 或有适当权限的 `GH_TOKEN`；远端标签必须已存在。

本地检查不能证明 GitHub 托管运行器已成功运行。首次远端发布仍需在提交并推送工作流后，检查 Actions 执行结果及 Release 附件。
