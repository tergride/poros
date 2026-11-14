import { readFileSync } from 'node:fs'

import { type Page, expect, test } from '@playwright/test'

// Every screen of both modes in both languages at the smallest, the default and a large window, and with the large text
// of the settings in the first two, against the core in memory: no warning or error in the console (vue-i18n warns of
// a missing message), nothing wider or higher than the window, no
// text cut off where the screen does not end it with «…», no phrase pressed into a column of single words, and no
// Russian text in English. Screenshots of the smallest window go to test-results/shots.

// Each language and size goes through the wizard on its own page, so they run side by side.
test.describe.configure({ mode: 'parallel' })

type Strings = { [key: string]: Strings & string }
const strings = (lang: string): Strings => JSON.parse(readFileSync(new URL(`../src/i18n/${lang}.json`, import.meta.url), 'utf8'))

const languages = [
  { lang: 'ru', locale: 'ru-RU' },
  { lang: 'en', locale: 'en-US' },
]
// The smallest window the shell allows, the one it opens with, and a large screen; the large text of the settings in
// the first two.
const sizes = [
  { width: 900, height: 600, large: false },
  { width: 1180, height: 760, large: false },
  { width: 1920, height: 1080, large: false },
  { width: 900, height: 600, large: true },
  { width: 1180, height: 760, large: true },
]

// The sections of the advanced mode with the tabs in their routes (src/router.ts); the settings pages of each mode are
// taken from its navigation.
const advancedScreens = [
  '/adv/profiles',
  '/adv/connections',
  '/adv/routing/rules',
  '/adv/routing/rules/hardened',
  '/adv/routing/warp',
  '/adv/routing/common',
  '/adv/logs',
  '/adv/stats/now',
  '/adv/stats/traffic',
  '/adv/tools/otp',
  '/adv/tools/misc',
]

/** What a screen shows wrong at this size. Runs in the page, so it carries everything it needs. */
function layoutProblems(): string[] {
  const problems: string[] = []
  const describe = (el: Element) => {
    const id = el.getAttribute('data-testid')
    const text = (el.textContent ?? '').replace(/\s+/g, ' ').trim().slice(0, 50)
    return `<${el.tagName.toLowerCase()}${id ? ` ${id}` : ''}> «${text}»`
  }

  const root = document.scrollingElement ?? document.documentElement
  if (root.scrollWidth > root.clientWidth) problems.push(`the page is ${root.scrollWidth}px wide in ${root.clientWidth}px`)
  if (root.scrollHeight > root.clientHeight) problems.push(`the page is ${root.scrollHeight}px high in ${root.clientHeight}px`)

  // A table of data scrolls sideways when its columns do not fit: its box is there for that.
  for (const el of document.body.querySelectorAll('*')) {
    const style = getComputedStyle(el)
    if ((style.overflowX !== 'auto' && style.overflowX !== 'scroll') || el.querySelector(':scope > table')) continue
    if (el.clientWidth > 0 && el.scrollWidth > el.clientWidth + 1) {
      problems.push(`scrolls sideways, ${el.scrollWidth}px in ${el.clientWidth}px: ${describe(el)}`)
    }
  }

  // Text cut by a box that hides its overflow, unless the box ends it with «…» or fades it out on purpose.
  const range = document.createRange()
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT)
  for (let node = walker.nextNode(); node; node = walker.nextNode()) {
    const parent = node.parentElement
    if (!node.textContent?.trim() || !parent || ['TEXTAREA', 'OPTION', 'SCRIPT', 'STYLE'].includes(parent.tagName)) continue
    if (!parent.checkVisibility({ checkOpacity: true, checkVisibilityCSS: true })) continue
    range.selectNodeContents(node)
    const text = range.getBoundingClientRect()
    if (text.width === 0 || text.height === 0) continue
    // A phrase pressed into a narrow column by its neighbours reads a word or two per line.
    const lines = new Set([...range.getClientRects()].map((line) => Math.round(line.top))).size
    if (lines >= 3 && node.textContent.trim().split(/\s+/).length / lines <= 2.5) {
      problems.push(`text pressed into ${Math.round(text.width)}px, ${lines} lines: ${describe(parent)}`)
    }
    let sideways = true
    let downwards = true
    for (let el: Element | null = parent; el && el !== document.body && (sideways || downwards); el = el.parentElement) {
      const style = getComputedStyle(el)
      const box = el.getBoundingClientRect()
      // The rectangles are in the pixels of the window, the sizes of a box in its own: they differ under the zoom of
      // the large text.
      const scale = el instanceof HTMLElement && el.offsetWidth > 0 ? box.width / el.offsetWidth : 1
      const left = box.left + el.clientLeft * scale
      const top = box.top + el.clientTop * scale
      const width = el.clientWidth * scale
      const height = el.clientHeight * scale
      const clips = (overflow: string) => overflow === 'hidden' || overflow === 'clip'
      const faded = style.maskImage !== 'none' && style.maskImage !== ''
      // A box of a pixel or two hides text for screen readers only.
      const tiny = el.clientWidth <= 2 || el.clientHeight <= 2
      if (sideways && style.overflowX !== 'visible') {
        sideways = false
        const cut = text.right > left + width + 1 || text.left < left - 1
        const seen = text.left < left + width && text.right > left
        if (clips(style.overflowX) && cut && seen && !tiny && !faded && style.textOverflow !== 'ellipsis') {
          problems.push(`text cut at the side: ${describe(parent)} in ${describe(el)}`)
        }
      }
      if (downwards && style.overflowY !== 'visible') {
        downwards = false
        // A few pixels are the room of the font above and below its letters.
        const cut = text.bottom > top + height + 4
        const seen = text.top < top + height
        const clamped = style.getPropertyValue('-webkit-line-clamp') !== 'none'
        if (clips(style.overflowY) && cut && seen && !tiny && !faded && !clamped) {
          problems.push(`text cut at the bottom: ${describe(parent)} in ${describe(el)}`)
        }
      }
    }
  }
  return problems
}

