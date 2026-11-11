// Shortcuts as the settings write them: modifiers in the order Ctrl, Alt, Shift, Win, then the key, «Ctrl+Alt+P».
// core/engine/hotkeys.go checks them and src-tauri/src/hotkeys.rs registers the global ones with Windows.

type Keys = Pick<KeyboardEvent, 'code' | 'ctrlKey' | 'altKey' | 'shiftKey' | 'metaKey'>

const named: Record<string, string> = {
  ArrowUp: 'Up',
  ArrowDown: 'Down',
  ArrowLeft: 'Left',
  ArrowRight: 'Right',
  Home: 'Home',
  End: 'End',
  PageUp: 'PageUp',
  PageDown: 'PageDown',
  Insert: 'Insert',
  Delete: 'Delete',
  Space: 'Space',
  Enter: 'Enter',
  Tab: 'Tab',
  Backspace: 'Backspace',
}

/** The name of a key by its place on the keyboard, whatever the layout; null for one a shortcut does not take. */
export function keyName(code: string): string | null {
  const letter = /^Key([A-Z])$/.exec(code)
  if (letter) return letter[1]!
  const digit = /^Digit([0-9])$/.exec(code)
  if (digit) return digit[1]!
  if (/^F([1-9]|1[0-9]|2[0-4])$/.test(code)) return code
  return named[code] ?? null
}

/** The shortcut of a key pressed with its modifiers; null while only modifiers are down, or for a key not taken. */
export function shortcutOf(event: Keys): string | null {
  const key = keyName(event.code)
  if (!key) return null
  const modifiers = [event.ctrlKey && 'Ctrl', event.altKey && 'Alt', event.shiftKey && 'Shift', event.metaKey && 'Win'].filter(Boolean)
  return [...modifiers, key].join('+')
}

/** Whether a key pressed is the shortcut; "" is no shortcut. */
export function isShortcut(event: Keys, shortcut: string): boolean {
  return shortcut !== '' && shortcutOf(event) === shortcut
}

/** Whether the cursor is in a field, where keys type rather than act. */
export function inField(target: EventTarget | null): boolean {
  return target instanceof HTMLElement && (target.isContentEditable || ['INPUT', 'TEXTAREA', 'SELECT'].includes(target.tagName))
}

/** Whether a shortcut takes the window's own actions: Ctrl or Alt with a key, or a function key alone. */
export function windowShortcut(shortcut: string): boolean {
  const parts = shortcut.split('+')
  return /^F\d+$/.test(parts.at(-1)!) || parts.includes('Ctrl') || parts.includes('Alt')
}
