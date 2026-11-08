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
  await expect(page.getByTestId('profile-row')).toHaveCount(3)
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

async function toAdvanced(page: Page): Promise<void> {
  await throughWizard(page)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByTestId('segment-advanced').click()
  await expect(page.getByTestId('profiles-table')).toBeVisible()
}

test('the profiles select as a list of Windows and act on the selection', async ({ page }) => {
  await toAdvanced(page)
  const rows = page.getByTestId('profile-row')
  await expect(rows).toHaveCount(3)
  await rows.nth(0).click()
  await rows.nth(2).click({ modifiers: ['Shift'] })
  await expect(page.getByTestId('bulk-bar')).toContainText('Выбрано: 3')
  await rows.nth(1).click({ modifiers: ['Control'] })
  await expect(page.getByTestId('bulk-bar')).toContainText('Выбрано: 2')
  await shot(page, 'adv-profiles-selected')

  // The right button keeps the selection for its menu.
  await rows.nth(0).click({ button: 'right' })
  await expect(page.getByRole('menu')).toContainText('Выбрано: 2')
  await page.getByRole('menuitem', { name: 'Клонировать' }).click()
  await expect(rows).toHaveCount(5)

  // Outside the selection it takes the row, and Delete removes it after a yes.
  await rows.nth(4).click({ button: 'right' })
  await page.getByRole('menuitem', { name: 'Удалить' }).click()
  await page.getByTestId('remove-profiles').click()
  await expect(rows).toHaveCount(4)

  await page.getByRole('button', { name: 'Тесты' }).click()
  await page.getByRole('menuitem', { name: 'Проверить задержку группы' }).click()
  await expect(rows.first()).toContainText('42 мс')
  await page.getByRole('button', { name: 'Очистка' }).click()
  await page.getByRole('menuitem', { name: 'Удалить недоступные' }).click()
  await expect(rows).toHaveCount(3)
})

test('profiles come from a pasted text and a link, into a group of the user', async ({ page }) => {
  await toAdvanced(page)
  await page.getByRole('button', { name: 'Импорт' }).click()
  await page.getByRole('menuitem', { name: 'Ввести ссылку…' }).click()
  await page.getByTestId('import-text').fill('trojan://p@one.example.com:443#One\ntrojan://p@two.example.com:443#Two')
  await page.getByTestId('import-add').click()
  await expect(page.getByTestId('notice')).toContainText('Добавлено: 2 сервера.')
  await expect(page.getByTestId('group-item')).toHaveCount(2)
  await page.getByTestId('group-item').nth(1).click()
  await expect(page.getByTestId('group-title')).toHaveText('Мои серверы')
  await expect(page.getByTestId('profile-row')).toHaveCount(2)

  const encode = (text: string) => Buffer.from(text).toString('base64url')
  const server = { type: 'trojan', name: 'Home', server: 'de.example.com', server_port: 443, trojan: { password: 'p' } }
  await page.evaluate(
    (detail) => window.dispatchEvent(new CustomEvent('poros-links', { detail })),
    [`poros://add/${encode(JSON.stringify(server))}`],
  )
  const ask = page.getByRole('dialog', { name: 'Добавить сервер из ссылки?' })
  await expect(ask).toContainText('de.example.com:443')
  await shot(page, 'adv-link-server')
  await page.getByTestId('link-add').click()
  await expect(page.getByTestId('profile-row')).toHaveCount(3)
})

test('the group editor keeps the address and the updates of a subscription', async ({ page }) => {
  await toAdvanced(page)
  await page.getByTestId('edit-group').click()
  const editor = page.getByRole('dialog', { name: 'Группа' })
  await editor.getByRole('tab', { name: 'Запрос' }).click()
  await editor.getByTestId('group-address').fill('104.16.1.1')
  await editor.getByRole('tab', { name: 'Обновление' }).click()
  await editor.getByTestId('group-test').click()
  await shot(page, 'adv-group-editor')
  await editor.getByTestId('group-save').click()
  await expect(editor).toBeHidden()

  await page.getByTestId('edit-group').click()
  await editor.getByRole('tab', { name: 'Запрос' }).click()
  await expect(editor.getByTestId('group-address')).toHaveValue('104.16.1.1')
})
