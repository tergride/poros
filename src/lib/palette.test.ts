import { describe, expect, it } from 'vitest'

import { score, settingLabels } from './palette'

describe('the search of the palette', () => {
  it('takes every word of the query, in any case and with «ё» as «е»', () => {
    expect(score('strict', 'Strict Route')).toBe(0)
    expect(score('route str', 'Strict Route')).toBe(1)
    expect(score('елки', 'Ёлки-палки')).toBe(0)
    expect(score('франкфурт', 'Germany', 'de.example.com Франкфурт')).toBe(2)
    expect(score('tun dns', 'TUN')).toBeNull()
  })

  it('finds the short labels of settings and leaves hints and buttons', () => {
    const labels = settingLabels({ adapter: 'Адаптер', routingHint: 'Подсети, которые…', bypassDefault: 'Вернуть диапазоны', nested: { mtu: 'MTU', count: '{n} строк' } })
    expect(labels).toEqual(['Адаптер', 'Вернуть диапазоны', 'MTU'])
  })
})
