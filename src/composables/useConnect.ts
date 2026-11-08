import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

import { CodeElevationCancelled, appControls, isCoreError } from '@/platform'
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
  // The modes were chosen in this run; they count for the next launches once a connection came of them, so that a
  // start that failed does not make the next launch connect on its own.
  const modesChosen = ref(false)
  // The kill switch mode before a change that waits for rights: Cancel in the dialog puts it back.
  const killSwitchBefore = ref<string | null>(null)

  watch(
    () => core.connection,
    (state) => {
      if (state.error === ReasonNeedsElevation) tunFailed.value = true
      if (state.state === ConnectionRunning) {
        tunFailed.value = false
        if (modesChosen.value) rememberModesAsked()
      }
    },
  )
  watch(
    () => core.killSwitch,
    (block) => {
      if (block.error === ReasonKillSwitchNeedsElevation) killSwitchFailed.value = true
      else if (block.on && !block.error) killSwitchFailed.value = false
    },
  )
  watch(
    () => core.settings?.killSwitch,
    (mode) => {
      if (mode === KillSwitchOff) killSwitchFailed.value = false
    },
  )

  /** What a start lacks rights for: only TUN, which the kill switch needs them for too. */
  function rightsNeeded(): RightsFor | null {
    const settings = core.settings
    if (!settings || core.status.elevated) return null
    return settings.tun ? 'tun' : null
  }

  /** The kill switch needs rights of its own only in Always with TUN: it keeps the TUN up without a connection. */
  function killSwitchNeedsRights(mode: string): boolean {
    return mode === KillSwitchAlways && !!core.settings?.tun && !core.status.elevated
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
    if (!settings.tun && !settings.systemProxy && !modesChosen.value && !modesAsked()) {
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
   * At launch: connects when the settings ask for it or the run before an update was connected, and asks for
   * rights right away when the connection needs them, the kill switch keeps the TUN from the launch or Poros runs
   * with rights. Those were the user's choices, so the UAC prompt comes instead of the question; a refusal leaves the
   * banner, and a connection that needs no rights starts anyway.
   */
  async function autoStart(): Promise<void> {
    const reconnect = await appControls.takeReconnect()
    const settings = core.settings
    if (!settings?.onboardingDone) return
    const wantsConnection =
      (settings.autoConnect || reconnect) &&
      !!settings.server &&
      core.connection.state === ConnectionIdle &&
      (settings.tun || settings.systemProxy || modesAsked())
    const reason = wantsConnection ? rightsNeeded() : null
    const blockFromLaunch = killSwitchNeedsRights(settings.killSwitch)
    const withRights = settings.runAsAdmin && !core.status.elevated
    if (reason || blockFromLaunch || withRights) {
      await elevate(wantsConnection, reason ?? (blockFromLaunch ? 'killSwitch' : null))
      return
    }
    if (wantsConnection) await connect()
  }

  async function stop(): Promise<void> {
    await core.core().call(MethodConnectionStop)
  }

  async function chooseModes(tun: boolean, systemProxy: boolean): Promise<void> {
    modesChosen.value = true
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

  /** Restarts the core with rights after the UAC prompt; false when the user declined, and the old core runs on. */
  async function gainRights(): Promise<boolean> {
    try {
      await core.core().elevate()
      await readyWithRights()
    } catch (error) {
      if (isCoreError(error) && error.code === CodeElevationCancelled) return false
      throw error
    }
    tunFailed.value = false
    killSwitchFailed.value = false
    return true
  }

  function failedWithout(reason: RightsFor | null): void {
    if (reason === 'tun') tunFailed.value = true
    if (reason === 'killSwitch') killSwitchFailed.value = true
  }

  /**
   * Restarts the core with rights after the UAC prompt and connects if asked to. A refusal leaves the banner of
   * what needed them; rights that were only wanted, `reason` null, leave a connection that runs without them.
   */
  async function elevate(thenConnect: boolean, reason: RightsFor | null): Promise<void> {
    killSwitchBefore.value = null
    adminOpen.value = false
    let gained: boolean
    try {
      gained = await gainRights()
    } catch (error) {
      failedWithout(reason)
      throw error
    }
    if (!gained) {
      failedWithout(reason)
      if (reason) return
    }
    if (thenConnect) await connect()
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

  /**
   * Rights at sign-in: the sign-in task is made by a core with rights, so turning them on asks for rights first,
   * unless the core has them. A refusal leaves the switch off; a running connection starts again in the new core.
   */
  async function setAdminAutostart(on: boolean): Promise<void> {
    if (on && !core.status.elevated) {
      const running = core.running
      if (!(await gainRights())) return
      if (running) await connect()
    }
    await core.updateSettings({ adminAutostart: on })
  }

  async function tryWithoutRights(): Promise<void> {
    killSwitchBefore.value = null
    adminOpen.value = false
    if (adminThenConnect.value) await connect()
  }

  /** System works without rights: TUN goes off and the connection starts again with System. */
  async function useSystemInstead(): Promise<void> {
    killSwitchBefore.value = null
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
   * The kill switch mode: the core applies it at once. When it needs rights, the dialog asks for them; the new core
   * comes up without the connection, so a running one starts again.
   */
  async function setKillSwitch(mode: string): Promise<void> {
    const before = core.settings?.killSwitch ?? KillSwitchOff
    await core.updateSettings({ killSwitch: mode })
    if (killSwitchNeedsRights(mode)) {
      killSwitchBefore.value = before
      askRights('killSwitch', core.running)
    }
  }

  /**
   * The rights dialog closed without an answer, by Cancel, Esc or a click beside it: a kill switch mode chosen just
   * before goes back, since without rights it would only stay as a banner. TUN keeps its switch, as its banner
   * offers System instead.
   */
  watch(adminOpen, (open) => {
    const before = killSwitchBefore.value
    if (open || before === null) return
    killSwitchBefore.value = null
    void core.updateSettings({ killSwitch: before }).catch(() => undefined)
  })

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
    setAdminAutostart,
    setSystemProxy,
    chooseServer,
    connect,
  }
})
