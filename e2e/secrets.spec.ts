import { type Page, expect, test } from '@playwright/test'

import { restoreFromAnotherComputer } from './lost'

// Data saved on another computer, as the core in memory restores it from a backup made so: what lost its secrets says
// so, a start with it leads to where they are typed in again, and what is typed in again works.

const shot = (page: Page, name: string) =>
  page.screenshot({ path: `test-results/shots/${name}.png`, animations: 'disabled' })

async function throughWizard(page: Page): Promise<void> {
  await page.goto('/')
  await page.getByRole('button', { name: 'Далее' }).click()
  await page.getByLabel('Ссылка подписки').fill('https://sub.example.com/u/8f3a2c')
  await page.getByLabel('Ссылка подписки').press('Enter')
  await expect(page.getByTestId('link-found')).toBeVisible()
  await page.getByRole('button', { name: 'Далее' }).click()
  await page.getByRole('button', { name: 'Далее' }).click()
  await page.getByRole('button', { name: 'Готово' }).click()
  await expect(page.getByTestId('portal')).toBeVisible()
}

/** The first Start asks for the modes; System alone needs no rights. */
async function startWithSystem(page: Page): Promise<void> {
  await page.getByTestId('start').click()
  const modes = page.getByRole('dialog', { name: 'Как пускать трафик через сервер' })
  await modes.getByRole('checkbox').first().click()
  await modes.getByRole('checkbox').nth(1).click()
  await page.getByTestId('modes-connect').click()
  await expect(modes).toBeHidden()
}

test('the simple mode marks what lost its secrets and takes the link of a subscription again', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('portal-settings').click()
  await page.locator('a[href="#/settings/backup"]').click()
  await restoreFromAnotherComputer(page)
  await page.getByRole('link', { name: 'К серверам' }).click()
  const group = page.getByTestId('server-group')
  await expect(group).toContainText('3 сервера · ждёт ссылку')
  await expect(page.getByTestId('server-lost')).toHaveCount(3)
  await expect(group.getByRole('button', { name: 'Обновить подписку' })).toHaveCount(0)

  // The refused start leads to the link of the subscription, or to a backup; the simple mode has no editor.
  await startWithSystem(page)
  const banner = page.getByTestId('connection-error')
  await expect(banner).toContainText('Пароли и ключи сохранены на другом компьютере и здесь не читаются')
  await expect(banner.getByTestId('lost-edit')).toHaveCount(0)
  await expect(banner.getByTestId('lost-backup')).toBeVisible()
  await shot(page, 'simple-secrets-lost')

  await banner.getByTestId('lost-link-again').click()
  const dialog = page.getByRole('dialog', { name: 'Ссылка подписки' })
  await expect(dialog).toContainText('Ссылка «sub.example.com» сохранена на другом компьютере')
  await dialog.getByTestId('link-again-url').fill('https://sub.example.com/u/8f3a2c')
  await shot(page, 'simple-link-again')
  await dialog.getByTestId('link-again-save').click()
  await expect(dialog).toBeHidden()
  await expect(page.getByTestId('server-lost')).toHaveCount(0)
  await expect(group).not.toContainText('ждёт ссылку')

  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal')).toHaveAttribute('data-state', 'running')
})

test('the advanced mode leads a refused start to the editor and takes the secret of a code of OTP again', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByTestId('segment-advanced').click()
  await expect(page.getByTestId('profiles-table')).toBeVisible()
  await page.keyboard.press('Escape')
  await page.getByRole('button', { name: 'Импорт' }).click()
  await page.getByRole('menuitem', { name: 'Ввести ссылку…' }).click()
  await page.getByTestId('import-text').fill('trojan://word@mine.example.com:443?security=tls&sni=mine.example.com#Mine')
  await page.getByTestId('import-add').click()
  await expect(page.getByTestId('notice')).toContainText('Добавлено: 1 сервер.')
  await page.getByTestId('rail-settings').click()
  await page.locator('a[href="#/adv/settings/backup"]').click()
  await restoreFromAnotherComputer(page, 'Mine')

  // The subscription takes its link again instead of an update.
  await page.getByTestId('rail-profiles').click()
  await expect(page.getByTestId('group-title')).toHaveText('sub.example.com')
  await expect(page.getByTestId('group-link-again')).toBeVisible()
  await expect(page.getByTestId('server-lost')).toHaveCount(3)
  await page.getByTestId('group-item').nth(1).click()
  await expect(page.getByTestId('server-lost')).toHaveCount(1)

  await startWithSystem(page)
  const banner = page.getByTestId('connection-error')
  await expect(banner).toContainText('Mine was saved on another computer')
  await expect(banner.getByTestId('lost-link-again')).toHaveCount(0)
  await shot(page, 'adv-secrets-lost')
  await banner.getByTestId('lost-edit').click()
  const editor = page.getByRole('dialog', { name: 'Профиль' })
  await expect(editor.getByTestId('profile-lost')).toHaveText('Сохранены на другом компьютере и здесь не читаются: Пароль. Введите их заново.')
  await shot(page, 'adv-profile-lost')
  await editor.locator('[data-field="trojan.password"] input').fill('word')
  await expect(editor.getByTestId('profile-lost')).toHaveCount(0)
  await editor.getByTestId('profile-save').click()
  await expect(editor).toBeHidden()
  await expect(page.getByTestId('server-lost')).toHaveCount(0)
  await page.getByTestId('start').click()
  await expect(page.getByTestId('connbar')).toHaveAttribute('data-state', 'running')

  // A code of OTP without its secret gives no code, until the secret is typed in again.
  await page.getByTestId('rail-tools').click()
  const row = page.getByTestId('otp-row')
  await expect(row.getByTestId('otp-lost')).toHaveText('Секрет сохранён на другом компьютере')
  await expect(row.getByTestId('otp-copy')).toBeDisabled()
  await expect(row.getByTestId('otp-left')).toHaveCount(0)
  await shot(page, 'adv-otp-lost')
  await row.getByTestId('otp-secret-again').click()
  const dialog = page.getByTestId('otp-dialog')
  await expect(dialog.getByTestId('otp-secret')).toHaveValue('')
  await dialog.getByTestId('otp-secret').fill('JBSWY3DPEHPK3PXP')
  await page.getByTestId('otp-save').click()
  await expect(row.getByTestId('otp-code')).toHaveText(/^\d{3} \d{3}$/)
  await expect(row.getByTestId('otp-lost')).toHaveCount(0)
})
