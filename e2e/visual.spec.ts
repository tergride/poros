import { type Locator, type Page, expect, test } from '@playwright/test'

// Reference screenshots of the key screens and dialogs of both modes, in Russian at 1180×760, against the core in
// memory. A change of a screen shows as a difference from its reference in test-results; after an intended change the
// references are written again with --update-snapshots and looked at. The clock stands still, so that «updated this
// minute» and the times of the connection do not move; only the speeds of a running connection tick on their own.

test.use({ timezoneId: 'Europe/Moscow' })
// Each test goes through the wizard on its own page, so they run side by side.
test.describe.configure({ mode: 'parallel' })

const startTime = new Date('2026-11-13T12:00:00+03:00').getTime()

test.beforeEach(async ({ page }) => {
  test.setTimeout(90_000)
  await page.clock.setFixedTime(startTime)
})

// Vue runs a handler only for an event later than the handler was attached, by Date.now(): with a clock that stands
// still, a handler on an ancestor of the target, as the menu of the right button on the table of profiles, stays
// silent. A step of the clock before such an event lets it through.
async function tick(page: Page): Promise<void> {
  await page.clock.setFixedTime(startTime + 1000)
}

/** Takes the screen as it is: no pointer over it and no focus ring outside a dialog. */
async function snap(page: Page, name: string, moving: Locator[] = []): Promise<void> {
  await page.mouse.move(0, 0)
  await page.evaluate(() => {
    const focused = document.activeElement as HTMLElement | null
    if (focused && !focused.closest('[role="dialog"]')) focused.blur()
  })
  await expect(page).toHaveScreenshot(`${name}.png`, { mask: moving })
}

/** Opens a screen through the router of the page, as a link in the window would. */
async function visit(page: Page, path: string): Promise<void> {
  const reached = await page.evaluate(async (to) => {
    type Router = { push(to: string): Promise<unknown>; currentRoute: { value: { path: string } } }
    const app = (document.querySelector('#app') as { __vue_app__?: { config: { globalProperties: { $router: Router } } } } | null)
      ?.__vue_app__
    const router = app!.config.globalProperties.$router
    await router.push(to)
    return router.currentRoute.value.path
  }, path)
  expect(reached).toBe(path)
}

async function throughWizard(page: Page, shots = false): Promise<void> {
  const step = async (name: string) => (shots ? snap(page, name) : undefined)
  const next = page.getByRole('button', { name: 'Далее' })
  await page.goto('/')
  // The speeds of a running connection tick every second: their numbers are hidden, at a width that keeps what is
  // beside them in place.
  await page.addStyleTag({
    content: '[data-testid="speed"] { visibility: hidden } [data-testid="connbar"] [data-testid="speed"] { width: 150px }',
  })
  await expect(page.getByRole('heading', { name: 'Выберите режим интерфейса' })).toBeVisible()
  await step('wizard-1-mode')
  await next.click()
  await expect(page.getByRole('heading', { name: 'Добавьте подписку' })).toBeVisible()
  await step('wizard-2-subscription')
  await page.getByLabel('Ссылка подписки').fill('https://sub.example.com/u/8f3a2c')
  await page.getByLabel('Ссылка подписки').press('Enter')
  await expect(page.getByTestId('link-found')).toBeVisible()
  await step('wizard-2-found')
  await next.click()
  await expect(page.getByRole('heading', { name: 'Как запускать Poros' })).toBeVisible()
  await step('wizard-3-start')
  await next.click()
  await expect(page.getByRole('heading', { name: 'Какой трафик пускать через сервер' })).toBeVisible()
  await step('wizard-4-routing')
  await page.getByRole('button', { name: 'Готово' }).click()
  await expect(page.getByTestId('portal')).toBeVisible()
}

/** The first Start asks for the modes; System alone connects without rights. */
async function connectWithSystem(page: Page, shot?: string): Promise<void> {
  await page.getByTestId('start').click()
  const modes = page.getByRole('dialog', { name: 'Как пускать трафик через сервер' })
  await expect(modes).toBeVisible()
  if (shot) await snap(page, shot)
  await modes.getByRole('checkbox').first().click()
  await modes.getByRole('checkbox').nth(1).click()
  await page.getByTestId('modes-connect').click()
  await expect(modes).toBeHidden()
}

