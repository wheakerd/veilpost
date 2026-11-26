import { expect, type Page } from '@playwright/test'
import { test, signIn, noStoredSecrets } from './real-auth'

async function preferences(page: Page) {
    return (
        (await page.evaluate(() =>
            localStorage.getItem('veilpost:preferences:v1'),
        )) ?? ''
    )
}

test.describe('signed in', () => {
    test.beforeEach(async ({ page, account }) => {
        await signIn(page, account)
        await page.emulateMedia({ colorScheme: 'light' })
    })

    test('open pages get tabs that close back to a neighbour and never persist', async ({
        page,
        account,
    }, testInfo) => {
        test.skip(
            testInfo.project.name === 'mobile',
            'the tab strip is desktop only',
        )
        await page.goto('/')
        const main = page.getByRole('navigation', { name: '主导航' })
        const tabs = page.getByRole('navigation', { name: '已打开的页面' })
        await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
        await main.getByRole('link', { name: '证书', exact: true }).click()
        await expect(tabs.getByRole('link')).toHaveText([
            '网络概览',
            'VPN 节点',
            '证书',
        ])
        await expect(tabs.getByRole('link', { name: '证书' })).toHaveAttribute(
            'aria-current',
            'page',
        )
        // The overview stays and cannot be closed.
        await expect(
            tabs.getByRole('button', { name: '关闭“网络概览”' }),
        ).toHaveCount(0)
        await tabs.getByRole('button', { name: '关闭“证书”' }).click()
        await expect(page).toHaveURL(/\/nodes$/)
        await expect(tabs.getByRole('link')).toHaveText([
            '网络概览',
            'VPN 节点',
        ])
        await tabs.getByRole('link', { name: '网络概览' }).click()
        await expect(
            page.getByRole('heading', { name: '网络概览', exact: true }),
        ).toBeVisible()
        await noStoredSecrets(page, account)
        expect(await preferences(page)).not.toContain('nodes')
    })

    test('the sidebar collapses to an icon rail and only that choice is stored', async ({
        page,
    }, testInfo) => {
        test.skip(
            testInfo.project.name === 'mobile',
            'narrow screens use the bottom bar',
        )
        await page.goto('/nodes')
        const main = page.getByRole('navigation', { name: '主导航' })
        const preferencesLink = main.getByRole('link', {
            name: '偏好设置',
            exact: true,
        })
        await expect(preferencesLink).toHaveCount(0)
        await expect(main.getByText('设置', { exact: true })).toHaveCount(0)
        expect(await preferences(page)).not.toContain('sidebarCollapsed')
        await page.getByRole('button', { name: '收起侧栏' }).click()
        await expect(preferencesLink).toHaveCount(0)
        const expand = page.getByRole('button', { name: '展开侧栏' })
        await expect(expand).toHaveAttribute('aria-expanded', 'false')
        const link = main.getByRole('link', { name: 'VPN 节点', exact: true })
        // The icon rail keeps the name for assistive tech and shows a tooltip.
        await expect(link).toHaveAccessibleName('VPN 节点')
        await link.hover()
        await expect(
            page.getByRole('tooltip', { name: 'VPN 节点' }),
        ).toBeVisible()
        await main
            .getByRole('link', { name: '网络概览', exact: true })
            .press('Enter')
        await expect(page).toHaveURL('http://127.0.0.1:4173/')
        const box = await page.locator('.sidebar').boundingBox()
        expect(box!.width).toBeLessThanOrEqual(64)
        expect(JSON.parse(await preferences(page))).toMatchObject({
            sidebarCollapsed: true,
        })
        await page.reload()
        await expect(expand).toBeVisible()
        await expand.click()
        await expect(
            page.getByRole('button', { name: '收起侧栏' }),
        ).toBeVisible()
        expect(await preferences(page)).not.toContain('sidebarCollapsed')
    })

    test('the toolbar switches appearance and language and reloads only the page', async ({
        page,
    }, testInfo) => {
        await page.goto('/settings/appearance')
        const html = page.locator('html')
        const mode = page.getByRole('button', { name: /^界面外观：/ })
        await expect(mode).toHaveAccessibleName('界面外观：浅色')
        await expect(
            page.getByRole('radio', { name: '跟随系统' }),
        ).toBeChecked()
        await mode.click()
        await expect(html).toHaveClass(/dark/)
        await expect(mode).toHaveAccessibleName('界面外观：深色')

        // Narrow screens drop the reload button to fit the bar.
        if (testInfo.project.name !== 'mobile') {
            const refresh = page.getByRole('button', { name: '刷新当前页' })
            const rotation = await refresh
                .locator('.refresh-icon')
                .evaluateHandle((icon) => {
                    const state = { started: false }
                    icon.addEventListener(
                        'transitionrun',
                        () => {
                            state.started = true
                        },
                        { once: true },
                    )
                    return state
                })
            await refresh.click()
            await expect
                .poll(() => rotation.evaluate((state) => state.started))
                .toBe(true)
        }
        await expect(
            page.getByRole('heading', { name: '偏好设置', exact: true }),
        ).toBeVisible()

        await page.getByRole('button', { name: '界面语言' }).click()
        await page.getByRole('menuitem', { name: 'English' }).click()
        await expect(html).toHaveAttribute('lang', 'en')
        await expect(
            page.getByRole('heading', { level: 1, name: 'Preferences' }),
        ).toBeVisible()
        await page.getByRole('button', { name: 'Interface language' }).click()
        await page.getByRole('menuitem', { name: 'Follow browser' }).click()
        await expect(html).toHaveAttribute('lang', 'zh-CN')
    })

    test('the account centre holds the account pages and signing out', async ({
        page,
        account,
    }) => {
        await page.goto('/')
        // The sidebar no longer reports the session.
        await expect(page.getByText('会话已验证')).toHaveCount(0)
        const trigger = page.getByRole('button', { name: '打开个人中心' })
        await trigger.click()
        const centre = page.getByRole('dialog', { name: '个人中心' })
        await expect(centre).toBeVisible()
        await expect(
            centre.getByRole('heading', { name: '资料' }),
        ).toBeVisible()
        await expect(centre.locator('.account-profile')).toContainText(
            account.username,
        )
        await expect(centre).toContainText('未启用两步验证')
        await centre.getByRole('button', { name: '登录与安全' }).click()
        await expect(
            centre.getByRole('button', { name: '启用两步验证' }),
        ).toBeVisible()
        await expect(page).toHaveURL('http://127.0.0.1:4173/')
        await centre.getByRole('button', { name: '登录会话' }).click()
        await expect(
            centre.getByText('只有当前浏览器处于登录状态'),
        ).toBeVisible()
        await expect(
            centre.getByRole('button', { name: '启用两步验证' }),
        ).toHaveCount(0)
        await page.keyboard.press('Escape')
        await expect(centre).toBeHidden()
        await expect(trigger).toBeFocused()
        await trigger.click()
        await expect(
            centre.getByRole('heading', { name: '资料' }),
        ).toBeVisible()
        await centre.getByRole('button', { name: '偏好设置' }).click()
        await expect(page).toHaveURL(/\/settings\/appearance$/)
        await expect(centre).toBeHidden()
        await trigger.click()
        await centre.getByRole('button', { name: '退出登录' }).click()
        await expect(page).toHaveURL(/\/login$/)
    })

    test('the tab bar closes from itself and comes back from preferences', async ({
        page,
    }, testInfo) => {
        test.skip(
            testInfo.project.name === 'mobile',
            'the tab strip is desktop only',
        )
        await page.goto('/nodes')
        const tabs = page.getByRole('navigation', { name: '已打开的页面' })
        await expect(tabs).toBeVisible()
        await tabs.getByRole('button', { name: '关闭页签栏' }).click()
        await expect(tabs).toHaveCount(0)
        expect(JSON.parse(await preferences(page))).toMatchObject({
            tabBar: false,
        })
        await page.goto('/settings/appearance')
        await page.getByRole('switch', { name: '启用页签栏' }).press('Space')
        await expect(tabs).toBeVisible()
    })
})

