import { type VueWrapper, mount } from '@vue/test-utils'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'
import { defineComponent, h, nextTick, ref, withDirectives } from 'vue'

import { position, vTooltip } from './tooltip'
import TooltipHost from './TooltipHost.vue'

describe('position', () => {
  const view = { width: 900, height: 600 }
  const target = { left: 100, right: 200, top: 100, bottom: 130 }

  it('puts the tooltip under the element, along its start or its end', () => {
    expect(position(target, { width: 150, height: 40 }, false, view)).toEqual({ left: 100, top: 136 })
    expect(position(target, { width: 150, height: 40 }, true, view)).toEqual({ left: 50, top: 136 })
  })

  it('puts it over the element where the window ends below', () => {
    expect(position({ ...target, top: 540, bottom: 570 }, { width: 150, height: 40 }, false, view)).toEqual({ left: 100, top: 494 })
  })

  it('keeps it inside the window', () => {
    expect(position({ ...target, left: 800, right: 880 }, { width: 300, height: 40 }, false, view).left).toBe(588)
    expect(position({ ...target, left: 2, right: 40 }, { width: 300, height: 40 }, true, view).left).toBe(12)
  })
})

describe('v-tooltip', () => {
  const text = ref('')
  const chipShown = ref(true)
  const Page = defineComponent({
    setup: () => () =>
      h('div', [
        chipShown.value ? withDirectives(h('button', { 'data-testid': 'chip' }, [h('b', 'Chip'), h('i', '!')]), [[vTooltip, text.value]]) : null,
        withDirectives(h('span', { 'data-testid': 'name' }, 'A long name'), [[vTooltip, 'A long name', '', { cut: true }]]),
        h('p', { 'data-testid': 'plain' }, 'Plain'),
        h(TooltipHost),
      ]),
  })
  let page: VueWrapper
  const tip = () => document.body.querySelector('[data-testid="tooltip"]')
  const el = (id: string) => page.get(`[data-testid="${id}"]`).element
  const point = (target: Element, type = 'pointerover', more: PointerEventInit = {}) =>
    target.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerType: 'mouse', ...more }))
  const rest = async (ms = 300) => {
    vi.advanceTimersByTime(ms)
    await nextTick()
  }

  beforeEach(() => {
    vi.useFakeTimers()
    text.value = 'Settings wait for a restart'
    chipShown.value = true
    page = mount(Page, { attachTo: document.body })
  })
  afterEach(() => {
    page.unmount()
    vi.useRealTimers()
    document.body.innerHTML = ''
  })

  it('shows after the pointer rests, describes the element and goes when the pointer leaves', async () => {
    const chip = el('chip')
    expect(chip.getAttribute('data-tooltip')).toBe('Settings wait for a restart')
    point(chip.querySelector('b')!)
    await nextTick()
    expect(tip()).toBeNull()
    await rest()
    expect(tip()?.textContent?.trim()).toBe('Settings wait for a restart')
    expect(chip.getAttribute('aria-describedby')).toBe(tip()?.id)

    text.value = 'Restart to apply'
    await nextTick()
    expect(tip()?.textContent?.trim()).toBe('Restart to apply')

    point(el('plain'))
    await nextTick()
    expect(tip()).toBeNull()
    expect(chip.hasAttribute('aria-describedby')).toBe(false)
  })

  it('keeps waiting while the pointer moves between the parts of the element', async () => {
    const chip = el('chip')
    point(chip.querySelector('b')!)
    await rest(200)
    point(chip.querySelector('i')!)
    await rest(100)
    expect(tip()).not.toBeNull()
  })

  it('goes on a press or Escape and stays away until the pointer leaves', async () => {
    const chip = el('chip')
    point(chip)
    await rest()
    expect(tip()).not.toBeNull()
    point(chip, 'pointerdown')
    await nextTick()
    expect(tip()).toBeNull()
    point(chip.querySelector('b')!)
    await rest()
    expect(tip()).toBeNull()

    point(el('plain'))
    await rest(1000)
    point(chip)
    await rest()
    expect(tip()).not.toBeNull()
    document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }))
    await nextTick()
    expect(tip()).toBeNull()
    point(chip.querySelector('i')!)
    await rest()
    expect(tip()).toBeNull()
  })

  it('shows the whole text of an element only while it is cut', async () => {
    const name = el('name')
    point(name)
    await rest()
    expect(tip()).toBeNull()

    Object.defineProperty(name, 'scrollWidth', { configurable: true, value: 240 })
    Object.defineProperty(name, 'clientWidth', { configurable: true, value: 120 })
    point(el('plain'))
    point(name)
    await rest()
    expect(tip()?.textContent?.trim()).toBe('A long name')
  })

  it('goes with its element, also while it waits, and an empty text shows nothing', async () => {
    point(el('chip'))
    await rest()
    expect(tip()).not.toBeNull()
    chipShown.value = false
    await nextTick()
    await nextTick()
    expect(tip()).toBeNull()

    chipShown.value = true
    await rest(1000)
    point(el('chip'))
    chipShown.value = false
    await nextTick()
    await rest(1000)
    expect(tip()).toBeNull()

    chipShown.value = true
    text.value = ''
    await nextTick()
    expect(el('chip').hasAttribute('data-tooltip')).toBe(false)
    point(el('chip'))
    await rest()
    expect(tip()).toBeNull()
  })
})
