import type { OverviewMessages } from '../zh-CN/overview'

export default {
    description: '節點、軟體與憑證的即時狀態',
    viewNodes: '查看 VPN 節點',
    summary: '運行摘要',
    online: '{online} / {total} 在線',
    ofTotal: '{count} / {total}',
    stats: {
        nodes: '在線節點',
        nodesDetail: '{pending} 個等待註冊',
        apps: '已安裝軟體',
        appsDetail: '內建目錄中的 VPN 軟體',
        certificates: '有效憑證',
        nextExpiry: '最近到期 {date}',
        noExpiry: '暫無即將到期的憑證',
        twoFactor: '兩步驟驗證',
        enabled: '已啟用',
        disabled: '未啟用',
        manage: '前往設定',
    },
    nodes: {
        title: '節點狀態',
        all: '全部節點',
        name: '名稱',
        status: '狀態',
        lastSeen: '最後通訊',
        states: { online: '在線', offline: '離線', pending: '等待註冊' },
        empty: '還沒有節點。建立節點並在伺服器上執行註冊命令後，這裡會顯示它的在線狀態。',
        unavailable: '暫時無法讀取節點狀態',
    },
    expiry: {
        title: '即將到期的憑證',
        all: '全部憑證',
        empty: '暫無有效憑證',
        unavailable: '暫時無法讀取憑證',
    },
} satisfies OverviewMessages
