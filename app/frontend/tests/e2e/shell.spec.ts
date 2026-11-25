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
        if (testInfo.project.name !== 'mobile')
            await page.getByRole('button', { name: '刷新当前页' }).click()
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

test('sign-in offers language and appearance switches over the globe', async ({
    page,
}) => {
    await page.emulateMedia({ colorScheme: 'dark' })
    await page.goto('/login')
    await expect(page.locator('canvas.lab-globe')).toHaveAttribute(
        'aria-hidden',
        'true',
    )
    const languages = page.getByRole('group', { name: '界面语言' })
    await expect(
        languages.getByRole('button', { name: '简体中文' }),
    ).toHaveAttribute('aria-pressed', 'true')
    await languages.getByRole('button', { name: 'English' }).click()
    await expect(
        page.getByRole('heading', { name: 'Sign in to the console' }),
    ).toBeVisible()
    await expect(page.locator('html')).toHaveAttribute('lang', 'en')
    const html = page.locator('html')
    const mode = page.getByRole('button', { name: /^Appearance: / })
    await expect(mode).toHaveAccessibleName('Appearance: Dark')
    await expect(html).toHaveClass(/dark/)
    await mode.click()
    await expect(mode).toHaveAccessibleName('Appearance: Light')
    await expect(html).not.toHaveClass(/dark/)
    await mode.click()
    await expect(mode).toHaveAccessibleName('Appearance: Dark')
    await expect(html).toHaveClass(/dark/)
    await expect(page.locator('kbd')).toHaveText('Enter')
})
