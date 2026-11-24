// Simplified Chinese is the source of truth: every other language pack must
// provide exactly these keys (checked by `satisfies` in each pack).
const common = {
    language: {
        // The language's own name, shown in the language selector.
        self: '简体中文',
        // Short label on the sign-in language switch.
        short: '简',
    },
    app: {
        name: 'Veilpost',
        description: '自建多 VPN 节点的统一控制面板',
        home: 'Veilpost 首页',
    },
    routes: {
        login: '登录',
        verify: '两步验证',
        overview: '网络概览',
        nodes: 'VPN 节点',
        appearance: '偏好设置',
        security: '账号安全',
        certificates: '证书',
        dnsProviders: 'DNS 服务商',
        notFound: '页面不存在',
        store: '软件商店',
        installedApp: '已安装软件',
    },
    shell: {
        account: {
            title: '个人中心',
            open: '打开个人中心',
            twoFactorOn: '已启用两步验证',
            twoFactorOff: '未启用两步验证',
        },
        hideTabs: '关闭页签栏',
        skipLink: '跳转到主要内容',
        navigation: '主导航',
        groups: {
            workspace: '工作空间',
            software: '软件',
            security: '安全',
            settings: '设置',
        },
        breadcrumb: '控制台',
        signOut: '退出登录',
        installedApps: '已安装软件',
        collapse: '收起侧栏',
        expand: '展开侧栏',
        refresh: '刷新当前页',
        // {mode} is one of modes.
        mode: '界面外观：{mode}',
        modes: { light: '浅色', dark: '深色', system: '跟随系统' },
        language: '界面语言',
        followBrowser: '跟随浏览器',
        preferences: '偏好设置',
        clock: '本地时间',
        version: 'v{version}',
        openPages: '已打开的页面',
        closeTab: '关闭“{title}”',
        closeOthers: '关闭其他',
    },
    loading: {
        title: '正在载入控制台',
        description: '正在恢复你的登录状态，请稍候。',
    },
    notFound: {
        title: '这个页面不存在',
        description: '请检查地址，或回到网络概览继续',
        back: '返回网络概览',
    },
    status: {
        unavailable: '暂不可用',
        loading: '正在读取',
        notConnected: '尚未接入',
    },
    discard: {
        message: '放弃尚未保存的修改？',
        confirm: '放弃修改',
        keep: '继续编辑',
    },
    password: {
        label: '密码',
        current: '当前密码',
        show: '显示',
        hide: '隐藏',
        showLabel: '显示密码',
        hideLabel: '隐藏密码',
        capsLock: '大写锁定已开启',
    },
    code: {
        label: '验证码',
        required: '请输入 6 位数字验证码。',
    },
    actions: {
        cancel: '取消',
        copied: '已复制',
        copyFailed: '无法自动复制，请选中文字手动复制。',
    },
    time: {
        justNow: '刚刚',
    },
}

export type CommonMessages = typeof common
export default common
