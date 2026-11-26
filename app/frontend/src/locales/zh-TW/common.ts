import type { CommonMessages } from '../zh-CN/common'

export default {
    language: {
        self: '繁體中文',
        // Short label on the sign-in language switch.
        short: '繁',
    },
    app: {
        name: 'Veilpost',
        description: '自建多 VPN 節點的統一控制台',
        home: 'Veilpost 首頁',
    },
    routes: {
        login: '登入',
        verify: '兩步驟驗證',
        overview: '網路概覽',
        nodes: 'VPN 節點',
        appearance: '偏好設定',
        security: '帳號安全',
        certificates: '憑證',
        dnsProviders: 'DNS 服務商',
        notFound: '找不到頁面',
        store: '軟體商店',
        installedApp: '已安裝軟體',
    },
    shell: {
        account: {
            title: '個人中心',
            open: '開啟個人中心',
            profile: '資料',
            loginSecurity: '登入與安全',
            twoFactorOn: '已啟用兩步驟驗證',
            twoFactorOff: '未啟用兩步驟驗證',
        },
        hideTabs: '關閉頁籤列',
        skipLink: '跳至主要內容',
        navigation: '主要導覽',
        groups: {
            workspace: '工作空間',
            software: '軟體',
            security: '安全',
            settings: '設定',
        },
        breadcrumb: '控制台',
        signOut: '登出',
        installedApps: '已安裝軟體',
        collapse: '收合側欄',
        expand: '展開側欄',
        refresh: '重新整理此頁',
        mode: '介面外觀：{mode}',
        modes: { light: '淺色', dark: '深色' },
        language: '介面語言',
        followBrowser: '跟隨瀏覽器',
        preferences: '偏好設定',
        clock: '本地時間',
        version: 'v{version}',
        openPages: '已開啟的頁面',
        closeTab: '關閉「{title}」',
        closeOthers: '關閉其他',
    },
    loading: {
        title: '正在載入控制台',
    },
    notFound: {
        title: '這個頁面不存在',
        description: '請檢查網址，或回到網路概覽繼續',
        back: '返回網路概覽',
    },
    status: {
        unavailable: '暫時無法使用',
        loading: '讀取中',
        notConnected: '尚未接入',
    },
    discard: {
        message: '放棄尚未儲存的修改？',
        confirm: '放棄修改',
        keep: '繼續編輯',
    },
    password: {
        label: '密碼',
        current: '目前的密碼',
        showLabel: '顯示密碼',
        hideLabel: '隱藏密碼',
        capsLock: '大寫鎖定已開啟',
    },
    code: {
        label: '驗證碼',
        required: '請輸入 6 位數驗證碼。',
    },
    actions: {
        cancel: '取消',
        copied: '已複製',
        copyFailed: '無法自動複製，請選取文字後手動複製。',
    },
    time: {
        justNow: '剛剛',
    },
} satisfies CommonMessages
