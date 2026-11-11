import { type Page, expect, test } from '@playwright/test'

// The simple mode against the core in memory: a first start through the wizard, then the main screen, the dialogs
// around Start and the settings. Screenshots go to test-results/shots for a look against the mockups.

const shot = (page: Page, name: string) =>
  page.screenshot({ path: `test-results/shots/${name}.png`, animations: 'disabled' })

async function throughWizard(page: Page): Promise<void> {
  await page.goto('/')
  await expect(page.getByRole('heading', { name: 'Выберите режим интерфейса' })).toBeVisible()
  await expect(page.getByTestId('wizard-mode-simple').getByRole('radio')).toBeChecked()
  await shot(page, 'wizard-0-mode')
  await page.getByRole('button', { name: 'Далее' }).click()
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

test('the wizard of the simple mode points a link to one server to the advanced mode', async ({ page }) => {
  await page.goto('/')
  await page.getByRole('button', { name: 'Далее' }).click()
  await page.getByLabel('Ссылка подписки').fill('trojan://p@tr.example.com:443#Home')
  await page.getByLabel('Ссылка подписки').press('Enter')
  await expect(page.getByTestId('link-failure')).toHaveText(
    'Это ссылка на один сервер, а не подписка. Серверы вручную добавляются в расширенном режиме, его можно выбрать на шаге 1.',
  )
  await shot(page, 'wizard-1-single')
})

test('the address of the local proxy says what it is and copies its links', async ({ page }) => {
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await throughWizard(page)
  // Without TUN and System the hint names the proxy; its address opens the dialog.
  await page.getByTestId('portal-proxy').click()
  const dialog = page.getByRole('dialog', { name: 'Локальный прокси' })
  await expect(dialog).toContainText('Подключаться может только этот компьютер')
  await dialog.getByTestId('proxy-socks5').click()
  await expect(dialog.getByTestId('proxy-socks5')).toContainText('Скопировано')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('socks5://127.0.0.1:2090')
  await shot(page, 'main-proxy')
  await dialog.getByTestId('proxy-http').click()
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe('http://127.0.0.1:2090')
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

test('the settings open from the portal, and a list of choices picks a value', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('portal-settings').click()
  await expect(page.getByRole('heading', { name: 'Подключение', exact: true })).toBeVisible()
  await page.getByRole('link', { name: 'Внешний вид', exact: true }).click()
  await page.getByRole('combobox', { name: 'Язык' }).click()
  await expect(page.getByRole('option', { name: 'English' })).toBeVisible()
  await shot(page, 'select-open')
  await page.getByRole('option', { name: 'English' }).click()
  await expect(page.getByRole('heading', { name: 'Appearance', exact: true })).toBeVisible()
  await expect(page.getByRole('combobox', { name: 'Language' })).toHaveText('English')

  // With large text the list still opens under its button, as the menus do.
  await page.getByRole('combobox', { name: 'Text size' }).click()
  await page.getByRole('option', { name: 'Large' }).click()
  const language = page.getByRole('combobox', { name: 'Language' })
  const button = await language.boundingBox()
  await language.click()
  const list = page.getByRole('listbox')
  await expect(list).toBeVisible()
  const opened = await list.boundingBox()
  expect(Math.abs(opened!.y - (button!.y + button!.height))).toBeLessThan(16)
  expect(Math.abs(opened!.x - button!.x)).toBeLessThan(16)
  await shot(page, 'select-open-large')
})

test('the DNS page saves the default servers and the way of names at once', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('portal-settings').click()
  await page.getByRole('link', { name: 'DNS', exact: true }).click()
  await expect(page.getByRole('heading', { name: 'DNS', exact: true })).toBeVisible()
  await page.getByRole('combobox', { name: 'Для трафика через сервер' }).click()
  await page.getByRole('option', { name: 'Cloudflare' }).click()
  await page.getByRole('combobox', { name: 'Для прямого трафика' }).click()
  await page.getByRole('option', { name: 'Свой…' }).click()
  const own = page.getByRole('textbox', { name: 'Для прямого трафика: свой сервер' })
  await own.fill('dns.example')
  await own.press('Enter')
  await expect(page.getByTestId('notice')).toContainText('dns.example')
  await own.fill('tls://dns.example')
  await own.press('Enter')
  await page.getByTestId('dns-routing').getByTestId('segment-proxy').click()
  await page.getByTestId('fake-ip').click()
  await shot(page, 'settings-dns')
  // The hosts file, ready answers and the rest belong to the advanced mode.
  await expect(page.getByTestId('dns-answers')).toHaveCount(0)

  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await page.getByRole('link', { name: 'DNS', exact: true }).click()
  await expect(page.getByRole('combobox', { name: 'Для трафика через сервер' })).toHaveText('Cloudflare')
  await expect(own).toHaveValue('tls://dns.example')
  await expect(page.getByTestId('dns-routing').getByTestId('segment-proxy')).toHaveAttribute('data-state', 'checked')
  await expect(page.getByTestId('fake-ip')).toHaveAttribute('data-state', 'checked')
})

test('rights at autostart come with autostart and ask for rights first', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('portal-settings').click()
  await page.getByRole('link', { name: 'Запуск', exact: true }).click()
  const autostart = page.getByTestId('autostart')
  const adminAutostart = page.getByTestId('admin-autostart')
  if ((await autostart.getAttribute('data-state')) === 'checked') await autostart.click()
  await expect(adminAutostart).toBeDisabled()
  await autostart.click()
  await expect(adminAutostart).toBeEnabled()
  await expect(page.getByTestId('admin-chip')).toBeHidden()
  // The sign-in task is made by a core with rights: the core restarts with them first.
  await adminAutostart.click()
  await expect(page.getByTestId('admin-chip')).toBeVisible()
  await expect(adminAutostart).toHaveAttribute('data-state', 'checked')
  await expect(page.getByTestId('run-as-admin')).toHaveAttribute('data-state', 'unchecked')
  await shot(page, 'settings-start')
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
  // A copy asks its kind and is made at once under a free name.
  await page.getByTestId('preset-copy').first().click()
  const kind = page.getByRole('dialog', { name: 'Копия пресета «Весь трафик»' })
  await expect(kind.getByTestId('new-preset-simple')).toBeChecked()
  await shot(page, 'preset-new-kind')
  await kind.getByTestId('new-preset-create').click()
  await expect(page.getByTestId('preset-name')).toHaveValue('Весь трафик — копия')
  await expect(page.getByTestId('preset-base')).toHaveText('Копия пресета «Весь трафик»')
  await page.getByTestId('preset-name').fill('Дом')

  // One list of rules: a narrower rule rises over a wider one, a wrong value is told at once.
  await page.getByTestId('rule-new-value').fill('example.org')
  await page.getByTestId('rule-new-value').press('Enter')
  await page.getByTestId('rule-new-value').fill('https://Ads.Example.org/banner')
  await page.getByTestId('rule-add').click()
  await expect(page.getByTestId('rule-value')).toHaveCount(2)
  await expect(page.getByTestId('rule-value').first()).toHaveValue('ads.example.org')
  await page.getByTestId('rule-new-value').fill('not a site')
  await page.getByTestId('rule-add').click()
  await expect(page.getByTestId('preset-failure')).toHaveText('«not a site» — не сайт.')
  await expect(page.getByTestId('rule-value')).toHaveCount(2)
  await page.getByTestId('rule-new-kind').click()
  await page.getByRole('option', { name: 'Программа', exact: true }).click()
  await page.getByTestId('rule-new-value').fill('steam')
  await page.getByTestId('rule-add').click()
  await expect(page.getByTestId('rule-value').nth(2)).toHaveValue('steam.exe')
  await expect(page.getByTestId('preset-failure')).toBeHidden()
  // Down does not take a narrower rule under a wider one.
  await page.getByTestId('rule-row').first().getByRole('button', { name: 'Вниз' }).click()
  await expect(page.getByTestId('rule-value').first()).toHaveValue('ads.example.org')
  await expect(page.getByTestId('rules')).toContainText('Локальные сети')

  await page.getByTestId('preset-default').getByTestId('segment-direct').click()
  await expect(page.getByTestId('preset-default').getByTestId('segment-direct')).toHaveAttribute('data-state', 'checked')
  await shot(page, 'preset-edit')
  await page.getByTestId('preset-save').click()
  await expect(page.getByText('На основе «Весь трафик»: 3 правила.')).toBeVisible()
  await expect(page.getByRole('radio', { name: 'Дом' })).toBeChecked()

  // A preset from scratch, made advanced before saving: the warning says it cannot go back.
  await page.getByTestId('preset-create').click()
  await page.getByRole('dialog', { name: 'Новый пресет' }).getByTestId('new-preset-create').click()
  await expect(page.getByTestId('preset-name')).toHaveValue('Новый пресет')
  await expect(page.getByTestId('preset-base')).toHaveCount(0)
  await page.getByTestId('preset-name').fill('Работа')
  await page.getByTestId('preset-to-advanced').click()
  await expect(page.getByRole('dialog', { name: 'Сделать пресет расширенным?' })).toContainText('Вернуть пресет в простой будет нельзя.')
  await shot(page, 'preset-to-advanced')
  await page.getByTestId('to-advanced-confirm').click()
  await expect(page.getByTestId('preset-advanced')).toBeVisible()
  await expect(page.getByTestId('preset-to-advanced')).toHaveCount(0)
  await expect(page.getByTestId('rule-rest')).toContainText('Через сервер')
  await shot(page, 'preset-new')
  await page.getByTestId('preset-save').click()
  await expect(page.getByText('Свой пресет: нет правил.')).toBeVisible()
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
  await page.getByTestId('preset-copy').first().click()
  await page.getByTestId('new-preset-create').click()
  await page.getByTestId('preset-name').fill('Дом')
  await page.getByTestId('rule-new-value').fill('example.org')
  await page.getByTestId('rule-new-value').press('Enter')
  await expect(page.getByTestId('rule-value')).toHaveCount(1)
  await page.getByTestId('preset-lists').click()

  const lists = page.getByRole('dialog', { name: 'Списки пресета «Дом»' })
  const text = lists.getByTestId('lists-text')
  await expect(text).toHaveValue('suffix:example.org')
  await expect(lists.getByTestId('lists-tab-proxy')).toHaveText('Через сервер · 1')
  await text.fill('suffix:example.org\nsuffix:video.example.com\n# работа\nmail.example.net\nsuffix:*.\nport:443')
  await expect(lists.getByTestId('lists-invalid')).toHaveText(
    'Не распознаны 3 строки: 4 «mail.example.net» — нет приставки, 5 «suffix:*.» — не сайт, 6 «port:443» — этот вид — в расширенном пресете. Исправьте или удалите их.',
  )
  await expect(lists.getByTestId('lists-apply')).toBeDisabled()
  await shot(page, 'preset-lists-invalid')
  await text.fill('suffix:example.org\nsuffix:video.example.com\n# работа\nsuffix:mail.example.net')

  await lists.getByTestId('lists-tab-direct').click()
  await text.fill('ip:192.168.50.0/24\nprocessName:steam.exe\nprocessPath:C:\\Games\\game.exe\nsuffix:Video.example.com')
  await expect(lists.getByTestId('lists-duplicate')).toContainText('4 «suffix:Video.example.com» (Через сервер)')
  await expect(lists.getByTestId('lists-apply')).toBeDisabled()
  await text.fill('ip:192.168.50.0/24\nprocessName:steam.exe\nprocessPath:C:\\Games\\game.exe')
  await expect(lists.getByTestId('lists-tab-direct')).toHaveText('Напрямую · 3')
  await expect(lists.getByTestId('lists-tab-proxy')).toHaveText('Через сервер · 3')
  await expect(lists.getByTestId('lists-apply')).toBeEnabled()
  await shot(page, 'preset-lists')
  await lists.getByTestId('lists-apply').click()
  await expect(lists).toBeHidden()
  // The line of the rule there was keeps its place; new lines come after it.
  await expect(page.getByTestId('rule-value')).toHaveCount(6)
  await expect(page.getByTestId('rule-value').first()).toHaveValue('example.org')

  await page.getByTestId('preset-save').click()
  await expect(page.getByText('На основе «Весь трафик»: 6 правил.')).toBeVisible()
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
    JSON.stringify({
      kind: 'poros-preset',
      v: 1,
      based_on: 'all',
      name: 'Дом',
      rules: [{ name: '', conditions: [{ kind: 'suffix', values: ['example.org'] }], invert: false, off: false, action: 'block' }],
      dns: 'tls://1.1.1.1',
    }),
  )
  await send(`poros://route/${preset}`)
  const ask = page.getByRole('dialog', { name: 'Добавить пресет из ссылки?' })
  await expect(ask).toContainText('Пресет «Дом» появится в «Моих пресетах»')
  await expect(ask.getByTestId('link-preset-summary')).toHaveText(
    'На основе «Весь трафик»: 1 правило. Свой DNS.',
  )
  await shot(page, 'link-preset')
  await ask.getByTestId('link-preset-add').click()
  await expect(ask).toBeHidden()

  const server = { type: 'trojan', name: 'Home', server: 'de.example.com', server_port: 443, trojan: { password: 'p' } }
  await send(`poros://add/${encode(JSON.stringify(server))}`)
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
  // The setting is saved, and the connection keeps running until the user restarts it from the chip.
  const chip = page.getByTestId('restart-chip')
  await expect(chip).toBeVisible()
  await expect(chip).toHaveAttribute('title', /не применены/)
  await chip.click()
  const dialog = page.getByRole('dialog', { name: 'Настройки не применены' })
  await expect(dialog).toContainText('подключение нужно перезапустить')
  await shot(page, 'settings-restart')
  await dialog.getByRole('button', { name: 'Отмена' }).click()
  await expect(dialog).toBeHidden()
  await expect(chip).toBeVisible()
  await chip.click()
  await dialog.getByTestId('restart-apply').click()
  await expect(chip).toBeHidden()
  await expect(page.getByTestId('restart-text')).toBeHidden()
})

