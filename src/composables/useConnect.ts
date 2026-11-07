import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

import { CodeElevationCancelled, isCoreError } from '@/platform'
import {
  ConnectionIdle,
  ConnectionRunning,
  KillSwitchAlways,
  KillSwitchOff,
  MethodConnectionStart,
  MethodConnectionStop,
  ReasonKillSwitchNeedsElevation,
  ReasonNeedsElevation,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const modesAskedKey = 'poros.modesAsked'

function modesAsked(): boolean {
  try {
    return localStorage.getItem(modesAskedKey) === '1'
  } catch {
    return false
  }
}

function rememberModesAsked(): void {
  try {
    localStorage.setItem(modesAskedKey, '1')
  } catch {
    // Without storage the question comes again next time; nothing breaks.
  }
}

/** What the core needs administrator rights for. */
export type RightsFor = 'tun' | 'killSwitch'

/**
 * Start, stop and the two switches, with what the mockups put around them: the question on the first Start, the
 * UAC request for TUN and the kill switch, and the banners when they stay without rights. The window keeps no
 * rights of its own: only the core is restarted with them.
 */
export const useConnect = defineStore('connect', () => {
  const core = useCoreStore()
  const modesOpen = ref(false)
  const adminOpen = ref(false)
  const adminFor = ref<RightsFor>('tun')
  // The connection starts once the rights come, or without them on "Try without rights".
  const adminThenConnect = ref(true)
  const tunFailed = ref(false)
  const killSwitchFailed = ref(false)

  watch(
    () => core.connection,
    (state) => {
      if (state.error === ReasonNeedsElevation) tunFailed.value = true
      if (state.state === ConnectionRunning) tunFailed.value = false
    },
  )
  watch(
    () => core.killSwitch,
    (block) => {
      if (block.error === ReasonKillSwitchNeedsElevation) killSwitchFailed.value = true
      if (block.on) killSwitchFailed.value = false
    },
  )
  watch(
    () => core.settings?.killSwitch,
    (mode) => {
      if (mode === KillSwitchOff) killSwitchFailed.value = false
    },
  )

  /** What a start lacks rights for: TUN first, since it can fall back to System. */
  function rightsNeeded(): RightsFor | null {
    const settings = core.settings
    if (!settings || core.status.elevated) return null
    if (settings.tun) return 'tun'
    if (settings.killSwitch !== KillSwitchOff) return 'killSwitch'
    return null
  }

  function askRights(reason: RightsFor, thenConnect: boolean): void {
    adminFor.value = reason
    adminThenConnect.value = thenConnect
    adminOpen.value = true
  }

  async function connect(): Promise<void> {
    await core.core().call(MethodConnectionStart)
  }

  /** The Start button: asks what to turn on the first time, then for rights when TUN or the kill switch needs them. */
  async function start(): Promise<void> {
    const settings = core.settings
    if (!settings) return
    if (!settings.tun && !settings.systemProxy && !modesAsked()) {
      modesOpen.value = true
      return
    }
    const reason = rightsNeeded()
    if (reason) {
      askRights(reason, true)
      return
    }
    await connect()
  }

  /**
   * At launch: connects when the settings ask for it, and asks for rights right away when the connection needs
   * them or the kill switch blocks from the launch. Those were the user's choices, so the UAC prompt comes instead
   * of the question; a refusal leaves the banner.
   */
  async function autoStart(): Promise<void> {
    const settings = core.settings
    if (!settings?.onboardingDone) return
    const wantsConnection =
      settings.autoConnect &&
      !!settings.server &&
      core.connection.state === ConnectionIdle &&
      (settings.tun || settings.systemProxy || modesAsked())
    const reason = wantsConnection ? rightsNeeded() : null
    const blockFromLaunch = settings.killSwitch === KillSwitchAlways && !core.status.elevated
    if (reason || blockFromLaunch) {
      await elevate(wantsConnection, reason ?? 'killSwitch')
      return
    }
    if (wantsConnection) await connect()
  }

  async function stop(): Promise<void> {
    await core.core().call(MethodConnectionStop)
  }

  async function chooseModes(tun: boolean, systemProxy: boolean): Promise<void> {
    rememberModesAsked()
    modesOpen.value = false
    await core.updateSettings({ tun, systemProxy })
    await start()
  }

  function readyWithRights(): Promise<void> {
    return new Promise((resolve, reject) => {
      const timer = setTimeout(() => {
        stopWatching()
        reject(new Error('the core with administrator rights did not start'))
      }, 60_000)
      const stopWatching = watch(
        () => core.status,
        (status) => {
          if (status.state === 'ready' && status.elevated && core.loaded) {
            clearTimeout(timer)
            stopWatching()
            resolve()
          }
        },
        { immediate: true, deep: true },
      )
    })
  }

  /** Restarts the core with rights after the UAC prompt and connects if asked to; a refusal leaves the banner. */
  async function elevate(thenConnect: boolean, reason: RightsFor): Promise<void> {
    adminOpen.value = false
    try {
      await core.core().elevate()
      await readyWithRights()
      tunFailed.value = false
      killSwitchFailed.value = false
      if (thenConnect) await connect()
    } catch (error) {
      if (reason === 'tun') tunFailed.value = true
      else killSwitchFailed.value = true
      if (!(isCoreError(error) && error.code === CodeElevationCancelled)) throw error
    }
  }

  /** The admin dialog's restart: the rights it was opened for, then the connection it was waiting for. */
  async function elevateFromDialog(): Promise<void> {
    await elevate(adminThenConnect.value, adminFor.value)
  }

  /** The TUN banner's restart: rights, then the connection. */
  async function elevateAndConnect(): Promise<void> {
    await elevate(true, 'tun')
  }

  /** The kill switch banner's restart: the block comes with the new core; a running connection starts again. */
  async function elevateForKillSwitch(): Promise<void> {
    await elevate(core.running, 'killSwitch')
  }

  async function tryWithoutRights(): Promise<void> {
    adminOpen.value = false
    if (adminThenConnect.value) await connect()
  }

  /** System works without rights: TUN goes off and the connection starts again with System. */
  async function useSystemInstead(): Promise<void> {
    adminOpen.value = false
    tunFailed.value = false
    await core.updateSettings({ tun: false, systemProxy: true })
    await connect()
  }

  /** The TUN switch: a running connection restarts with it, asking for rights first when they are missing. */
  async function setTun(on: boolean): Promise<void> {
    await core.updateSettings({ tun: on })
    if (core.connection.state !== ConnectionRunning) return
    if (on && !core.status.elevated) {
      askRights('tun', true)
      return
    }
    await connect()
  }

  /**
   * The kill switch mode: the core applies it at once. Without rights the dialog asks for them; the new core
   * comes up without the connection, so a running one starts again.
   */
  async function setKillSwitch(mode: string): Promise<void> {
    await core.updateSettings({ killSwitch: mode })
    if (mode !== KillSwitchOff && !core.status.elevated) askRights('killSwitch', core.running)
  }

  /** The System switch: the core applies it to a running connection by itself. */
  async function setSystemProxy(on: boolean): Promise<void> {
    await core.updateSettings({ systemProxy: on })
  }

  /** Another server: a running connection moves to it. */
  async function chooseServer(id: string): Promise<void> {
    if (core.settings?.server === id) return
    await core.updateSettings({ server: id })
    if (core.connection.state === ConnectionRunning) await connect()
  }

  return {
    modesOpen,
    adminOpen,
    adminFor,
    tunFailed,
    killSwitchFailed,
    start,
    autoStart,
    stop,
    chooseModes,
    elevateFromDialog,
    elevateAndConnect,
    elevateForKillSwitch,
    tryWithoutRights,
    useSystemInstead,
    setTun,
    setKillSwitch,
    setSystemProxy,
    chooseServer,
    connect,
  }
})
