import { expect, type Page } from '@playwright/test'
import jsQR from 'jsqr'
import {
    test,
    login,
    signIn,
    code,
    enableMfa,
    expireSession,
    headers,
    noStoredSecrets,
    signOut,
} from './real-auth'
import { tick } from './element'

function wrongCode(secret: string) {
    const valid = new Set([-1, 0, 1].map((offset) => code(secret, offset)))
    return Array.from({ length: 10 }, (_, index) =>
        String(index).padStart(6, '0'),
    ).find((value) => !valid.has(value))!
}

async function scanSetupQRCode(page: Page, username: string) {
    const qr = page.getByRole('img', { name: '验证器绑定二维码' })
    await expect(qr).toBeVisible()
    await expect(qr).toHaveJSProperty('complete', true)
    // Decode at the displayed size, including the narrower mobile layout.
    const pixels = await qr.evaluate((element: HTMLImageElement) => {
        const { width, height } = element.getBoundingClientRect()
        const canvas = document.createElement('canvas')
        canvas.width = Math.round(width)
        canvas.height = Math.round(height)
        const context = canvas.getContext('2d')!
        context.imageSmoothingEnabled = false
        context.drawImage(element, 0, 0, canvas.width, canvas.height)
        const image = context.getImageData(0, 0, canvas.width, canvas.height)
        return {
            data: Array.from(image.data),
            width: image.width,
            height: image.height,
        }
    })
    const result = jsQR(
        new Uint8ClampedArray(pixels.data),
        pixels.width,
        pixels.height,
    )
    expect(
        result,
        'The displayed enrollment QR must be scannable',
    ).not.toBeNull()
    const uri = new URL(result!.data)
    expect(uri.protocol).toBe('otpauth:')
    expect(uri.hostname).toBe('totp')
    expect(decodeURIComponent(uri.pathname)).toBe(`/Veilpost:${username}`)
    expect(uri.searchParams.get('issuer')).toBe('Veilpost')
    expect(uri.searchParams.get('algorithm')).toBe('SHA1')
    expect(uri.searchParams.get('digits')).toBe('6')
    expect(uri.searchParams.get('period')).toBe('30')
    return uri.searchParams.get('secret')!
}

test('real login protects deep links, shows progress and restores sessions after refresh', async ({
    page,
    account,
}, testInfo) => {
    await page.goto('/nodes?view=list')
    await expect(page).toHaveURL(/\/login\?redirect=/)
    await expect(page.getByRole('navigation', { name: '主导航' })).toHaveCount(
        0,
    )
    await login(page, account, 'incorrect-password')
    await expect(page.getByRole('alert')).toContainText('账号或密码不正确')
    await expect(page.getByLabel('密码', { exact: true })).toHaveValue('')
    const network = await page.context().newCDPSession(page)
    await network.send('Network.enable')
    await network.send('Network.emulateNetworkConditions', {
        offline: false,
        latency: 500,
        downloadThroughput: -1,
        uploadThroughput: -1,
    })
    await login(page, account)
    await expect(
        page.getByRole('button', { name: '正在登录…', exact: true }),
    ).toBeDisabled()
    await page.screenshot({
        path: testInfo.outputPath('login-loading.png'),
        fullPage: true,
    })
    await expect(page).toHaveURL('http://127.0.0.1:4173/nodes?view=list')
    await network.send('Network.emulateNetworkConditions', {
        offline: false,
        latency: 0,
        downloadThroughput: -1,
        uploadThroughput: -1,
    })
    await page.reload()
    await expect(
        page.getByText(account.username, { exact: true }),
    ).toBeVisible()
    await noStoredSecrets(page, account)
    await page
        .getByRole('navigation', { name: '主导航' })
        .getByRole('link', { name: '账号安全', exact: true })
        .click()
    await expect(page).toHaveURL('http://127.0.0.1:4173/settings/security')
    await signOut(page)
    await expect(page).toHaveURL('http://127.0.0.1:4173/login')
    await page.goBack()
    await expect(
        page.getByRole('heading', { name: '登录控制台' }),
    ).toBeVisible()
})

