import { type Locator, type Page, expect, test } from '@playwright/test'

// The main flows of both modes with the keyboard alone, against the core in memory: Tab and Shift+Tab, Enter, Space,
// Escape and the arrows. A screen fails when Tab cannot reach something on it that a click can, a dialog fails when the
// focus leaves it while it is open or Escape does not close it.

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const w = window as unknown as { porosFocus: Element[] }
    w.porosFocus = []
    document.addEventListener('focusin', (event) => w.porosFocus.push(event.target as Element), true)
  })
})

// A radio checks on an arrow only while the key is still down, as a person holds it for a moment.
const held = { delay: 50 }

/** Presses Tab, or Shift+Tab, until the element has the focus; fails when it never gets it. */
async function tabTo(page: Page, target: Locator, back = false): Promise<void> {
  await expect(target).toBeVisible()
  for (let i = 0; i < 100; i++) {
    if (await target.evaluate((el) => el === document.activeElement || el.contains(document.activeElement))) return
    await page.keyboard.press(back ? 'Shift+Tab' : 'Tab')
  }
  throw new Error(`Tab does not reach ${target}`)
}

/** Where the focus is, and whether it stays in the open dialog. Runs in the page. */
function focusState(): { where: string; inDialog: boolean | null } {
  const focused = document.activeElement
  const dialog = document.querySelector('[role="dialog"]')
  const where = !focused || focused === document.body ? 'body' : `<${focused.tagName.toLowerCase()}> «${(focused.textContent || focused.getAttribute('aria-label') || '').trim().slice(0, 40)}»`
  return { where, inDialog: dialog ? !!focused && focused !== document.body && dialog.contains(focused) : null }
}

/**
 * What a click reaches and Tab does not, on the screen or in its open dialog: controls, links, fields and whatever has
 * a pointer for a cursor. A control counts as reached when it, a box around it or a control inside it had the focus,
 * or, in a group the arrows move through (radios, tabs), when another of the group had it. Runs in the page.
 */
function unreached(): string[] {
  const reached = (window as unknown as { porosFocus: Element[] }).porosFocus
  const scope = document.querySelector('[role="dialog"]') ?? document.body
  const roles = ['button', 'link', 'switch', 'checkbox', 'radio', 'tab', 'combobox', 'menuitem', 'option', 'textbox', 'slider']
  const native = 'a[href], button, input:not([type="hidden"]), select, textarea, [contenteditable="true"]'
  const groups = '[role="radiogroup"], [role="tablist"], [role="toolbar"], [role="group"], [role="listbox"], [role="menu"], [role="grid"]'
  const shown = (el: Element) => {
    const box = el.getBoundingClientRect()
    return box.width > 1 && box.height > 1 && el.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true }) && !el.closest('[inert], [aria-hidden="true"]')
  }
  const interactive = (el: Element) => {
    if (el.matches(':disabled, [aria-disabled="true"]')) return false
    if (el.matches(native) || roles.includes(el.getAttribute('role') ?? '')) return true
    if (el.matches('[tabindex]') && el.getAttribute('tabindex') !== '-1' && el.getAttribute('role') !== 'dialog') return true
    // Something clicked without being a control: the topmost box with the pointer, as the cursor is inherited.
    return getComputedStyle(el).cursor === 'pointer' && !!el.parentElement && getComputedStyle(el.parentElement).cursor !== 'pointer'
  }
  const ok = (el: Element) => {
    if (reached.some((r) => r === el || r.contains(el) || el.contains(r))) return true
    const group = el.closest(groups)
    return !!group && reached.some((r) => group.contains(r))
  }
  return [...scope.querySelectorAll('*')]
    .filter((el) => shown(el) && interactive(el) && !ok(el))
    .map((el) => {
      const id = el.getAttribute('data-testid')
      const text = (el.textContent || el.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim().slice(0, 50)
      return `<${el.tagName.toLowerCase()}${id ? ` ${id}` : ''}> «${text}»`
    })
}

/**
 * The focus after a key: a number for the element, where it is, whether it is in the open dialog and whether it shows
 * the focus with an outline, a ring or a style of its own (a field shows its caret). Runs in the page.
 */
