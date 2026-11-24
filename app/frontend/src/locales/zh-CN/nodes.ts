const nodes = {
    description: '将不同协议的节点，汇集到同一个工作空间',
    create: '创建节点',
    listTitle: '节点列表',
    listLabel: '节点',
    columns: { name: '名称', status: '状态', id: '节点 ID', actions: '操作' },
    copyId: '复制“{name}”的节点 ID',
    onlineCount: '{count} 个在线',
    form: {
        description:
            '创建后会生成一次性注册令牌，15 分钟内有效，只显示一次。请输入当前密码确认。',
        name: '节点名称',
        nameHint: '1–64 个字符，仅用于在控制台中区分节点。',
        submit: '创建并生成注册令牌',
        missing: '请输入节点名称和当前密码。',
        missingPassword: '请输入当前密码。',
    },
    enrollment: {
        title: '注册节点“{name}”',
        instructions:
            '在节点服务器上运行以下命令，按提示粘贴注册令牌后回车。令牌在 {time} 前有效，只能使用一次，关闭后无法再次查看。',
        command: '注册命令',
        token: '注册令牌',
        copyCommand: '复制命令',
        copyToken: '复制令牌',
        // {stateDir} and {runCommand} are rendered as code.
        note: '令牌通过标准输入传递，不要写进命令行参数或脚本。默认状态目录为 {stateDir}，通常需要 root 权限；注册成功后运行 {runCommand} 保持连接。',
        finish: '完成，隐藏令牌',
        hidden: '注册令牌已隐藏。节点“{name}”完成注册后会显示为在线。',
        expired: '注册令牌已过期。可以在节点列表中重新签发。',
    },
    reissue: {
        open: '重新签发令牌',
        description:
            '重新签发后，该节点之前的注册令牌立即失效；新令牌 15 分钟内有效，只显示一次。请输入当前密码确认。',
        submit: '签发新令牌',
    },
    revoke: {
        open: '吊销',
        description:
            '吊销后，该节点的签名立即失效，代理程序会停止连接；重新接入需要创建新节点。请输入当前密码确认。',
        submit: '确认吊销',
        done: '节点“{name}”已吊销，其代理程序将无法再连接控制台。',
    },
    delete: {
        open: '删除',
        description:
            '删除后，该节点的记录将从列表中移除，无法恢复。请输入当前密码确认。',
        submit: '确认删除',
        done: '节点“{name}”已删除',
    },
    state: {
        revoked: '已吊销',
        pending: '等待注册',
        expired: '令牌已过期',
        online: '在线',
        offline: '离线',
    },
    detail: {
        revokedAt: '吊销于 {time}',
        tokenValidUntil: '注册令牌在 {time} 前有效',
        tokenExpired: '注册令牌已失效；可以重新签发令牌。',
        lastSeen: '最后通信：{time}',
        neverSeen: '注册完成，尚未收到代理程序的签名请求。',
    },
    meta: {
        agentVersion: '代理程序版本',
        unknown: '未知',
        createdAt: '创建时间',
    },
    unavailable: {
        stale: '暂时无法刷新节点状态，下面是上次读取的结果。',
        empty: '暂时无法读取节点列表，请稍后刷新页面。',
    },
    empty: {
        title: '这里将显示你的 VPN 节点',
        first: '当前尚未接入 VPN 节点',
        second: '创建节点后，在服务器上运行注册命令，即可在这里查看真实的在线状态',
    },
    protocols: '入口协议由软件商店中的 VPN 软件提供',
    storeLink: '前往软件商店',
    note: '当前仅提供节点注册与在线状态；VPN 配置下发、入口协议与流量监控尚未开放。',
}

export type NodesMessages = typeof nodes
export default nodes