test('the kill switch guards System without rights and asks for them only for TUN in Always', async ({ page }) => {
  await throughWizard(page)
  await page.getByTestId('system').click()
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Настройки' }).click()
  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await expect(page.getByTestId('segment-off')).toHaveAttribute('data-state', 'checked')
  await shot(page, 'settings-conn')

  // With System alone the kill switch needs no rights; how it works shows while the pointer rests on the setting.
  await expect(page.getByTestId('kill-switch')).not.toContainText('Как работает Kill switch')
  await page.getByTestId('kill-switch').getByTestId('option-text').hover()
  await expect(page.getByTestId('tooltip')).toContainText('Как работает Kill switch')
  await shot(page, 'settings-conn-note')
  await page.mouse.move(0, 0)
  await expect(page.getByTestId('tooltip')).toBeHidden()
  await page.getByTestId('segment-connected').click()
  await expect(page.getByRole('dialog')).toBeHidden()
  await expect(page.getByTestId('kill-switch')).toContainText('трафик VPN не пойдёт напрямую')

  await page.getByRole('link', { name: 'К серверам' }).click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await expect(page.getByTestId('stop').getByTestId('kill-switch-lock')).toBeVisible()
  // The chip of the title bar shows the mode, guarding and not blocking yet.
  const chip = page.getByTestId('kill-switch-chip')
  await expect(chip).toHaveAttribute('title', /^Kill switch при подключении/)
  await expect(chip).not.toHaveAttribute('data-blocking')
  await shot(page, 'main-ks-on')
  await page.getByTestId('stop').click()
  await expect(page.getByTestId('portal-state')).toHaveText('Не подключено')
  await expect(page.getByTestId('kill-switch-lock')).toBeHidden()

  // Always with TUN keeps the TUN up without a connection, which takes rights: Cancel puts the mode back.
  await page.getByTestId('tun').click()
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Настройки' }).click()
  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await page.getByTestId('segment-always').click()
  await expect(page.getByRole('dialog', { name: 'Для Kill switch с TUN нужны права администратора' })).toBeVisible()
  await shot(page, 'settings-conn-admin')
  await page.getByRole('button', { name: 'Отмена' }).click()
  await expect(page.getByTestId('segment-connected')).toHaveAttribute('data-state', 'checked')

  // Without rights the refusal runs without its TUN, and the banner offers them.
  await page.getByTestId('segment-always').click()
  await page.getByRole('button', { name: 'Попробовать без прав' }).click()
  await page.getByRole('link', { name: 'К серверам' }).click()
  await expect(page.getByTestId('kill-switch-failed')).toContainText('Kill switch не включился без прав администратора.')
  await expect(page.getByTestId('portal-state')).toHaveText('Трафик VPN закрыт до подключения')
  // Blocking, the chip is red and says so; it opens the setting.
  await expect(chip).toHaveAttribute('data-blocking', 'always')
  await expect(chip).toHaveAttribute('title', /^Kill switch сейчас блокирует трафик VPN/)
  await shot(page, 'main-ks-failed')
  await chip.click()
  await expect(page.getByTestId('kill-switch')).toBeVisible()
  await page.getByRole('link', { name: 'К серверам' }).click()

  await page.getByTestId('kill-switch-failed').getByRole('button', { name: 'Перезапустить с правами' }).click()
  await expect(page.getByTestId('admin-chip')).toBeVisible()
  await expect(page.getByTestId('kill-switch-failed')).toBeHidden()
  await expect(page.getByTestId('start').getByTestId('kill-switch-lock')).toBeVisible()
  await expect(page.getByTestId('portal-state')).toHaveText('Трафик VPN закрыт до подключения')
})

