import { expect, type Page } from '@playwright/test'
import { test, signIn } from './real-auth'

// The circular light/dark switch (src/theme/index.ts): the dark view spreads
// out of the pressed point and gathers back into it on the way to light.

interface SwitchAnimation {
    pseudoElement: string
    // The clip-path keyframes, empty for animations that do not clip.
    clipPath: string[]
    // Set for CSS animations, such as the default cross-fade.
    cssName: string | null
}

declare global {
    interface Window {
        switchAnimations?: SwitchAnimation[]
        switchAnimationObjects?: Animation[]
    }
}

// Samples the view-transition animations every frame from now on, so a
// 560 ms circle cannot slip between two polls.
async function recordSwitchAnimations(page: Page) {
    await page.evaluate(() => {
        const found: SwitchAnimation[] = []
        const objects: Animation[] = []
        window.switchAnimations = found
        window.switchAnimationObjects = objects
        const sample = () => {
            for (const animation of document.documentElement.getAnimations({
                subtree: true,
            })) {
                const effect = animation.effect
                if (
                    objects.includes(animation) ||
                    !(effect instanceof KeyframeEffect) ||
                    !effect.pseudoElement?.startsWith('::view-transition')
                )
                    continue
                objects.push(animation)
                found.push({
                    pseudoElement: effect.pseudoElement,
                    clipPath: effect
                        .getKeyframes()
                        .map((frame) => frame.clipPath)
                        .filter((value) => typeof value === 'string'),
                    cssName:
                        animation instanceof CSSAnimation
                            ? animation.animationName
                            : null,
                })
            }
            requestAnimationFrame(sample)
        }
        sample()
    })
    return {
        list: () => page.evaluate(() => window.switchAnimations ?? []),
        // Whether every recorded animation has stopped (finished, or
        // cancelled once the transition settled).
        settled: () =>
            page.evaluate(() =>
                (window.switchAnimationObjects ?? []).every(
                    (animation) => animation.playState !== 'running',
                ),
            ),
    }
}

const circles = (animations: SwitchAnimation[]) =>
    animations.filter((animation) => animation.clipPath.length > 0)

// circle(<radius>px at <x>px <y>px) as numbers.
function parseCircle(value: string | undefined) {
    const numbers = (value ?? '').match(/-?\d+(?:\.\d+)?/g)?.map(Number)
    expect(numbers, `clip-path ${value ?? 'missing'}`).toHaveLength(3)
    const [radius = 0, x = 0, y = 0] = numbers ?? []
    return { radius, x, y }
}

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
    await page.emulateMedia({
        reducedMotion: 'no-preference',
        colorScheme: 'light',
    })
})

const modeButton = (page: Page) =>
    page.getByRole('button', { name: /^界面外观：/ })

