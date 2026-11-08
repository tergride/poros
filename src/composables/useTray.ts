import { watchDebounced } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { type TrayActions, type TrayWords, statusText, trayFallback, trayMenu } from '@/lib/trayMenu'
import { useAttempt } from '@/composables/useAttempt'
import { appControls, setTray } from '@/platform'
import type { Preset } from '@/platform/core-types'
import { ConnectionRunning } from '@/platform/core-types'
import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

/**
 * Keeps the tray menu in step with the core; actions that need a question show the window first. Until the core has
 * given its data, and when it never does, the menu still shows the window and quits.
 */
export function useTray(): void {
  const core = useCoreStore()
  const connect = useConnect()
  const router = useRouter()
  const attempt = useAttempt()
  const { t, te, locale } = useI18n()

  const presetName = (preset: Preset) =>
    preset.builtin && te(`presets.name.${preset.id}`) ? t(`presets.name.${preset.id}`) : preset.name

  const words: TrayWords = {
    connected: (server) => t('tray.connected', { server }),
    connecting: () => t('tray.connecting'),
    disconnected: () => t('tray.disconnected'),
    blocked: (full) => t(full ? 'tray.blockedFull' : 'tray.blocked'),
    blockedAlways: (full) => t(full ? 'tray.blockedAlwaysFull' : 'tray.blockedAlways'),
    coreDown: () => t('tray.coreDown'),
    starting: () => t('boot.starting'),
    connect: () => t('tray.connect'),
    disconnect: () => t('tray.disconnect'),
    stop: () => t('tray.stop'),
    route: (preset) => t('tray.route', { preset }),
    presets: () => t('tray.presets'),
    show: () => t('tray.show'),
    exit: () => t('tray.exit'),
    presetName,
  }

  // A dialog that opens in a hidden window would wait unseen; so would a failure told only in the window.
  async function withWindow(action: () => Promise<void>): Promise<void> {
    await attempt(action)
    if (connect.modesOpen || connect.adminOpen || core.ready === false) await appControls.show()
  }

  async function showAt(path: string): Promise<void> {
    await router.push(path)
    await appControls.show()
  }

  const actions: TrayActions = {
    connect: () => void withWindow(connect.start),
    disconnect: () => void withWindow(connect.stop),
    choosePreset: (id) =>
      void withWindow(async () => {
        await core.updateSettings({ preset: id })
        if (core.connection.state === ConnectionRunning) await connect.connect()
      }),
    showPresets: () => void showAt('/settings/presets'),
    setTun: (on) => void withWindow(() => connect.setTun(on)),
    setSystemProxy: (on) => void withWindow(() => connect.setSystemProxy(on)),
    show: () => void appControls.show(),
    exit: () => void appControls.exit(),
  }

  watchDebounced(
    () => [core.loaded, core.settings, core.connection, core.servers, core.presets, core.killSwitch, core.status, locale.value],
    () => {
      const settings = core.settings
      if (!core.loaded || !settings) {
        void setTray(trayFallback(core.status.state === 'down', actions, words), 'Poros').catch(() => undefined)
        return
      }
      const state = {
        connection: core.connection,
        settings,
        servers: core.servers,
        presets: core.presets,
        blocked: core.killSwitch.on,
        full: core.killSwitch.full,
        coreDown: !core.ready,
      }
      void setTray(trayMenu(state, actions, words), `Poros · ${statusText(state, words)}`).catch(() => undefined)
    },
    { debounce: 150, immediate: true, deep: true },
  )
}
