import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'

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

export const windowControls = {
  minimize: (): Promise<void> => (inTauri ? getCurrentWindow().minimize() : Promise.resolve()),
  toggleMaximize: (): Promise<void> => (inTauri ? getCurrentWindow().toggleMaximize() : Promise.resolve()),
  close: (): Promise<void> => (inTauri ? getCurrentWindow().close() : Promise.resolve()),
}
