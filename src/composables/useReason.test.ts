import { describe, expect, it } from 'vitest'

import { sentence } from './useReason'

describe('a reason told on its own', () => {
  it('starts with a capital and ends with a stop, once', () => {
    expect(sentence('подписка не загрузилась')).toBe('Подписка не загрузилась.')
    expect(sentence('Это ссылка на один сервер.')).toBe('Это ссылка на один сервер.')
    expect(sentence('  the core stopped  ')).toBe('The core stopped.')
    expect(sentence('«x» — это не сайт')).toBe('«x» — это не сайт.')
    expect(sentence('')).toBe('')
  })
})