function focusStep(): { id: string; where: string; inDialog: boolean | null; shown: boolean } {
  const w = window as unknown as { porosWalk?: WeakMap<Element, number>; porosNext?: number }
  w.porosWalk ??= new WeakMap()
  const focused = document.activeElement
  const dialog = document.querySelector('[role="dialog"]')
  const inDialog = dialog ? !!focused && focused !== document.body && dialog.contains(focused) : null
  if (!focused || focused === document.body) return { id: 'body', where: 'body', inDialog, shown: true }
  if (!w.porosWalk.has(focused)) w.porosWalk.set(focused, (w.porosNext = (w.porosNext ?? 0) + 1))
  const id = focused.getAttribute('data-testid')
  const label = (focused.textContent || focused.getAttribute('aria-label') || '').replace(/\s+/g, ' ').trim().slice(0, 40)
  const where = `<${focused.tagName.toLowerCase()}${id ? ` ${id}` : ''}> «${label}»`
  const style = getComputedStyle(focused)
  const outline = style.outlineStyle !== 'none' && parseFloat(style.outlineWidth) > 0 && !/^rgba\(.*,\s*0\)$|transparent/.test(style.outlineColor)
  const field = focused.matches('input:not([type="checkbox"]):not([type="radio"]), textarea, [contenteditable="true"]')
  // A border of its own on the focus, as the lists of choices have, is declared by a class, or by the box around it.
  const declared =
    /(^|\s)focus(-visible)?:/.test(focused.getAttribute('class') ?? '') ||
    /focus-within:|has-\[:focus/.test(focused.parentElement?.getAttribute('class') ?? '')
  return { id: String(w.porosWalk.get(focused)), where, inDialog, shown: field || outline || declared || style.boxShadow !== 'none' }
}

/**
 * Tabs once around the screen, or around its open dialog: what Tab did not reach and where the focus did not show; in a
 * dialog, where the focus left it, going forward or back.
 */
async function keyboardProblems(page: Page, name: string): Promise<string[]> {
  const inDialog = (await page.evaluate(focusState)).inDialog !== null
  await page.evaluate(() => ((window as unknown as { porosFocus: Element[] }).porosFocus = []))
  const lost: string[] = []
  let first = ''
  let last = ''
  for (let i = 0; i < 200; i++) {
    await page.keyboard.press('Tab')
    const state = await page.evaluate(focusStep)
    if (inDialog && !state.inDialog) lost.push(`${name}: Tab took the focus out of the dialog to ${state.where}`)
    if (state.id === 'body') continue
    if (state.id === first) break
    if (state.id === last) {
      lost.push(`${name}: Tab stays on ${state.where}`)
      break
    }
    if (!state.shown) lost.push(`${name}: the focus does not show on ${state.where}`)
    first ||= state.id
    last = state.id
  }
  if (inDialog) {
    for (let i = 0; i < 3; i++) {
      await page.keyboard.press('Shift+Tab')
      const state = await page.evaluate(focusState)
      if (!state.inDialog) lost.push(`${name}: Shift+Tab took the focus out of the dialog to ${state.where}`)
    }
  }
  const missed = (await page.evaluate(unreached)).map((what) => `${name}: Tab does not reach ${what}`)
  return [...lost, ...missed]
}

async function tabAround(page: Page, name: string): Promise<void> {
  expect(await keyboardProblems(page, name)).toEqual([])
}

/** An open dialog: the focus is inside it, Tab goes around it, Escape closes it. */
async function dialogByKeyboard(page: Page, dialog: Locator, name: string): Promise<void> {
  await expect(dialog).toBeVisible()
  expect((await page.evaluate(focusState)).inDialog, `${name} takes the focus when it opens`).toBe(true)
  await tabAround(page, name)
  await page.keyboard.press('Escape')
  await expect(dialog, `${name} closes with Escape`).toBeHidden()
}

/** Opens the item of an open menu by the arrows. */
async function menuItem(page: Page, name: string): Promise<void> {
  // An item under the resting pointer of an earlier click would take the focus from the keyboard.
  await page.mouse.move(0, 0)
  const item = page.getByRole('menuitem', { name })
  await expect(item).toBeVisible()
  for (let i = 0; i < 20 && !(await item.evaluate((el) => el === document.activeElement)); i++) await page.keyboard.press('ArrowDown')
  await expect(item).toBeFocused()
  await page.keyboard.press('Enter')
}

test('the keyboard check finds what Tab misses and a focus that does not show', async ({ page }) => {
  await page.setContent(`
    <button>reached</button>
    <button style="outline: none">no ring</button>
    <div style="cursor: pointer">clicked only</div>
    <button tabindex="-1">skipped</button>
    <div role="radiogroup"><button role="radio" tabindex="0">one</button><button role="radio" tabindex="-1">two</button></div>
    <input type="file" style="display: none">`)
  await page.evaluate(() => {
    const w = window as unknown as { porosFocus: Element[] }
    w.porosFocus = []
    document.addEventListener('focusin', (event) => w.porosFocus.push(event.target as Element), true)
  })
  expect(await keyboardProblems(page, 'sample')).toEqual([
    'sample: the focus does not show on <button> «no ring»',
    'sample: Tab does not reach <div> «clicked only»',
    'sample: Tab does not reach <button> «skipped»',
  ])
})

async function throughWizardByKeyboard(page: Page, checked: boolean): Promise<void> {
  const around = (name: string) => (checked ? tabAround(page, name) : Promise.resolve())
  const next = page.getByRole('button', { name: 'Далее' })
  await page.goto('/')
  await expect(page.getByTestId('wizard-mode-simple')).toBeVisible()
  await around('wizard-1')
  // The mode is a group of radios: the arrows pick the other one and come back.
  const simple = page.getByTestId('wizard-mode-simple').getByRole('radio')
  await tabTo(page, simple)
  await page.keyboard.press('ArrowRight', held)
  await expect(page.getByTestId('wizard-mode-advanced').getByRole('radio')).toBeChecked()
  await page.keyboard.press('ArrowLeft', held)
  await expect(simple).toBeChecked()
  await tabTo(page, next)
  await page.keyboard.press('Enter')

  const link = page.getByLabel('Ссылка подписки')
  await expect(link).toBeVisible()
  await around('wizard-2')
  await tabTo(page, link)
  await page.keyboard.type('https://sub.example.com/u/8f3a2c')
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('link-found')).toBeVisible()
  await tabTo(page, next)
  await page.keyboard.press('Enter')

  const links = page.getByTestId('wizard-links')
  await expect(links).toBeVisible()
  await around('wizard-3')
  await tabTo(page, links)
  await page.keyboard.press('Space')
  await expect(links).not.toBeChecked()
  await page.keyboard.press('Space')
  await expect(links).toBeChecked()
  await tabTo(page, next)
  await page.keyboard.press('Enter')

  const all = page.getByRole('radio', { name: /^Весь трафик/ })
  await expect(all).toBeVisible()
  await around('wizard-4')
  await tabTo(page, all)
  await page.keyboard.press('ArrowDown', held)
  await expect(page.getByRole('radio', { name: /^Усиленный обход/ })).toBeChecked()
  await page.keyboard.press('ArrowUp', held)
  await expect(all).toBeChecked()
  await tabTo(page, page.getByRole('button', { name: 'Готово' }))
  await page.keyboard.press('Enter')
  await expect(page.getByTestId('portal')).toBeVisible()
}

