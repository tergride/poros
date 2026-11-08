import { type Page, expect, test } from '@playwright/test'

// The frame of the advanced mode against the core in memory: the move between the modes, the rail, the connection
// bar and the settings that wait for «Save».

const shot = (page: Page, name: string) =>
  page.screenshot({ path: `test-results/shots/${name}.png`, animations: 'disabled' })

async function throughWizard(page: Page): Promise<void> {
  await page.goto('/')
  await page.getByLabel('Ссылка подписки').fill('https://sub.example.com/u/8f3a2c')
  await page.getByLabel('Ссылка подписки').press('Enter')
  await expect(page.getByTestId('link-found')).toBeVisible()
  await page.getByRole('button', { name: 'Далее' }).click()
  await page.getByRole('button', { name: 'Далее' }).click()
  await page.getByRole('button', { name: 'Готово' }).click()
  await expect(page.getByTestId('portal')).toBeVisible()
}

test('the mode chip moves to the advanced mode on the same screen and back', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('portal-settings').click()
  await expect(page).toHaveURL(/#\/settings\/conn$/)

  await page.getByTestId('mode-chip').click()
  await expect(page.getByRole('dialog', { name: 'Перейти в расширенный режим?' })).toBeVisible()
  await page.getByTestId('mode-switch').click()
  // The same settings page, in the advanced frame.
  await expect(page).toHaveURL(/#\/adv\/settings\/conn$/)
  await expect(page.getByTestId('connbar')).toBeVisible()
  await expect(page.getByTestId('mode-chip')).toHaveText('Расширенный режим')
  await shot(page, 'adv-settings')

  await page.getByTestId('rail-profiles').click()
  await expect(page.getByTestId('server')).toHaveCount(3)
  await page.getByTestId('rail-stats').click()
  await expect(page.getByTestId('section-soon')).toBeVisible()
  await page.getByTestId('rail-profiles').click()
  await shot(page, 'adv-profiles')

  // A start from the connection bar, as from the portal.
  await page.getByTestId('start').click()
  // System alone: TUN would ask for rights.
  await page.getByRole('dialog').getByRole('checkbox').first().click()
  await page.getByRole('dialog').getByRole('checkbox').nth(1).click()
  await page.getByTestId('modes-connect').click()
  await expect(page.getByTestId('connbar')).toHaveAttribute('data-state', 'running')
  await expect(page.getByTestId('connbar-state')).toHaveText('Germany')

  // Back: the profiles stand for the servers, and the connection runs on.
  await page.getByTestId('mode-chip').click()
  await page.getByTestId('mode-switch').click()
  await expect(page).toHaveURL(/#\/main$/)
  await expect(page.getByTestId('portal')).toHaveAttribute('data-state', 'running')
})

test('settings of the advanced mode wait for Save and ask before leaving with changes', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByTestId('segment-advanced').click()
  await expect(page).toHaveURL(/#\/adv\/profiles$/)

  await page.getByTestId('rail-settings').click()
  await expect(page).toHaveURL(/#\/adv\/settings\/conn$/)
  const lan = page.getByTestId('allow-lan')
  await lan.click()
  await expect(lan).toHaveAttribute('data-state', 'checked')
  await expect(page.getByTestId('savebar')).toBeVisible()
  await shot(page, 'adv-settings-dirty')

  // Leaving asks; Escape stays on the page with the change.
  await page.getByRole('link', { name: 'Подписки' }).click()
  await expect(page.getByRole('dialog', { name: 'Сохранить изменения?' })).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page).toHaveURL(/#\/adv\/settings\/conn$/)
  await expect(lan).toHaveAttribute('data-state', 'checked')

  // Leaving without saving drops the change.
  await page.getByRole('link', { name: 'Подписки' }).click()
  await page.getByTestId('unsaved-leave').click()
  await expect(page).toHaveURL(/#\/adv\/settings\/subs$/)
  await page.getByRole('link', { name: 'Подключение' }).click()
  await expect(page.getByTestId('allow-lan')).toHaveAttribute('data-state', 'unchecked')

  // Save keeps it.
  await page.getByTestId('allow-lan').click()
  await page.getByTestId('settings-save').click()
  await expect(page.getByTestId('savebar')).toBeHidden()
  await page.getByRole('link', { name: 'Подписки' }).click()
  await expect(page).toHaveURL(/#\/adv\/settings\/subs$/)
  await page.getByRole('link', { name: 'Подключение' }).click()
  await expect(page.getByTestId('allow-lan')).toHaveAttribute('data-state', 'checked')
})
