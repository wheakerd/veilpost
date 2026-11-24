# veilpost-agent

[English](README.md) | 简体中文

节点代理程序，对应 [节点设计](../../docs/nodes-design.zh-CN.md) 第 1 阶段：用一次性令牌注册、以 Ed25519 签名请求长轮询控制台并上报在线状态与版本。本阶段不管理任何容器，不应用 VPN 配置。接口契约见 [nodes-api.md](../frontend/docs/nodes-api.zh-CN.md)。

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
