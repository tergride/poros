import { computed } from 'vue'
import type { Router } from 'vue-router'
import { useRouter } from 'vue-router'

import { useCoreStore } from '@/stores/core'

export type UiMode = 'simple' | 'advanced'

/** The settings pages each mode has, by the id in its routes. */
const simpleSettings = ['presets', 'conn', 'subs', 'start', 'look', 'help']
export const advancedSettings = ['conn', 'subs', 'start', 'look', 'help']

/**
 * The screen of a mode that stands for path: the same settings page, the profiles for the servers. The wizard and the
 * editor of a preset belong to both.
 */
export function routeInMode(mode: UiMode, path: string): string {
  if (path.startsWith('/wizard') || path.startsWith('/preset/') || path === '/') return path
  if (mode === 'advanced') {
    if (path.startsWith('/adv')) return path
    const page = /^\/settings\/([^/]+)/.exec(path)?.[1]
    if (page === 'presets') return '/adv/routing'
    if (page) return `/adv/settings/${advancedSettings.includes(page) ? page : 'conn'}`
    return '/adv/profiles'
  }
  if (!path.startsWith('/adv')) return path
  if (path.startsWith('/adv/routing')) return '/settings/presets'
  const page = /^\/adv\/settings\/([^/]+)/.exec(path)?.[1]
  if (page) return `/settings/${simpleSettings.includes(page) ? page : 'conn'}`
  return '/main'
}

// The mode a switch is on its way to: the screen of the new mode opens before the setting changes, so that a page
// with unsaved changes can still keep the user on it.
let pendingMode: UiMode | null = null

/** Keeps every screen in the mode of the settings: a link or the tray asking for a screen of the other mode gets its counterpart. */
export function installModeGuard(router: Router): void {
  router.beforeEach((to) => {
    const core = useCoreStore()
    if (!core.settings) return true
    const mode: UiMode = pendingMode ?? (core.settings.uiMode === 'advanced' ? 'advanced' : 'simple')
    const path = routeInMode(mode, to.path)
    return path === to.path ? true : path
  })
}

/** The mode of the interface: changing it keeps the connection and the screen, as far as the other mode has it. */
export function useUiMode() {
  const core = useCoreStore()
  const router = useRouter()
  const mode = computed<UiMode>(() => (core.settings?.uiMode === 'advanced' ? 'advanced' : 'simple'))

  async function switchTo(next: UiMode): Promise<void> {
    if (next === mode.value) return
    pendingMode = next
    try {
      const failure = await router.push(routeInMode(next, router.currentRoute.value.path))
      if (failure) return
      await core.updateSettings({ uiMode: next })
    } finally {
      pendingMode = null
    }
  }

  return { mode, switchTo }
}
