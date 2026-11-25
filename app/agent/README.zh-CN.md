# veilpost-agent

[English](README.md) | 简体中文

节点代理程序使用一次性令牌注册，通过 Ed25519 签名轮询上报状态，并在已启用安装的宿主机接收 Docker Engine 安装任务。[节点设计](../../docs/nodes-design.zh-CN.md) · [接口契约](../frontend/docs/nodes-api.zh-CN.md)

纯 Go 标准库实现，无第三方依赖，`CGO_ENABLED=0` 构建为单个二进制。Go 版本与 `app/backend/.go-version` 一致，模块路径为 `github.com/wheakerd/veilpost/app/agent`。

## 使用

1. 在控制台“VPN 节点”页创建节点，输入当前密码后得到注册命令和一次性令牌（15 分钟有效，只显示一次）。
2. 在节点上运行注册命令，按提示粘贴令牌后回车：

    ```bash
    veilpost-agent enroll --server https://console.example --token-stdin
    ```

    令牌只从标准输入读取，不接受命令行参数或环境变量。服务器地址必须为 HTTPS；只有回环地址（本地开发）允许 HTTP。

3. 保持连接：

    ```bash
    veilpost-agent run
    ```

`--state-dir` 可替换默认状态目录 `/var/lib/veilpost-agent`。目录权限为 `0700`，其中 `identity.key`（Ed25519 私钥，PKCS#8 PEM）与 `node.json`（服务器地址、节点 ID、名称）权限均为 `0600`。已有身份时 `enroll` 拒绝覆盖；注册失败时删除新生成的私钥。`run` 拒绝组或其他用户可访问的目录或私钥文件。私钥从不离开节点。

退出码：`0` 收到 SIGINT / SIGTERM 正常停止；`1` 运行失败；`2` 参数错误；`3` 节点已在控制台吊销。作为系统服务运行时，应让退出码 3 不再自动重启（例如 systemd 的 `RestartPreventExitStatus=3`）。重新接入需要在控制台创建新节点并重新注册。

运行时，代理程序在每次长轮询（最长 25 秒）结束后立即发起下一次；连接失败按 5 秒到 5 分钟指数退避。系统时钟与控制台相差超过 60 秒时请求会被拒绝，请启用 NTP 时间同步。日志输出到标准错误，不包含令牌或私钥。

## 在节点宿主机安装 Docker Engine

在 Debian 或 Ubuntu 宿主机准备由 root 拥有的目录，例如 `/etc/veilpost/docker-sources`。每个 `<source-id>.sources` 文件是一组 APT 软件源，控制台显示文件名中的源标识。标识使用小写字母、数字和连字符。每段 deb822 配置通过 `Signed-By` 指向一个由 root 拥有的密钥文件，使用绝对路径。配置可包含安装依赖所需的签名发行版软件源。仅授予 root 写入目录、配置和密钥的权限。

例如在 `mirror.sources` 中写入以下配置，并换成节点信任的源地址、发行版代号和密钥文件。

```text
Types: deb
URIs: https://packages.example/docker/linux/ubuntu
Suites: noble
Components: stable
Signed-By: /etc/apt/keyrings/docker.asc
```

以 root 运行节点代理的宿主机服务，并启用软件源目录。

```bash
veilpost-agent run --docker-sources-dir /etc/veilpost/docker-sources
```

在 **VPN 节点** 中对在线节点选择 **安装 Docker**，选择软件源并输入当前密码确认。节点代理通过所选软件源安装 `docker-ce`、`docker-ce-cli` 和 `containerd.io`。任务会单独报告节点已安装 Docker Engine 的情况。控制台显示结果待核实时，先检查节点的 APT 状态再提交新任务。

## 开发

```bash
make fmt     # gofmt
make vet
make test    # 竞态检测单元测试
make build   # bin/veilpost-agent（CGO_ENABLED=0）
make check   # fmt-check、vet、test、build
```

Makefile 优先使用仓库 `.runtime/toolchains/` 中的 Go，也支持 `GO=/path/to/go`；构建与模块缓存位于仓库 `.runtime/`。版本号通过 `-ldflags "-X main.version=... -X main.commit=..."` 注入，默认 `dev`。

测试位于 `tests/agent/`，作为外部包只调用公开接口，所需的协议常量与签名函数在 `internal/agent/export.go` 中导出。测试覆盖签名格式（与后端固定同一组已知签名向量）、状态目录与密钥文件权限、注册失败清理、签名请求、退避和吊销退出。与真实控制台的端到端联调测试本阶段暂不实施。
