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
        await main.getByRole('link', { name: '偏好设置', exact: true }).click()
        await expect(tabs.getByRole('link')).toHaveText([
            '网络概览',
            'VPN 节点',
            '偏好设置',
        ])
        await expect(
            tabs.getByRole('link', { name: '偏好设置' }),
        ).toHaveAttribute('aria-current', 'page')
        // The overview stays and cannot be closed.
        await expect(
            tabs.getByRole('button', { name: '关闭“网络概览”' }),
        ).toHaveCount(0)
        await tabs.getByRole('button', { name: '关闭“偏好设置”' }).click()
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
        expect(await preferences(page)).not.toContain('sidebarCollapsed')
        await page.getByRole('button', { name: '收起侧栏' }).click()
        const expand = page.getByRole('button', { name: '展开侧栏' })
        await expect(expand).toHaveAttribute('aria-expanded', 'false')
        const link = page
            .getByRole('navigation', { name: '主导航' })
            .getByRole('link', { name: 'VPN 节点', exact: true })
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
        await expect(mode).toHaveAccessibleName('界面外观：跟随系统')
        await mode.click()
        await expect(html).not.toHaveClass(/dark/)
        await expect(
            page.getByRole('button', { name: '界面外观：浅色' }),
        ).toBeVisible()
        await page.getByRole('button', { name: '界面外观：浅色' }).click()
        await expect(html).toHaveClass(/dark/)

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
        await expect(centre).toContainText(account.username)
        await expect(centre).toContainText('未启用两步验证')
        await centre.getByRole('link', { name: '账号安全' }).click()
        await expect(page).toHaveURL(/\/settings\/security$/)
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
    await page.emulateMedia({ colorScheme: 'light' })
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
    await page.getByRole('button', { name: /^Appearance: / }).click()
    await expect(page.locator('html')).not.toHaveClass(/dark/)
    await page.getByRole('button', { name: 'Appearance: Light' }).click()
    await expect(page.locator('html')).toHaveClass(/dark/)
    await expect(page.locator('kbd')).toHaveText('Enter')
})
