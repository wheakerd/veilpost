import type { NodesMessages } from '../zh-CN/nodes'

export default {
    description: '將不同協定的節點，匯集到同一個工作區',
    create: '建立節點',
    listTitle: '節點列表',
    listLabel: '節點',
    columns: { name: '名稱', status: '狀態', id: '節點 ID', actions: '操作' },
    copyId: '複製「{name}」的節點 ID',
    onlineCount: '{count} 個線上',
    form: {
        description:
            '建立後會產生一次性註冊權杖，15 分鐘內有效，只會顯示一次。請輸入目前的密碼確認。',
        name: '節點名稱',
        nameHint: '1–64 個字元，僅用於在控制台中區分節點。',
        submit: '建立並產生註冊權杖',
        missing: '請輸入節點名稱和目前的密碼。',
        missingPassword: '請輸入目前的密碼。',
    },
    enrollment: {
        title: '註冊節點「{name}」',
        instructions:
            '在節點伺服器上執行以下指令，依提示貼上註冊權杖後按 Enter。權杖在 {time} 前有效，只能使用一次，關閉後無法再次查看。',
        command: '註冊指令',
        token: '註冊權杖',
        copyCommand: '複製指令',
        copyToken: '複製權杖',
        note: '權杖透過標準輸入傳遞，請勿寫進指令列參數或指令碼。預設狀態目錄為 {stateDir}，通常需要 root 權限；註冊成功後執行 {runCommand} 保持連線。',
        finish: '完成，隱藏權杖',
        hidden: '已隱藏註冊權杖。節點「{name}」完成註冊後會顯示為線上。',
        expired: '註冊權杖已逾時。可以在節點列表中重新簽發。',
    },
    reissue: {
        open: '重新簽發權杖',
        description:
            '重新簽發後，此節點先前的註冊權杖會立即失效；新權杖 15 分鐘內有效，只會顯示一次。請輸入目前的密碼確認。',
        submit: '簽發新權杖',
    },
    revoke: {
        open: '撤銷',
        description:
            '撤銷後，此節點的簽章立即失效，代理程式會停止連線；若要重新接入，需要建立新節點。請輸入目前的密碼確認。',
        submit: '確認撤銷',
        done: '節點「{name}」已撤銷，其代理程式將無法再連線到控制台。',
    },
    delete: {
        open: '刪除',
        description:
            '刪除後，此節點的紀錄會從列表中移除，且無法復原。請輸入目前的密碼確認。',
        submit: '確認刪除',
        done: '節點「{name}」已刪除',
    },
    state: {
        revoked: '已撤銷',
        pending: '等待註冊',
        expired: '權杖已逾時',
        online: '線上',
        offline: '離線',
    },
    detail: {
        revokedAt: '撤銷於 {time}',
        tokenValidUntil: '註冊權杖在 {time} 前有效',
        tokenExpired: '註冊權杖已失效；可以重新簽發權杖。',
        lastSeen: '最後通訊：{time}',
        neverSeen: '註冊完成，尚未收到代理程式的簽章請求。',
    },
    meta: {
        agentVersion: '代理程式版本',
        unknown: '不明',
        createdAt: '建立時間',
    },
    unavailable: {
        stale: '暫時無法重新整理節點狀態，以下是上次讀取的結果。',
        empty: '暫時無法讀取節點列表，請稍後重新整理頁面。',
    },
    empty: {
        title: '這裡會顯示你的 VPN 節點',
        first: '目前尚未接入 VPN 節點',
        second: '建立節點後，在伺服器上執行註冊指令，即可在這裡查看實際的上線狀態',
    },
    protocols: '入口協定由軟體商店中的 VPN 軟體提供',
    storeLink: '前往軟體商店',
    note: '目前僅提供節點註冊與上線狀態；VPN 設定派送、入口協定與流量監控尚未開放。',
} satisfies NodesMessages