test('the simple mode: the wizard, Start and Stop, the dialogs and the settings', async ({ page }) => {
  test.setTimeout(180_000)
  await throughWizardByKeyboard(page, true)
  await tabAround(page, 'main')

  // The first Start asks for the modes: Escape leaves the question, then TUN off and System on connect.
  const start = page.getByTestId('start')
  await tabTo(page, start)
  await page.keyboard.press('Enter')
  const modes = page.getByRole('dialog', { name: 'Как пускать трафик через сервер' })
  await dialogByKeyboard(page, modes, 'dialog-modes')
  await tabTo(page, start)
  await page.keyboard.press('Enter')
  await expect(modes).toBeVisible()
  await tabTo(page, modes.getByRole('checkbox').first())
  await page.keyboard.press('Space')
  await tabTo(page, modes.getByRole('checkbox').nth(1))
  await page.keyboard.press('Space')
  await tabTo(page, page.getByTestId('modes-connect'))
  await page.keyboard.press('Enter')
  const portal = page.getByTestId('portal')
  await expect(portal).toHaveAttribute('data-state', 'running')
  // Stop and Start again, the remembered modes connect at once: the focus stays where the button changes, so Enter
  // goes on toggling.
  await tabTo(page, page.getByTestId('stop'))
  await page.keyboard.press('Enter')
  await expect(portal).toHaveAttribute('data-state', 'idle')
  await expect(start).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(portal).toHaveAttribute('data-state', 'running')
  await expect(page.getByTestId('stop')).toBeFocused()
  await tabAround(page, 'main-connected')

  await tabTo(page, page.getByTestId('portal-ip'))
  await page.keyboard.press('Enter')
  await dialogByKeyboard(page, page.getByRole('dialog', { name: 'IP и DNS' }), 'dialog-ip')

  // An update downloaded while connected asks before the restart.
  await tabTo(page, page.getByRole('button', { name: 'Меню' }))
  await page.keyboard.press('Enter')
  await menuItem(page, 'Проверить обновления')
  await expect(page.getByTestId('updates')).toContainText('Версия 0.1.1 загружена и готова к установке.')
  await tabTo(page, page.getByRole('link', { name: 'К серверам' }))
  await page.keyboard.press('Enter')
  await tabTo(page, page.getByTestId('update-ready').getByRole('button', { name: 'Перезапустить и обновить' }))
  await page.keyboard.press('Enter')
  await dialogByKeyboard(page, page.getByRole('dialog', { name: 'Перезапустить и обновить?' }), 'dialog-update')

  await tabTo(page, page.getByTestId('mode-chip'))
  await page.keyboard.press('Enter')
  await dialogByKeyboard(page, page.getByRole('dialog', { name: 'Перейти в расширенный режим?' }), 'dialog-mode')

  // Every page of the settings from its link.
  await tabTo(page, page.getByTestId('portal-settings'))
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/#\/settings\/conn$/)
  const pages = await page.locator('a[href^="#/settings/"]').evaluateAll((links) => links.map((a) => a.getAttribute('href')!))
  expect(pages).toContain('#/settings/backup')
  for (const href of pages) {
    await tabTo(page, page.locator(`a[href="${href}"]`))
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(new RegExp(`${href}$`))
    await tabAround(page, href.slice(2).replace('/', '-'))
  }

  // A setting changed under the running connection waits for its restart.
  await tabTo(page, page.locator('a[href="#/settings/conn"]'))
  await page.keyboard.press('Enter')
  await tabTo(page, page.getByTestId('allow-lan'))
  await page.keyboard.press('Space')
  await tabTo(page, page.getByTestId('restart-chip'))
  await page.keyboard.press('Enter')
  await dialogByKeyboard(page, page.getByRole('dialog', { name: 'Настройки не применены' }), 'dialog-restart')

  // A backup made and chosen again by the keyboard; the chooser of Windows is Playwright's here.
  await tabTo(page, page.locator('a[href="#/settings/backup"]'))
  await page.keyboard.press('Enter')
  await tabTo(page, page.getByTestId('backup-create'))
  const saving = page.waitForEvent('download')
  await page.keyboard.press('Enter')
  const file = (await (await saving).path())!
  await tabTo(page, page.getByTestId('backup-choose'))
  const choosing = page.waitForEvent('filechooser')
  await page.keyboard.press('Enter')
  await (await choosing).setFiles(file)
  await dialogByKeyboard(page, page.getByRole('dialog', { name: 'Восстановить из копии?' }), 'dialog-restore')
})

