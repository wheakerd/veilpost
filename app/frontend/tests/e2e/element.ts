import { expect, type Locator, type Page } from '@playwright/test'

// Element Plus selects are comboboxes with a listbox in a popper, not native
// <select> elements; these helpers drive and read them like a person would.
export async function choose(page: Page, field: Locator, option: string) {
    // The visible wrapper takes the click; the inner input sits beneath the
    // selected label.
    await chosen(page, field).locator('.el-select__wrapper').click()
    const item = page.getByRole('option', { name: option, exact: true })
    await item.click()
    await expect(item).toBeHidden()
}

export function chosen(page: Page, field: Locator) {
    return page.locator('.el-select', { has: field })
}

// Element Plus checkboxes and the console's chip radios keep the native
// input visually hidden inside its label, so a person clicks the label.
export function labelOf(control: Locator) {
    return control.locator('xpath=ancestor::label[1]')
}

export async function tick(control: Locator) {
    await labelOf(control).click()
    await expect(control).toBeChecked()
}

export function paletteOption(page: Page, name: string) {
    return page.getByRole('radio', { name, exact: true }).first()
}
