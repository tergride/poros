import { invoke } from '@tauri-apps/api/core'
import { listen } from '@tauri-apps/api/event'
import { getCurrentWindow } from '@tauri-apps/api/window'

import type { TrayItem } from '@/lib/trayMenu'

import { type CoreClient, createTauriCore } from './core'
import { createStubCore } from './stubCore'

export type { CoreClient, CoreError, CoreEvent, CoreMethod, CoreStatus } from './core'
export { CodeElevationCancelled, CodeStartFailed, isCoreError } from './core'

export interface AppInfo {
  version: string
  dataDir: string
}

const inTauri = typeof window !== 'undefined' && '__TAURI_INTERNALS__' in window

// Outside Tauri (Vite in a browser, Playwright, Vitest) the shell answers with fixed values.
const stubInfo: AppInfo = { version: 'dev', dataDir: 'browser' }

export function appInfo(): Promise<AppInfo> {
  return inTauri ? invoke<AppInfo>('app_info') : Promise.resolve(stubInfo)
}

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

/** The program as a whole: show or hide its window, quit. */
export const appControls = {
  show: (): Promise<void> => (inTauri ? invoke('app_show') : Promise.resolve()),
  hide: (): Promise<void> => (inTauri ? invoke('app_hide') : Promise.resolve()),
  exit: (): Promise<void> => (inTauri ? invoke('app_exit') : Promise.resolve()),
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