test('dark spreads out of the pressed point and light gathers back into it', async ({
    page,
}) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.goto('/')
    const html = page.locator('html')
    const mode = modeButton(page)
    await expect(mode).toHaveAccessibleName('界面外观：浅色')
    await expect(html).not.toHaveClass(/dark/)

    const recorder = await recordSwitchAnimations(page)
    const box = await mode.boundingBox()
    expect(box).not.toBeNull()
    const pressX = box!.x + box!.width / 2
    const pressY = box!.y + box!.height / 2
    const viewport = page.viewportSize()!

    // The first click switches from system light to dark and expands the circle.
    await mode.click()
    await expect(mode).toHaveAccessibleName('界面外观：深色')
    await expect(html).toHaveClass(/dark/)
    await expect
        .poll(async () => circles(await recorder.list()))
        .toHaveLength(1)
    const [expand] = circles(await recorder.list())
    expect(expand?.pseudoElement).toBe('::view-transition-new(root)')
    const from = parseCircle(expand?.clipPath[0])
    const to = parseCircle(expand?.clipPath.at(-1))
    expect(from.radius).toBe(0)
    expect(Math.abs(from.x - pressX)).toBeLessThan(2)
    expect(Math.abs(from.y - pressY)).toBeLessThan(2)
    expect(to.x).toBe(from.x)
    expect(to.y).toBe(from.y)
    const farthest = Math.hypot(
        Math.max(pressX, viewport.width - pressX),
        Math.max(pressY, viewport.height - pressY),
    )
    expect(to.radius).toBeGreaterThanOrEqual(farthest - 2)
    await expect.poll(recorder.settled).toBe(true)

    // Dark to light: the old dark view shrinks into the button.
    await mode.click()
    await expect(mode).toHaveAccessibleName('界面外观：浅色')
    await expect(html).not.toHaveClass(/dark/)
    await expect
        .poll(async () => circles(await recorder.list()))
        .toHaveLength(2)
    const contract = circles(await recorder.list())[1]
    expect(contract?.pseudoElement).toBe('::view-transition-old(root)')
    expect(parseCircle(contract?.clipPath[0]).radius).toBeGreaterThanOrEqual(
        farthest - 2,
    )
    const end = parseCircle(contract?.clipPath.at(-1))
    expect(end.radius).toBe(0)
    expect(Math.abs(end.x - pressX)).toBeLessThan(2)
    expect(Math.abs(end.y - pressY)).toBeLessThan(2)
    await expect.poll(recorder.settled).toBe(true)

    // The circle replaces the default cross-fade of the root snapshots.
    const fades = (await recorder.list()).filter(
        (animation) =>
            animation.cssName !== null &&
            /^::view-transition-(old|new)\(root\)$/.test(
                animation.pseudoElement,
            ),
    )
    expect(fades).toEqual([])
    await expect(html).not.toHaveClass(/theme-(expand|contract)/)
    expect(errors).toEqual([])
})

test('a switch during a running one settles on the latest mode without errors', async ({
    page,
}) => {
    const errors: string[] = []
    page.on('pageerror', (error) => errors.push(error.message))
    await page.goto('/')
    const html = page.locator('html')
    const mode = modeButton(page)
    await expect(mode).toHaveAccessibleName('界面外观：浅色')
    const recorder = await recordSwitchAnimations(page)
    // Switch to dark, then back to light without waiting for the circle.
    await mode.focus()
    await page.keyboard.press('Enter')
    await page.keyboard.press('Enter')
    await expect(mode).toHaveAccessibleName('界面外观：浅色')
    await expect(html).not.toHaveClass(/dark/)
    await expect.poll(recorder.settled).toBe(true)
    await expect(html).not.toHaveClass(/dark/)
    await expect(html).not.toHaveClass(/theme-(expand|contract)/)
    // A keyboard switch starts from the button, not from a stale press.
    const box = (await mode.boundingBox())!
    for (const animation of circles(await recorder.list())) {
        const point = parseCircle(
            animation.pseudoElement === '::view-transition-new(root)'
                ? animation.clipPath[0]
                : animation.clipPath.at(-1),
        )
        expect(Math.abs(point.x - (box.x + box.width / 2))).toBeLessThan(2)
        expect(Math.abs(point.y - (box.y + box.height / 2))).toBeLessThan(2)
    }
    expect(errors).toEqual([])
})

test('the system reduced-motion setting switches instantly without a circle', async ({
    page,
}) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto('/')
    const html = page.locator('html')
    const mode = modeButton(page)
    const recorder = await recordSwitchAnimations(page)
    await expect(mode).toHaveAccessibleName('界面外观：浅色')
    await mode.click()
    // The class changes in the same task as the click; no transition defers it.
    expect(
        await page.evaluate(() =>
            document.documentElement.classList.contains('dark'),
        ),
    ).toBe(true)
    await mode.click()
    await expect(html).not.toHaveClass(/dark/)
    expect(await recorder.list()).toEqual([])
})

test('the reduced-motion preference switches instantly without a circle', async ({
    page,
}) => {
    await page.goto('/settings/appearance')
    await page.getByRole('switch', { name: '减少动态效果' }).press('Space')
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    const html = page.locator('html')
    const recorder = await recordSwitchAnimations(page)
    await page
        .getByRole('radiogroup', { name: '界面外观' })
        .getByText('深色', { exact: true })
        .click()
    await expect(html).toHaveClass(/dark/)
    await page
        .getByRole('radiogroup', { name: '界面外观' })
        .getByText('浅色', { exact: true })
        .click()
    await expect(html).not.toHaveClass(/dark/)
    expect(await recorder.list()).toEqual([])
})
