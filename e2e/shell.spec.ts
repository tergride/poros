import { expect, test } from '@playwright/test'

test('the shell renders with the mockup fonts and the system language', async ({ page }) => {
  await page.goto('/')

  await expect(page.getByRole('button', { name: 'Свернуть' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Развернуть' })).toBeVisible()
  await expect(page.getByRole('button', { name: 'Закрыть' })).toBeVisible()
  await expect(page.getByTestId('app-info')).toContainText('Версия dev')

  const loaded = await page.evaluate(async () => {
    await document.fonts.ready
    return [...document.fonts].filter((font) => font.status === 'loaded').map((font) => font.family.replaceAll('"', ''))
  })
  expect(loaded).toEqual(expect.arrayContaining(['Onest', 'Unbounded', 'JetBrains Mono']))

  await page.screenshot({ path: 'test-results/shots/shell.png' })
})
