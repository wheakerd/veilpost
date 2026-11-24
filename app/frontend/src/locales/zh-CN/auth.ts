const auth = {
    layout: {
        meta: '本地控制台',
        status: '等待身份验证',
        statusVerify: '等待两步验证',
        footer: '仅限授权管理员访问',
        entering: '正在进入控制台…',
    },
    login: {
        step: '01 · 账号与密码',
        // {key} is rendered as a keycap.
        keyHint: '按 {key} 登录',
        title: '登录控制台',
        description: '使用你的管理账号继续',
        expired: '登录或验证已过期，请重新登录。',
        retry: '重试连接',
        // {command} is rendered as code.
        setupRequired:
            '尚未创建管理员。请在本机项目终端执行 {command} 初始化账号。',
        recheck: '已创建账号，重新检查',
        username: '账号',
        missingCredentials: '请输入账号和密码。',
        connecting: '正在连接…',
        signingIn: '正在登录…',
        submit: '登录',
        helpTwoFactor: '启用两步验证的账号将在下一步验证身份',
        helpContact: '无法登录时，请联系控制台管理员',
    },
    verify: {
        step: '02 · 动态验证码',
        title: '两步验证',
        totpDescription: '打开验证器，输入当前的 6 位动态验证码',
        recoveryDescription:
            '输入一枚尚未使用的恢复码。每枚恢复码只能使用一次。',
        // {time} is rendered in bold.
        remaining: '本次验证剩余 {time}',
        recoveryLabel: '恢复码',
        totpHelp: '验证码每 30 秒更新，同一组验证码只能使用一次',
        recoveryHelp: '恢复码是启用两步验证时保存的备用凭据',
        recoveryRequired: '请输入恢复码。',
        passed: '验证通过，正在继续…',
        verifying: '正在验证…',
        submit: '验证并登录',
        useRecovery: '使用恢复码',
        useAuthenticator: '使用验证器',
        back: '返回账号登录',
    },
}

export type AuthMessages = typeof auth
export default auth
