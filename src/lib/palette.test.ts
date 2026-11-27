import { describe, expect, it } from 'vitest'

import { findLabel, score, settingLabels } from './palette'

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

  it('finds the element that shows a label as its own text, not inside a hint', () => {
    const page = document.createElement('div')
    page.innerHTML = `
      <p>Strict Route keeps the routes</p>
      <label><input type="checkbox"> Strict Route </label>
      <div><strong>TUN Routing<svg></svg></strong><span>Hint</span></div>`
    expect(findLabel(page, 'Strict Route')?.tagName).toBe('LABEL')
    expect(findLabel(page, 'TUN Routing')?.tagName).toBe('STRONG')
    expect(findLabel(page, 'Strict')).toBeNull()
  })
})