test('the layout check finds cut text and sideways scrolling, and lets «…» and hidden text be', async ({ page }) => {
  await page.setContent(`
    <body style="margin:0;font:14px sans-serif">
      <div style="width:60px;overflow:hidden;white-space:nowrap"><span>cut at the side of its box</span></div>
      <div style="width:60px;overflow:hidden;white-space:nowrap;text-overflow:ellipsis">ends with an ellipsis</div>
      <div style="width:120px;height:18px;overflow:hidden">a second line under the first one is cut</div>
      <div style="width:60px;overflow:auto"><div style="width:200px">wide</div></div>
      <span style="position:absolute;width:1px;height:1px;overflow:hidden;white-space:nowrap">for screen readers</span>
      <div style="display:none;width:10px;overflow:hidden;white-space:nowrap">not shown</div>
      <div style="display:flex;width:300px"><span>a hint of a few short words</span><b style="flex:none;width:260px"></b></div>
    </body>`)
  const found = await page.evaluate(layoutProblems)
  expect(found).toEqual([
    expect.stringMatching(/^scrolls sideways, 200px in 60px/),
    expect.stringMatching(/^text cut at the side: <span> «cut at the side/),
    expect.stringMatching(/^text cut at the bottom: <div> «a second line/),
    expect.stringMatching(/^text pressed into \d+px, \d lines: <span> «a hint of a few/),
  ])
})

