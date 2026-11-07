import { type Page, expect, test } from '@playwright/test'

// The simple mode against the core in memory: a first start through the wizard, then the main screen, the dialogs
// around Start and the settings. Screenshots go to test-results/shots for a look against the mockups.

const shot = (page: Page, name: string) =>
  page.screenshot({ path: `test-results/shots/${name}.png`, animations: 'disabled' })

async function throughWizard(page: Page): Promise<void> {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Добавьте подписку' })).toBeVisible()
  await shot(page, 'wizard-1')
  await page.getByLabel('Ссылка подписки').fill('https://sub.example.com/u/8f3a2c')
  await page.getByLabel('Ссылка подписки').press('Enter')
  await expect(page.getByTestId('link-found')).toContainText('Подписка найдена: 3 сервера, «sub.example.com»')
  await shot(page, 'wizard-1-found')
  await page.getByRole('button', { name: 'Далее' }).click()
  await expect(page.getByRole('heading', { name: 'Как запускать Poros' })).toBeVisible()
  await shot(page, 'wizard-2')
  await page.getByRole('button', { name: 'Далее' }).click()
  await expect(page.getByRole('heading', { name: 'Какой трафик пускать через сервер' })).toBeVisible()
  await expect(page.getByText('Усиленный обход')).toBeVisible()
  await shot(page, 'wizard-3')
  await page.getByRole('button', { name: 'Готово' }).click()
  await expect(page.getByTestId('portal')).toBeVisible()
}

test('the shell renders with the mockup fonts and the system language', async ({ page }) => {
  await page.goto('/')
  await expect(page.getByRole('button', { name: 'Свернуть' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Закрыть' })).toBeVisible()
  const loaded = await page.evaluate(async () => {
    await document.fonts.ready
    return [...document.fonts].filter((font) => font.status === 'loaded').map((font) => font.family.replaceAll('"', ''))
  })
  expect(loaded).toEqual(expect.arrayContaining(['Onest', 'Unbounded', 'JetBrains Mono']))
})

test('a first start goes through the wizard to the servers', async ({ page }) => {
  await throughWizard(page)
  await expect(page.getByTestId('server')).toHaveCount(3)
  await expect(page.getByTestId('portal-server')).toHaveText('Germany')
  await shot(page, 'main-off')

  await page.getByRole('button', { name: 'Проверить серверы' }).click()
  await expect(page.getByTestId('server').first()).toContainText('42 мс')
  await expect(page.getByTestId('server').nth(2)).toContainText('нет ответа')
})

test('the first Start asks what to turn on, and TUN asks for rights', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('start').click()
  await expect(page.getByRole('dialog', { name: 'Как пускать трафик через сервер' })).toBeVisible()
  await shot(page, 'main-modes')
  await page.getByRole('dialog').getByRole('checkbox').first().click()
  await page.getByRole('dialog').getByRole('checkbox').nth(1).click()
  await page.getByTestId('modes-connect').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await expect(page.getByTestId('portal')).toContainText('Загрузка')
  await expect(page.getByTestId('system')).toHaveAttribute('data-state', 'checked')
  await expect(page.getByTestId('tun')).toHaveAttribute('data-state', 'unchecked')
  await shot(page, 'main-on')

  await page.getByTestId('portal-ip').click()
  await expect(page.getByTestId('leak-verdict')).toContainText('Утечки DNS нет')
  await shot(page, 'main-ip')
  await page.keyboard.press('Escape')

  await page.getByTestId('tun').click()
  await expect(page.getByRole('dialog', { name: 'Для TUN нужны права администратора' })).toBeVisible()
  await shot(page, 'main-admin')
  await page.getByTestId('admin-restart').click()
  await expect(page.getByTestId('admin-chip')).toBeVisible()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await page.getByTestId('stop').click()
  await expect(page.getByTestId('portal-state')).toHaveText('Не подключено')
})

test('a preset copy is made, saved and chosen', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await expect(page.getByRole('heading', { name: 'Пресеты', exact: true })).toBeVisible()
  await shot(page, 'settings-presets')
  await page.getByRole('button', { name: 'Создать копию' }).first().click()
  await page.getByTestId('preset-name').fill('Дом')
  await page.getByLabel('Сайт, IP или подсеть').fill('example.org')
  await page.getByLabel('Сайт, IP или подсеть').press('Enter')
  await expect(page.getByTestId('site')).toHaveCount(1)
  await shot(page, 'preset-edit')
  await page.getByTestId('preset-save').click()
  await expect(page.getByText('Дом', { exact: true })).toBeVisible()
  await expect(page.getByRole('radio', { name: 'Дом' })).toBeChecked()

  for (const [link, heading] of [
    ['Подключение', 'Подключение'],
    ['Подписки', 'Подписки'],
    ['Запуск', 'Запуск'],
    ['Внешний вид', 'Внешний вид'],
    ['Помощь', 'Помощь'],
  ] as const) {
    await page.getByRole('link', { name: link, exact: true }).click()
    await expect(page.getByRole('heading', { name: heading, exact: true })).toBeVisible()
    await shot(page, `settings-${link}`)
  }
})

test('settings of a running connection offer a restart', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('system').click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Настройки' }).click()
  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await page.getByRole('switch').nth(1).click()
  await expect(page.getByTestId('restart-banner')).toBeVisible()
  await shot(page, 'settings-restart')
  await page.getByRole('button', { name: 'Перезапустить подключение' }).click()
  await expect(page.getByTestId('restart-banner')).toBeHidden()
})
