import { expect, type Page } from '@playwright/test'
import { test, signIn, login } from './real-auth'
import { labelOf, paletteOption } from './element'

// Scrolls whatever scrolls the page: the page panel on wide screens, the
// window on narrow ones.
async function scrollPage(page: Page, top: number) {
    await page.evaluate((y) => {
        const main = document.getElementById('main-content')
        const overflow = main ? getComputedStyle(main).overflowY : ''
        if (main && (overflow === 'auto' || overflow === 'scroll'))
            main.scrollTo(0, y)
        else window.scrollTo(0, y)
    }, top)
}

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
    await page.emulateMedia({
        reducedMotion: 'no-preference',
        colorScheme: 'light',
    })
})

test('navigation stays reachable while scrolling and rapid changes settle on the latest page', async ({
    page,
}, testInfo) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.goto('/')
    for (const name of ['VPN 节点', '账号安全', '网络概览', '偏好设置']) {
        await page
            .getByRole('navigation', { name: '主导航' })
            .getByRole('link', { name, exact: true })
            .click()
    }
    await expect(
        page.getByRole('heading', { name: '偏好设置', exact: true }),
    ).toBeVisible()
    const selected = page
        .getByRole('navigation', { name: '主导航' })
        .getByRole('link', {
            name: '偏好设置',
            exact: true,
        })
    await expect(selected).toHaveAttribute('aria-current', 'page')
    await expect
        .poll(async () => {
            const link = await selected.boundingBox()
            // Narrow screens mark the selection with the tab bar's lens.
            const indicator = await page
                .locator(
                    testInfo.project.name === 'mobile'
                        ? '.tab-lens'
                        : '.sidebar-menu .el-menu-item.is-active',
                )
                .boundingBox()
            return link && indicator
                ? Math.abs(link.y - indicator.y) +
                      Math.abs(link.x - indicator.x)
                : 100
        })
        .toBeLessThan(2)
    await scrollPage(page, 100_000)
    const navigation = page.getByRole('navigation', { name: '主导航' })
    for (const link of await navigation.getByRole('link').all()) {
        const box = await link.boundingBox()
        expect(box).not.toBeNull()
        // Touch screens get 44px targets; the desktop sidebar uses 36px rows.
        const minimum = testInfo.project.name === 'mobile' ? 44 : 36
        expect(box!.height).toBeGreaterThanOrEqual(minimum)
        expect(box!.width).toBeGreaterThanOrEqual(minimum)
        expect(box!.y).toBeGreaterThanOrEqual(0)
        expect(box!.y + box!.height).toBeLessThanOrEqual(
            page.viewportSize()!.height,
        )
    }
    await page.getByRole('button', { name: '全部恢复默认' }).click()
    await page
        .getByRole('navigation', { name: '主导航' })
        .getByRole('link', { name: 'VPN 节点', exact: true })
        .click()
    await expect(
        page.getByRole('heading', { name: 'VPN 节点', exact: true }),
    ).toBeVisible()
    expect(errors).toEqual([])
})

test('palette press gives feedback before release, can be cancelled and supports keyboard selection', async ({
    page,
}) => {
    await page.goto('/settings/appearance')
    const palette = paletteOption(page, '玫瑰红')
    const chip = labelOf(palette)
    await chip.scrollIntoViewIfNeeded()
    const box = await chip.boundingBox()
    expect(box).not.toBeNull()
    await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2)
    await page.mouse.down()
    await expect(palette).not.toBeChecked()
    await expect
        .poll(() =>
            chip.evaluate((element) => getComputedStyle(element).transform),
        )
        .not.toBe('none')
    await page.mouse.move(1, 1)
    await page.mouse.up()
    await expect(palette).not.toBeChecked()
    await palette.focus()
    await palette.press('Space')
    await expect(palette).toBeChecked()
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'rose')
})

test('user and system reduced motion keep feedback without moving controls or pages', async ({
    page,
}) => {
    await page.goto('/settings/appearance')
    await page.getByRole('switch', { name: '减少动态效果' }).press('Space')
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    const palette = paletteOption(page, '玫瑰红')
    const chip = labelOf(palette)
    await chip.scrollIntoViewIfNeeded()
    const box = await chip.boundingBox()
    await page.mouse.move(box!.x + box!.width / 2, box!.y + box!.height / 2)
    await page.mouse.down()
    await expect(chip).toHaveCSS('transform', 'none')
    await page.mouse.up()
    await expect(palette).toBeChecked()
    await page.getByRole('switch', { name: '减少动态效果' }).press('Space')
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    await page
        .getByRole('navigation', { name: '主导航' })
        .getByRole('link', { name: 'VPN 节点', exact: true })
        .click()
    await expect(page.locator('.route-content')).toHaveCSS('transform', 'none')
    await expect(
        page.getByRole('heading', { name: 'VPN 节点', exact: true }),
    ).toBeVisible()
})