for (const { lang, locale } of languages) {
  const s = strings(lang)
  for (const size of sizes) {
    const where = `${lang}-${size.width}${size.large ? '-large' : ''}`
    const viewport = { width: size.width, height: size.height }

    test.describe(`${lang}, ${size.width}×${size.height}${size.large ? ', large text' : ''}`, () => {
      test.use({ locale, viewport })

      let problems: string[] = []

      test.beforeEach(({ page }) => {
        problems = []
        page.on('console', (message) => {
          if (message.type() === 'error' || message.type() === 'warning') problems.push(`console ${message.type()}: ${message.text()}`)
        })
        page.on('pageerror', (error) => problems.push(`page error: ${error.message}`))
      })

      test.afterEach(() => {
        expect(problems, 'the problems of the screens').toEqual([])
      })

      /** Checks the screen as it is now; names it in the problems and in its screenshot. */
      async function check(page: Page, name: string): Promise<void> {
        await page.evaluate(async () => {
          await document.fonts.ready
          await new Promise((resolve) => setTimeout(resolve, 100))
          await new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve)))
        })
        expect(await page.evaluate(() => document.documentElement.lang)).toBe(lang)
        const found = await page.evaluate(layoutProblems)
        if (lang === 'en') {
          const russian = await page.evaluate(() => document.body.innerText.match(/[^\n]*[\u0400-\u04ff][^\n]*/g) ?? [])
          found.push(...russian.map((line) => `Russian text in English: ${line.slice(0, 80)}`))
        }
        problems.push(...found.map((problem) => `${name}: ${problem}`))
        if (size.width === 900 || found.length > 0) {
          await page.screenshot({ path: `test-results/shots/layout-${where}-${name}.png`, animations: 'disabled' })
        }
      }

      /** Opens a screen through the router of the page, as a link in the window would. */
      async function visit(page: Page, path: string, checked = true): Promise<void> {
        const reached = await page.evaluate(async (to) => {
          type Router = { push(to: string): Promise<unknown>; currentRoute: { value: { path: string } } }
          const app = (document.querySelector('#app') as { __vue_app__?: { config: { globalProperties: { $router: Router } } } } | null)
            ?.__vue_app__
          const router = app!.config.globalProperties.$router
          await router.push(to)
          return router.currentRoute.value.path
        }, path)
        expect(reached, `${path} is a screen of this mode`).toBe(path)
        if (checked) await check(page, path.slice(1).replaceAll('/', '-'))
      }

      async function throughWizard(page: Page, checked: boolean): Promise<void> {
        const step = async (name: string) => (checked ? check(page, name) : undefined)
        const next = page.getByRole('button', { name: s.common.next, exact: true })
        await page.goto('/')
        await expect(page.getByTestId('wizard-mode-simple')).toBeVisible()
        await step('wizard-1')
        await next.click()
        const link = page.getByLabel(s.wizard.subscription.label, { exact: true })
        await expect(link).toBeVisible()
        await step('wizard-2')
        await link.fill('https://sub.example.com/u/8f3a2c')
        await link.press('Enter')
        await expect(page.getByTestId('link-found')).toBeVisible()
        await step('wizard-2-found')
        await next.click()
        await expect(page.getByTestId('wizard-links')).toBeVisible()
        await step('wizard-3')
        await next.click()
        await expect(page.getByRole('radiogroup', { name: s.wizard.steps.routing })).toBeVisible()
        await step('wizard-4')
        await page.getByRole('button', { name: s.common.done, exact: true }).click()
        await expect(page.getByTestId('portal')).toBeVisible()
      }

      /** The large text of «Appearance», which the settings of both modes share. */
      async function largeText(page: Page): Promise<void> {
        await visit(page, '/settings/look', false)
        await page.getByRole('combobox', { name: s.settings.look.textSize, exact: true }).click()
        await page.getByRole('option', { name: s.settings.look.large, exact: true }).click()
        await expect(page.locator('html')).toHaveClass(/text-large/)
      }

      const pagesIn = (page: Page, prefix: string) =>
        page.evaluate(
          (start) => [...new Set([...document.querySelectorAll('a[href]')].map((a) => a.getAttribute('href')!.slice(1)))].filter((h) => h.startsWith(start)),
          prefix,
        )

      test('the simple mode', async ({ page }) => {
        test.setTimeout(60_000)
        await throughWizard(page, !size.large)
        if (size.large) {
          await largeText(page)
          for (const step of [1, 2, 3, 4]) await visit(page, `/wizard/${step}`)
          await visit(page, '/main', false)
        }
        await check(page, 'main')

        await visit(page, '/settings/conn')
        const settings = await pagesIn(page, '/settings/')
        expect(settings).toContain('/settings/presets')
        for (const path of settings) await visit(page, path)

        // The editor of a preset: a copy of the first one, not saved.
        await visit(page, '/settings/presets')
        await page.getByTestId('preset-copy').first().click()
        await page.getByTestId('new-preset-create').click()
        await expect(page.getByTestId('preset-name')).toBeVisible()
        await check(page, 'preset')
      })

      test('the advanced mode', async ({ page }) => {
        test.setTimeout(60_000)
        await throughWizard(page, false)
        if (size.large) {
          await largeText(page)
          await visit(page, '/main', false)
        }
        await page.getByRole('button', { name: s.menu.open, exact: true }).click()
        await page.getByTestId('segment-advanced').click()
        await expect(page.getByTestId('profiles-table')).toBeVisible()
        await page.keyboard.press('Escape')
        await expect(page.getByTestId('segment-advanced')).toBeHidden()

        for (const path of advancedScreens) await visit(page, path)
        await visit(page, '/adv/settings/conn')
        const settings = await pagesIn(page, '/adv/settings/')
        expect(settings).toContain('/adv/settings/hotkeys')
        for (const path of settings) await visit(page, path)
      })
    })
  }
}
