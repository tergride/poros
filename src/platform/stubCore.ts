import type {
  LinkInfo,
  ConnectionState,
  InheritedRule,
  NetworkState,
  Preset,
  PresetCopy,
  PresetEntry,
  Server,
  Settings,
  Subscription,
  SubscriptionUpdate,
} from './core-types'
import {
  ActionBlock,
  ActionDirect,
  ActionProxy,
  ConnectionIdle,
  ConnectionRunning,
  ConnectionStarting,
  ConnectionStopping,
  EventConnectionChanged,
  EventConnectionTraffic,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersTested,
  EventSettingsChanged,
  EventSubscriptionsChanged,
  LeakNone,
  MethodAppInfo,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodLogsGet,
  MethodNetworkCheck,
  MethodNetworkState,
  MethodPresetsExport,
  MethodPresetsGet,
  MethodLinksParse,
  MethodPresetsImport,
  MethodPresetsList,
  MethodPresetsRemove,
  MethodPresetsSave,
  MethodServersList,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  MethodSubscriptionsRename,
  MethodSubscriptionsUpdate,
  ReasonCheckOff,
  ReasonDuplicate,
  ReasonInvalidEntry,
  LinkPreset,
  LinkSubscription,
  ReasonInvalidLink,
  ReasonInvalidURL,
  ReasonInvalidValue,
  ReasonNeedsElevation,
  ReasonNoServer,
  ReasonNotFound,
  ReasonSingleServer,
  ReasonUnknownPreset,
} from './core-types'
import type { CoreClient, CoreError, CoreEvent, CoreEvents, CoreMethod, CoreMethods, CoreStatus } from './core'

const defaultSettings: Settings = {
  uiMode: 'simple',
  language: '',
  onboardingDone: false,
  tun: false,
  systemProxy: false,
  server: '',
  preset: 'all',
  checkIp: true,
  allowLan: false,
  ipv6: true,
  realityOnXray: true,
  subscriptionAutoUpdate: true,
  subscriptionUpdateHours: 0,
  subscriptionViaProxy: false,
  sendDeviceInfo: false,
  showChanges: true,
  autoConnect: true,
  autostart: false,
  urlScheme: true,
  startMinimized: false,
  closeToTray: true,
  killSwitch: false,
  textSize: 'normal',
}

/** The servers every subscription of the stub core holds, with the delay a test gives them. */
const stubServers: (Omit<Server, 'id' | 'subscriptionId' | 'latency'> & { measured: number })[] = [
  { name: '🇩🇪 Germany', protocol: 'vless', host: 'de.example.com', port: 443, country: 'DE', measured: 42 },
  { name: '🇳🇱 Netherlands', protocol: 'vless', host: 'nl.example.com', port: 443, country: 'NL', measured: 48 },
  { name: '🇫🇮 Finland', protocol: 'trojan', host: 'fi.example.com', port: 443, country: 'FI', measured: -1 },
]

const builtins: (Preset & { inherited: InheritedRule[] })[] = [
  { id: 'all', name: 'All traffic', builtin: true, basedOn: '', sites: 0, apps: 0, inherited: [{ name: 'Local networks', action: ActionDirect }] },
  { id: 'hardened', name: 'Hardened bypass', builtin: true, basedOn: '', sites: 0, apps: 0, inherited: [{ name: 'Local networks', action: ActionDirect }] },
]

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

const stubLog = [
  '2026/11/07 14:02:11 sing-box 1.15.0, Xray 26.10.21',
  '2026/11/07 14:02:13 connection: running',
  '2026/11/07 14:05:47 WARN outbound/vless[proxy]: stream reset, reconnecting',
]

const presetLinkPrefix = 'poros://route/'

function failed(reason: string, message: string): CoreError {
  return { code: 1, message, data: { reason } }
}

const actions: string[] = [ActionProxy, ActionDirect, ActionBlock]

function checkEntries(entries: PresetEntry[], valid: (value: string) => boolean) {
  for (const entry of entries) {
    if (!actions.includes(entry.action)) throw failed(ReasonInvalidEntry, `"${entry.value}": unknown action`)
    if (!valid(entry.value.trim())) throw failed(ReasonInvalidEntry, `"${entry.value}" is not valid here`)
  }
}

