import { describe, expect, it } from 'vitest'

import { russianPlural } from '@/i18n'

import { countryHint, flagOf, formatBytes, formatDate, formatDuration, latencyTone, serverTitle } from './format'

describe('the formats of the screens', () => {
  it('drops the flag a name starts with only when the country is shown beside it', () => {
    expect(serverTitle('🇩🇪 Germany', 'DE')).toBe('Germany')
    expect(serverTitle('🇩🇪🇳🇱 Relay', 'DE')).toBe('Relay')
    expect(serverTitle('🇩🇪 Germany', '')).toBe('🇩🇪 Germany')
    expect(serverTitle('🇩🇪', 'DE')).toBe('🇩🇪')
  })

  it('writes sizes in the largest unit above one', () => {
    const unit = (name: string) => name
    expect(formatBytes(0, 'ru', unit)).toBe('0 B')
    expect(formatBytes(1536, 'ru', unit)).toBe('1,5 KB')
    expect(formatBytes(38.2 * 1024 ** 3, 'ru', unit)).toBe('38,2 GB')
    expect(formatBytes(150 * 1024 ** 2, 'en', unit)).toBe('150 MB')
  })

  it('counts a connection in hours, minutes and seconds', () => {
    expect(formatDuration(768_000)).toBe('00:12:48')
    expect(formatDuration(-5)).toBe('00:00:00')
  })

  it('writes a date with its year only when it is not this one', () => {
    const now = new Date(2026, 10, 7).getTime()
    expect(formatDate(new Date(2026, 11, 7).getTime(), 'ru', now)).toBe('7 декабря')
    expect(formatDate(new Date(2027, 11, 7).getTime(), 'ru', now)).toBe('7 декабря 2027 г.')
    expect(formatDate(new Date(2025, 10, 2).getTime(), 'en', now)).toBe('November 2, 2025')
  })

  it('shows a country as its flag and names it in the hint', () => {
    expect(flagOf('de')).toBe('🇩🇪')
    expect(flagOf('BY')).toBe('🇧🇾')
    expect(flagOf('')).toBe('')
    expect(flagOf('DEU')).toBe('')
    expect(countryHint('de', 'ru')).toBe('DE — Германия')
    expect(countryHint('DE', 'en')).toBe('DE — Germany')
    expect(countryHint('XX', 'en')).toBe('XX')
  })

  it('colours a delay', () => {
    expect([0, -1, 42, 96, 238].map(latencyTone)).toEqual(['none', 'none', 'good', 'mid', 'bad'])
  })

  it('picks the Russian plural form', () => {
    expect([0, 1, 2, 5, 11, 21, 22, 25, 111].map((n) => russianPlural(n, 4))).toEqual([0, 1, 2, 3, 3, 1, 2, 3, 3])
  })
})