test('real MFA challenge survives refresh and requires a valid TOTP', async ({
    page,
    account,
}) => {
    const mfa = await enableMfa(page, account)
    await page.goto('/nodes')
    await login(page, account)
    await expect(page).toHaveURL(/\/login\/verify/)
    await page.reload()
    await page.goto('/settings/security')
    await expect(page).toHaveURL(/\/login\/verify/)
    await page.getByLabel('验证码', { exact: true }).fill('123')
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page.getByRole('alert')).toContainText('6 位数字')
    await page.getByLabel('验证码', { exact: true }).fill(wrongCode(mfa.secret))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page.getByRole('alert')).toContainText('验证码不正确')
    await page.getByLabel('验证码', { exact: true }).fill(code(mfa.secret, 1))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page).toHaveURL('http://127.0.0.1:4173/settings/security')
    await noStoredSecrets(page, account)
})

test('sign-in feedback reports Caps Lock, shakes on rejection, fills code cells and enters the console', async ({
    page,
    account,
}) => {
    const mfa = await enableMfa(page, account)
    await page.context().clearCookies()
    await page.goto('/login')
    // Count the shakes the forms receive, whatever their timing.
    await page.evaluate(() => {
        const prototype = Element.prototype
        const descriptor = Object.getOwnPropertyDescriptor(prototype, 'animate')
        const original = descriptor?.value as Element['animate']
        const counter = window as unknown as { shakes: number }
        counter.shakes = 0
        prototype.animate = function (
            this: Element,
            ...parameters: Parameters<Element['animate']>
        ) {
            if (this.matches('form.auth-form')) counter.shakes++
            return original.apply(this, parameters)
        }
    })
    const shakes = () =>
        page.evaluate<number>(
            () => (window as unknown as { shakes: number }).shakes,
        )

    // Caps Lock is reported beside the password label while it is on.
    const password = page.getByLabel('密码', { exact: true })
    await password.dispatchEvent('keydown', {
        key: 'a',
        modifierCapsLock: true,
    })
    await expect(page.getByText('大写锁定已开启')).toBeVisible()
    await expect(password).toHaveAttribute('aria-describedby', 'password-caps')
    await password.dispatchEvent('keyup', { key: 'a', modifierCapsLock: false })
    await expect(page.getByText('大写锁定已开启')).toHaveCount(0)

    await login(page, account, 'incorrect-password')
    await expect(page.getByRole('alert')).toContainText('账号或密码不正确')
    expect(await shakes()).toBe(1)

    await login(page, account)
    await expect(page).toHaveURL(/\/login\/verify/)
    const field = page.getByLabel('验证码', { exact: true })
    await field.fill(wrongCode(mfa.secret))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page.getByRole('alert')).toContainText('验证码不正确')
    await expect.poll(shakes).toBe(2)

    const value = code(mfa.secret, 1)
    await field.fill(value)
    await expect(page.locator('.code-cell')).toHaveText(value.split(''))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(page.getByText('验证通过，正在继续…')).toBeVisible()
    await expect(page.getByText('正在进入控制台…')).toBeVisible()
    await expect(page).toHaveURL('http://127.0.0.1:4173/')
})

test('recovery codes are consumed by the real backend exactly once', async ({
    page,
    account,
}) => {
    const mfa = await enableMfa(page, account)
    for (const iteration of [0, 1]) {
        await page.goto('/login')
        await login(page, account)
        await page.getByRole('button', { name: '使用恢复码' }).click()
        await page
            .getByLabel('恢复码', { exact: true })
            .fill(mfa.recoveryCodes[0]!)
        await page.getByRole('button', { name: '验证并登录' }).click()
        if (iteration === 0) {
            await expect(page).toHaveURL('http://127.0.0.1:4173/')
            await signOut(page)
        } else {
            await expect(page.getByRole('alert')).toContainText(
                '恢复码无效或已使用',
            )
            await expect(
                page.getByRole('navigation', { name: '主导航' }),
            ).toHaveCount(0)
            await page
                .getByLabel('恢复码', { exact: true })
                .fill(mfa.recoveryCodes[1]!)
            await page.getByRole('button', { name: '验证并登录' }).click()
            await expect(page).toHaveURL('http://127.0.0.1:4173/')
        }
    }
})

