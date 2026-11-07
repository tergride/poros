import { describe, expect, it } from 'vitest'

import { isBrowserKey, keepsMenu } from './browserKeys'

const key = (k: string, mods: Partial<Record<'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey', boolean>> = {}) => ({
  key: k,
  ctrlKey: false,
  metaKey: false,
  altKey: false,
  shiftKey: false,
  ...mods,
})

describe('the browser kept out of the window', () => {
  it('stops the browser commands: reload, print, find, downloads, history, back', () => {
    for (const press of [key('F5'), key('r', { ctrlKey: true }), key('R', { ctrlKey: true, shiftKey: true }), key('p', { ctrlKey: true }),
      key('f', { ctrlKey: true }), key('F3'), key('j', { ctrlKey: true }), key('h', { ctrlKey: true }), key('ArrowLeft', { altKey: true }),
      key('BrowserBack'), key('n', { ctrlKey: true })]) {
      expect(isBrowserKey(press), press.key).toBe(true)
    }
  })

  it('leaves the keys of the interface and of text editing alone', () => {
    for (const press of [key('a', { ctrlKey: true }), key('c', { ctrlKey: true }), key('v', { ctrlKey: true }), key('x', { ctrlKey: true }),
      key('z', { ctrlKey: true }), key('Enter'), key('Escape'), key('ArrowLeft'), key('Tab'), key('r')]) {
      expect(isBrowserKey(press), press.key).toBe(false)
    }
  })

  it('keeps the developer tools for a development build only', () => {
    expect(isBrowserKey(key('I', { ctrlKey: true, shiftKey: true }))).toBe(true)
    expect(isBrowserKey(key('I', { ctrlKey: true, shiftKey: true }), true)).toBe(false)
  })

  it('opens the browser menu in text fields only, where it cuts, copies and pastes', () => {
    const input = document.createElement('input')
    const button = document.createElement('button')
    const span = document.createElement('span')
    const editor = document.createElement('div')
    editor.setAttribute('contenteditable', 'true')
    editor.append(span)
    expect(keepsMenu(input)).toBe(true)
    expect(keepsMenu(span)).toBe(true)
    expect(keepsMenu(button)).toBe(false)
    expect(keepsMenu(null)).toBe(false)
  })
})
