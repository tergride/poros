import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'
import { getCurrentWindow } from '@tauri-apps/api/window'

import type { TrayItem } from '@/lib/trayMenu'

import { type CoreClient, createTauriCore } from './core'
import { createStubCore } from './stubCore'

export type { CoreClient, CoreError, CoreEvent, CoreMethod, CoreStatus } from './core'
export { CodeElevationCancelled, CodeStartFailed, StatusDataDamaged, StatusNotAnswering, isCoreError } from './core'

const inTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window

let corePromise: Promise<CoreClient> | undefined

/** The one client of the core for the whole interface: the real core in Tauri, the in-memory one elsewhere. */
export function useCore(): Promise<CoreClient> {
  corePromise ??= inTauri ? createTauriCore() : Promise.resolve(createStubCore())
  return corePromise
}

/** The window's own buttons. Close hides to the tray when the settings say so: the shell decides. */
export const windowControls = {
  minimize: (): Promise<void> => (inTauri ? getCurrentWindow().minimize() : Promise.resolve()),
  toggleMaximize: (): Promise<void> => (inTauri ? getCurrentWindow().toggleMaximize() : Promise.resolve()),
  close: (): Promise<void> => (inTauri ? getCurrentWindow().close() : Promise.resolve()),
}

/** Who opens `poros://` links: this copy, another copy (its program) or nobody. */
export interface UrlScheme {
  owner: 'this' | 'other' | 'none'
  program: string
}

// Outside Tauri the links live in memory, like the stub core's data.
let stubScheme: UrlScheme = { owner: 'none', program: '' }

/** What Windows holds for `poros://`; Poros writes it only when the user asks, never at launch. */
export const urlScheme = {
  get: (): Promise<UrlScheme> => (inTauri ? invoke<UrlScheme>('url_scheme_get') : Promise.resolve({ ...stubScheme })),
  set: (on: boolean): Promise<UrlScheme> => {
    if (inTauri) return invoke<UrlScheme>('url_scheme_set', { on })
    stubScheme = { owner: on ? 'this' : 'none', program: '' }
    return Promise.resolve({ ...stubScheme })
  },
}

/** The program as a whole: show or hide its window, quit, install the downloaded update. */
export const appControls = {
  show: (): Promise<void> => (inTauri ? invoke('app_show') : Promise.resolve()),
  hide: (): Promise<void> => (inTauri ? invoke('app_hide') : Promise.resolve()),
  exit: (): Promise<void> => (inTauri ? invoke('app_exit') : Promise.resolve()),
  /** The updater replaces the files and starts Poros again, connected when `reconnect` says so; Poros exits. */
  update: (reconnect: boolean): Promise<void> =>
    inTauri ? invoke('app_update', { reconnect }) : Promise.resolve(),
  /** Whether this start follows an update of a connected Poros; true once. */
  takeReconnect: (): Promise<boolean> => (inTauri ? invoke<boolean>('app_take_reconnect') : Promise.resolve(false)),
  /** Writes a line to the log of the shell and the interface, next to the core's; nothing waits for it. */
  log: (text: string): void => {
    if (inTauri) void invoke('app_log', { text }).catch(() => undefined)
  },
  /** Opens the folder of the logs in Explorer. */
  openLogs: (): Promise<void> => (inTauri ? invoke('app_open_logs') : Promise.resolve()),
  /** Opens the data folder in Explorer. */
  openData: (): Promise<void> => (inTauri ? invoke('app_open_data') : Promise.resolve()),
  /** Puts text on the clipboard through the shell, which works while the window is hidden, as for the tray. */
  copy: (text: string): Promise<void> => (inTauri ? invoke('app_copy', { text }) : navigator.clipboard.writeText(text)),
  /** Keeps the window out of snapshots of the screen, while the core reads the QR codes on it, or lets it back in. */
  excludeFromCapture: (on: boolean): Promise<void> => (inTauri ? invoke('app_capture_exclude', { on }) : Promise.resolve()),
}

/** The shortcuts of Windows: their actions as they are pressed, and the ones another program holds. */
export const hotkeys = {
  /** The shell shows the window itself; the other actions come here. Outside Tauri a `poros-hotkey` event stands in. */
  onAction: async (handler: (action: string) => void): Promise<void> => {
    if (!inTauri) {
      window.addEventListener('poros-hotkey', (event) => handler((event as CustomEvent<string>).detail))
      return
    }
    await listen<string>('hotkey', ({ payload }) => handler(payload))
  },
  failed: (): Promise<string[]> => (inTauri ? invoke<string[]>('hotkeys_failed') : Promise.resolve([])),
  onFailed: async (handler: (actions: string[]) => void): Promise<() => void> =>
    inTauri ? await listen<string[]>('hotkeys-failed', ({ payload }) => handler(payload)) : () => undefined,
}

/** A menu at the pointer, as the tray's; outside Tauri there is none. */
export async function popupMenu(items: TrayItem[]): Promise<void> {
  if (!inTauri) return
  const { popupTrayMenu } = await import('./tray')
  await popupTrayMenu(items)
}

export async function setTray(items: TrayItem[], tooltip: string): Promise<void> {
  if (!inTauri) return
  const { setTrayMenu } = await import('./tray')
  await setTrayMenu(items, tooltip)
}

/**
 * Calls `handler` with the `poros://` links of the command line and of every later start.
 */
export async function onLinks(handler: (links: string[]) => void): Promise<void> {
  if (!inTauri) {
    // Playwright stands in for a second start with a `poros-links` event.
    window.addEventListener('poros-links', (event) => handler((event as CustomEvent<string[]>).detail))
    return
  }
  const take = async () => {
    const links = await invoke<string[]>('take_links')
    if (links.length) handler(links)
  }
  await listen('app-links', () => void take())
  await take()
}
