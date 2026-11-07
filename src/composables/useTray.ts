import { watchDebounced } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { type TrayActions, type TrayWords, statusText, trayMenu } from '@/lib/trayMenu'
import { appControls, setTray } from '@/platform'
import type { Preset } from '@/platform/core-types'
import { ConnectionRunning } from '@/platform/core-types'
import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

/** Keeps the tray menu in step with the core; actions that need a question show the window first. */
export function useTray(): void {
  const core = useCoreStore()
  const connect = useConnect()
  const router = useRouter()
  const { t, te, locale } = useI18n()

  const presetName = (preset: Preset) =>
    preset.builtin && te(`presets.name.${preset.id}`) ? t(`presets.name.${preset.id}`) : preset.name

  const words: TrayWords = {
    connected: (server) => t('tray.connected', { server }),
    connecting: () => t('tray.connecting'),
    disconnected: () => t('tray.disconnected'),
    connect: () => t('tray.connect'),
    disconnect: () => t('tray.disconnect'),
    route: (preset) => t('tray.route', { preset }),
    presets: () => t('tray.presets'),
    show: () => t('tray.show'),
    exit: () => t('tray.exit'),
    presetName,
  }

  // A dialog that opens in a hidden window would wait unseen.
  async function withWindow(action: () => Promise<void>): Promise<void> {
    await action()
    if (connect.modesOpen || connect.adminOpen) await appControls.show()
  }

  async function showAt(path: string): Promise<void> {
    await router.push(path)
    await appControls.show()
  }

  const actions: TrayActions = {
    connect: () => void withWindow(connect.start),
    disconnect: () => void connect.stop(),
    choosePreset: (id) =>
      void (async () => {
        await core.updateSettings({ preset: id })
        if (core.connection.state === ConnectionRunning) await connect.connect()
      })(),
    showPresets: () => void showAt('/settings/presets'),
    setTun: (on) => void withWindow(() => connect.setTun(on)),
    setSystemProxy: (on) => void connect.setSystemProxy(on),
    show: () => void appControls.show(),
    exit: () => void appControls.exit(),
  }

  watchDebounced(
    () => [core.loaded, core.settings, core.connection, core.servers, core.presets, locale.value],
    () => {
      const settings = core.settings
      if (!core.loaded || !settings) return
      const state = {
        connection: core.connection,
        settings,
        servers: core.servers,
        presets: core.presets,
      }
      void setTray(trayMenu(state, actions, words), `Poros · ${statusText(state, words)}`)
    },
    { debounce: 150, immediate: true },
  )
}
