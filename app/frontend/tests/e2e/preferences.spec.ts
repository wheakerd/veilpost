import { expect, type Locator, type Page } from '@playwright/test'
import { test, signIn, noStoredSecrets } from './real-auth'
import { labelOf, paletteOption, tick } from './element'

const preferencesKey = 'veilpost:preferences:v1'

async function stored(page: Page) {
    return JSON.parse(
        (await page.evaluate(
            (key) => localStorage.getItem(key),
            preferencesKey,
        )) ?? '{}',
    ) as Record<string, unknown>
}

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
    await page.emulateMedia({ colorScheme: 'light' })
})

test('the toolbar opens the preferences drawer and changes apply at once', async ({
    page,
    account,
}) => {
    await page.goto('/nodes')
    await page.getByRole('button', { name: '偏好设置' }).click()
    const drawer = page.getByRole('dialog', { name: '偏好设置' })
    await expect(drawer).toBeVisible()
    const html = page.locator('html')

    await drawer
        .getByRole('radiogroup', { name: '视觉模式' })
        .getByText('色弱友好', { exact: true })
        .click()
    await expect(html).toHaveAttribute('data-vision', 'colorSafe')
    await drawer
        .getByRole('radiogroup', { name: '视觉模式' })
        .getByText('灰阶模式', { exact: true })
        .click()
    await expect(html).toHaveCSS('filter', 'grayscale(1)')

    // Glass strength moves by keyboard and resets together with the tint.
    const before = await html.evaluate((root) =>
        root.style.getPropertyValue('--glass-blur'),
    )
    await drawer.getByRole('slider', { name: '强度' }).press('End')
    await expect
        .poll(() =>
            html.evaluate((root) =>
                root.style.getPropertyValue('--glass-blur'),
            ),
        )
        .not.toBe(before)
    expect(await stored(page)).toMatchObject({
        vision: 'grayscale',
        glassStrength: 100,
    })
    await drawer
        .getByRole('button', { name: '恢复默认', exact: true })
        .first()
        .click()
    expect(await stored(page)).toMatchObject({
        glassTint: 20,
        glassStrength: 60,
    })

    await tick(drawer.getByRole('radio', { name: '缩放聚焦' }))
    expect(await stored(page)).toMatchObject({ pageTransition: 'zoom' })

    await page.keyboard.press('Escape')
    await expect(drawer).toBeHidden()
    await expect(
        page.getByRole('heading', { name: 'VPN 节点', level: 1 }),
    ).toBeVisible()
    await noStoredSecrets(page, account)
})

// The sheet's vertical offset from its resting place, in pixels.
function sheetOffset(page: Page) {
    return page.getByRole('dialog', { name: '偏好设置' }).evaluate((sheet) => {
        const transform = getComputedStyle(sheet).transform
        return Math.round(
            transform === 'none' ? 0 : new DOMMatrixReadOnly(transform).m42,
        )
    })
}

test('on narrow screens preferences are a sheet that pulls down to close', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name !== 'mobile',
        'The bottom sheet is the narrow-screen drawer',
    )
    await page.goto('/')
    const open = page.getByRole('button', { name: '偏好设置' })
    const sheet = page.getByRole('dialog', { name: '偏好设置' })
    await open.click()
    await expect(sheet).toBeVisible()
    await expect.poll(() => sheetOffset(page)).toBe(0)
    const box = await sheet
        .getByRole('heading', { name: '偏好设置' })
        .boundingBox()
    if (!box) throw new Error('The sheet heading has no box')
    const x = box.x + box.width / 2
    const y = box.y + box.height / 2

    // A slow, short pull follows the pointer and settles back on release.
    await page.mouse.move(x, y)
    await page.mouse.down()
    for (let step = 1; step <= 10; step++) {
        await page.mouse.move(x, y + step * 8)
        await page.waitForTimeout(20)
    }
    expect(await sheetOffset(page)).toBeGreaterThan(60)
    await page.mouse.up()
    await expect.poll(() => sheetOffset(page)).toBe(0)
    await expect(sheet).toBeVisible()

    // Keep the flick's 20 ms sample intervals independent of runner latency.
    const session = await page.context().newCDPSession(page)
    const steps = 6
    const distance = 240
    const sampleInterval = 0.02
    const startedAt = Date.now() / 1000 - (steps + 1) * sampleInterval
    await session.send('Input.dispatchMouseEvent', {
        type: 'mousePressed',
        x,
        y,
        button: 'left',
        buttons: 1,
        clickCount: 1,
        timestamp: startedAt,
    })
    for (let step = 1; step <= steps; step++) {
        const offset = (step * distance) / steps
        await session.send('Input.dispatchMouseEvent', {
            type: 'mouseMoved',
            x,
            y: y + offset,
            button: 'left',
            buttons: 1,
            timestamp: startedAt + step * sampleInterval,
        })
        await expect.poll(() => sheetOffset(page)).toBe(offset)
    }
    await session.send('Input.dispatchMouseEvent', {
        type: 'mouseReleased',
        x,
        y: y + distance,
        button: 'left',
        buttons: 0,
        clickCount: 1,
        timestamp: startedAt + (steps + 1) * sampleInterval,
    })
    await session.detach()
    await expect(sheet).toBeHidden()

    // It opens again at rest, not where the flick left it.
    await open.click()
    await expect(sheet).toBeVisible()
    await expect.poll(() => sheetOffset(page)).toBe(0)
})

