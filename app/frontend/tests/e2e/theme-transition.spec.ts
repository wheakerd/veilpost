import { expect, type Locator, type Page } from '@playwright/test'
import { test, signIn } from './real-auth'

declare global {
    interface Window {
        appearanceIconAnimations?: Animation[]
    }
}

// Hold the feedback in place to check that it never blocks another input.
async function recordIconAnimations(page: Page) {
    await page.evaluate(() => {
        const recorded: Animation[] = []
        window.appearanceIconAnimations = recorded
        const animate = Reflect.get(SVGElement.prototype, 'animate')
        SVGElement.prototype.animate = function (
            this: SVGElement,
            ...args: Parameters<SVGElement['animate']>
        ) {
            const animation = animate.call(this, ...args)
            if (
                this.closest('button')
                    ?.getAttribute('aria-label')
                    ?.startsWith('界面外观：')
            ) {
                recorded.push(animation)
                animation.pause()
            }
            return animation
        }
    })
    return () =>
        page.evaluate(() =>
            (window.appearanceIconAnimations ?? []).map(
                (animation) => animation.effect?.getTiming().duration,
            ),
        )
}

const modeButton = (page: Page) =>
    page.getByRole('button', { name: /^界面外观：/ })

test.beforeEach(async ({ page, account }) => {
    await signIn(page, account)
    await page.emulateMedia({
        reducedMotion: 'no-preference',
        colorScheme: 'light',
    })
})

test('appearance controls expand and contract the dark layer and accept a second press', async ({
    page,
}, testInfo) => {
    await page.goto('/settings/appearance')
    const mode = modeButton(page)
    const press = (control: Locator) =>
        testInfo.project.use.hasTouch ? control.tap() : control.click()
    const html = page.locator('html')
    const recorded = await recordIconAnimations(page)
    const history = await page.evaluateHandle(() => {
        const circles: {
            layer: string
            direction: string
            duration: string
        }[] = []
        document.addEventListener('animationstart', (event) => {
            if (event.animationName !== 'appearance-reveal') return
            const style = getComputedStyle(
                document.documentElement,
                event.pseudoElement,
            )
            circles.push({
                layer: event.pseudoElement,
                direction: style.animationDirection,
                duration: style.animationDuration,
            })
        })
        return circles
    })
    const circles = () => history.jsonValue()
    const expand = {
        layer: '::view-transition-new(root)',
        direction: 'normal',
        duration: '0.59s',
    }
    const contract = {
        layer: '::view-transition-old(root)',
        direction: 'reverse',
        duration: '0.59s',
    }
    await press(mode)
    await expect(html).toHaveClass(/dark/)
    await expect.poll(recorded).toEqual([150])
    await expect.poll(circles).toEqual([expand])

    await press(mode)
    await expect(html).not.toHaveClass(/dark/)
    await expect.poll(recorded).toEqual([150, 150])
    await expect.poll(circles).toEqual([expand, contract])

    const appearance = page.getByRole('radiogroup', { name: '界面外观' })
    await press(appearance.getByText('深色', { exact: true }))
    await expect.poll(circles).toEqual([expand, contract, expand])
    await press(appearance.getByText('跟随系统', { exact: true }))
    await expect.poll(circles).toEqual([expand, contract, expand, contract])
    await expect(html).not.toHaveClass(/dark|appearance-reveal/)
})

test('system changes and keyboard switching update the mode without icon motion', async ({
    page,
}) => {
    await page.goto('/')
    const html = page.locator('html')
    const mode = modeButton(page)
    const recorded = await recordIconAnimations(page)
    await page.emulateMedia({ colorScheme: 'dark' })
    await expect(html).toHaveClass(/dark/)

    await mode.press('Enter')
    await expect(html).not.toHaveClass(/dark/)
    await mode.press('Space')
    await expect(html).toHaveClass(/dark/)
    expect(await recorded()).toEqual([])
})

test('system reduced motion suppresses appearance icon feedback', async ({
    page,
}) => {
    await page.emulateMedia({ reducedMotion: 'reduce' })
    await page.goto('/')
    const recorded = await recordIconAnimations(page)
    await modeButton(page).click()
    await expect(page.locator('html')).toHaveClass(/dark/)
    await expect(page.locator('html')).not.toHaveClass(/appearance-reveal/)
    expect(await recorded()).toEqual([])
})

test('the reduced-motion preference suppresses appearance icon feedback', async ({
    page,
}) => {
    await page.goto('/settings/appearance')
    await page.getByRole('switch', { name: '减少动态效果' }).press('Space')
    await expect(page.locator('html')).toHaveAttribute(
        'data-reduced-motion',
        'true',
    )
    const recorded = await recordIconAnimations(page)
    const appearance = page.getByRole('radiogroup', { name: '界面外观' })
    await appearance.getByText('深色', { exact: true }).click()
    await expect(page.locator('html')).toHaveClass(/dark/)
    await modeButton(page).click()
    await expect(page.locator('html')).not.toHaveClass(/dark/)
    await expect(page.locator('html')).not.toHaveClass(/appearance-reveal/)
    expect(await recorded()).toEqual([])
})
