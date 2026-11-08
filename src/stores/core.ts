import { defineStore } from 'pinia'
import { computed, ref, shallowRef } from 'vue'

import { type CoreClient, type CoreStatus, useCore } from '@/platform'
import type {
  ConnectionState,
  Group,
  KillSwitchState,
  NetworkState,
  Preset,
  Server,
  Settings,
  Traffic,
  UpdateState,
} from '@/platform/core-types'
import {
  ConnectionIdle,
  ConnectionRunning,
  EventConnectionChanged,
  EventConnectionTraffic,
  EventGroupsChanged,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersIPTested,
  EventServersSpeedTested,
  EventServersTested,
  EventSettingsChanged,
  EventUpdatesChanged,
  MethodConnectionState,
  MethodGroupsList,
  MethodKillSwitchState,
  MethodNetworkState,
  MethodPresetsList,
  MethodServersList,
  MethodServersIPTest,
  MethodServersSpeedTest,
  MethodServersStopTests,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodUpdatesState,
  UpdateIdle,
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

const noAddress = { ip: '', ipv6: '', countryCode: '', city: '', isp: '', asn: 0 }

const noBlock: KillSwitchState = { on: false, full: false, error: '', errorText: '' }

const noUpdates: UpdateState = {
  available: false,
  current: '',
  state: UpdateIdle,
  version: '',
  checkedAt: 0,
  error: '',
  errorText: '',
  installError: '',
  installVersion: '',
}

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
  const groups = ref<Group[]>([])
  const servers = ref<Server[]>([])
  const presets = ref<Preset[]>([])
  const connection = ref<ConnectionState>({ ...idle })
  const network = ref<NetworkState>({ ...noCheck })
  const killSwitch = ref<KillSwitchState>({ ...noBlock })
  const updates = ref<UpdateState>({ ...noUpdates })
  const traffic = ref<Traffic | null>(null)
  const testing = ref(false)
  // Speed and IP tests run on their own, beside the delay test.
  const speedTesting = ref(false)
  const ipTesting = ref(false)
  const loaded = ref(false)

  const running = computed(() => connection.value.state === ConnectionRunning)
  /** The core answers: a call now gets a reply. */
  const ready = computed(() => status.value.state === 'ready' && status.value.reason === undefined)
  const server = computed(() => servers.value.find((s) => s.id === settings.value?.server) ?? null)
  const preset = computed(() => presets.value.find((p) => p.id === settings.value?.preset) ?? null)

  function core(): CoreClient {
    if (!client.value) throw new Error('the core store is not started')
    return client.value
  }

  async function loadServers(): Promise<void> {
    servers.value = await core().call(MethodServersList, { groupId: '' })
  }

  // Reads everything again: at the start and whenever a new core takes over. A new core knows no delays and no
  // addresses yet; the ones measured before stay until it measures again, since the servers and the network are the
  // same. A test the old core ran is over: its results never come.
  async function load(): Promise<void> {
    const c = core()
    testing.value = false
    speedTesting.value = false
    ipTesting.value = false
    const [s, groupList, list, presetList, conn, net, block, upd] = await Promise.all([
      c.call(MethodSettingsGet),
      c.call(MethodGroupsList),
      c.call(MethodServersList, { groupId: '' }),
      c.call(MethodPresetsList),
      c.call(MethodConnectionState),
      c.call(MethodNetworkState),
      c.call(MethodKillSwitchState),
      c.call(MethodUpdatesState),
    ])
    const known = new Map(servers.value.map((server) => [server.id, server.latency]))
    settings.value = s
    groups.value = groupList
    servers.value = list.map((server) => (server.latency === 0 && known.get(server.id) ? { ...server, latency: known.get(server.id)! } : server))
    presets.value = presetList
    connection.value = conn
    if (net.checkedAt !== 0 || net.checking || network.value.checkedAt === 0 || conn.state !== ConnectionIdle) network.value = net
    killSwitch.value = block
    updates.value = upd
    loaded.value = true
  }

  let started: Promise<void> | undefined

  /** Connects to the core once; later calls wait for the same start. */
  function start(): Promise<void> {
    started ??= (async () => {
      const c = await useCore()
      client.value = c
      c.on(EventSettingsChanged, (next) => (settings.value = next))
      c.on(EventGroupsChanged, (next) => (groups.value = next))
      c.on(EventServersChanged, () => void loadServers())
      c.on(EventPresetsChanged, (next) => (presets.value = next))
      c.on(EventConnectionChanged, (next) => {
        connection.value = next
        if (next.state !== ConnectionRunning) traffic.value = null
      })
      c.on(EventConnectionTraffic, (next) => (traffic.value = next))
      c.on(EventNetworkChanged, (next) => (network.value = next))
      c.on(EventKillSwitchChanged, (next) => (killSwitch.value = next))
      c.on(EventUpdatesChanged, (next) => (updates.value = next))
      c.on(EventServersTested, (report) => {
        const results = new Map(report.results.map((r) => [r.serverId, r.latency]))
        servers.value = servers.value.map((s) => (results.has(s.id) ? { ...s, latency: results.get(s.id)! } : s))
        if (report.done) testing.value = false
      })
      c.on(EventServersSpeedTested, (report) => {
        const results = new Map(report.results.map((r) => [r.serverId, r]))
        servers.value = servers.value.map((s) => {
          const result = results.get(s.id)
          return result ? { ...s, speedDown: result.download, speedUp: result.upload } : s
        })
        if (report.done) speedTesting.value = false
      })
      c.on(EventServersIPTested, (report) => {
        const results = new Map(report.results.map((r) => [r.serverId, r]))
        servers.value = servers.value.map((s) => {
          const result = results.get(s.id)
          return result ? { ...s, exitIp: result.ip, exitCountry: result.country } : s
        })
        if (report.done) ipTesting.value = false
      })
      c.onStatus((next) => {
        const wasReady = status.value.state === 'ready'
        status.value = next
        if (next.state !== 'ready') testing.value = speedTesting.value = ipTesting.value = false
        if (next.state === 'ready' && !wasReady) void load().catch(() => undefined)
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

  /** A delay test of a group, of all servers, or of those ids names. */
  async function testServers(groupId = '', ids?: string[]): Promise<void> {
    testing.value = true
    try {
      await core().call(MethodServersTest, { groupId, ids })
    } catch (error) {
      testing.value = false
      throw error
    }
  }

  async function speedTestServers(groupId = '', ids?: string[]): Promise<void> {
    speedTesting.value = true
    try {
      await core().call(MethodServersSpeedTest, { groupId, ids })
    } catch (error) {
      speedTesting.value = false
      throw error
    }
  }

  async function ipTestServers(groupId = '', ids?: string[]): Promise<void> {
    ipTesting.value = true
    try {
      await core().call(MethodServersIPTest, { groupId, ids })
    } catch (error) {
      ipTesting.value = false
      throw error
    }
  }

  async function stopTests(): Promise<void> {
    await core().call(MethodServersStopTests)
    testing.value = speedTesting.value = ipTesting.value = false
  }

  return {
    client,
    status,
    settings,
    groups,
    servers,
    presets,
    connection,
    network,
    killSwitch,
    updates,
    traffic,
    testing,
    speedTesting,
    ipTesting,
    loaded,
    running,
    ready,
    server,
    preset,
    core,
    start,
    load,
    updateSettings,
    testServers,
    speedTestServers,
    ipTestServers,
    stopTests,
  }
})
