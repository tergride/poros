import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { type CoreClient, type CoreStatus, useCore } from '@/platform'
import type {
  ConnectionState,
  KillSwitchState,
  NetworkState,
  Preset,
  Server,
  Settings,
  Subscription,
  Traffic,
} from '@/platform/core-types'
import {
  ConnectionIdle,
  ConnectionRunning,
  EventConnectionChanged,
  EventConnectionTraffic,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersTested,
  EventSettingsChanged,
  EventSubscriptionsChanged,
  MethodConnectionState,
  MethodKillSwitchState,
  MethodNetworkState,
  MethodPresetsList,
  MethodServersList,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsList,
} from '@/platform/core-types'

const idle: ConnectionState = {
  state: ConnectionIdle,
  serverId: '',
  tun: false,
  systemProxy: false,
  since: 0,
  error: '',
  errorText: '',
}

const noAddress = { ip: '', countryCode: '', city: '', isp: '', asn: 0 }

const noBlock: KillSwitchState = { on: false, error: '', errorText: '' }

const noCheck: NetworkState = {
  checking: false,
  direct: noAddress,
  proxy: noAddress,
  dns: [],
  leak: '',
  checkedAt: 0,
  error: '',
  errorText: '',
}

/**
 * What the core holds, kept current by its events: the interface reads from here and changes things through the
 * core, never in its own copy.
 */
export const useCoreStore = defineStore('core', () => {
  const client = shallowRef<CoreClient | null>(null)
  const status = ref<CoreStatus>({ state: 'starting', elevated: false })
  const settings = ref<Settings | null>(null)
  const subscriptions = ref<Subscription[]>([])
  const servers = ref<Server[]>([])
  const presets = ref<Preset[]>([])
  const connection = ref<ConnectionState>({ ...idle })
  const network = ref<NetworkState>({ ...noCheck })
  const killSwitch = ref<KillSwitchState>({ ...noBlock })
  const traffic = ref<Traffic | null>(null)
  const testing = ref(false)
  const loaded = ref(false)

  const running = computed(() => connection.value.state === ConnectionRunning)
  const server = computed(() => servers.value.find((s) => s.id === settings.value?.server) ?? null)
  const preset = computed(() => presets.value.find((p) => p.id === settings.value?.preset) ?? null)

  function core(): CoreClient {
    if (!client.value) throw new Error('the core store is not started')
    return client.value
  }

  async function loadServers(): Promise<void> {
    servers.value = await core().call(MethodServersList, { subscriptionId: '' })
  }

  // Reads everything again: at the start and whenever a new core takes over.
  async function load(): Promise<void> {
    const c = core()
    const [s, subs, list, presetList, conn, net, block] = await Promise.all([
      c.call(MethodSettingsGet),
      c.call(MethodSubscriptionsList),
      c.call(MethodServersList, { subscriptionId: '' }),
      c.call(MethodPresetsList),
      c.call(MethodConnectionState),
      c.call(MethodNetworkState),
      c.call(MethodKillSwitchState),
    ])
    settings.value = s
    subscriptions.value = subs
    servers.value = list
    presets.value = presetList
    connection.value = conn
    network.value = net
    killSwitch.value = block
    loaded.value = true
  }

  let started: Promise<void> | undefined

  /** Connects to the core once; later calls wait for the same start. */
  function start(): Promise<void> {
    started ??= (async () => {
      const c = await useCore()
      client.value = c
      c.on(EventSettingsChanged, (next) => (settings.value = next))
      c.on(EventSubscriptionsChanged, (next) => (subscriptions.value = next))
      c.on(EventServersChanged, () => void loadServers())
      c.on(EventPresetsChanged, (next) => (presets.value = next))
      c.on(EventConnectionChanged, (next) => {
        connection.value = next
        if (next.state !== ConnectionRunning) traffic.value = null
      })
      c.on(EventConnectionTraffic, (next) => (traffic.value = next))
      c.on(EventNetworkChanged, (next) => (network.value = next))
      c.on(EventKillSwitchChanged, (next) => (killSwitch.value = next))
      c.on(EventServersTested, (report) => {
        const results = new Map(report.results.map((r) => [r.serverId, r.latency]))
        servers.value = servers.value.map((s) => (results.has(s.id) ? { ...s, latency: results.get(s.id)! } : s))
        if (report.done) testing.value = false
      })
      c.onStatus((next) => {
        const wasReady = status.value.state === 'ready'
        status.value = next
        if (next.state === 'ready' && !wasReady) void load()
      })
      status.value = await c.status()
      if (status.value.state === 'ready') await load()
    })()
    return started
  }

  /** Changes some settings; the core answers with all of them. */
  async function updateSettings(patch: Partial<Settings>): Promise<Settings> {
    if (!settings.value) throw new Error('the settings are not loaded yet')
    settings.value = await core().call(MethodSettingsSet, { ...settings.value, ...patch })
    return settings.value
  }

  async function testServers(subscriptionId = ''): Promise<void> {
    testing.value = true
    try {
      await core().call(MethodServersTest, { subscriptionId })
    } catch (error) {
      testing.value = false
      throw error
    }
  }

  return {
    client,
    status,
    settings,
    subscriptions,
    servers,
    presets,
    connection,
    network,
    killSwitch,
    traffic,
    testing,
    loaded,
    running,
    server,
    preset,
    core,
    start,
    load,
    updateSettings,
    testServers,
  }
})
