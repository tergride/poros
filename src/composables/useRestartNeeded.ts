import { defineStore } from 'pinia'
import { computed, ref, watch } from 'vue'

import type { Settings } from '@/platform/core-types'
import { ConnectionRunning, ConnectionStarting } from '@/platform/core-types'
import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

// The settings a running connection was built with; System and the kill switch apply on the fly, and the server
// restarts by itself.
const builtWith = (s: Settings) =>
  JSON.stringify([
    s.tun,
    s.tunMtu,
    s.tunStrictRoute,
    s.tunRouting,
    s.tunIpv4,
    s.tunIpv6,
    s.tunBypass,
    s.preset,
    s.allowLan,
    s.proxyPort,
    s.proxyAuth,
    s.proxyUsername,
    s.proxyPassword,
    s.ipv6,
    s.realityOnXray,
    s.dnsRemote,
    s.dnsDirect,
    s.dnsRouting,
    s.fakeIp,
    s.dnsHosts,
    s.dnsAnswers,
    s.dnsCache,
    s.dnsCacheSize,
    s.dnsIgnoreTtl,
    s.dnsOptimistic,
    s.dnsTimeout,
    s.dnsClientSubnet,
    s.dnsObject,
    s.addressFirst,
    s.warp,
    s.warpMode,
    s.warpWireGuard,
    s.warpMasque,
    s.muxDefault,
    s.muxProtocol,
    s.muxConcurrency,
    s.fragmentDefault,
    s.fingerprintDefault,
    s.spoofDefault,
    s.spoofSni,
    s.spoofMethod,
    s.quicIdleTimeout,
    s.quicKeepAlive,
    s.quicStreamWindow,
    s.quicConnectionWindow,
    s.quicMaxStreams,
    s.quicInitialPacketSize,
    s.quicDisablePmtud,
    s.ntp,
    s.ntpServer,
    s.ntpPort,
    s.ntpInterval,
    s.ntpOutbound,
    s.xraySystemCerts,
    s.insecureDefault,
    s.stats,
    s.trafficHistory,
  ])

/**
 * Note 6: a saved setting the running connection depends on waits for a restart instead of breaking the connection.
 * A chip in the title bar says so and opens a dialog to restart; it stays until the connection restarts or stops.
 */
export const useRestartNeeded = defineStore('restartNeeded', () => {
  const core = useCoreStore()
  const connect = useConnect()
  const applied = ref('')
  // A preset, a server or an endpoint saved while the connection runs on it changes the connection without changing
  // the settings. Of presets, those saved since the start read the connection's count, whichever is chosen now.
  const savedPresets = ref<string[]>([])
  const presetEdited = computed(() => savedPresets.value.includes(core.connection.preset ?? ''))
  const serverEdited = ref(false)
  const endpointEdited = ref(false)
  const dialogOpen = ref(false)

  watch(
    () => core.connection.state === ConnectionRunning,
    (running) => {
      if (running && core.settings) {
        applied.value = builtWith({ ...core.settings, preset: core.connection.preset ?? core.settings.preset })
        serverEdited.value = false
        endpointEdited.value = false
      }
    },
    { immediate: true },
  )

  // Synchronously: a quick start tells its preset and comes up within one tick.
  watch(
    () => core.connection,
    (now) => {
      if (now.state === ConnectionStarting && now.preset) savedPresets.value = []
    },
    { flush: 'sync' },
  )

  const needed = computed(
    () => core.running && !!core.settings && (builtWith(core.settings) !== applied.value || presetEdited.value || serverEdited.value || endpointEdited.value),
  )

  // The dialog has nothing to ask about once the connection stopped or took the settings. Synchronously: a change
  // and its undoing within one tick must not leave the dialog open.
  watch(
    needed,
    (now) => {
      if (!now) dialogOpen.value = false
    },
    { flush: 'sync' },
  )

  async function apply(): Promise<void> {
    dialogOpen.value = false
    await connect.connect()
  }

  /** Tells whether a connection, up or coming up, is built of the preset. */
  function presetInUse(id: string): boolean {
    return (core.running || core.connection.state === ConnectionStarting) && core.connection.preset === id
  }

  /** Notes a saved preset: the connection built of it before waits for a restart. */
  function presetSaved(id: string): void {
    savedPresets.value.push(id)
  }

  /** Marks a saved server, if the running connection goes through it. */
  function serverSaved(id: string): void {
    if (core.running && (core.connection.servers ?? [core.connection.serverId]).includes(id)) serverEdited.value = true
  }

  /** Marks a changed endpoint, if the running connection runs with it or it is on now. */
  function endpointChanged(id: string, on: boolean): void {
    if (core.running && (on || (core.connection.endpoints ?? []).includes(id))) endpointEdited.value = true
  }

  return { needed, serverEdited, endpointEdited, dialogOpen, apply, presetInUse, presetSaved, serverSaved, endpointChanged }
})