const looksLikeSite = (value: string) =>
  /^(\*\.)?([\p{L}\p{N}_-]+\.)+[\p{L}\p{N}_-]+$/u.test(value.replace(/^\w+:\/\/([^/]+).*$/, '$1')) ||
  /^[\d.]+(\/\d+)?$/.test(value) ||
  /^[0-9a-f:]+(\/\d+)?$/i.test(value)

const looksLikeApp = (value: string) => value !== '' && !/[*?"<>|]/.test(value)

function encodeLink(copy: PresetCopy): string {
  const data = JSON.stringify({
    kind: 'poros-preset',
    v: 1,
    based_on: copy.basedOn,
    name: copy.name,
    sites: copy.sites,
    apps: copy.apps,
    dns: copy.dns,
  })
  const base64 = btoa(String.fromCharCode(...new TextEncoder().encode(data)))
  return presetLinkPrefix + base64.replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
}

function decodeLink(link: string): PresetCopy {
  const text = link.trim()
  if (!text.toLowerCase().startsWith(presetLinkPrefix)) {
    throw failed(ReasonInvalidLink, `a preset link starts with ${presetLinkPrefix}`)
  }
  try {
    const base64 = text.slice(presetLinkPrefix.length).replaceAll('-', '+').replaceAll('_', '/')
    const bytes = Uint8Array.from(atob(base64), (c) => c.charCodeAt(0))
    const data = JSON.parse(new TextDecoder().decode(bytes)) as Record<string, unknown>
    if (data.kind !== 'poros-preset' || data.v !== 1) throw new Error()
    return {
      id: '',
      basedOn: String(data.based_on ?? ''),
      name: String(data.name ?? ''),
      sites: (data.sites as PresetEntry[] | undefined) ?? [],
      apps: (data.apps as PresetEntry[] | undefined) ?? [],
      dns: String(data.dns ?? ''),
      inherited: [],
    }
  } catch {
    throw failed(ReasonInvalidLink, 'the link holds no Poros preset')
  }
}

function decodeBase64(data: string): string {
  const bytes = Uint8Array.from(atob(data.replaceAll('-', '+').replaceAll('_', '/')), (c) => c.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

/** The rules of core/engine/links.go: nothing is added, the link is only read. */
function parseOutsideLink(link: string): LinkInfo {
  const match = /^(poros|throne):\/\/([^/]*)\/?(.*)$/i.exec(link.trim())
  if (!match) throw failed(ReasonInvalidLink, 'not a poros:// link')
  const [, scheme = '', command = '', data = ''] = match
  switch (command.toLowerCase()) {
    case 'addsub': {
      let text: string
      try {
        text = decodeBase64(data.trim()).trim()
      } catch {
        throw failed(ReasonInvalidLink, 'the subscription link is damaged')
      }
      const cut = text.indexOf('#')
      const url = cut < 0 ? text : text.slice(0, cut)
      let name = cut < 0 ? '' : text.slice(cut + 1)
      try {
        name = decodeURIComponent(name)
      } catch {
        // A name that is not escaped stays as it is.
      }
      if (!/^https?:\/\/[^/\s]/i.test(url)) {
        if (/^[a-z][a-z0-9+.-]*:\/\/\S/i.test(url)) {
          throw failed(ReasonSingleServer, 'this is a link to one server, not a subscription')
        }
        throw failed(ReasonInvalidURL, 'a subscription link starts with http:// or https://')
      }
      return { kind: LinkSubscription, url, name: name.trim() }
    }
    case 'add':
      throw failed(ReasonSingleServer, 'this is a link to one server, not a subscription')
    case 'route':
      if (scheme.toLowerCase() !== 'poros') throw failed(ReasonInvalidLink, 'Throne routing profiles are not Poros presets')
      return { kind: LinkPreset, url: '', name: '', preset: decodeLink(presetLinkPrefix + data) }
  }
  throw failed(ReasonInvalidLink, 'unknown poros:// link')
}

/**
 * A core in memory for the interface alone: Vite in a browser, Vitest and Playwright.
 * It follows the rules of the Go engine closely enough for screens and tests, not for real traffic.
 */
export function createStubCore(): CoreClient {
  let settings = { ...defaultSettings }
  let subscriptions: Subscription[] = []
  let servers: Server[] = []
  let copies: PresetCopy[] = []
  let connection = { ...idle }
  let network = { ...noCheck }
  let status: CoreStatus = { state: 'ready', elevated: false }
  let traffic: ReturnType<typeof setInterval> | undefined
  let nextId = 1
  const handlers = new Map<string, Set<(params: never) => void>>()
  const statusHandlers = new Set<(status: CoreStatus) => void>()
  const emit = <E extends CoreEvent>(event: E, params: CoreEvents[E]) =>
    handlers.get(event)?.forEach((handler) => handler(params as never))
  const changed = (subscriptionId: string) => {
    emit(EventSubscriptionsChanged, [...subscriptions])
    if (subscriptionId) emit(EventServersChanged, { subscriptionId })
  }
  const find = (id: string) => {
    const sub = subscriptions.find((s) => s.id === id)
    if (!sub) throw failed(ReasonNotFound, `there is no subscription "${id}"`)
    return sub
  }
  const presetList = (): Preset[] => [
    ...builtins.map(({ inherited: _, ...preset }) => preset),
    ...copies.map((c) => ({ id: c.id, name: c.name, builtin: false, basedOn: c.basedOn, sites: c.sites.length, apps: c.apps.length })),
  ]
  const setStatus = (next: CoreStatus) => {
    status = next
    statusHandlers.forEach((handler) => handler({ ...status }))
  }
  // Like the engine, a start or stop answers at once and settles a moment later; the latest request wins.
  let serial = 0
  const setConnection = (next: ConnectionState) => {
    connection = next
    emit(EventConnectionChanged, { ...connection })
    clearInterval(traffic)
    traffic = undefined
    if (next.state === ConnectionRunning) {
      let total = 0
      traffic = setInterval(() => {
        total += 1_300_000
        emit(EventConnectionTraffic, { upload: 115_000, download: 1_300_000, totalUpload: total / 10, totalDownload: total })
      }, 1000)
    }
  }
  // Like the engine: a check after each start and stop while the user keeps it on.
  const checkNetwork = (): NetworkState => {
    network = { ...network, checking: true }
    emit(EventNetworkChanged, { ...network })
    setTimeout(() => {
      const connected = connection.state === ConnectionRunning
      // Documentation addresses: the stub asks no outside service.
      network = {
        ...noCheck,
        direct: { ip: '203.0.113.5', countryCode: 'RU', city: 'Moscow', isp: 'Home Telecom', asn: 64500 },
        proxy: connected
          ? { ip: '198.51.100.24', countryCode: 'DE', city: 'Frankfurt am Main', isp: 'Example Hosting', asn: 64501 }
          : noAddress,
        dns: connected ? [{ ip: '192.0.2.53', countryCode: 'DE', city: '', isp: 'Example DNS', asn: 64510, home: false }] : [],
        leak: connected ? LeakNone : '',
        checkedAt: Date.now(),
      }
      emit(EventNetworkChanged, { ...network })
    }, 0)
    return { ...network }
  }
  const settle = (next: () => ConnectionState) => {
    const mine = ++serial
    setTimeout(() => {
      if (mine !== serial) return
      setConnection(next())
      if (settings.checkIp) checkNetwork()
    }, 0)
  }
  const noChanges = (sub: Subscription): SubscriptionUpdate => ({
    subscription: { ...sub },
    added: 0,
    removed: 0,
    changed: 0,
    addedNames: [],
    removedNames: [],
    skips: [],
  })
  const saveCopy = (draft: PresetCopy): Preset => {
    const name = draft.name.trim()
    if (!name) throw failed(ReasonInvalidValue, 'the preset needs a name')
    const base = builtins.find((b) => b.id === draft.basedOn)
    if (!base) throw failed(ReasonUnknownPreset, `there is no built-in preset "${draft.basedOn}"`)
    checkEntries(draft.sites, looksLikeSite)
    checkEntries(draft.apps, looksLikeApp)
    const copy = { ...draft, name, id: draft.id || String(nextId++), inherited: base.inherited }
    if (draft.id && !copies.some((c) => c.id === draft.id)) throw failed(ReasonNotFound, `there is no preset "${draft.id}"`)
    copies = draft.id ? copies.map((c) => (c.id === draft.id ? copy : c)) : [...copies, copy]
    emit(EventPresetsChanged, presetList())
    return presetList().find((p) => p.id === copy.id)!
  }
  const copyOf = (id: string): PresetCopy => {
    const base = builtins.find((b) => b.id === id)
    if (base) return { id: '', basedOn: id, name: base.name, sites: [], apps: [], dns: '', inherited: base.inherited }
    const copy = copies.find((c) => c.id === id)
    if (!copy) throw failed(ReasonNotFound, `there is no preset "${id}"`)
    return structuredClone(copy)
  }

  const methods: { [M in CoreMethod]: (params: CoreMethods[M]['params']) => CoreMethods[M]['result'] } = {
    [MethodAppInfo]: () => ({ singBox: 'stub', xray: 'stub', dataDir: 'browser', elevated: status.elevated }),
    [MethodSettingsGet]: () => ({ ...settings }),
    [MethodSettingsSet]: (next) => {
      if (!presetList().some((p) => p.id === next.preset)) throw failed(ReasonUnknownPreset, `there is no preset "${next.preset}"`)
      settings = { ...next }
      emit(EventSettingsChanged, { ...settings })
      return { ...settings }
    },
    [MethodSubscriptionsList]: () => [...subscriptions],
    [MethodSubscriptionsAdd]: ({ url, name }) => {
      const link = url.trim()
      let host: string
      try {
        const parsed = new URL(link)
        if (parsed.protocol !== 'http:' && parsed.protocol !== 'https:') throw new Error()
        host = parsed.hostname
      } catch {
        if (/^[a-z][a-z0-9+.-]*:\/\/\S/i.test(link)) {
          throw failed(ReasonSingleServer, 'this is a link to one server, not a subscription')
        }
        throw failed(ReasonInvalidURL, 'a subscription link starts with http:// or https://')
      }
      if (subscriptions.some((s) => s.url === link)) throw failed(ReasonDuplicate, 'this subscription is already added')
      const now = Date.now()
      const sub: Subscription = {
        id: String(nextId++),
        name: name.trim() || host,
        url: link,
        addedAt: now,
        updatedAt: now,
        servers: stubServers.length,
        skipped: 0,
        upload: 20_000_000_000,
        download: 18_200_000_000,
        total: 100_000_000_000,
        expire: now + 35 * 24 * 3600 * 1000,
        webPageUrl: '',
        supportUrl: '',
        announce: '',
        error: '',
        errorText: '',
      }
      const added = stubServers.map(({ measured: _, ...server }) => ({ ...server, id: String(nextId++), subscriptionId: sub.id, latency: 0 }))
      subscriptions = [...subscriptions, sub]
      servers = [...servers, ...added]
      changed(sub.id)
      return { ...noChanges(sub), added: added.length, addedNames: added.map((s) => s.name) }
    },
    [MethodSubscriptionsUpdate]: ({ id }) => {
      const sub = { ...find(id), updatedAt: Date.now() }
      subscriptions = subscriptions.map((s) => (s.id === id ? sub : s))
      changed(id)
      return noChanges(sub)
    },
    [MethodSubscriptionsRemove]: ({ id }) => {
      find(id)
      subscriptions = subscriptions.filter((s) => s.id !== id)
      servers = servers.filter((s) => s.subscriptionId !== id)
      changed(id)
      return null
    },
    [MethodSubscriptionsRename]: ({ id, name }) => {
      const sub = { ...find(id), name: name.trim() || new URL(find(id).url).hostname }
      subscriptions = subscriptions.map((s) => (s.id === id ? sub : s))
      changed('')
      return { ...sub }
    },
    [MethodServersList]: ({ subscriptionId }) =>
      servers.filter((s) => subscriptionId === '' || s.subscriptionId === subscriptionId).map((s) => ({ ...s })),
    [MethodServersTest]: ({ subscriptionId }) => {
      const tested = servers.filter((s) => subscriptionId === '' || s.subscriptionId === subscriptionId)
      tested.forEach((server, i) => {
        setTimeout(() => {
          const measured = stubServers.find((s) => s.host === server.host)?.measured ?? -1
          servers = servers.map((s) => (s.id === server.id ? { ...s, latency: measured } : s))
          emit(EventServersTested, { results: [{ serverId: server.id, latency: measured }], done: i === tested.length - 1 })
        }, 0)
      })
      if (tested.length === 0) setTimeout(() => emit(EventServersTested, { results: [], done: true }), 0)
      return null
    },
    [MethodPresetsList]: () => presetList(),
    [MethodPresetsGet]: ({ id }) => copyOf(id),
    [MethodPresetsSave]: (draft) => saveCopy(draft),
    [MethodPresetsRemove]: ({ id }) => {
      const copy = copies.find((c) => c.id === id)
      if (!copy) throw failed(ReasonNotFound, `there is no preset "${id}"`)
      copies = copies.filter((c) => c.id !== id)
      emit(EventPresetsChanged, presetList())
      if (settings.preset === id) {
        settings = { ...settings, preset: copy.basedOn }
        emit(EventSettingsChanged, { ...settings })
      }
      return null
    },
    [MethodPresetsExport]: ({ id }) => ({ link: encodeLink(copyOf(id)) }),
    [MethodPresetsImport]: ({ link }) => saveCopy(decodeLink(link)),
    [MethodLinksParse]: ({ link }) => parseOutsideLink(link),
    [MethodConnectionStart]: () => {
      setConnection({ ...idle, state: ConnectionStarting })
      settle(() => {
        if (!servers.some((s) => s.id === settings.server)) {
          return { ...idle, error: ReasonNoServer, errorText: 'no server is chosen' }
        }
        if (settings.tun && !status.elevated) {
          return { ...idle, error: ReasonNeedsElevation, errorText: 'TUN needs administrator rights' }
        }
        return {
          ...idle,
          state: ConnectionRunning,
          serverId: settings.server,
          tun: settings.tun,
          systemProxy: settings.systemProxy,
          since: Date.now(),
        }
      })
      return { ...connection }
    },
    [MethodConnectionStop]: () => {
      if (connection.state !== ConnectionIdle) setConnection({ ...connection, state: ConnectionStopping })
      settle(() => ({ ...idle }))
      return { ...connection }
    },
    [MethodConnectionState]: () => ({ ...connection }),
    [MethodNetworkCheck]: () => {
      if (!settings.checkIp) throw failed(ReasonCheckOff, 'the IP and DNS check is turned off')
      return checkNetwork()
    },
    [MethodNetworkState]: () => ({ ...network }),
    [MethodLogsGet]: () => [...stubLog],
  }

  return {
    async call(method, ...params) {
      const run = methods[method] as (params: unknown) => CoreMethods[typeof method]['result']
      return run(params[0])
    },
    on(event, handler) {
      const set = handlers.get(event) ?? new Set()
      set.add(handler as (params: never) => void)
      handlers.set(event, set)
      return () => set.delete(handler as (params: never) => void)
    },
    status: async () => ({ ...status }),
    onStatus(handler) {
      statusHandlers.add(handler)
      return () => statusHandlers.delete(handler)
    },
    // A new core comes up without a connection, as the real one does.
    async elevate() {
      setStatus({ state: 'starting', elevated: true })
      setConnection({ ...idle })
      setTimeout(() => setStatus({ state: 'ready', elevated: true }), 0)
    },
    async restart() {
      setStatus({ state: 'starting', elevated: false })
      setConnection({ ...idle })
      setTimeout(() => setStatus({ state: 'ready', elevated: false }), 0)
    },
  }
}