test('the tab bar and loading bar can be turned off', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'the tab strip is desktop only',
    )
    await page.goto('/settings/appearance')
    const tabs = page.getByRole('navigation', { name: '已打开的页面' })
    await expect(tabs).toBeVisible()
    await page.getByRole('switch', { name: '启用页签栏' }).press('Space')
    await expect(tabs).toBeHidden()
    await page.getByRole('switch', { name: '加载进度条' }).press('Space')
    expect(await stored(page)).toMatchObject({
        tabBar: false,
        progressBar: false,
    })
    await page.reload()
    await expect(
        page.getByRole('heading', { name: '偏好设置', level: 1 }),
    ).toBeVisible()
    await expect(tabs).toBeHidden()
    await page.getByRole('switch', { name: '启用页签栏' }).press('Space')
    await expect(tabs).toBeVisible()
})

test('sidebar groups fold, and with one group at a time opening one folds the others', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'narrow screens use the bottom bar',
    )
    await page.goto('/settings/appearance')
    const main = page.getByRole('navigation', { name: '主导航' })
    // Groups are Element Plus submenus; their titles fold and unfold them.
    const group = (name: string) =>
        main.locator('.el-sub-menu').filter({
            has: page.locator('.el-sub-menu__title', { hasText: name }),
        })
    const title = (name: string) =>
        group(name).locator('.el-sub-menu__title').first()
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'true')
    await title('安全').click()
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'false')
    await expect(main.getByRole('link', { name: '证书' })).toBeHidden()
    await title('安全').click()
    await expect(main.getByRole('link', { name: '证书' })).toBeVisible()

    await page.getByRole('switch', { name: '菜单栏行为' }).press('Space')
    // Only the current page's group stays open.
    await expect(group('安全')).toHaveAttribute('aria-expanded', 'false')
    await expect(group('设置')).toHaveAttribute('aria-expanded', 'true')
    await title('工作空间').click()
    await expect(group('工作空间')).toHaveAttribute('aria-expanded', 'true')
    await expect(group('设置')).toHaveAttribute('aria-expanded', 'false')
    // Navigating opens the destination's group.
    await main.getByRole('link', { name: 'VPN 节点' }).click()
    await expect(page).toHaveURL(/\/nodes$/)
    expect(await stored(page)).toMatchObject({ navMultiple: false })
})

test('the sidebar edge resizes by pointer and keyboard and restores its default', async ({
    page,
}, testInfo) => {
    test.skip(
        testInfo.project.name === 'mobile',
        'narrow screens use the bottom bar',
    )
    await page.goto('/settings/appearance')
    const sidebar = page.locator('.sidebar')
    const handle = page.getByRole('separator', { name: '调整侧栏宽度' })
    const start = (await sidebar.boundingBox())!.width

    const box = (await handle.boundingBox())!
    await page.mouse.move(box.x + box.width / 2, box.y + 200)
    await page.mouse.down()
    await page.mouse.move(box.x + box.width / 2 + 48, box.y + 200, {
        steps: 6,
    })
    // The edge follows the pointer while it is held.
    await expect
        .poll(async () => (await sidebar.boundingBox())!.width)
        .toBeGreaterThan(start + 40)
    await page.mouse.up()
    const dragged = (await stored(page)).sidebarWidth
    expect(typeof dragged).toBe('number')

    // Past the maximum it resists, then settles at the limit.
    await handle.focus()
    await handle.press('End')
    await expect.poll(async () => (await stored(page)).sidebarWidth).toBe(22)
    await expect(
        page.getByText('已固定为 22 rem', { exact: false }),
    ).toBeVisible()

    await page
        .getByRole('button', { name: '恢复默认', exact: true })
        .last()
        .click()
    expect(await stored(page)).not.toHaveProperty('sidebarWidth')
    await expect
        .poll(async () => Math.round((await sidebar.boundingBox())!.width))
        .toBe(Math.round(start))
})

test('kept pages return with their state while secrets are cleared', async ({
    page,
}, testInfo) => {
    await page.goto('/nodes')
    await page.getByRole('button', { name: '创建节点' }).click()
    await page.getByLabel('节点名称').fill('kept-name')
    await page.getByLabel('当前密码', { exact: true }).fill('not-the-password')
    const main = page.getByRole('navigation', { name: '主导航' })
    await main.getByRole('link', { name: '网络概览', exact: true }).click()
    await expect(
        page.getByRole('heading', { name: '网络概览', exact: true }),
    ).toBeVisible()
    await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await expect(page.getByLabel('节点名称')).toHaveValue('kept-name')
    await expect(page.getByLabel('当前密码', { exact: true })).toHaveValue('')

    // The refresh button reloads the page from scratch.
    if (testInfo.project.name !== 'mobile') {
        await page.getByRole('button', { name: '刷新当前页' }).click()
        await expect(page.getByLabel('节点名称')).toBeHidden()
    }

    // Without the page cache every visit starts fresh.
    await page.goto('/settings/appearance')
    await page.getByRole('switch', { name: '页面状态缓存' }).press('Space')
    await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await page.getByRole('button', { name: '创建节点' }).click()
    await page.getByLabel('节点名称').fill('fresh-name')
    await main.getByRole('link', { name: '网络概览', exact: true }).click()
    await main.getByRole('link', { name: 'VPN 节点', exact: true }).click()
    await expect(page.getByLabel('节点名称')).toBeHidden()
    expect(await stored(page)).toMatchObject({ pageCache: false })
})