test('the advanced mode: the rail, the tabs, the settings and the table of profiles', async ({ page }) => {
  test.setTimeout(180_000)
  await throughWizardByKeyboard(page, false)
  await tabTo(page, page.getByTestId('mode-chip'))
  await page.keyboard.press('Enter')
  await tabTo(page, page.getByTestId('mode-switch'))
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/#\/adv\/profiles$/)

  for (const section of ['profiles', 'connections', 'routing', 'logs', 'stats', 'tools', 'settings']) {
    await tabTo(page, page.getByTestId(`rail-${section}`))
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(new RegExp(`#/adv/${section}`))
    await tabAround(page, `adv-${section}`)
  }

  // The tabs of a section go by the arrows.
  await tabTo(page, page.getByTestId('rail-routing'))
  await page.keyboard.press('Enter')
  await tabTo(page, page.getByRole('tab', { name: 'Пресеты и правила' }))
  await page.keyboard.press('ArrowRight')
  await expect(page).toHaveURL(/#\/adv\/routing\/warp$/)
  await tabAround(page, 'adv-routing-warp')
  await tabTo(page, page.getByRole('tab', { name: 'WARP' }), true)
  await page.keyboard.press('ArrowRight')
  await expect(page).toHaveURL(/#\/adv\/routing\/common$/)
  await tabAround(page, 'adv-routing-common')

  await tabTo(page, page.getByTestId('rail-settings'))
  await page.keyboard.press('Enter')
  await expect(page).toHaveURL(/#\/adv\/settings\/conn$/)
  const pages = await page.locator('a[href^="#/adv/settings/"]').evaluateAll((links) => links.map((a) => a.getAttribute('href')!))
  expect(pages).toContain('#/adv/settings/hotkeys')
  for (const href of pages) {
    await tabTo(page, page.locator(`a[href="${href}"]`))
    await page.keyboard.press('Enter')
    await expect(page).toHaveURL(new RegExp(`${href}$`))
    await tabAround(page, href.slice(2).replaceAll('/', '-'))
  }

  // The table of profiles as a list of Windows: the arrows, Shift, Home and End select, the menu key opens its menu,
  // Enter connects.
  await tabTo(page, page.getByTestId('rail-profiles'))
  await page.keyboard.press('Enter')
  const rows = page.getByTestId('profile-row')
  await tabTo(page, page.getByTestId('profiles-table').locator('xpath=..'))
  await page.keyboard.press('ArrowDown')
  await expect(rows.nth(0)).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('ArrowDown')
  await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true')
  await expect(rows.nth(0)).toHaveAttribute('aria-selected', 'false')
  await page.keyboard.press('Shift+ArrowDown')
  await expect(rows.nth(1)).toHaveAttribute('aria-selected', 'true')
  await expect(rows.nth(2)).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Home')
  await expect(rows.nth(0)).toHaveAttribute('aria-selected', 'true')
  await expect(rows.nth(2)).toHaveAttribute('aria-selected', 'false')
  await page.keyboard.press('End')
  await expect(rows.nth(2)).toHaveAttribute('aria-selected', 'true')
  await page.keyboard.press('Home')

  await page.keyboard.press('Shift+F10')
  await menuItem(page, 'QR-код и ссылка')
  await dialogByKeyboard(page, page.getByRole('dialog', { name: 'Germany' }), 'dialog-qr')

  await tabTo(page, page.getByTestId('profiles-table').locator('xpath=..'))
  await page.keyboard.press('Enter')
  await expect(page.getByRole('dialog', { name: 'Как пускать трафик через сервер' })).toBeVisible()
  await tabTo(page, page.getByTestId('modes-none'))
  await page.keyboard.press('Enter')
  const connbar = page.getByTestId('connbar')
  await expect(connbar).toHaveAttribute('data-state', 'running')
  await tabTo(page, connbar.getByTestId('stop'))
  await page.keyboard.press('Enter')
  await expect(connbar).toHaveAttribute('data-state', 'idle')
  await expect(connbar.getByTestId('start')).toBeFocused()
  await page.keyboard.press('Enter')
  await expect(connbar).toHaveAttribute('data-state', 'running')
  await expect(connbar.getByTestId('stop')).toBeFocused()
})
