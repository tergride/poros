import { invoke } from '@tauri-apps/api/core'
import { getCurrentWindow } from '@tauri-apps/api/window'

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

export const windowControls = {
  minimize: (): Promise<void> => (inTauri ? getCurrentWindow().minimize() : Promise.resolve()),
  toggleMaximize: (): Promise<void> => (inTauri ? getCurrentWindow().toggleMaximize() : Promise.resolve()),
  close: (): Promise<void> => (inTauri ? getCurrentWindow().close() : Promise.resolve()),
}
