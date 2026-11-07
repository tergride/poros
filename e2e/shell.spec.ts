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

test('the first Start can connect without TUN and System', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('start').click()
  await page.getByTestId('modes-none').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await expect(page.getByTestId('portal')).toContainText('настроенные на прокси 127.0.0.1:2090')
  await page.getByTestId('stop').click()
  await page.getByTestId('start').click()
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
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

  await page.getByTestId('preset-create').click()
  await expect(page.getByTestId('preset-name')).toHaveValue('')
  await expect(page.getByTestId('preset-base')).toHaveCount(0)
  await page.getByTestId('preset-name').fill('Работа')
  await page.getByLabel('Сайт, IP или подсеть').fill('example.com')
  await page.getByLabel('Сайт, IP или подсеть').press('Enter')
  await shot(page, 'preset-new')
  await page.getByTestId('preset-save').click()
  await expect(page.getByText('Свой пресет: сайтов — 1, приложений — 0')).toBeVisible()
  await expect(page.getByRole('radio', { name: 'Работа' })).toBeChecked()

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

  // The wizard took the poros:// links; the switch shows that and gives them up at once.
  await page.getByRole('link', { name: 'Запуск', exact: true }).click()
  const links = page.getByTestId('url-scheme')
  await expect(links).toHaveAttribute('data-state', 'checked')
  await links.click()
  await expect(links).toHaveAttribute('data-state', 'unchecked')
})

test('the lists of a preset take a pasted list, name bad lines and save with the preset', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await page.getByRole('button', { name: 'Создать копию' }).first().click()
  await page.getByTestId('preset-name').fill('Дом')
  await page.getByLabel('Сайт, IP или подсеть').fill('example.org')
  await page.getByLabel('Сайт, IP или подсеть').press('Enter')
  await page.getByTestId('preset-lists').click()

  const lists = page.getByRole('dialog', { name: 'Списки пресета «Дом»' })
  const text = lists.getByTestId('lists-text')
  await expect(text).toHaveValue('example.org')
  await expect(lists.getByTestId('lists-tab-proxy')).toHaveText('Через сервер · 1')
  await text.fill('example.org\nvideo.example.com\n# работа\nnot a site\n*.')
  await expect(lists.getByTestId('lists-invalid')).toHaveText(
    'Не распознаны 2 строки: 4 «not a site», 5 «*.». Исправьте или удалите их.',
  )
  await expect(lists.getByTestId('lists-apply')).toBeDisabled()
  await shot(page, 'preset-lists-invalid')
  await text.fill('example.org\nvideo.example.com\n# работа\nmail.example.net')

  await lists.getByTestId('lists-tab-direct').click()
  await text.fill('192.168.50.0/24\nsteam.exe\nC:\\Games\\game.exe\nVideo.example.com')
  await expect(lists.getByTestId('lists-duplicate')).toContainText('4 «Video.example.com» (Через сервер)')
  await expect(lists.getByTestId('lists-apply')).toBeDisabled()
  await text.fill('192.168.50.0/24\nsteam.exe\nC:\\Games\\game.exe')
  await expect(lists.getByTestId('lists-tab-direct')).toHaveText('Напрямую · 3')
  await expect(lists.getByTestId('lists-tab-proxy')).toHaveText('Через сервер · 3')
  await expect(lists.getByTestId('lists-apply')).toBeEnabled()
  await shot(page, 'preset-lists')
  await lists.getByTestId('lists-apply').click()
  await expect(lists).toBeHidden()
  await expect(page.getByTestId('site')).toHaveCount(4)

  await page.getByTestId('preset-save').click()
  await expect(page.getByText('На основе «Весь трафик»: добавлено: сайтов — 4, приложений — 2')).toBeVisible()
})

test('poros:// links open a dialog and add only after a yes', async ({ page }) => {
  await throughWizard(page)
  const encode = (text: string) => Buffer.from(text).toString('base64url')
  const send = (link: string) =>
    page.evaluate((detail) => window.dispatchEvent(new CustomEvent('poros-links', { detail })), [link])

  await send(`poros://addsub/${encode('https://vpn.example.org/s/71bd#Work')}`)
  const add = page.getByRole('dialog', { name: 'Добавить подписку' })
  await expect(add.getByLabel('Ссылка подписки')).toHaveValue('https://vpn.example.org/s/71bd')
  await expect(add.getByPlaceholder('Возьмём из подписки')).toHaveValue('Work')
  await shot(page, 'link-subscription')
  await add.getByTestId('add-subscription').click()
  await expect(add).toBeHidden()
  await expect(page.getByTestId('server-group')).toHaveCount(2)

  const preset = encode(
    JSON.stringify({ kind: 'poros-preset', v: 1, based_on: 'all', name: 'Дом', sites: [{ value: 'example.org', action: 'block' }], dns: 'tls://1.1.1.1' }),
  )
  await send(`poros://route/${preset}`)
  const ask = page.getByRole('dialog', { name: 'Добавить пресет из ссылки?' })
  await expect(ask).toContainText('Пресет «Дом» появится в «Моих пресетах»')
  await expect(ask.getByTestId('link-preset-summary')).toHaveText(
    'На основе «Весь трафик»: добавлено: сайтов — 1, приложений — 0. Свой DNS.',
  )
  await shot(page, 'link-preset')
  await ask.getByTestId('link-preset-add').click()
  await expect(ask).toBeHidden()

  await send(`poros://add/${encode('{}')}`)
  await expect(add.getByTestId('link-failure')).toContainText('Это ссылка на один сервер')
  await page.keyboard.press('Escape')
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await expect(page.getByText('Дом', { exact: true })).toBeVisible()
})