test('security settings bind a real verifier, persist state and disable MFA', async ({
    page,
    account,
}, testInfo) => {
    await signIn(page, account)
    await page.goto('/settings/security')
    await page
        .getByRole('button', { name: '启用两步验证', exact: true })
        .click()
    await page.getByLabel('当前密码').fill('incorrect-password')
    await page.getByRole('button', { name: '继续绑定' }).click()
    await expect(page.getByRole('alert')).toContainText('当前密码不正确')
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '继续绑定' }).click()
    const secret = await scanSetupQRCode(page, account.username)
    await expect(page.getByTestId('setup-secret')).not.toBeVisible()
    await expect(
        page.getByRole('heading', { name: '扫描二维码' }),
    ).toBeFocused()
    await page.screenshot({
        path: testInfo.outputPath('security-qr.png'),
        fullPage: true,
    })
    await page.getByText('无法扫码？手动输入密钥', { exact: true }).click()
    await expect(page.getByTestId('setup-secret')).toHaveText(secret)
    await page.getByLabel('验证码', { exact: true }).fill(code(secret))
    await page.getByRole('button', { name: '验证并启用' }).click()
    await expect(
        page.getByRole('heading', { name: '保存你的恢复码' }),
    ).toBeVisible()
    await expect(
        page.getByRole('img', { name: '验证器绑定二维码' }),
    ).toHaveCount(0)
    await expect(page.getByRole('button', { name: '完成设置' })).toBeDisabled()
    await noStoredSecrets(page, account)
    await tick(page.getByLabel('我已安全保存恢复码'))
    await page.getByRole('button', { name: '完成设置' }).click()
    await page.reload()
    await expect(page.getByText('已启用', { exact: true })).toBeVisible()
    await page
        .getByRole('button', { name: '关闭两步验证', exact: true })
        .click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByLabel('验证码', { exact: true }).fill(code(secret, 1))
    await page.getByRole('button', { name: '确认关闭两步验证' }).click()
    await expect(page.getByText('未启用', { exact: true })).toBeVisible()
    await page.screenshot({
        path: testInfo.outputPath('security-feedback.png'),
        fullPage: true,
    })
})

test('server-side expiry clears full and pending sessions', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    await page.goto('/settings/security')
    // Expire only after the status request, so the button click meets expiry.
    await expect(page.getByText('只有当前浏览器处于登录状态')).toBeVisible()
    await expireSession(page)
    await page
        .getByRole('button', { name: '启用两步验证', exact: true })
        .click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '继续绑定' }).click()
    await expect(
        page.getByRole('heading', { name: '登录控制台' }),
    ).toBeVisible()
    const mfa = await enableMfa(page, account)
    await page.goto('/login')
    await login(page, account)
    await expect(page).toHaveURL(/\/login\/verify/)
    await expireSession(page)
    await page.getByLabel('验证码', { exact: true }).fill(code(mfa.secret, 1))
    await page.getByRole('button', { name: '验证并登录' }).click()
    await expect(
        page.getByRole('heading', { name: '登录控制台' }),
    ).toBeVisible()
})

test('QR binding remains scannable in dark mode and clears after expiry', async ({
    page,
    account,
}, testInfo) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.clock.install()
    await signIn(page, account)
    await page.goto('/settings/security')
    await page
        .getByRole('button', { name: '启用两步验证', exact: true })
        .click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '继续绑定' }).click()
    const secret = await scanSetupQRCode(page, account.username)
    await noStoredSecrets(page, account)
    expect(
        await page.evaluate(() =>
            JSON.stringify([localStorage, sessionStorage]),
        ),
    ).not.toContain(secret)
    await page.screenshot({
        path: testInfo.outputPath('security-qr-dark.png'),
        fullPage: true,
    })
    // An image decode failure must reveal the manual fallback without changing MFA state.
    await page
        .getByRole('img', { name: '验证器绑定二维码' })
        .evaluate((image) => {
            image.dispatchEvent(new Event('error'))
        })
    await expect(page.getByTestId('setup-secret')).toBeVisible()
    await page.clock.fastForward(5 * 60_000 + 1000)
    await expect(page.getByRole('alert')).toContainText('本次绑定已过期')
    await expect(page.getByTestId('setup-secret')).toHaveCount(0)
    await expect(
        page.getByRole('img', { name: '验证器绑定二维码' }),
    ).toHaveCount(0)
    await expect(page.getByText('未启用', { exact: true })).toBeVisible()
})

