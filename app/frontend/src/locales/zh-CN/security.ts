// Messages with a {count} may use plural forms separated by "|" in other
// languages (English: one | other; Russian: one | few | many).
const security = {
    description: '管理登录验证方式，保护你的网络控制台',
    twoFactor: {
        title: '两步验证',
        description: '账号密码之外，再验证一次你的身份',
        enabled: '已启用',
        disabled: '未启用',
        protectedTitle: '验证器正在保护你的账号',
        unprotectedTitle: '使用验证器保护登录',
        protectedBody:
            '登录时输入验证器动态码；无法使用验证器时，可使用已保存的恢复码。',
        unprotectedBody:
            '绑定验证器后，登录时还需要输入 6 位动态验证码。支持常见的 TOTP 验证器。',
        recoveryRemaining: '剩余 {count} 个可用恢复码',
        recoveryLow: '剩余 {count} 个可用恢复码，建议重新生成。',
        regenerate: '重新生成恢复码',
        enable: '启用两步验证',
        disable: '关闭两步验证',
    },
    confirm: {
        title: '先确认是你本人',
        description: '输入当前密码，开始绑定验证器',
        submit: '继续绑定',
    },
    setup: {
        scanTitle: '扫描二维码',
        scanBody: '在验证器中选择“添加账号”，扫描下方二维码',
        qrAlt: '验证器绑定二维码',
        added: '添加后，验证器会显示每 30 秒更新的 6 位验证码',
        qrUnavailable: '二维码暂时无法显示，请手动添加账号。',
        manualSummary: '无法扫码？手动输入密钥',
        manualKey: '类型选择“基于时间”，输入此绑定密钥：',
        copyKey: '复制密钥',
        verifyTitle: '验证并完成绑定',
        verifyBody: '输入新添加账号的动态验证码。验证通过后才会启用。',
        submit: '验证并启用',
        cancel: '取消绑定',
        expired: '本次绑定已过期，请重新开始设置。',
    },
    recovery: {
        confirmFirst: '请先确认已保存恢复码',
        title: '保存你的恢复码',
        body: '这些恢复码仅在此处显示。请保存在密码管理器等安全位置；丢失验证器时，每枚恢复码可以登录一次。',
        listLabel: '恢复码',
        copy: '复制恢复码',
        copied: '已复制，请保存在安全的位置。',
        saved: '我已安全保存恢复码',
        finishSetup: '完成设置',
        finish: '完成',
        leaveTitle: '恢复码尚未保存',
        leave: '仍然离开',
        stay: '留在此页',
        leaveConfirm:
            '恢复码仅在此处显示。尚未保存时离开，可能无法找回。确定离开？',
        regenerated: '新的恢复码已生效，旧恢复码已全部失效。',
        enabled: '两步验证已启用，下次登录时需要验证身份。',
    },
    regenerate: {
        title: '重新生成恢复码',
        description:
            '生成后，所有旧恢复码立即失效。请输入当前密码和验证器动态码确认。',
        submit: '生成新的恢复码',
    },
    disable: {
        title: '关闭两步验证',
        description:
            '关闭后，登录只需账号和密码，现有恢复码也将失效。请输入当前密码和验证器动态码确认。',
        submit: '确认关闭两步验证',
        done: '两步验证已关闭',
    },
    password: {
        title: '登录密码',
        description: '定期更换密码；修改后，其他设备需要重新登录。',
        change: '修改密码',
        next: '新密码',
        confirm: '确认新密码',
        hint: '至少 12 个字符，不能与当前密码相同',
        save: '保存新密码',
        missing: '请输入当前密码和新密码。',
        mismatch: '两次输入的新密码不一致。',
        changed: '密码已修改，其他设备上的登录已退出。',
    },
    sessions: {
        noOthers: '没有其他会话可退出',
        title: '登录会话',
        unavailable: '暂时无法读取会话信息，请稍后刷新页面。',
        loading: '正在读取其他会话…',
        others: '除当前浏览器外，还有 {count} 个会话处于登录状态',
        onlyCurrent: '只有当前浏览器处于登录状态',
        revoke: '退出其他会话',
        revoked: '其他会话已退出',
    },
    // {command} is rendered as code.
    lostEverything:
        '验证器和恢复码都丢失时，请由服务器管理员执行 {command} 恢复访问',
}

export type SecurityMessages = typeof security
export default security