test('settings of a running connection offer a restart', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('system').click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Настройки' }).click()
  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await page.getByRole('switch').first().click()
  await expect(page.getByTestId('restart-banner')).toBeVisible()
  await shot(page, 'settings-restart')
  await page.getByRole('button', { name: 'Перезапустить подключение' }).click()
  await expect(page.getByTestId('restart-banner')).toBeHidden()
})

test('the kill switch asks for rights, blocks from a Start and lets go on Stop', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('system').click()
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Настройки' }).click()
  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await expect(page.getByTestId('segment-off')).toHaveAttribute('data-state', 'checked')
  await shot(page, 'settings-conn')

  // Without rights the dialog asks for them; Cancel puts the mode back, and the note says how the kill switch works.
  await expect(page.getByTestId('kill-switch').getByTestId('option-note')).toContainText('Как работает Kill switch')
  await page.getByTestId('segment-connected').click()
  await expect(page.getByRole('dialog', { name: 'Для Kill switch нужны права администратора' })).toBeVisible()
  await shot(page, 'settings-conn-admin')
  await page.getByRole('button', { name: 'Отмена' }).click()
  await expect(page.getByTestId('segment-off')).toHaveAttribute('data-state', 'checked')
  await expect(page.getByTestId('restart-banner')).toBeHidden()

  // "Try without rights" keeps the mode, which a Start without rights cannot apply.
  await page.getByTestId('segment-connected').click()
  await page.getByRole('button', { name: 'Попробовать без прав' }).click()
  await expect(page.getByTestId('kill-switch')).toContainText('пока вы не нажмёте «Стоп»')

  // A Start without rights does not connect past the block: the banner says why.
  await page.getByRole('link', { name: 'К серверам' }).click()
  await page.getByTestId('start').click()
  await expect(page.getByRole('dialog', { name: 'Для Kill switch нужны права администратора' })).toBeVisible()
  await page.getByRole('button', { name: 'Попробовать без прав' }).click()
  await expect(page.getByTestId('kill-switch-failed')).toContainText('Kill switch не включился без прав администратора.')
  await expect(page.getByTestId('connection-error')).toBeHidden()
  await expect(page.getByTestId('portal-state')).toHaveText('Не подключено')
  await shot(page, 'main-ks-failed')

  await page.getByTestId('kill-switch-failed').getByRole('button', { name: 'Перезапустить с правами' }).click()
  await expect(page.getByTestId('admin-chip')).toBeVisible()
  await expect(page.getByTestId('kill-switch-failed')).toBeHidden()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await expect(page.getByTestId('stop').getByTestId('kill-switch-lock')).toBeVisible()
  await shot(page, 'main-ks-on')

  await page.getByTestId('stop').click()
  await expect(page.getByTestId('portal-state')).toHaveText('Не подключено')
  await expect(page.getByTestId('kill-switch-lock')).toBeHidden()
})

test('an update is checked from the menu, downloaded and offered over the servers', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Проверить обновления' }).click()
  await expect(page.getByTestId('updates')).toContainText('Версия 0.1.1 загружена и готова к установке.')
  await expect(page.getByTestId('updates').getByRole('button', { name: 'Перезапустить и обновить' })).toBeVisible()
  await expect(page.getByText('Проверять обновления автоматически')).toBeVisible()
  await shot(page, 'settings-help-ready')

  await page.getByRole('link', { name: 'К серверам' }).click()
  await expect(page.getByTestId('update-ready')).toHaveText(/Готово обновление Poros 0\.1\.1\./)
  await expect(page.getByTestId('update-ready')).not.toContainText('Подключение вернётся само')
  await page.getByTestId('system').click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('update-ready')).toContainText('Подключение вернётся само.')
  await shot(page, 'main-update-ready')
  await page.getByTestId('update-ready').getByRole('button', { name: 'Позже' }).click()
  await expect(page.getByTestId('update-ready')).toBeHidden()
})
