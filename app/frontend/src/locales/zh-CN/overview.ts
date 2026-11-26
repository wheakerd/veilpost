const overview = {
    description: '节点、软件与证书的实时状态',
    viewNodes: '查看 VPN 节点',
    destinations: '管理入口',
    summary: '运行摘要',
    online: '{online} / {total} 在线',
    ofTotal: '{count} / {total}',
    stats: {
        nodes: '在线节点',
        nodesDetail: '{pending} 个等待注册',
        apps: '已安装软件',
        appsDetail: '内置目录中的 VPN 软件',
        certificates: '有效证书',
        nextExpiry: '最近到期 {date}',
        noExpiry: '暂无即将到期的证书',
        twoFactor: '两步验证',
        enabled: '已启用',
        disabled: '未启用',
        manage: '去设置',
    },
    nodes: {
        title: '节点状态',
        all: '全部节点',
        name: '名称',
        status: '状态',
        lastSeen: '最后通信',
        states: { online: '在线', offline: '离线', pending: '等待注册' },
        empty: '还没有节点。创建节点并在服务器上运行注册命令后，这里显示它的在线状态。',
        unavailable: '暂时无法读取节点状态',
    },
    expiry: {
        title: '即将到期的证书',
        all: '全部证书',
        empty: '暂无有效证书',
        unavailable: '暂时无法读取证书',
    },
}

export type OverviewMessages = typeof overview
export default overview