test('sign-in adapts its layout and offers language and appearance switches', async ({
    page,
    account,
}, testInfo) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.goto('/login')
    const artwork = page.locator('.auth-art')
    await expect(artwork).toHaveAttribute('aria-hidden', 'true')
    await expect(artwork).toHaveText('')
    if (testInfo.project.name === 'mobile') {
        await expect(artwork).toBeHidden()
        const brand = (await page.locator('.auth-brand').boundingBox())!
        const appearance = (await page.locator('.auth-mode').boundingBox())!
        const brandCenter = brand.y + brand.height / 2
        const appearanceCenter = appearance.y + appearance.height / 2
        expect(Math.abs(brandCenter - appearanceCenter)).toBeLessThanOrEqual(1)
    } else {
        await expect(artwork).toBeVisible()
        await expect(artwork.locator('img')).toHaveAttribute(
            'src',
            '/favicon.svg',
        )
    }
    await expect(page.getByLabel('账号', { exact: true })).toBeVisible()
    await expect(page.locator('.route-content')).toHaveCSS('opacity', '1')
    const touchControls = page.locator(
        '.auth-languages button, .auth-mode, .password-toggle, .auth-card .form-submit',
    )
    await expect(touchControls).toHaveCount(7)
    for (const control of await touchControls.all()) {
        const box = await control.boundingBox()
        expect(box?.width).toBeGreaterThanOrEqual(44)
        expect(box?.height).toBeGreaterThanOrEqual(44)
    }
    await page.screenshot({
        path: testInfo.outputPath('sign-in-dark.png'),
        animations: 'disabled',
    })
    await page.emulateMedia({ reducedMotion: 'no-preference' })
    const submit = page.getByRole('button', { name: '登录', exact: true })
    const submitBox = (await submit.boundingBox())!
    const submitX = submitBox.x + submitBox.width / 2
    await page.mouse.move(submitX, submitBox.y + submitBox.height / 2)
    await page.mouse.down()
    await expect(submit).toHaveCSS('scale', '0.95')
    await page.mouse.move(submitX, submitBox.y - 2)
    await page.mouse.up()
    await expect(submit).toHaveCSS('scale', '1')
    await page.getByLabel('账号', { exact: true }).fill(account.username)
    const passwordToggle = page.locator('.password-toggle')
    const password = page.getByLabel('密码', { exact: true })
    await expect(passwordToggle).toHaveText('')
    await expect(passwordToggle).toHaveAccessibleName('显示密码')
    await expect(passwordToggle.locator('svg')).toHaveAttribute(
        'aria-hidden',
        'true',
    )
    const toggleBox = (await passwordToggle.boundingBox())!
    await page.mouse.move(
        toggleBox.x + toggleBox.width / 2,
        toggleBox.y + toggleBox.height / 2,
    )
    await page.mouse.down()
    await expect(passwordToggle).toHaveAttribute('aria-pressed', 'false')
    await expect
        .poll(() =>
            passwordToggle.evaluate(
                (element) => getComputedStyle(element).transform,
            ),
        )
        .not.toBe('none')
    await page.mouse.up()
    await expect(passwordToggle).toHaveAttribute('aria-pressed', 'true')
    await expect(passwordToggle).toHaveAccessibleName('隐藏密码')
    await expect(password).toHaveAttribute('type', 'text')
    await expect(passwordToggle).toHaveCSS('transform', 'none')
    await passwordToggle.press('Space')
    await expect(passwordToggle).toHaveAttribute('aria-pressed', 'false')
    await expect(password).toHaveAttribute('type', 'password')
    await expect(passwordToggle).toHaveCSS('transform', 'none')
    const languages = page.getByRole('group', { name: '界面语言' })
    await expect(
        languages.getByRole('button', { name: '简体中文' }),
    ).toHaveAttribute('aria-pressed', 'true')
    await languages.getByRole('button', { name: 'English' }).click()
    await expect(
        page.getByRole('heading', { name: 'Sign in to the console' }),
    ).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    const username = page.getByLabel('Username', { exact: true })
    await expect(username).toHaveValue(account.username)
    const html = page.locator('html')
    const mode = page.getByRole('button', { name: /^Appearance: / })
    await expect(mode).toHaveAccessibleName('Appearance: Dark')
    await expect(html).toHaveClass(/dark/)
    await mode.click()
    await expect(mode).toHaveAccessibleName('Appearance: Light')
    await expect(html).not.toHaveClass(/dark/)
    await username.fill('')
    await username.blur()
    await page.screenshot({
        path: testInfo.outputPath('sign-in-light.png'),
        animations: 'disabled',
    })
    await mode.click()
    await expect(mode).toHaveAccessibleName('Appearance: Dark')
    await expect(html).toHaveClass(/dark/)
})