test('the wizard, the main screen and its dialogs', async ({ page }) => {
  await throughWizard(page, true)
  await snap(page, 'main-idle')

  await connectWithSystem(page, 'dialog-modes')
  await expect(page.getByTestId('portal-state')).toContainText('Подключено')
  await snap(page, 'main-connected')

  await page.getByTestId('portal-ip').click()
  await expect(page.getByTestId('leak-verdict')).toContainText('Утечки DNS нет')
  await snap(page, 'dialog-ip')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toBeHidden()

  // An update downloaded while connected asks before the restart drops the connection.
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByRole('menuitem', { name: 'Проверить обновления' }).click()
  await expect(page.getByTestId('updates')).toContainText('Версия 0.1.1 загружена и готова к установке.')
  await page.getByRole('link', { name: 'К серверам' }).click()
  await page.getByTestId('update-ready').getByRole('button', { name: 'Перезапустить и обновить' }).click()
  await expect(page.getByRole('dialog', { name: 'Перезапустить и обновить?' })).toBeVisible()
  await snap(page, 'dialog-update')
  await page.getByRole('dialog').getByRole('button', { name: 'Отмена' }).click()
  await expect(page.getByRole('dialog')).toBeHidden()

  // A setting changed under a running connection waits for its restart.
  await page.getByTestId('portal-settings').click()
  await page.getByRole('link', { name: 'Подключение', exact: true }).click()
  await page.getByRole('switch').first().click()
  await page.getByTestId('restart-chip').click()
  await expect(page.getByRole('dialog', { name: 'Настройки не применены' })).toBeVisible()
  await snap(page, 'dialog-restart')
})

test('the settings of the simple mode, a backup to restore and the change of mode', async ({ page }) => {
  await throughWizard(page)
  for (const id of ['presets', 'conn', 'dns', 'subs', 'start', 'look', 'backup', 'help']) {
    await visit(page, `/settings/${id}`)
    await snap(page, `settings-${id}`)
  }

  await visit(page, '/settings/backup')
  const saving = page.waitForEvent('download')
  await page.getByTestId('backup-create').click()
  const download = await saving
  // The file under its own name: a download lies under a new random name each time, and the dialog names the file.
  const buffer = (await import('node:fs')).readFileSync((await download.path())!)
  await page.getByTestId('backup-file').setInputFiles({ name: download.suggestedFilename(), mimeType: 'application/json', buffer })
  await expect(page.getByTestId('restore-dialog')).toContainText('Текущие данные заменятся данными из файла.')
  await snap(page, 'dialog-restore')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toBeHidden()

  await page.getByTestId('mode-chip').click()
  await expect(page.getByRole('dialog', { name: 'Перейти в расширенный режим?' })).toBeVisible()
  await snap(page, 'dialog-mode')
})

test('the advanced mode: its sections, settings, a QR code and the connections', async ({ page }) => {
  await throughWizard(page)
  await page.getByRole('button', { name: 'Меню' }).click()
  await page.getByTestId('segment-advanced').click()
  await expect(page.getByTestId('profiles-table')).toBeVisible()
  await page.keyboard.press('Escape')
  await expect(page.getByTestId('segment-advanced')).toBeHidden()
  await snap(page, 'adv-profiles')

  const sections: [string, string][] = [
    ['/adv/routing/rules/all', 'adv-routing'],
    ['/adv/logs', 'adv-logs'],
    ['/adv/stats/now', 'adv-stats-now'],
    ['/adv/stats/traffic', 'adv-stats-traffic'],
    ['/adv/tools/otp', 'adv-tools-otp'],
    ['/adv/tools/misc', 'adv-tools-misc'],
  ]
  for (const [path, name] of sections) {
    await visit(page, path)
    await snap(page, name)
  }
  for (const id of ['conn', 'dns', 'subs', 'start', 'look', 'help', 'inbound', 'tests', 'tun', 'protocols', 'net', 'security', 'logging', 'hotkeys', 'backup']) {
    await visit(page, `/adv/settings/${id}`)
    await snap(page, `adv-settings-${id}`)
  }

  await visit(page, '/adv/profiles')
  await tick(page)
  await page.getByTestId('profile-row').first().click({ button: 'right' })
  await page.getByTestId('profile-qr').click()
  await expect(page.getByTestId('qr-picture')).toBeVisible()
  await snap(page, 'dialog-qr')
  await page.keyboard.press('Escape')
  await expect(page.getByRole('dialog')).toBeHidden()

  await connectWithSystem(page)
  await expect(page.getByTestId('connbar')).toHaveAttribute('data-state', 'running')
  await visit(page, '/adv/connections')
  await expect(page.getByTestId('conn-active')).toHaveText('Активно: 5')
  // The chart of the speed fills a point a second.
  await snap(page, 'adv-connections', [page.getByTestId('speed-chart')])
})