test('page transition previews play only on hover', async ({ page }) => {
    await page.goto('/settings/appearance')
    const option = page.getByRole('radio', { name: '侧向滑入' })
    const preview = labelOf(option).locator('.preview-page')
    await expect(preview).toHaveCSS('animation-name', 'none')
    await labelOf(option).hover()
    await expect(preview).toHaveCSS('animation-name', 'preview-slide')
})

test('preferences saved under the old key carry over once', async ({
    page,
}) => {
    await page.addInitScript(() => {
        // Only on the first load: a reload must not bring the old key back.
        if (sessionStorage.getItem('seeded')) return
        sessionStorage.setItem('seeded', '1')
        localStorage.setItem(
            'vpn-console:preferences:v1',
            JSON.stringify({ palette: 'rose', mode: 'dark' }),
        )
    })
    await page.goto('/')
    await expect(page.locator('html')).toHaveAttribute('data-theme', 'rose')
    await expect(page.locator('html')).toHaveClass(/dark/)
    const keys = await page.evaluate(() => Object.keys(localStorage))
    expect(keys).toEqual([preferencesKey])
    expect(await stored(page)).toMatchObject({
        palette: 'rose',
        mode: 'dark',
    })
    await page.evaluate(() => {
        sessionStorage.clear()
    })
})

// The palette chips of one panel sit in two columns, and no name overflows
// its capsule. Measured until they rest, since the drawer slides in.
async function expectTwoColumns(group: Locator) {
    const chips = group.locator('label')
    await expect(chips).toHaveCount(8)
    await expect
        .poll(() =>
            chips.evaluateAll(
                (elements) =>
                    new Set(
                        elements.map((element) =>
                            Math.round(element.getBoundingClientRect().x),
                        ),
                    ).size,
            ),
        )
        .toBe(2)
    expect(
        await chips.evaluateAll((elements) =>
            elements.every(
                (element) => element.scrollWidth <= element.clientWidth,
            ),
        ),
    ).toBe(true)
}

test('palette chips fit two columns, mark the choice and set the text on the primary', async ({
    page,
}) => {
    // A palette retired from the list falls back to the default.
    await page.addInitScript((key) => {
        if (sessionStorage.getItem('seeded')) return
        sessionStorage.setItem('seeded', '1')
        localStorage.setItem(key, JSON.stringify({ palette: 'harbor' }))
    }, preferencesKey)
    await page.goto('/nodes')
    const html = page.locator('html')
    await expect(html).toHaveAttribute('data-theme', 'abyss')

    await page.getByRole('button', { name: '偏好设置' }).click()
    const drawer = page.getByRole('dialog', { name: '偏好设置' })
    await expect(drawer).toBeVisible()
    const group = drawer.getByRole('radiogroup', { name: '主题色' })
    await expectTwoColumns(group)
    const chosen = group.getByRole('radio', { name: '深海蓝', exact: true })
    await expect(chosen).toBeChecked()
    await expect(labelOf(chosen).locator('svg')).toBeVisible()
    await expect(
        labelOf(
            group.getByRole('radio', { name: '晴空蓝', exact: true }),
        ).locator('svg'),
    ).toHaveCount(0)
    const swatch = labelOf(chosen).locator('.palette-swatch')
    // White on the blue-violet, ink on the amber.
    await expect(swatch).toHaveCSS('color', 'rgb(255, 255, 255)')
    const amber = group.getByRole('radio', { name: '琥珀橙', exact: true })
    await tick(amber)
    await expect(html).toHaveAttribute('data-theme', 'amber')
    await expect(labelOf(amber).locator('.palette-swatch')).toHaveCSS(
        'color',
        'rgb(18, 20, 24)',
    )
    expect(await stored(page)).toMatchObject({ palette: 'amber' })
    await page.keyboard.press('Escape')
    await expect(drawer).toBeHidden()

    // On a 320px screen the preferences page still holds two columns.
    await page.setViewportSize({ width: 320, height: 720 })
    await page.goto('/settings/appearance')
    await expectTwoColumns(page.getByRole('radiogroup', { name: '主题色' }))
    expect(
        await page.evaluate(
            () => document.documentElement.scrollWidth <= window.innerWidth,
        ),
    ).toBe(true)
    await expect(paletteOption(page, '琥珀橙')).toBeChecked()
    await page.evaluate(() => {
        sessionStorage.clear()
    })
})