test('an update is checked from the menu, downloaded and offered over the servers', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Проверить обновления' }).click()
  await expect(page.getByTestId('updates')).toContainText('Версия 0.1.1 загружена и готова к установке.')
  await expect(page.getByTestId('updates').getByRole('button', { name: 'Перезапустить и обновить' })).toBeVisible()
  await expect(page.getByText('Проверять обновления автоматически')).toBeVisible()
  await shot(page, 'settings-help-ready')
  // Without a connection the update installs at once, without a question.
  await page.getByTestId('updates').getByRole('button', { name: 'Перезапустить и обновить' }).click()
  await expect(page.getByRole('dialog', { name: 'Перезапустить и обновить?' })).toBeHidden()

  await page.getByRole('link', { name: 'К серверам' }).click()
  await expect(page.getByTestId('update-ready')).toHaveText(/Готово обновление Poros 0\.1\.1\./)
  await expect(page.getByTestId('update-ready')).not.toContainText('Подключение вернётся само')
  await page.getByTestId('system').click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('update-ready')).toContainText('Подключение вернётся само.')
  await shot(page, 'main-update-ready')
  // A running connection drops for the restart: Poros asks first, and Cancel leaves everything as it was.
  await page.getByTestId('update-ready').getByRole('button', { name: 'Перезапустить и обновить' }).click()
  const confirm = page.getByRole('dialog', { name: 'Перезапустить и обновить?' })
  await expect(confirm).toContainText('подключение к VPN прервётся, и трафик пойдёт напрямую')
  await expect(confirm).not.toContainText('Kill switch')
  await shot(page, 'main-update-confirm')
  await confirm.getByRole('button', { name: 'Отмена' }).click()
  await expect(confirm).toBeHidden()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await page.getByTestId('update-ready').getByRole('button', { name: 'Перезапустить и обновить' }).click()
  await page.getByTestId('update-confirm').click()
  await expect(confirm).toBeHidden()
  await page.getByTestId('update-ready').getByRole('button', { name: 'Позже' }).click()
  await expect(page.getByTestId('update-ready')).toBeHidden()
})