test('the narrow tab bar lens follows a drag and opens the tab it is thrown to', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name !== 'mobile',
        'The lens is the narrow-screen tab bar selection',
    )
    await page.goto('/')
    const nav = page.getByRole('navigation', { name: '主导航' })
    const lens = page.locator('.tab-lens')
    const lensLeft = async () => (await lens.boundingBox())?.x ?? 0
    const overview = await nav
        .getByRole('link', { name: '网络概览', exact: true })
        .boundingBox()
    const security = await nav
        .getByRole('link', { name: '账号安全', exact: true })
        .boundingBox()
    if (!overview || !security) throw new Error('Tabs have no box')
    const y = overview.y + overview.height / 2
    await expect.poll(lensLeft).toBeCloseTo(overview.x, 0)

    // Pressed, the lens lifts; dragged, it follows the pointer.
    await page.mouse.move(overview.x + overview.width / 2, y)
    await page.mouse.down()
    await expect
        .poll(() =>
            lens.evaluate((element) =>
                Number(element.style.getPropertyValue('--lens-lift')),
            ),
        )
        .toBeGreaterThan(0.5)
    const target = security.x + security.width / 2
    for (let step = 1; step <= 10; step++) {
        await page.mouse.move(
            overview.x +
                overview.width / 2 +
                ((target - overview.x - overview.width / 2) * step) / 10,
            y,
        )
        await page.waitForTimeout(16)
    }
    await expect.poll(lensLeft).toBeGreaterThan(overview.x + overview.width)
    await page.mouse.up()
    await expect(page).toHaveURL(/\/settings\/security$/)
    await expect.poll(lensLeft).toBeCloseTo(security.x, 0)

    // A tap and the keyboard still open tabs.
    await nav.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await expect(page).toHaveURL(/\/nodes$/)
    await nav.getByRole('link', { name: '偏好设置', exact: true }).focus()
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(/\/settings\/appearance$/)
})

test('the top bar lifts only while content passes beneath it', async ({
    page,
}) => {
    await page.goto('/store')
    const topbar = page.locator('.topbar')
    const shadow = () =>
        topbar.evaluate((element) => getComputedStyle(element).boxShadow)
    await expect(topbar).not.toHaveAttribute('data-scrolled')
    const resting = await shadow()
    await scrollPage(page, 240)
    await expect(topbar).toHaveAttribute('data-scrolled', '')
    await expect.poll(shadow).not.toBe(resting)
    await scrollPage(page, 0)
    await expect(topbar).not.toHaveAttribute('data-scrolled')
    await expect.poll(shadow).toBe(resting)
})

test('transparency and contrast preferences provide solid readable navigation', async ({
    page,
}, testInfo) => {
    await page.goto('/settings/appearance')
    const session = await page.context().newCDPSession(page)
    await session.send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-reduced-transparency', value: 'reduce' }],
    })
    await expect(page.locator('.topbar')).toHaveCSS('backdrop-filter', 'none')
    await expect(page.getByRole('navigation', { name: '主导航' })).toHaveCSS(
        'backdrop-filter',
        'none',
    )
    await session.send('Emulation.setEmulatedMedia', {
        features: [{ name: 'prefers-contrast', value: 'more' }],
    })
    await expect(page.locator('.topbar')).toHaveCSS('backdrop-filter', 'none')
    await expect(
        page.locator(
            testInfo.project.name === 'mobile'
                ? '.nav-selection'
                : '.sidebar-menu .el-menu-item.is-active',
        ),
    ).toHaveCSS('outline-style', 'solid')
    await page
        .getByRole('radiogroup', { name: '界面外观' })
        .getByText('深色', { exact: true })
        .click()
    await expect(page.locator('html')).toHaveClass('dark')
    await expect(page.locator('.topbar')).toHaveCSS(
        'background-color',
        'rgb(18, 21, 27)',
    )
})