test('cancelled binding clears temporary secrets and network failure gives retry feedback', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    await page.goto('/settings/security')
    await page
        .getByRole('button', { name: '启用两步验证', exact: true })
        .click()
    await page.getByLabel('当前密码').fill(account.password)
    await page.getByRole('button', { name: '继续绑定' }).click()
    await page.getByRole('button', { name: '取消绑定' }).click()
    await expect(page.getByTestId('setup-secret')).toHaveCount(0)
    await expect(
        page.getByRole('img', { name: '验证器绑定二维码' }),
    ).toHaveCount(0)
    await expect(page.getByText('未启用', { exact: true })).toBeVisible()
    await page.context().setOffline(true)
    await signOut(page)
    await expect(page.getByRole('alert')).toContainText('认证服务暂不可用')
    await expect(page.getByRole('navigation', { name: '主导航' })).toBeVisible()
    await page.context().setOffline(false)
    // The failure is reported in the account centre, which stays open.
    await page
        .getByRole('dialog', { name: '个人中心' })
        .getByRole('button', { name: '退出登录' })
        .click()
    await expect(page).toHaveURL('http://127.0.0.1:4173/login')
})

test('the account center protects unsaved recovery codes when leaving its security panel', async ({
    page,
    account,
}) => {
    await signIn(page, account)
    await page.goto('/')
    await page.getByRole('button', { name: '打开个人中心' }).click()
    const centre = page.getByRole('dialog', { name: '个人中心' })
    await centre.getByRole('button', { name: '登录与安全' }).click()
    await centre.getByRole('button', { name: '启用两步验证' }).click()
    await centre.getByLabel('当前密码', { exact: true }).fill(account.password)
    await centre.getByRole('button', { name: '继续绑定' }).click()
    const secret = await scanSetupQRCode(page, account.username)
    await centre.getByLabel('验证码', { exact: true }).fill(code(secret))
    await centre.getByRole('button', { name: '验证并启用' }).click()
    const recovery = centre.getByRole('list', { name: '恢复码' })
    await expect(recovery).toBeVisible()

    const warning = page.locator('.el-message-box').filter({
        hasText: '恢复码尚未保存',
    })
    await centre.getByRole('button', { name: '登录会话' }).click()
    await expect(warning).toBeVisible()
    await warning.getByRole('button', { name: '留在此页' }).click()
    await expect(warning).toBeHidden()
    await expect(recovery).toBeVisible()

    await page.keyboard.press('Escape')
    await expect(warning).toBeVisible()
    await warning.getByRole('button', { name: '仍然离开' }).click()
    await expect(centre).toBeHidden()
    await expect(recovery).toHaveCount(0)
    await noStoredSecrets(page, account)
})

test('external return paths are rejected with real authenticated sessions', async ({
    page,
    account,
}) => {
    for (const destination of [
        'https://example.com',
        '//example.com',
        '/login/verify',
    ]) {
        await page.goto(`/login?redirect=${encodeURIComponent(destination)}`)
        await login(page, account)
        await expect(page).toHaveURL('http://127.0.0.1:4173/')
        await page.request.post('/api/auth/logout', { headers, data: {} })
    }
})

