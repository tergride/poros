// WebView2 is a browser: left alone, it reloads the page on F5, prints on Ctrl+P, opens its downloads on Ctrl+J and
// its own menu on a right click. In an app these leave the user on a browser page or lose what was on the screen.

const keys = new Set(['f3', 'f5', 'browserback', 'browserforward', 'browserrefresh', 'browsersearch', 'browserhome'])
// With Ctrl: reload, print, find and find next, downloads, history, page source, save, open, new window.
const ctrlKeys = new Set(['r', 'p', 'f', 'g', 'j', 'h', 'u', 's', 'o', 'n'])
// With Ctrl+Shift: the developer tools, kept in a development build.
const devKeys = new Set(['i', 'j', 'c'])

/** Whether a key press is one of the browser's own commands. */
export function isBrowserKey(event: Pick<KeyboardEvent, 'key' | 'ctrlKey' | 'metaKey' | 'altKey' | 'shiftKey'>, dev = false): boolean {
  const key = event.key.toLowerCase()
  const ctrl = event.ctrlKey || event.metaKey
  if (keys.has(key)) return true
  if (event.altKey && !ctrl && (key === 'arrowleft' || key === 'arrowright')) return true
  if (ctrl && event.shiftKey && devKeys.has(key)) return !dev
  return ctrl && !event.altKey && ctrlKeys.has(key)
}

/** A right click opens the browser's menu only in text fields, where it cuts, copies and pastes. */
export function keepsMenu(target: EventTarget | null): boolean {
  return target instanceof Element && target.closest('input, textarea, [contenteditable="true"]') !== null
}

/** Keeps the browser's commands out of the window: keys, the context menu and the mouse's back and forward buttons. */
export function keepBrowserOut(target: Window, dev = false): void {
  target.addEventListener(
    'keydown',
    (event) => {
      if (isBrowserKey(event, dev)) event.preventDefault()
    },
    { capture: true },
  )
  target.addEventListener('contextmenu', (event) => {
    if (!keepsMenu(event.target)) event.preventDefault()
  })
  target.addEventListener('mouseup', (event) => {
    if (event.button === 3 || event.button === 4) event.preventDefault()
  })
}
