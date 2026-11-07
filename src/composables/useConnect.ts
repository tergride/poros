import { defineStore } from 'pinia'
import { ref, watch } from 'vue'

import { CodeElevationCancelled, isCoreError } from '@/platform'
import { ConnectionRunning, MethodConnectionStart, MethodConnectionStop, ReasonNeedsElevation } from '@/platform/core-types'
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

/**
 * Start, stop and the two switches, with what the mockups put around them: the question on the first Start, the
 * UAC request for TUN and the banner when TUN stays without rights. The window keeps no rights of its own: only the
 * core is restarted with them.
 */
export const useConnect = defineStore('connect', () => {
  const core = useCoreStore()
  const modesOpen = ref(false)
  const adminOpen = ref(false)
  const tunFailed = ref(false)

  watch(
    () => core.connection,
    (state) => {
      if (state.error === ReasonNeedsElevation) tunFailed.value = true
      if (state.state === ConnectionRunning) tunFailed.value = false
    },
  )

  async function connect(): Promise<void> {
    await core.core().call(MethodConnectionStart)
  }

  /** The Start button: asks what to turn on the first time, then for rights when TUN needs them. */
  async function start(): Promise<void> {
    const settings = core.settings
    if (!settings) return
    if (!settings.tun && !settings.systemProxy && !modesAsked()) {
      modesOpen.value = true
      return
    }
    if (settings.tun && !core.status.elevated) {
      adminOpen.value = true
      return
    }
    await connect()
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

  /** Restarts the core with rights after the UAC prompt and connects; a refusal leaves the banner. */
  async function elevateAndConnect(): Promise<void> {
    adminOpen.value = false
    try {
      await core.core().elevate()
      await readyWithRights()
      tunFailed.value = false
      await connect()
    } catch (error) {
      tunFailed.value = true
      if (!(isCoreError(error) && error.code === CodeElevationCancelled)) throw error
    }
  }

  async function tryWithoutRights(): Promise<void> {
    adminOpen.value = false
    await connect()
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
      adminOpen.value = true
      return
    }
    await connect()
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
    tunFailed,
    start,
    stop,
    chooseModes,
    elevateAndConnect,
    tryWithoutRights,
    useSystemInstead,
    setTun,
    setSystemProxy,
    chooseServer,
    connect,
  }
})