test('password change rotates sessions and replaces the old password', async ({
    page,
    browser,
    account,
}) => {
    const other = await browser.newContext()
    try {
        await signIn(page, account)
        expect(
            (
                await other.request.post(
                    'http://127.0.0.1:4173/api/auth/login',
                    {
                        headers,
                        data: account,
                    },
                )
            ).status(),
        ).toBe(200)
        await page.goto('/settings/security')
        await expect(page.getByText('还有 1 个会话处于登录状态')).toBeVisible()
        await page.getByRole('button', { name: '修改密码' }).click()
        const current = page.getByLabel('当前密码', { exact: true })
        const next = page.getByLabel('新密码', { exact: true })
        const confirm = page.getByLabel('确认新密码', { exact: true })
        await expect(current).toBeFocused()
        const nextPassword = `${account.password}-next`
        await current.fill(account.password)
        await next.fill(nextPassword)
        await confirm.fill(`${nextPassword}!`)
        await page.getByRole('button', { name: '保存新密码' }).click()
        await expect(page.getByRole('alert')).toContainText(
            '两次输入的新密码不一致',
        )
        await confirm.fill(account.password)
        await next.fill(account.password)
        await page.getByRole('button', { name: '保存新密码' }).click()
        await expect(page.getByRole('alert')).toContainText(
            '不能与当前密码相同',
        )
        await current.fill(account.password)
        await next.fill(nextPassword)
        await confirm.fill(nextPassword)
        await page.getByRole('button', { name: '保存新密码' }).click()
        await expect(
            page.getByRole('status').filter({ hasText: '密码已修改' }),
        ).toBeVisible()
        await expect(page.getByText('只有当前浏览器处于登录状态')).toBeVisible()
        await noStoredSecrets(page, account)
        expect(
            await page.evaluate(() =>
                JSON.stringify([localStorage, sessionStorage]),
            ),
        ).not.toContain(nextPassword)
        const otherSession = await other.request.get(
            'http://127.0.0.1:4173/api/auth/session',
        )
        expect((await otherSession.json()).status).toBe('anonymous')
        await signOut(page)
        await login(page, account)
        await expect(page.getByRole('alert')).toContainText('账号或密码不正确')
        await login(page, account, nextPassword)
        await expect(page).toHaveURL('http://127.0.0.1:4173/')
    } finally {
        await other.close()
    }
})

test('recovery codes can be regenerated and other sessions revoked', async ({
    page,
    browser,
    account,
}) => {
    const mfa = await enableMfa(page, account)
    const other = await browser.newContext()
    try {
        for (const [context, recoveryCode] of [
            [page.context(), mfa.recoveryCodes[0]],
            [other, mfa.recoveryCodes[1]],
        ] as const) {
            await context.request.post('http://127.0.0.1:4173/api/auth/login', {
                headers,
                data: account,
            })
            const verified = await context.request.post(
                'http://127.0.0.1:4173/api/auth/verify',
                { headers, data: { method: 'recovery', code: recoveryCode } },
            )
            expect(verified.status()).toBe(200)
        }
        await page.goto('/settings/security')
        await expect(page.getByTestId('recovery-remaining')).toContainText(
            '剩余 6 个可用恢复码',
        )
        await page.getByRole('button', { name: '重新生成恢复码' }).click()
        await page.getByLabel('当前密码').fill(account.password)
        await page
            .getByLabel('验证码', { exact: true })
            .fill(wrongCode(mfa.secret))
        await page.getByRole('button', { name: '生成新的恢复码' }).click()
        await expect(page.getByRole('alert')).toContainText('验证码不正确')
        await page.getByLabel('当前密码').fill(account.password)
        await page
            .getByLabel('验证码', { exact: true })
            .fill(code(mfa.secret, 1))
        await page.getByRole('button', { name: '生成新的恢复码' }).click()
        await expect(
            page.getByRole('heading', { name: '保存你的恢复码' }),
        ).toBeVisible()
        const codes = await page
            .getByRole('list', { name: '恢复码' })
            .locator('code')
            .allTextContents()
        expect(codes).toHaveLength(8)
        expect(codes).not.toContain(mfa.recoveryCodes[2])
        await noStoredSecrets(page, account)
        await tick(page.getByLabel('我已安全保存恢复码'))
        await page.getByRole('button', { name: '完成', exact: true }).click()
        await expect(
            page.getByRole('status').filter({ hasText: '旧恢复码已全部失效' }),
        ).toBeVisible()
        await expect(page.getByTestId('recovery-remaining')).toContainText(
            '剩余 8 个可用恢复码',
        )
        await expect(page.getByText('还有 1 个会话处于登录状态')).toBeVisible()
        await page.getByRole('button', { name: '退出其他会话' }).click()
        await expect(page.getByText('只有当前浏览器处于登录状态')).toBeVisible()
        const otherSession = await other.request.get(
            'http://127.0.0.1:4173/api/auth/session',
        )
        expect((await otherSession.json()).status).toBe('anonymous')
        await other.request.post('http://127.0.0.1:4173/api/auth/login', {
            headers,
            data: account,
        })
        const stale = await other.request.post(
            'http://127.0.0.1:4173/api/auth/verify',
            {
                headers,
                data: { method: 'recovery', code: mfa.recoveryCodes[2] },
            },
        )
        expect((await stale.json()).code).toBe('INVALID_RECOVERY_CODE')
    } finally {
        await other.close()
    }
})