test('200 percent text fits console and login at desktop and narrow mobile widths', async ({
    page,
    account,
}, testInfo) => {
    if (testInfo.project.name === 'mobile') {
        await page.setViewportSize({ width: 320, height: 740 })
    }
    const paths = ['/', '/nodes', '/settings/security', '/settings/appearance']
    for (const path of paths) {
        await page.goto(path)
        await expect(page.locator('h1')).toBeVisible()
        await expect(page.locator('.route-content')).toHaveCSS('opacity', '1')
        await page.evaluate(() => {
            document.documentElement.style.fontSize = '200%'
        })
        expect(
            await page.evaluate(
                () => document.documentElement.scrollWidth <= innerWidth,
            ),
        ).toBe(true)
    }
    const modeOptions = await page.locator('.el-radio-button__inner').all()
    for (const option of modeOptions) {
        expect(
            await option.evaluate(
                (element) => element.scrollWidth <= element.clientWidth,
            ),
        ).toBe(true)
    }
    await page.screenshot({
        path: testInfo.outputPath('appearance-large-text.png'),
        fullPage: true,
    })
    await page.context().clearCookies()
    await page.goto('/login')
    await expect(
        page.getByRole('heading', { name: '登录控制台' }),
    ).toBeVisible()
    await page.evaluate(() => {
        document.documentElement.style.fontSize = '200%'
    })
    expect(
        await page.evaluate(
            () => document.documentElement.scrollWidth <= innerWidth,
        ),
    ).toBe(true)
    await login(page, account)
    await expect(page).toHaveURL('http://127.0.0.1:4173/')
})

test('unsaved form edits ask before Cancel or Escape drops them', async ({
    page,
}) => {
    await page.goto('/nodes')
    const open = page.getByRole('button', { name: '创建节点' })
    const name = page.getByLabel('节点名称')
    const prompt = page.getByRole('alertdialog', {
        name: '放弃尚未保存的修改？',
    })

    // Without edits Cancel closes at once.
    await open.click()
    await expect(name).toBeVisible()
    await page.getByRole('button', { name: '取消', exact: true }).click()
    await expect(name).toHaveCount(0)

    // With edits it asks in place and keeps focus on keeping them.
    await open.click()
    await name.fill('edge-tokyo')
    await page.getByRole('button', { name: '取消', exact: true }).click()
    await expect(prompt).toBeVisible()
    await expect(prompt.getByRole('button', { name: '继续编辑' })).toBeFocused()
    // A second Escape returns to the form, edits and focus intact.
    await page.keyboard.press('Escape')
    await expect(prompt).toHaveCount(0)
    await expect(name).toHaveValue('edge-tokyo')
    // Escape in the form asks again; discarding closes it.
    await name.press('Escape')
    await expect(prompt).toBeVisible()
    await prompt.getByRole('button', { name: '放弃修改' }).click()
    await expect(name).toHaveCount(0)
})

test('a control that cannot be used says why on focus', async ({ page }) => {
    await page.goto('/settings/security')
    const button = page.getByRole('button', { name: '退出其他会话' })
    await expect(button).toBeDisabled()
    const wrapper = page.locator('.disabled-reason', { has: button })
    await wrapper.focus()
    await expect(page.getByRole('tooltip')).toContainText('没有其他会话可退出')
    const described = await wrapper.getAttribute('aria-describedby')
    expect(described).toBeTruthy()
    await expect(page.locator(`[id="${described}"]`)).toHaveText(
        '没有其他会话可退出',
    )
})

test('on wide screens only the page panel scrolls and keeps its place per page', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'narrow screens scroll the window',
    )
    await page.setViewportSize({ width: 1280, height: 640 })
    await page.goto('/store')
    const main = page.locator('#main-content')
    const sidebar = page.locator('.sidebar')
    const topbar = page.locator('.topbar')
    const before = [await sidebar.boundingBox(), await topbar.boundingBox()]
    await scrollPage(page, 300)
    await expect
        .poll(() => main.evaluate((element) => element.scrollTop))
        .toBeGreaterThan(200)
    // The window itself never moves, so the sidebar and top bar stay put.
    await page.evaluate(() => {
        window.scrollTo(0, 500)
    })
    expect(await page.evaluate(() => window.scrollY)).toBe(0)
    expect([await sidebar.boundingBox(), await topbar.boundingBox()]).toEqual(
        before,
    )
    // Leaving the page and coming back through its tab returns to the place.
    const left = await main.evaluate((element) => element.scrollTop)
    await page
        .getByRole('navigation', { name: '主导航' })
        .getByRole('link', { name: 'VPN 节点', exact: true })
        .click()
    await expect(page).toHaveURL(/\/nodes$/)
    await page
        .getByRole('navigation', { name: '已打开的页面' })
        .getByRole('link', { name: '软件商店' })
        .click()
    await expect(page).toHaveURL(/\/store$/)
    await expect
        .poll(() => main.evaluate((element) => element.scrollTop))
        .toBe(left)
})
