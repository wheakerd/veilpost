import type { AuthMessages } from '../zh-CN/auth'

export default {
    layout: {
        meta: '本機控制台',
        status: '等待身分驗證',
        statusVerify: '等待兩步驟驗證',
        footer: '僅限授權管理員存取',
        entering: '正在進入控制台…',
    },
    login: {
        step: '01 · 帳號與密碼',
        // {key} is rendered as a keycap.
        keyHint: '按 {key} 登入',
        title: '登入控制台',
        description: '使用你的管理帳號繼續',
        expired: '登入或驗證已逾時，請重新登入。',
        retry: '重試連線',
        setupRequired:
            '尚未建立管理員。請在本機專案的終端機執行 {command} 初始化帳號。',
        recheck: '已建立帳號，重新檢查',
        username: '帳號',
        missingCredentials: '請輸入帳號和密碼。',
        connecting: '正在連線…',
        signingIn: '正在登入…',
        submit: '登入',
        helpTwoFactor: '已啟用兩步驟驗證的帳號，會在下一步驗證身分',
        helpContact: '無法登入時，請聯絡控制台管理員',
    },
    verify: {
        step: '02 · 動態驗證碼',
        title: '兩步驟驗證',
        totpDescription: '開啟驗證器，輸入目前的 6 位數動態驗證碼',
        recoveryDescription:
            '輸入一組尚未使用的復原碼。每組復原碼只能使用一次。',
        remaining: '本次驗證剩餘 {time}',
        recoveryLabel: '復原碼',
        totpHelp: '驗證碼每 30 秒更新一次，同一組驗證碼只能使用一次',
        recoveryHelp: '復原碼是啟用兩步驟驗證時儲存的備用憑證',
        recoveryRequired: '請輸入復原碼。',
        passed: '驗證通過，正在繼續…',
        verifying: '正在驗證…',
        submit: '驗證並登入',
        useRecovery: '使用復原碼',
        useAuthenticator: '使用驗證器',
        back: '返回帳號登入',
    },
} satisfies AuthMessages
