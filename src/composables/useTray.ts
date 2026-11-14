import { watchDebounced } from '@vueuse/core'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { type TrayActions, type TrayState, type TrayWords, presetMenu, statusText, trayFallback, trayMenu } from '@/lib/trayMenu'
import { useAttempt } from '@/composables/useAttempt'
import { appControls, hotkeys, popupMenu, setTray } from '@/platform'
import { ConnectionIdle, KillSwitchConnected, MethodOTPCode, type OTPCode, type Preset } from '@/platform/core-types'
import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

/**
 * Keeps the tray menu in step with the core; actions that need a question show the window first. Until the core has
 * given its data, and when it never does, the menu still shows the window and quits. The shortcuts of Windows act as
 * its items: the shell shows the window itself, the rest comes here.
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
    otp: () => t('tray.otp'),
    otpHint: () => t('tray.otpHint'),
    otpName: (code: OTPCode) => [code.name, code.account].filter(Boolean).join(' · ') || t('adv.tools.unnamed'),
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
    choosePreset: (id) => void withWindow(() => connect.choosePreset(id)),
    showPresets: () => void showAt('/settings/presets'),
    // The code of the moment, or the next HOTP one; the shell writes the clipboard, as the window may be hidden.
    copyOtp: (id) =>
      void withWindow(async () => {
        const code = await core.core().call(MethodOTPCode, { id })
        await appControls.copy(code.code)
      }),
    setTun: (on) => void withWindow(() => connect.setTun(on)),
    setSystemProxy: (on) => void withWindow(() => connect.setSystemProxy(on)),
    show: () => void appControls.show(),
    exit: () => void appControls.exit(),
  }

  const trayState = (): TrayState | null =>
    core.loaded && core.settings
      ? {
          connection: core.connection,
          settings: core.settings,
          servers: core.servers,
          presets: core.presets,
          otp: core.otp,
          blocked: core.killSwitch.on,
          full: core.killSwitch.full,
          coreDown: !core.ready,
        }
      : null

  void hotkeys.onAction((action) => {
    const state = trayState()
    if (!state || state.coreDown) return
    const { settings, connection } = state
    switch (action) {
      case 'presets':
        void popupMenu(presetMenu(state, actions, words)).catch(() => undefined)
        break
      case 'system':
        actions.setSystemProxy(!settings.systemProxy)
        break
      case 'tun':
        actions.setTun(!settings.tun)
        break
      case 'start': {
        // As the first item of the tray: Stop while a connection runs or the kill switch holds after a lost one.
        const held = !!state.blocked && connection.state === ConnectionIdle && settings.killSwitch === KillSwitchConnected
        if (held || connection.state !== ConnectionIdle) actions.disconnect()
        else if (settings.server) actions.connect()
        break
      }
    }
  })

  watchDebounced(
    () => [core.loaded, core.settings, core.connection, core.servers, core.presets, core.otp, core.killSwitch, core.status, locale.value],
    () => {
      const state = trayState()
      if (!state) {
        void setTray(trayFallback(core.status.state === 'down', actions, words), 'Poros').catch(() => undefined)
        return
      }
      void setTray(trayMenu(state, actions, words), `Poros · ${statusText(state, words)}`).catch(() => undefined)
    },
    { debounce: 150, immediate: true, deep: true },
  )
}