test('browser autofill stays inside the sign-in fields in both appearances', async ({
    page,
}, testInfo) => {
    await page.goto('/login')
    const group = page.locator('.login-fields')
    await page.getByLabel('账号', { exact: true }).fill('operator')
    const password = page.getByLabel('密码', { exact: true })
    await password.fill('autofill-preview')
    await password.blur()

    // Exercise Chromium's own autofill paint without using a saved credential.
    const session = await page.context().newCDPSession(page)
    await session.send('DOM.enable')
    await session.send('CSS.enable')
    const { root } = await session.send('DOM.getDocument')
    for (const selector of ['#username', '#password']) {
        const { nodeId } = await session.send('DOM.querySelector', {
            nodeId: root.nodeId,
            selector,
        })
        await session.send('CSS.forcePseudoState', {
            nodeId,
            forcedPseudoClasses: ['autofill'],
        })
    }

    for (const colorScheme of ['light', 'dark'] as const) {
        await page.emulateMedia({ colorScheme })
        await expect(page.locator('html')).toHaveCSS(
            'color-scheme',
            colorScheme,
        )
        await expect(group).toHaveCSS('overflow', 'hidden')
        const rows = group.locator('.form-field')
        for (const row of await rows.all()) {
            const fill = await row.evaluate(
                (el) => getComputedStyle(el).backgroundColor,
            )
            const input = row.locator('input')
            expect(await input.evaluate((el) => el.matches(':autofill'))).toBe(
                true,
            )
            const shadow = await input.evaluate(
                (el) => getComputedStyle(el).boxShadow,
            )
            expect(shadow).toContain(fill)
            expect(shadow).toContain('inset')
        }
        const path = testInfo.outputPath(`sign-in-autofill-${colorScheme}.png`)
        await page
            .locator('.auth-panel')
            .screenshot({ path, animations: 'disabled' })
    }
    await session.detach()
})
