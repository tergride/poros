import type {
  LinkInfo,
  ConnectionState,
  InheritedRule,
  KillSwitchState,
  NetworkState,
  Preset,
  PresetCopy,
  PresetEntries,
  PresetEntry,
  PresetLists,
  PresetListsRead,
  Server,
  Settings,
  Subscription,
  SubscriptionUpdate,
  UpdateState,
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
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersTested,
  EventSettingsChanged,
  EventSubscriptionsChanged,
  EventUpdatesChanged,
  LeakNone,
  MethodAppInfo,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodKillSwitchState,
  MethodLogsGet,
  MethodNetworkCheck,
  MethodNetworkState,
  MethodPresetsExport,
  MethodPresetsGet,
  MethodLinksParse,
  MethodPresetsImport,
  MethodPresetsList,
  MethodPresetsReadLists,
  MethodPresetsRemove,
  MethodPresetsSave,
  MethodPresetsWriteLists,
  MethodServersList,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsList,
  MethodSubscriptionsRemove,
  MethodSubscriptionsRename,
  MethodSubscriptionsUpdate,
  MethodUpdatesCheck,
  MethodUpdatesInstall,
  MethodUpdatesState,
  ReasonCheckOff,
  ReasonDuplicate,
  ReasonInvalidEntry,
  LinkPreset,
  LinkSubscription,
  ReasonInvalidLink,
  ReasonInvalidURL,
  ReasonInvalidValue,
  KillSwitchAlways,
  KillSwitchConnected,
  KillSwitchOff,
  ReasonKillSwitchNeedsElevation,
  ReasonLogonTaskNeedsElevation,
  ReasonNeedsElevation,
  ReasonNoServer,
  ReasonNotFound,
  ReasonSingleServer,
  ReasonPresetNameTaken,
  ReasonUnknownPreset,
  ReasonUpdateNotReady,
  ReasonUpdateUnavailable,
  UpdateChecking,
  UpdateDownloading,
  UpdateIdle,
  UpdateReady,
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
  startMinimized: false,
  closeToTray: true,
  adminAutostart: false,
  runAsAdmin: false,
  killSwitch: KillSwitchOff,
  textSize: 'normal',
  updateCheck: true,
  debugLog: false,
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

/** The standard rules of a preset made from scratch, as in core/engine/presets.go. */
const standardPreset = 'all'
const foundation = (basedOn: string) => builtins.find((b) => b.id === (basedOn || standardPreset))

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

const noBlock: KillSwitchState = { on: false, error: '', errorText: '' }

/** A release build of 0.1.0, with 0.1.1 out: a check finds it and downloads it. */
const stubUpdates: UpdateState = {
  available: true,
  current: '0.1.0',
  state: UpdateIdle,
  version: '',
  checkedAt: 0,
  error: '',
  errorText: '',
  installError: '',
  installVersion: '',
}
const stubRelease = '0.1.1'

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

const isAddress = (value: string) =>
  /^[\d.]+(\/\d+)?$/.test(value) || /^\[?[0-9a-f]*:[0-9a-f]*:[0-9a-f:.]*\]?(\/\d+)?$/i.test(value)

/** What a site entry matches, null for no site, address or subnet. */
function siteKey(value: string): string | null {
  const host = value
    .trim()
    .replace(/^\w+:\/\/([^/]+).*$/, '$1')
    .toLowerCase()
    .replace(/^\*\./, '')
    .replace(/^\./, '')
  return isAddress(host) || /^([\p{L}\p{N}_-]+\.)+[\p{L}\p{N}_-]+$/u.test(host) ? host : null
}

const looksLikeSite = (value: string) => siteKey(value) !== null

const looksLikeApp = (value: string) => value !== '' && !/[*?"<>|]/.test(value)

/** The prefixes of the lines of the lists window, as in core/engine/presetlists.go; its tabs are `actions`. */
const sitePrefixes = ['domain', 'full', 'suffix', 'ip']
const appPrefixes = ['processname', 'processpath']
const unreadablePrefixes = ['keyword', 'regex', 'regexp', 'ruleset', 'geosite', 'geoip']

function readListLine(text: string): { site: boolean; value: string; key: string } | null {
  let value = text
  let site: boolean | undefined
  const colon = text.indexOf(':')
  if (colon >= 0) {
    const prefix = text.slice(0, colon).trim().toLowerCase()
    if (unreadablePrefixes.includes(prefix)) return null
    if (sitePrefixes.includes(prefix)) site = true
    if (appPrefixes.includes(prefix)) site = false
    if (site !== undefined) value = text.slice(colon + 1).trim()
  }
  site ??= value.includes('://') || isAddress(value) || !(/\.exe$/i.test(value) || value.includes('\\') || value.startsWith('/'))
  if (site) {
    const key = siteKey(value)
    return key === null ? null : { site, value, key: `site ${key}` }
  }
  return looksLikeApp(value) ? { site, value, key: `app ${value.toLowerCase()}` } : null
}

function readLists(lists: Record<string, string> | null): PresetListsRead {
  const read: PresetListsRead = { sites: [], apps: [], problems: [] }
  for (const action of Object.keys(lists ?? {})) {
    if (!actions.includes(action)) throw failed(ReasonInvalidValue, `there is no list "${action}"`)
  }
  const tabOf = new Map<string, string>()
  for (const action of actions) {
    for (const [i, line] of (lists?.[action] ?? '').split('\n').entries()) {
      const text = line.trim()
      if (text === '' || text.startsWith('#')) continue
      const entry = readListLine(text)
      if (!entry) {
        read.problems.push({ action, line: i + 1, text, reason: ReasonInvalidEntry, other: '' })
        continue
      }
      const other = tabOf.get(entry.key)
      if (other === undefined) {
        tabOf.set(entry.key, action)
        const list = entry.site ? read.sites : read.apps
        list.push({ value: entry.value, action })
      } else if (other !== action) read.problems.push({ action, line: i + 1, text, reason: ReasonDuplicate, other })
    }
  }
  return read
}

function writeLists({ sites, apps }: PresetEntries): PresetLists {
  const lines = new Map<string, string[]>()
  const write = (entries: PresetEntry[], site: boolean, prefix: string) => {
    for (const entry of entries) {
      if (!actions.includes(entry.action)) throw failed(ReasonInvalidEntry, `"${entry.value}": unknown action`)
      let value = entry.value.trim()
      const bare = readListLine(value)
      if ((!bare || bare.site !== site || bare.value !== value) && readListLine(prefix + value)) value = prefix + value
      lines.set(entry.action, [...(lines.get(entry.action) ?? []), value])
    }
  }
  write(sites, true, 'domain:')
  write(apps, false, 'processName:')
  return { lists: Object.fromEntries(actions.map((action) => [action, (lines.get(action) ?? []).join('\n')])) }
}

function encodeLink(copy: PresetCopy): string {
  const data = JSON.stringify({
    kind: 'poros-preset',
    v: 1,
    based_on: copy.basedOn,
    name: copy.name,
    sites: copy.sites,
    apps: copy.apps,
    dns: copy.dns,
    default: copy.default,
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
      default: String(data.default ?? ''),
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
  // The block outlives a core, as the guard process does: a new core of the stub keeps it.
  let block = { ...noBlock }
  let updates = { ...stubUpdates }
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
        direct: { ip: '203.0.113.5', ipv6: '', countryCode: 'RU', city: 'Moscow', isp: 'Home Telecom', asn: 64500 },
        proxy: connected
          ? {
              ip: '198.51.100.24',
              ipv6: settings.ipv6 ? '2001:db8:4000:5f:580:44c1:19ff:fe43' : '',
              countryCode: 'DE',
              city: 'Frankfurt am Main',
              isp: 'Example Hosting',
              asn: 64501,
            }
          : noAddress,
        dns: connected ? [{ ip: '192.0.2.53', countryCode: 'DE', city: '', isp: 'Example DNS', asn: 64510, home: false }] : [],
        leak: connected ? LeakNone : '',
        checkedAt: Date.now(),
      }
      emit(EventNetworkChanged, { ...network })
    }, 0)
    return { ...network }
  }
  const setBlock = (next: KillSwitchState) => {
    if (next.on === block.on && next.error === block.error && next.errorText === block.errorText) return
    block = { ...next }
    emit(EventKillSwitchChanged, { ...block })
  }
  // The rules of core/engine/killswitch.go: the block needs rights to come on, and any core turns it off.
  const arm = (): boolean => {
    if (block.on) return true
    if (!status.elevated) {
      setBlock({ on: false, error: ReasonKillSwitchNeedsElevation, errorText: 'the kill switch needs administrator rights' })
      return false
    }
    setBlock({ on: true, error: '', errorText: '' })
    return true
  }
  const disarm = () => setBlock({ ...noBlock })
  const syncBlock = (previous: string) => {
    if (settings.killSwitch === KillSwitchOff) disarm()
    else if (settings.killSwitch === KillSwitchAlways) arm()
    else if (connection.state === ConnectionRunning) arm()
    else if (previous === KillSwitchAlways) disarm()
  }
  // A new core joins the block the previous one left, or turns it on in the always mode.
  const startBlock = () => {
    if (block.on && settings.killSwitch === KillSwitchOff) disarm()
    else if (block.on) setBlock({ on: true, error: '', errorText: '' })
    else if (settings.killSwitch === KillSwitchAlways) arm()
    else disarm()
  }
  const setUpdates = (next: UpdateState) => {
    updates = next
    emit(EventUpdatesChanged, { ...updates })
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
  // As in the core: a name of another preset, built-in or not, is taken, whatever its case.
  const nameTaken = (name: string, id: string): boolean =>
    builtins.some((b) => b.name.toLowerCase() === name.toLowerCase()) ||
    copies.some((c) => c.id !== id && c.name.trim().toLowerCase() === name.toLowerCase())
  const freeName = (name: string): string => {
    let free = name.trim()
    for (let n = 2; free && nameTaken(free, ''); n++) free = `${name.trim()} ${n}`
    return free
  }
  // As in the core: the chosen server moves on when it goes away, to its subscription first, else to the first left.
  const keepServerChosen = (subscriptionId: string): void => {
    if (!settings.server || servers.some((s) => s.id === settings.server)) return
    const next = servers.find((s) => s.subscriptionId === subscriptionId) ?? servers[0]
    settings = { ...settings, server: next?.id ?? '' }
    emit(EventSettingsChanged, { ...settings })
  }
  const saveCopy = (draft: PresetCopy): Preset => {
    const name = draft.name.trim()
    if (!name) throw failed(ReasonInvalidValue, 'the preset needs a name')
    if (nameTaken(name, draft.id)) throw failed(ReasonPresetNameTaken, `there is a preset named "${name}" already`)
    const base = foundation(draft.basedOn)
    if (!base) throw failed(ReasonUnknownPreset, `there is no built-in preset "${draft.basedOn}"`)
    checkEntries(draft.sites, looksLikeSite)
    checkEntries(draft.apps, looksLikeApp)
    if (!['', ActionProxy, ActionDirect, ActionBlock].includes(draft.default)) {
      throw failed(ReasonInvalidValue, `the default must be proxy, direct or block, not "${draft.default}"`)
    }
    const copy = { ...draft, name, id: draft.id || String(nextId++), inherited: base.inherited }
    if (draft.id && !copies.some((c) => c.id === draft.id)) throw failed(ReasonNotFound, `there is no preset "${draft.id}"`)
    copies = draft.id ? copies.map((c) => (c.id === draft.id ? copy : c)) : [...copies, copy]
    emit(EventPresetsChanged, presetList())
    return presetList().find((p) => p.id === copy.id)!
  }
  const copyOf = (id: string): PresetCopy => {
    if (!id) return { id: '', basedOn: '', name: '', sites: [], apps: [], dns: '', default: ActionProxy, inherited: foundation('')!.inherited }
    const base = builtins.find((b) => b.id === id)
    if (base) return { id: '', basedOn: id, name: base.name, sites: [], apps: [], dns: '', default: ActionProxy, inherited: base.inherited }
    const copy = copies.find((c) => c.id === id)
    if (!copy) throw failed(ReasonNotFound, `there is no preset "${id}"`)
    return structuredClone(copy)
  }

  const methods: { [M in CoreMethod]: (params: CoreMethods[M]['params']) => CoreMethods[M]['result'] } = {
    [MethodAppInfo]: () => ({ singBox: 'stub', xray: 'stub', dataDir: 'browser', elevated: status.elevated }),
    [MethodSettingsGet]: () => ({ ...settings }),
    [MethodSettingsSet]: (next) => {
      if (!presetList().some((p) => p.id === next.preset)) throw failed(ReasonUnknownPreset, `there is no preset "${next.preset}"`)
      if (next.server && !servers.some((s) => s.id === next.server)) throw failed(ReasonNotFound, `there is no server "${next.server}"`)
      if (![KillSwitchOff, KillSwitchConnected, KillSwitchAlways].includes(next.killSwitch)) {
        throw failed(ReasonInvalidValue, `killSwitch must be off, connected or always, not "${next.killSwitch}"`)
      }
      // As core/engine/settings.go: the sign-in task is made by a core with rights.
      if (next.adminAutostart && !settings.adminAutostart && !status.elevated) {
        throw failed(ReasonLogonTaskNeedsElevation, 'the sign-in task takes administrator rights')
      }
      const previous = settings.killSwitch
      settings = { ...next }
      emit(EventSettingsChanged, { ...settings })
      if (settings.killSwitch !== previous) setTimeout(() => syncBlock(previous), 0)
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
      keepServerChosen('')
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
        settings = { ...settings, preset: copy.basedOn || standardPreset }
        emit(EventSettingsChanged, { ...settings })
      }
      return null
    },
    [MethodPresetsExport]: ({ id }) => ({ link: encodeLink(copyOf(id)) }),
    [MethodPresetsImport]: ({ link }) => {
      const copy = decodeLink(link)
      return saveCopy({ ...copy, name: freeName(copy.name) })
    },
    [MethodPresetsWriteLists]: (entries) => writeLists(entries),
    [MethodPresetsReadLists]: ({ lists }) => readLists(lists),
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
        if (settings.killSwitch !== KillSwitchOff && !arm()) {
          return { ...idle, error: ReasonKillSwitchNeedsElevation, errorText: 'the kill switch needs administrator rights' }
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
      settle(() => {
        if (settings.killSwitch !== KillSwitchAlways) disarm()
        return { ...idle }
      })
      return { ...connection }
    },
    [MethodConnectionState]: () => ({ ...connection }),
    [MethodNetworkCheck]: () => {
      if (!settings.checkIp) throw failed(ReasonCheckOff, 'the IP and DNS check is turned off')
      return checkNetwork()
    },
    [MethodNetworkState]: () => ({ ...network }),
    [MethodLogsGet]: () => [...stubLog],
    [MethodKillSwitchState]: () => ({ ...block }),
    [MethodUpdatesState]: () => ({ ...updates }),
    [MethodUpdatesCheck]: () => {
      if (!updates.available) throw failed(ReasonUpdateUnavailable, 'this build does not update itself')
      if (updates.state === UpdateIdle) {
        setUpdates({ ...updates, state: UpdateChecking, error: '', errorText: '' })
        setTimeout(() => setUpdates({ ...updates, state: UpdateDownloading, version: stubRelease }), 0)
        setTimeout(() => setUpdates({ ...updates, state: UpdateReady, version: stubRelease, checkedAt: Date.now() }), 50)
      }
      return { ...updates }
    },
    [MethodUpdatesInstall]: () => {
      if (updates.state !== UpdateReady) throw failed(ReasonUpdateNotReady, 'no update is downloaded')
      return { archive: `browser/update/Poros-${updates.version}-windows-amd64.zip` }
    },
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
      // As in the core, the block is settled before the new core answers.
      setTimeout(() => {
        startBlock()
        setStatus({ state: 'ready', elevated: true })
      }, 0)
    },
    async restart() {
      setStatus({ state: 'starting', elevated: false })
      setConnection({ ...idle })
      // As in the core, the block is settled before the new core answers.
      setTimeout(() => {
        startBlock()
        setStatus({ state: 'ready', elevated: false })
      }, 0)
    },
    async resetData() {
      await this.restart()
    },
  }
}
