import { type Page, expect, test } from '@playwright/test'

// The simple mode against the core in memory: a first start through the wizard, then the main screen, the dialogs
// around Start and the settings. Screenshots go to test-results/shots for a look.

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

test('the shell renders with its fonts and the system language', async ({ page }) => {
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

  // A line without a colon offers the prefixes of the kinds, typing narrows them, Enter or a click puts one in.
  const prefixes = lists.getByTestId('lists-prefix')
  await text.click()
  await text.press('Control+End')
  await expect(prefixes).toHaveCount(0)
  await text.press('Enter')
  await expect(prefixes).toHaveText(['suffix:Сайт', 'ip:IP или подсеть', 'processName:Программа', 'processPath:Путь к программе'])
  await text.pressSequentially('pro')
  await expect(prefixes).toHaveText(['processName:Программа', 'processPath:Путь к программе'])
  await shot(page, 'preset-lists-prefixes')
  await text.press('ArrowDown')
  await text.press('Enter')
  await expect(text).toHaveValue('suffix:example.org\nprocessPath:')
  await expect(prefixes).toHaveCount(0)
  await text.pressSequentially('C:\\Games\\game.exe')
  await text.press('Enter')
  await expect(prefixes).toHaveCount(4)
  await text.press('Escape')
  await expect(prefixes).toHaveCount(0)
  await expect(lists).toBeVisible()
  await text.pressSequentially('i')
  await prefixes.filter({ hasText: 'suffix:' }).click()
  await text.pressSequentially('example.net')
  await expect(text).toHaveValue('suffix:example.org\nprocessPath:C:\\Games\\game.exe\nsuffix:example.net')

  await lists.getByTestId('lists-help').click()
  const help = page.getByRole('dialog', { name: 'Как писать списки' })
  await expect(help.getByTestId('lists-help-kinds').locator('tbody tr')).toHaveCount(4)
  await shot(page, 'preset-lists-help')
  await page.keyboard.press('Escape')
  await expect(help).toBeHidden()
  await expect(lists).toBeVisible()

  // A wrong line keeps the height of the field, and the list of prefixes for the last line opens past the field.
  const field = (await text.boundingBox())!
  await text.fill(Array.from({ length: 30 }, (_, i) => `suffix:site${i}.example.com`).join('\n'))
  await text.press('Control+End')
  await text.press('Enter')
  await text.pressSequentially('s')
  await expect(lists.getByTestId('lists-invalid')).toHaveText('Не распознана 1 строка: 31 «s» — нет приставки. Исправьте или удалите её.')
  expect((await text.boundingBox())!.height).toBe(field.height)
  expect((await lists.getByTestId('lists-prefixes').boundingBox())!.y).toBeGreaterThan(field.y + field.height - 30)
  await shot(page, 'preset-lists-last-line')

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

test('a preset is shared as a QR code, a link and a file, and imported from each', async ({ page }) => {
  await throughWizard(page)
  await page.context().grantPermissions(['clipboard-read', 'clipboard-write'])
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await page.getByTestId('preset-copy').first().click()
  await page.getByTestId('new-preset-create').click()
  await page.getByTestId('preset-name').fill('Канал')
  await page.getByTestId('rule-new-value').fill('example.org')
  await page.getByTestId('rule-new-value').press('Enter')
  await page.getByTestId('preset-save').click()
  const mine = page.getByTestId('preset').filter({ hasText: 'Канал' })
  await expect(mine).toBeVisible()

  // «Share»: the code, the link to copy, the picture of the code and the file.
  await mine.getByRole('button', { name: 'Ещё' }).click()
  await page.getByTestId('preset-share').click()
  const share = page.getByRole('dialog', { name: 'Поделиться «Канал»' })
  await expect(share.getByTestId('share-qr')).toBeVisible()
  await shot(page, 'preset-share')
  const link = await share.getByTestId('share-link').inputValue()
  expect(link).toMatch(/^poros:\/\/route\//)
  await share.getByTestId('share-copy-link').click()
  await expect(share.getByTestId('share-copy-link')).toHaveText('Скопировано')
  expect(await page.evaluate(() => navigator.clipboard.readText())).toBe(link)
  await share.getByTestId('share-copy-qr').click()
  await expect(share.getByTestId('share-copy-qr')).toHaveText('QR-код скопирован')
  await expect.poll(() => page.evaluate(async () => (await navigator.clipboard.read()).flatMap((item) => item.types))).toContain('image/png')
  const saving = page.waitForEvent('download')
  await share.getByTestId('share-export').click()
  const download = await saving
  expect(download.suggestedFilename()).toBe('Канал.json')
  const file = (await import('node:fs')).readFileSync(await download.path())
  expect(JSON.parse(file.toString('utf8'))).toMatchObject({ kind: 'poros-preset', v: 1, name: 'Канал' })
  await expect(share.getByTestId('share-saved')).toHaveText('Сохранено: Канал.json')
  await shot(page, 'preset-share-done')
  await page.keyboard.press('Escape')
  await expect(share).toBeHidden()

  // «Import a preset»: a pasted picture, a picked picture and file, a dropped picture and a link. The stub reads
  // every picture as what the page puts into window.porosPictureQr.
  await page.evaluate((text) => {
    ;(window as { porosPictureQr?: string[] }).porosPictureQr = [text]
  }, link)
  const dialog = page.getByRole('dialog', { name: 'Импорт пресета' })
  const importing = async () => {
    await page.getByTestId('preset-import').click()
    await expect(dialog).toBeVisible()
  }
  // Each brings the name of a preset there is: Poros asks, and here it adds a new one.
  const added = async (name: string) => {
    await page.getByTestId('same-preset-create').click()
    await expect(dialog, `the dialog closes on «${name}» added`).toBeHidden()
    await expect(page.getByText(`Пресет «${name}» добавлен.`)).toBeVisible()
  }
  const send = (type: 'dragover' | 'drop', target: string) =>
    page.evaluate(
      ([kind, testid]) => {
        const zone = document.querySelector(`[data-testid="${testid}"]`)
        if (!zone) throw new Error(`no ${testid}`)
        const data = new DataTransfer()
        data.items.add(new File(['png'], 'preset.png', { type: 'image/png' }))
        const init = { bubbles: true, cancelable: true }
        zone.dispatchEvent(new DragEvent(kind, { ...init, dataTransfer: data }))
      },
      [type, target] as const,
    )
  const picture = { name: 'preset.png', mimeType: 'image/png', buffer: Buffer.from('png') }

  // The picture of the code copied in «Share» is still in the clipboard.
  await importing()
  await shot(page, 'preset-import')
  await page.keyboard.press('Control+V')
  await added('Канал 2')

  await importing()
  await dialog.getByTestId('import-file').setInputFiles(picture)
  await added('Канал 3')

  await importing()
  await dialog.getByTestId('import-file').setInputFiles({ name: 'Канал.json', mimeType: 'application/json', buffer: file })
  await added('Канал 4')

  await importing()
  await send('dragover', 'import-drop')
  await expect(dialog.getByTestId('import-drop')).toHaveClass(/border-act/)
  await send('drop', 'import-drop')
  await added('Канал 5')

  await importing()
  await dialog.getByTestId('import-link').fill(link)
  await dialog.getByTestId('import-link').press('Enter')
  await added('Канал 6')

  // What fails is said in the dialog, which stays open.
  await importing()
  await page.evaluate(() => {
    ;(window as { porosPictureQr?: string[] }).porosPictureQr = ['https://example.com/']
  })
  await dialog.getByTestId('import-file').setInputFiles(picture)
  await expect(dialog.getByTestId('import-error')).toHaveText('На картинке нет QR-кода пресета Poros.')
  await dialog.getByTestId('import-file').setInputFiles({ name: 'notes.txt', mimeType: 'text/plain', buffer: Buffer.from('x') })
  await expect(dialog.getByTestId('import-error')).toHaveText('Подходит картинка PNG, JPEG или GIF с QR-кодом или файл пресета .json.')
  await dialog.getByTestId('import-file').setInputFiles({ name: 'other.json', mimeType: 'application/json', buffer: Buffer.from('{"kind":"other"}') })
  await expect(dialog.getByTestId('import-error')).toHaveText('В файле нет пресета Poros.')
  await dialog.getByTestId('import-link').fill('https://example.com/')
  await dialog.getByTestId('import-add').click()
  await expect(dialog.getByTestId('import-error')).toContainText('Это не ссылка на пресет Poros')
  await shot(page, 'preset-import-error')
  await page.keyboard.press('Escape')
  // Two built-in presets and six of the user's.
  await expect(page.getByTestId('preset')).toHaveCount(8)
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

  // The same subscription again, and as the plain URL: nothing to add, it is updated or shown, from any screen.
  await send(`poros://addsub/${encode('https://vpn.example.org/s/71bd#Другое название')}`)
  const known = page.getByRole('dialog', { name: 'Подписка уже добавлена' })
  await expect(known).toContainText('Подписка «Work» уже есть в Poros.')
  await expect(known.getByTestId('known-summary')).toContainText('3 сервера · обновлена')
  await shot(page, 'link-known')
  await known.getByTestId('known-update').click()
  await expect(known).toBeHidden()
  await expect(page.getByTestId('server-group')).toHaveCount(2)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Настройки' }).click()
  await send('poros://addsub/https://vpn.example.org/s/71bd')
  await known.getByTestId('known-show').click()
  await expect(known).toBeHidden()
  await expect(page).toHaveURL(/#\/main$/)
  await expect(page.getByTestId('server-group').filter({ hasText: 'Work' }).getByTestId('server')).toHaveCount(3)

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

test('an import under a taken name asks to replace the preset or to add a new one', async ({ page }) => {
  await throughWizard(page)
  const linkTo = (name: string, sites: string[]) =>
    `poros://route/${Buffer.from(
      JSON.stringify({
        kind: 'poros-preset',
        v: 1,
        name,
        rules: sites.map((site) => ({ conditions: [{ kind: 'suffix', values: [site] }], action: 'proxy' })),
        default: 'direct',
      }),
    ).toString('base64url')}`
  const send = (link: string) =>
    page.evaluate((detail) => window.dispatchEvent(new CustomEvent('poros-links', { detail })), [link])
  const asking = page.getByRole('dialog', { name: 'Пресет с таким названием уже есть' })
  const presets = page.getByTestId('preset')

  await send(linkTo('Игры', ['old.example']))
  await page.getByTestId('link-preset-add').click()
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await presets.filter({ hasText: 'Игры' }).getByRole('radio').click()
  await page.getByRole('link', { name: 'К серверам' }).click()
  await page.getByTestId('system').click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')

  // A link from the browser: what there is and what comes, and nothing changes without an answer.
  await send(linkTo('игры', ['new.example', 'more.example']))
  await expect(asking).toContainText('В «Моих пресетах» уже есть «Игры»')
  await expect(asking.getByTestId('same-preset-current')).toContainText('1 правило')
  await expect(asking.getByTestId('same-preset-incoming')).toContainText('2 правила')
  await expect(asking.getByTestId('same-preset-incoming')).toContainText('По умолчанию трафик идёт напрямую.')
  await expect(asking).toContainText('появится «игры 2»')
  await expect(asking.getByTestId('same-preset-in-use')).toBeVisible()
  await expect(asking.getByRole('button', { name: 'Отмена' }), 'Enter at once replaces nothing').toBeFocused()
  await shot(page, 'same-preset')
  await page.keyboard.press('Escape')
  await expect(asking).toBeHidden()
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await expect(presets).toHaveCount(3)
  await expect(presets.nth(2)).toContainText('1 правило')
  await expect(page.getByTestId('restart-chip')).toBeHidden()

  await send(linkTo('игры', ['new.example', 'more.example']))
  await asking.getByTestId('same-preset-create').click()
  await expect(asking).toBeHidden()
  await expect(presets).toHaveCount(4)
  await expect(presets.nth(3)).toContainText('игры 2')
  await expect(page.getByTestId('restart-chip')).toBeHidden()

  // The editor has changes of its own: replaced, it shows the new preset and cannot save the old one back.
  await presets.nth(2).getByRole('button', { name: 'Изменить' }).click()
  await page.getByTestId('rule-new-value').fill('edited.example')
  await page.getByTestId('rule-new-value').press('Enter')
  await expect(page.getByTestId('rule-value')).toHaveCount(2)
  await send(linkTo('Игры', ['new.example', 'more.example', 'third.example']))
  await asking.getByTestId('same-preset-replace').click()
  await expect(asking).toBeHidden()
  await expect(page.getByTestId('rule-value')).toHaveCount(3)
  await expect(page.getByTestId('rule-value').first()).toHaveValue('new.example')
  await expect(page.getByTestId('restart-chip'), 'the running connection has the old preset').toBeVisible()
  await page.getByTestId('preset-save').click()
  await expect(presets).toHaveCount(4)
  await expect(presets.nth(2)).toContainText('3 правила')
  await expect(presets.nth(2).getByRole('radio')).toBeChecked()

  // «Import a preset»: Cancel goes back to it with the link still there, Replace closes both.
  await page.getByTestId('preset-import').click()
  const importing = page.getByRole('dialog', { name: 'Импорт пресета' })
  const last = linkTo('Игры', ['last.example'])
  await importing.getByTestId('import-link').fill(last)
  await importing.getByTestId('import-link').press('Enter')
  await expect(asking).toBeVisible()
  await expect(importing).toBeHidden()
  await page.keyboard.press('Escape')
  await expect(asking).toBeHidden()
  await expect(importing.getByTestId('import-link')).toHaveValue(last)
  await importing.getByTestId('import-add').click()
  await asking.getByTestId('same-preset-replace').click()
  await expect(asking).toBeHidden()
  await expect(importing).toBeHidden()
  await expect(page.getByText('Пресет «Игры» заменён.')).toBeVisible()
  await expect(presets).toHaveCount(4)
  await expect(presets.nth(2)).toContainText('1 правило')
  await expect(presets.nth(2).getByRole('radio')).toBeChecked()
})

test('a replaced preset of the connection waits for a restart, chosen or not', async ({ page }) => {
  await throughWizard(page)
  const send = (name: string, site: string) => {
    const rules = [{ conditions: [{ kind: 'suffix', values: [site] }], action: 'proxy' }]
    const link = `poros://route/${Buffer.from(JSON.stringify({ kind: 'poros-preset', v: 1, name, rules })).toString('base64url')}`
    return page.evaluate((detail) => window.dispatchEvent(new CustomEvent('poros-links', { detail })), [link])
  }
  const asking = page.getByRole('dialog', { name: 'Пресет с таким названием уже есть' })
  const presets = page.getByTestId('preset')
  const choose = (name: string) => presets.filter({ hasText: name }).getByRole('radio').click()

  for (const name of ['Работа', 'Музыка']) {
    await send(name, 'old.example')
    await page.getByTestId('link-preset-add').click()
    await expect(page.getByTestId('link-preset-add')).toBeHidden()
  }
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await choose('Работа')
  await page.getByRole('link', { name: 'К серверам' }).click()
  await page.getByTestId('system').click()
  await page.getByTestId('start').click()
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await page.getByRole('link', { name: /Пресет:/ }).click()
  await choose('Музыка')

  await send('Музыка', 'music.example')
  await expect(asking).toBeVisible()
  await expect(asking.getByTestId('same-preset-in-use'), 'the chosen one is not the one of the connection').toBeHidden()
  await page.keyboard.press('Escape')
  await expect(asking).toBeHidden()

  await send('Работа', 'work.example')
  await expect(asking.getByTestId('same-preset-in-use')).toBeVisible()
  await asking.getByTestId('same-preset-replace').click()
  await expect(asking).toBeHidden()
  await choose('Работа')
  await expect(page.getByTestId('restart-chip'), 'the connection has the old rules').toBeVisible()
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
  // On connection is the mode a new Poros starts with.
  await expect(page.getByTestId('segment-connected')).toHaveAttribute('data-state', 'checked')
  await shot(page, 'settings-conn')

  // With System alone the kill switch needs no rights; how it works shows while the pointer rests on the setting.
  await expect(page.getByTestId('kill-switch')).not.toContainText('Как работает Kill switch')
  await page.getByTestId('kill-switch').getByTestId('option-text').hover()
  await expect(page.getByTestId('tooltip')).toContainText('Как работает Kill switch')
  await shot(page, 'settings-conn-note')
  await page.mouse.move(0, 0)
  await expect(page.getByTestId('tooltip')).toBeHidden()
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
  // The kill switch is on by default, and the restart lets it go too.
  await expect(confirm).toContainText('Kill switch на время перезапуска тоже снимается.')
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
