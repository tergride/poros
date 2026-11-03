import { describe, expect, it } from 'vitest'

import { resolveLocale } from './locale'

describe('resolveLocale', () => {
  it('takes the first supported language of the system list', () => {
    expect(resolveLocale(['de-DE', 'ru-RU', 'en-US'])).toBe('ru')
    expect(resolveLocale(['en-GB', 'ru'])).toBe('en')
  })

  it('falls back to English', () => {
    expect(resolveLocale(['uk-UA', 'de'])).toBe('en')
    expect(resolveLocale([])).toBe('en')
  })
})
