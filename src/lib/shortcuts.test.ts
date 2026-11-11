import { describe, expect, it } from 'vitest'

import { isShortcut, keyName, shortcutOf, windowShortcut } from './shortcuts'

const keys = (code: string, mods: { ctrl?: boolean; alt?: boolean; shift?: boolean; win?: boolean } = {}) => ({
  code,
  ctrlKey: !!mods.ctrl,
  altKey: !!mods.alt,
  shiftKey: !!mods.shift,
  metaKey: !!mods.win,
})

describe('shortcuts', () => {
  it('names keys by their place, whatever the layout', () => {
    expect(keyName('KeyK')).toBe('K')
    expect(keyName('Digit7')).toBe('7')
    expect(keyName('F12')).toBe('F12')
    expect(keyName('ArrowUp')).toBe('Up')
    expect(keyName('ControlLeft')).toBeNull()
    expect(keyName('Comma')).toBeNull()
  })

  it('writes modifiers in one order, as the core checks them', () => {
    expect(shortcutOf(keys('KeyP', { win: true, alt: true, ctrl: true }))).toBe('Ctrl+Alt+Win+P')
    expect(shortcutOf(keys('ShiftLeft', { shift: true }))).toBeNull()
    expect(isShortcut(keys('KeyK', { ctrl: true }), 'Ctrl+K')).toBe(true)
    expect(isShortcut(keys('KeyK', { ctrl: true, shift: true }), 'Ctrl+K')).toBe(false)
    expect(isShortcut(keys('KeyK', { ctrl: true }), '')).toBe(false)
  })

  it('keeps shortcuts that would type out of the window', () => {
    expect(windowShortcut('Ctrl+V')).toBe(true)
    expect(windowShortcut('F7')).toBe(true)
    expect(windowShortcut('Shift+R')).toBe(false)
    expect(windowShortcut('Win+Up')).toBe(false)
  })
})
