import type {
  LinkInfo,
  ConnectionState,
  Group,
  GroupSettings,
  InheritedRule,
  KillSwitchState,
  NetworkState,
  Preset,
  PresetCopy,
  Server,
  ServerCheck,
  ServerForms,
  Settings,
  SkipReason,
  SubscriptionUpdate,
  UpdateState,
  WARPDevice,
} from './core-types'
import {
  ActionBlock,
  ActionDirect,
  ActionEndpoint,
  ActionProxy,
  ConnectionIdle,
  ConnectionRunning,
  DNSRoutingAuto,
  TunBypassDefault,
  TunIPv4Default,
  TunIPv6Default,
  WARPModeMASQUE,
  WARPModeWireGuard,
  ConnectionStarting,
  ConnectionStopping,
  CoreSingBox,
  CoreXray,
  EventConnectionChanged,
  EventConnectionTraffic,
  EventKillSwitchChanged,
  EventNetworkChanged,
  EventPresetsChanged,
  EventServersChanged,
  EventServersIPTested,
  EventServersSpeedTested,
  EventServersTested,
  EventGroupsChanged,
  EventSettingsChanged,
  EventUpdatesChanged,
  GroupManual,
  GroupSubscription,
  LeakNone,
  LinkServer,
  MethodAppDebugInfo,
  MethodAppInfo,
  MethodConnectionStart,
  MethodConnectionState,
  MethodConnectionStop,
  MethodConnectionsClose,
  MethodConnectionsList,
  MethodGroupsAdd,
  MethodGroupsConfigure,
  MethodGroupsList,
  MethodGroupsRemove,
  MethodGroupsRename,
  MethodKillSwitchState,
  MethodLogsGet,
  MethodNetworkCheck,
  MethodNetworkState,
  MethodDNSCheck,
  MethodPresetsCheck,
  MethodWARPRegister,
  MethodPresetsExport,
  MethodPresetsGet,
  MethodLinksParse,
  MethodPresetsImport,
  MethodPresetsList,
  MethodPresetsReadLists,
  MethodPresetsRemove,
  MethodPresetsSave,
  MethodPresetsWriteLists,
  MethodServersAdd,
  MethodServersCheck,
  MethodServersClone,
  MethodServersDeepLinks,
  MethodServersExport,
  MethodServersForms,
  MethodServersGet,
  MethodServersLinks,
  MethodServersList,
  MethodServersMove,
  MethodServersRemove,
  MethodServersSave,
  MethodServersClean,
  MethodServersClearResults,
  MethodServersIPTest,
  MethodServersResetTraffic,
  MethodServersResolve,
  MethodServersSpeedTest,
  MethodServersStopTests,
  MethodServersTest,
  MethodSettingsGet,
  MethodSettingsSet,
  MethodSubscriptionsAdd,
  MethodSubscriptionsUpdate,
  MethodUpdatesCheck,
  MethodUpdatesInstall,
  MethodUpdatesState,
  ReasonCheckOff,
  SpeedTestFull,
  ReasonDuplicate,
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
  ReasonSpoofNeedsElevation,
  ReasonNoServer,
  ReasonNoServers,
  ReasonNotFound,
  ReasonNotManual,
  ReasonServerUnsupported,
  ReasonSingleServer,
  ReasonSubscriptionURL,
  ReasonPresetNameTaken,
  ReasonAdvancedOnly,
  ReasonUnknownPreset,
  ReasonUpdateNotReady,
  ReasonUpdateUnavailable,
  UpdateChecking,
  UpdateDownloading,
  UpdateIdle,
  UpdateReady,
} from './core-types'
import type { CoreClient, CoreError, CoreEvent, CoreEvents, CoreMethod, CoreMethods, CoreStatus } from './core'
import type { Profile } from './profile-types'
import stubForms from './stubForms.json'
import { checkDnsObject, checkDnsSettings } from './stubDns'
import { checkRules, readLists, writeLists } from './stubPresets'

const noWarpDevice: WARPDevice = { endpoint: '', privateKey: '', peerKey: '', addresses: '', reserved: '', serverName: '', http: '', registeredAt: 0 }

const defaultSettings: Settings = {
  uiMode: 'simple',
  language: '',
  onboardingDone: false,
  tun: false,
  systemProxy: false,
  tunMtu: 1500,
  tunStrictRoute: true,
  tunRouting: false,
  tunIpv4: TunIPv4Default,
  tunIpv6: TunIPv6Default,
  tunBypass: TunBypassDefault,
  server: '',
  preset: 'all',
  checkIp: true,
  allowLan: false,
  proxyPort: 0,
  proxyAuth: false,
  proxyUsername: '',
  proxyPassword: '',
  latencyUrl: '',
  latencyTimeout: 3000,
  latencyConcurrency: 10,
  muxDefault: false,
  muxProtocol: 'smux',
  muxConcurrency: 8,
  fragmentDefault: false,
  fingerprintDefault: '',
  spoofDefault: false,
  spoofSni: '',
  spoofMethod: '',
  quicIdleTimeout: '',
  quicKeepAlive: '',
  quicStreamWindow: '',
  quicConnectionWindow: '',
  quicMaxStreams: 0,
  quicInitialPacketSize: 0,
  quicDisablePmtud: false,
  ntp: false,
  ntpServer: 'time.cloudflare.com',
  ntpPort: 123,
  ntpInterval: 30,
  ntpOutbound: 'direct',
  ignoreTlsErrors: false,
  xraySystemCerts: false,
  insecureDefault: false,
  stats: true,
  deviceData: '',
  logLines: 500,
  updateBeta: false,
  ipv6: true,
  realityOnXray: true,
  dnsRemote: '',
  dnsDirect: '',
  dnsRouting: DNSRoutingAuto,
  fakeIp: false,
  dnsHosts: true,
  dnsAnswers: '',
  dnsCache: true,
  dnsCacheSize: 4096,
  dnsIgnoreTtl: false,
  dnsOptimistic: false,
  dnsTimeout: 10,
  dnsClientSubnet: '',
  dnsObject: '',
  addressFirst: '',
  warp: false,
  warpMode: WARPModeWireGuard,
  warpWireGuard: noWarpDevice,
  warpMasque: noWarpDevice,
  warpHosts: '',
  subscriptionAutoUpdate: true,
  subscriptionUpdateHours: 0,
  subscriptionViaProxy: false,
  subscriptionUserAgent: '',
  subscriptionTls: '',
  subscriptionHttp: '',
  speedTestMode: SpeedTestFull,
  speedTestUrl: 'https://speed.cloudflare.com/__down?bytes=25000000',
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

/** The settings of a new group: nothing changed, as in core/engine. */
const noGroupSettings: GroupSettings = {
  serverAddress: '',
  frontProxy: '',
  landingProxy: '',
  userAgent: '',
  tls: '',
  http: '',
  deviceInfo: '',
  deviceId: '',
  os: '',
  osVersion: '',
  deviceModel: '',
  update: '',
  updateHours: 0,
  keepWorking: false,
  removeDuplicates: false,
  removeInsecure: false,
  test: false,
  removeUnavailable: false,
  sortByLatency: false,
}

/** The servers every subscription of the stub core holds, with the delay a test gives them. */
const stubServers: (Omit<Server, 'id' | 'groupId' | 'latency'> & { measured: number })[] = [
  { name: '🇩🇪 Germany', protocol: 'vless', host: 'de.example.com', port: 443, country: 'DE', security: '', upload: 0, download: 0, speedDown: '', speedUp: '', exitIp: '', exitCountry: '', measured: 42 },
  { name: '🇳🇱 Netherlands', protocol: 'vless', host: 'nl.example.com', port: 443, country: 'NL', security: '', upload: 0, download: 0, speedDown: '', speedUp: '', exitIp: '', exitCountry: '', measured: 48 },
  { name: '🇫🇮 Finland', protocol: 'trojan', host: 'fi.example.com', port: 443, country: 'FI', security: '', upload: 0, download: 0, speedDown: '', speedUp: '', exitIp: '', exitCountry: '', measured: -1 },
]

const builtinRules: InheritedRule[] = [
  { name: 'Route DNS', action: 'dns', first: true },
  { name: 'Local networks', action: ActionDirect, first: false },
]
const builtins: (Preset & { inherited: InheritedRule[] })[] = [
  { id: 'all', name: 'All traffic', builtin: true, basedOn: '', advanced: false, rules: 2, endpoints: 0, inherited: builtinRules },
  { id: 'hardened', name: 'Hardened bypass', builtin: true, basedOn: '', advanced: false, rules: 2, endpoints: 0, inherited: builtinRules },
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

const noBlock: KillSwitchState = { on: false, full: false, error: '', errorText: '' }

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

const stubConnection = {
  network: 'tcp',
  protocol: 'tls',
  processPath: '',
  source: '127.0.0.1:52144',
  inbound: 'tun-in',
  outbound: 'proxy',
  rule: '',
}
const stubConnections = [
  { ...stubConnection, id: 'c1', start: 0, process: 'chrome.exe', destination: 'www.example.com:443', upload: 120_000, download: 2_100_000 },
  { ...stubConnection, id: 'c2', start: 400, process: 'chrome.exe', destination: 'cdn.example.net:443', network: 'udp', protocol: 'quic', source: '127.0.0.1:52160', upload: 9_000, download: 310_000 },
  { ...stubConnection, id: 'c3', start: 800, process: 'telegram.exe', destination: '149.154.167.51:443', protocol: '', source: '127.0.0.1:52190', upload: 6_000, download: 48_000 },
  { ...stubConnection, id: 'c4', start: 1200, process: 'steam.exe', destination: 'cache1.example.com:443', outbound: 'direct', source: '127.0.0.1:53012', upload: 30_000, download: 9_600_000 },
  { ...stubConnection, id: 'c5', start: 1600, process: '', destination: '1.1.1.1:53', network: 'udp', protocol: 'dns', outbound: 'direct', source: '172.19.0.1:61000', upload: 200, download: 1_000 },
].map((c) => ({ ...c, processPath: c.process ? `C:\\Program Files\\${c.process}` : '' }))
const closedConnections = new Set<string>()

const presetLinkPrefix = 'poros://route/'

function failed(reason: string, message: string): CoreError {
  return { code: 1, message, data: { reason } }
}

function encodeLink(copy: PresetCopy): string {
  const data = JSON.stringify({
    kind: 'poros-preset',
    v: 1,
    based_on: copy.basedOn,
    name: copy.name,
    advanced: copy.advanced,
    // The endpoints are this computer's profiles: the link leaves them out, and the rules to them.
    rules: copy.rules.filter((rule) => rule.action !== ActionEndpoint),
    dns: copy.dns,
    direct_dns: copy.directDns,
    default: copy.default,
  })
  return presetLinkPrefix + encodeBase64(data)
}

function encodeBase64(data: string): string {
  const base64 = btoa(String.fromCharCode(...new TextEncoder().encode(data)))
  return base64.replaceAll('+', '-').replaceAll('/', '_').replace(/=+$/, '')
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
    const known = ['kind', 'v', 'based_on', 'name', 'advanced', 'rules', 'dns', 'direct_dns', 'default']
    if (data.kind !== 'poros-preset' || data.v !== 1 || Object.keys(data).some((key) => !known.includes(key))) throw new Error()
    return {
      id: '',
      builtin: false,
      basedOn: String(data.based_on ?? ''),
      name: String(data.name ?? ''),
      advanced: data.advanced === true,
      rules: (data.rules as PresetCopy['rules'] | undefined) ?? [],
      dns: String(data.dns ?? ''),
      directDns: String(data.direct_dns ?? ''),
      default: String(data.default ?? ''),
      inherited: [],
      endpoints: [],
    }
  } catch {
    throw failed(ReasonInvalidLink, 'the link holds no Poros preset')
  }
}

function decodeBase64(data: string): string {
  const bytes = Uint8Array.from(atob(data.replaceAll('-', '+').replaceAll('_', '/')), (c) => c.charCodeAt(0))
  return new TextDecoder().decode(bytes)
}

/** A poros://add/ link holds the profile as JSON in base64url; null for anything else. */
function readPorosLink(data: string): Profile | null {
  try {
    const profile = JSON.parse(decodeBase64(data.trim())) as Profile
    return validProfile(profile) ? profile : null
  } catch {
    return null
  }
}

// The profiles Poros makes up have no address of their own.
const compoundTypes = ['chain', 'custom', 'direct']

/** Enough of profile.Validate for the screens: a type with its part, an address and a port. */
function validProfile(p: Profile): boolean {
  const part = (p as unknown as Record<string, unknown>)[p.type]
  if (typeof part !== 'object' || part === null) return false
  return compoundTypes.includes(p.type) || (!!p.server?.trim() && p.server_port >= 1 && p.server_port <= 65535)
}

/** The forms of core/profile/fields.go, which TestStubForms in core/engine keeps the same. */
const forms = stubForms as ServerForms

/** The rules of servers.check as far as screens need them: Xray for XHTTP and VLESS encryption, the link once valid. */
function checkProfile(p: Profile): ServerCheck {
  const vless = p.vless
  const core = vless && (vless.encryption || vless.transport?.type === 'xhttp') ? CoreXray : CoreSingBox
  if (!forms.forms.some((f) => f.type === p.type)) return { core, problem: `unknown server type "${p.type}"`, link: '' }
  if (!validProfile(p)) return { core, problem: 'the server address is missing', link: '' }
  if (p.type === 'chain' && (p.chain?.servers?.length ?? 0) < 2) return { core, problem: 'a chain needs at least two servers', link: '' }
  const shared = Object.values(shareSchemes).includes(p.type)
  return { core, problem: '', link: p.type === 'chain' ? '' : shared ? writeShareLink(p) : 'poros://add/' + encodeBase64(JSON.stringify(p)) }
}

const shareSchemes: Record<string, string> = {
  vless: 'vless',
  vmess: 'vmess',
  trojan: 'trojan',
  ss: 'shadowsocks',
  hysteria2: 'hysteria2',
  hy2: 'hysteria2',
  tuic: 'tuic',
  socks: 'socks',
  anytls: 'anytls',
  wireguard: 'wireguard',
  wg: 'wireguard',
  hysteria: 'hysteria',
  'naive+https': 'naive',
  'naive+quic': 'naive',
  ssh: 'ssh',
  snell: 'snell',
}

/** A throne://add/ link holds a sing-box outbound with the name as its tag. */
function readThroneLink(data: string): Profile | null {
  try {
    const { tag, type, server, server_port: port, ...rest } = JSON.parse(decodeBase64(data.trim())) as Record<string, unknown>
    if (!Object.values(shareSchemes).includes(type as string)) return null
    const profile = { type, name: tag ?? '', server, server_port: port, [type as string]: rest } as unknown as Profile
    return validProfile(profile) ? profile : null
  } catch {
    return null
  }
}

/** A share link the stub reads back: the scheme, the address and the name, as readShareLink needs. */
function writeShareLink(p: Profile): string {
  const scheme = Object.keys(shareSchemes).find((key) => shareSchemes[key] === p.type)!
  const host = p.server.includes(':') ? `[${p.server}]` : p.server
  return `${scheme}://${host}:${p.server_port}` + (p.name ? '#' + encodeURIComponent(p.name) : '')
}

/** A share link read as far as the stub needs: the scheme gives the protocol, the fragment the name. */
function readShareLink(line: string): Profile | null {
  if (line.toLowerCase().startsWith('poros://add/')) return readPorosLink(line.slice('poros://add/'.length))
  if (line.toLowerCase().startsWith('throne://add/')) return readThroneLink(line.slice('throne://add/'.length))
  try {
    const url = new URL(line)
    const type = shareSchemes[url.protocol.slice(0, -1).toLowerCase()]
    if (!type || !url.hostname || !url.port) return null
    const name = decodeURIComponent(url.hash.slice(1))
    return { type, name, server: url.hostname, server_port: Number(url.port), [type]: {} } as Profile
  } catch {
    return null
  }
}

const regional = (c: string | undefined) => c !== undefined && c.codePointAt(0)! >= 0x1f1e6 && c.codePointAt(0)! <= 0x1f1ff

const noMeasures = { security: '', upload: 0, download: 0, speedDown: '', speedUp: '', exitIp: '', exitCountry: '' }

/** The view of a server, as serverRecord.view in core/engine/servers.go; the stub knows only SOCKS as insecure. */
function serverOf(p: Profile, id: string, groupId: string): Server {
  const name = p.name?.trim() || (compoundTypes.includes(p.type) ? p.type : p.server + ':' + p.server_port)
  const flag = [...name].slice(0, 2)
  const country = regional(flag[0]) && regional(flag[1]) ? flag.map((c) => String.fromCharCode(c.codePointAt(0)! - 0x1f1e6 + 65)).join('') : ''
  const security = p.type === 'socks' ? 'none' : ''
  return { id, groupId, name, protocol: p.type, host: p.server, port: p.server_port, country, latency: 0, ...noMeasures, security }
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
    case 'add': {
      const profile = scheme.toLowerCase() === 'poros' ? readPorosLink(data) : readThroneLink(data)
      if (!profile) throw failed(ReasonInvalidLink, `the ${scheme.toLowerCase()}://add/ link is damaged`)
      return { kind: LinkServer, url: '', name: '', server: serverOf(profile, '', ''), profile }
    }
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
  let groups: Group[] = []
  let servers: Server[] = []
  // The whole profile of each server, for servers.get; subscriptions bring theirs from stubServers.
  const profiles = new Map<string, Profile>()
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
  // As core/engine/groups.go: subscriptions first, each kind in the order it was added; each counts its servers.
  const groupList = (): Group[] =>
    [...groups]
      .sort((a, b) => Number(a.kind === GroupManual) - Number(b.kind === GroupManual) || a.addedAt - b.addedAt)
      .map((g) => ({ ...g, servers: servers.filter((s) => s.groupId === g.id).length }))
  const changed = (...ids: string[]) => {
    emit(EventGroupsChanged, groupList())
    for (const groupId of ids) emit(EventServersChanged, { groupId })
  }
  const find = (id: string) => {
    const group = groups.find((g) => g.id === id)
    if (!group) throw failed(ReasonNotFound, `there is no group "${id}"`)
    return group
  }
  const manual = (id: string) => {
    const group = find(id)
    if (group.kind !== GroupManual) throw failed(ReasonNotManual, `servers go only to a group of the user's, and "${id}" is a subscription`)
    return group
  }
  const newGroup = (name: string): Group => ({
    id: String(nextId++),
    kind: GroupManual,
    name,
    url: '',
    addedAt: Date.now(),
    updatedAt: 0,
    servers: 0,
    skipped: 0,
    upload: 0,
    download: 0,
    total: 0,
    expire: 0,
    webPageUrl: '',
    supportUrl: '',
    announce: '',
    error: '',
    errorText: '',
    settings: { ...noGroupSettings },
  })
  const addServer = (p: Profile, groupId: string, after = -1): Server => {
    const server = serverOf(p, String(nextId++), groupId)
    profiles.set(server.id, p)
    servers = after < 0 ? [...servers, server] : [...servers.slice(0, after + 1), server, ...servers.slice(after + 1)]
    return server
  }
  const findServer = (id: string) => {
    const server = servers.find((s) => s.id === id)
    if (!server) throw failed(ReasonNotFound, `there is no server "${id}"`)
    return server
  }
  // The group of the chosen server before a change, so that a choice that went away stays in that group.
  const chosenGroup = () => servers.find((s) => s.id === settings.server)?.groupId ?? ''
  const presetList = (): Preset[] => [
    ...builtins.map(({ inherited: _, ...preset }) => preset),
    ...copies.map((c) => ({ id: c.id, name: c.name, builtin: false, basedOn: c.basedOn, advanced: c.advanced, rules: c.rules.length, endpoints: c.endpoints.length })),
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
      let tick = 0
      traffic = setInterval(() => {
        const download = Math.round(1_300_000 * (1 + 0.4 * Math.sin(tick++ / 2)))
        total += download
        emit(EventConnectionTraffic, { upload: Math.round(download / 11), download, totalUpload: total / 10, totalDownload: total })
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
    if (
      next.on === block.on &&
      next.full === block.full &&
      next.error === block.error &&
      next.errorText === block.errorText
    ) {
      return
    }
    block = { ...next }
    emit(EventKillSwitchChanged, { ...block })
  }
  // The rules of core/engine/killswitch.go: the kill switch holds VPN traffic without rights; a core without them
  // misses only the TUN of the refusal and asks for them.
  const arm = (): boolean => {
    if (settings.tun && !status.elevated) {
      setBlock({ on: true, full: false, error: ReasonKillSwitchNeedsElevation, errorText: 'the kill switch needs administrator rights' })
      return false
    }
    setBlock({ on: true, full: false, error: '', errorText: '' })
    return true
  }
  const disarm = () => setBlock({ ...noBlock })
  const syncBlock = (previous: string) => {
    if (settings.killSwitch === KillSwitchOff) disarm()
    else if (settings.killSwitch === KillSwitchAlways) arm()
    else if (connection.state === ConnectionRunning) arm()
    else if (previous === KillSwitchAlways) disarm()
  }
  // A new core refuses in the always mode; the connected mode starts with nothing to hold.
  const startBlock = () => {
    if (settings.killSwitch === KillSwitchAlways) arm()
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
  const noChanges = (group: Group): SubscriptionUpdate => ({
    group: groupList().find((g) => g.id === group.id) ?? { ...group },
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
  // As in the core: the chosen server moves on when it goes away, to its group first, else to the first left.
  const keepServerChosen = (groupId: string): void => {
    if (!settings.server || servers.some((s) => s.id === settings.server)) return
    const order = groupList().map((g) => g.id)
    const next = servers.find((s) => s.groupId === groupId) ?? [...servers].sort((a, b) => order.indexOf(a.groupId) - order.indexOf(b.groupId))[0]
    settings = { ...settings, server: next?.id ?? '' }
    emit(EventSettingsChanged, { ...settings })
  }
  const saveCopy = (draft: PresetCopy): Preset => {
    if (draft.builtin) throw failed(ReasonInvalidValue, 'a built-in preset does not change')
    const name = draft.name.trim()
    if (!name) throw failed(ReasonInvalidValue, 'the preset needs a name')
    if (!['', ActionProxy, ActionDirect, ActionBlock].includes(draft.default)) {
      throw failed(ReasonInvalidValue, `the default must be proxy, direct or block, not "${draft.default}"`)
    }
    if (!draft.advanced && draft.directDns.trim()) throw failed(ReasonAdvancedOnly, 'a simple preset has no direct DNS')
    if (!draft.advanced && draft.endpoints.length > 0) throw failed(ReasonAdvancedOnly, 'only an advanced preset has endpoints')
    for (const [i, id] of draft.endpoints.entries()) {
      if (draft.endpoints.indexOf(id) !== i) throw failed(ReasonInvalidValue, `the endpoint "${id}" is there twice`)
      const server = servers.find((s) => s.id === id)
      if (!server) throw failed(ReasonNotFound, `there is no server "${id}"`)
      if (server.protocol !== 'openvpn' && server.protocol !== 'openconnect') throw failed(ReasonInvalidValue, `an endpoint is an OpenVPN or OpenConnect profile`)
    }
    const base = foundation(draft.basedOn)
    if (!base) throw failed(ReasonUnknownPreset, `there is no built-in preset "${draft.basedOn}"`)
    const rules = checkRules({ advanced: draft.advanced, rules: draft.rules, endpoints: draft.endpoints })
    if (nameTaken(name, draft.id)) throw failed(ReasonPresetNameTaken, `there is a preset named "${name}" already`)
    const old = copies.find((c) => c.id === draft.id)
    if (draft.id && !old) throw failed(ReasonNotFound, `there is no preset "${draft.id}"`)
    if (old?.advanced && !draft.advanced) throw failed(ReasonInvalidValue, 'an advanced preset does not become simple')
    const copy: PresetCopy = {
      ...draft,
      name,
      rules,
      dns: draft.dns.trim(),
      directDns: draft.directDns.trim(),
      endpoints: [...draft.endpoints],
      id: draft.id || String(nextId++),
      inherited: base.inherited,
    }
    copies = draft.id ? copies.map((c) => (c.id === draft.id ? copy : c)) : [...copies, copy]
    emit(EventPresetsChanged, presetList())
    return presetList().find((p) => p.id === copy.id)!
  }
  const copyOf = (id: string): PresetCopy => {
    const base = builtins.find((b) => b.id === id)
    if (base) {
      return {
        id,
        builtin: true,
        basedOn: '',
        name: base.name,
        advanced: false,
        rules: [],
        dns: '',
        directDns: '',
        default: ActionProxy,
        inherited: base.inherited,
        endpoints: [],
      }
    }
    const copy = copies.find((c) => c.id === id)
    if (!copy) throw failed(ReasonNotFound, `there is no preset "${id}"`)
    return structuredClone({ ...copy, default: copy.default || ActionProxy })
  }

  const methods: { [M in CoreMethod]: (params: CoreMethods[M]['params']) => CoreMethods[M]['result'] } = {
    [MethodAppInfo]: () => ({ singBox: 'stub', xray: 'stub', dataDir: 'browser', elevated: status.elevated }),
    [MethodAppDebugInfo]: () => `Poros stub\nsing-box stub, Xray stub\nSettings: ${JSON.stringify({ ...settings, proxyPassword: '' }, null, 2)}\n`,
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
      checkDnsSettings(next)
      const previous = settings.killSwitch
      settings = { ...next }
      emit(EventSettingsChanged, { ...settings })
      if (settings.killSwitch !== previous) setTimeout(() => syncBlock(previous), 0)
      return { ...settings }
    },
    [MethodGroupsList]: () => groupList(),
    [MethodGroupsAdd]: ({ name }) => {
      if (!name.trim()) throw failed(ReasonInvalidValue, 'a group needs a name')
      const group = newGroup(name.trim())
      groups = [...groups, group]
      changed()
      return groupList().find((g) => g.id === group.id)!
    },
    [MethodGroupsRename]: ({ id, name }) => {
      const group = find(id)
      if (!name.trim() && group.kind === GroupManual) throw failed(ReasonInvalidValue, 'a group needs a name')
      const renamed = { ...group, name: name.trim() || new URL(group.url).hostname }
      groups = groups.map((g) => (g.id === id ? renamed : g))
      changed()
      return groupList().find((g) => g.id === id)!
    },
    [MethodGroupsConfigure]: ({ id, url, settings }) => {
      const group = find(id)
      if (group.kind === GroupManual && url) throw failed(ReasonInvalidValue, "a group of the user's has no URL")
      if (settings.frontProxy && settings.frontProxy === settings.landingProxy) {
        throw failed(ReasonInvalidValue, 'the front and the landing proxy are two servers')
      }
      if ((settings.removeUnavailable || settings.sortByLatency) && !settings.test) {
        throw failed(ReasonInvalidValue, 'removing unavailable servers and sorting by delay go by the test after the update')
      }
      const configured = { ...group, url: url.trim() || group.url, settings: { ...settings } }
      groups = groups.map((g) => (g.id === id ? configured : g))
      changed(id)
      return groupList().find((g) => g.id === id)!
    },
    [MethodGroupsRemove]: ({ id }) => {
      find(id)
      groups = groups.filter((g) => g.id !== id)
      servers = servers.filter((s) => s.groupId !== id)
      changed(id)
      keepServerChosen('')
      return null
    },
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
      if (groups.some((g) => g.url === link)) throw failed(ReasonDuplicate, 'this subscription is already added')
      const now = Date.now()
      const sub: Group = {
        ...newGroup(name.trim() || host),
        kind: GroupSubscription,
        url: link,
        addedAt: now,
        updatedAt: now,
        upload: 20_000_000_000,
        download: 18_200_000_000,
        total: 100_000_000_000,
        expire: now + 35 * 24 * 3600 * 1000,
      }
      groups = [...groups, sub]
      const added = stubServers.map(({ measured: _, ...s }) =>
        addServer({ type: s.protocol, name: s.name, server: s.host, server_port: s.port, [s.protocol]: {} } as Profile, sub.id),
      )
      changed(sub.id)
      return { ...noChanges(sub), added: added.length, addedNames: added.map((s) => s.name) }
    },
    [MethodSubscriptionsUpdate]: ({ id }) => {
      const group = find(id)
      if (group.kind === GroupManual) throw failed(ReasonInvalidValue, `the group "${id}" is no subscription`)
      const sub = { ...group, updatedAt: Date.now() }
      groups = groups.map((g) => (g.id === id ? sub : g))
      changed(id)
      return noChanges(sub)
    },
    [MethodServersList]: ({ groupId }) => {
      const order = groupList().map((g) => g.id)
      return servers
        .filter((s) => groupId === '' || s.groupId === groupId)
        .sort((a, b) => order.indexOf(a.groupId) - order.indexOf(b.groupId))
        .map((s) => ({ ...s }))
    },
    [MethodServersAdd]: ({ groupId, newGroup: newName, text }) => {
      const body = text.trim()
      if (/^https?:\/\/[^\s:@/]+(\/\S*)?$/i.test(body)) throw failed(ReasonSubscriptionURL, "this is a subscription's link: add it as a subscription")
      const lines = body.split('\n').map((line) => line.trim()).filter((line) => line && !line.startsWith('#'))
      const read = lines.map(readShareLink)
      const found = read.filter((p): p is Profile => p !== null)
      if (found.length === 0) throw failed(ReasonNoServers, 'the text holds no servers')
      if (!groupId && !newName.trim()) throw failed(ReasonInvalidValue, 'a group needs a name')
      let group: Group
      if (groupId) group = manual(groupId)
      else {
        group = newGroup(newName.trim())
        groups = [...groups, group]
      }
      const ids = found.map((p) => addServer(p, group.id).id)
      const broken = read.length - found.length
      const skips: SkipReason[] = broken ? [{ reason: 'the link could not be read', unsupported: false, count: broken, names: [] }] : []
      changed(group.id)
      return { group: groupList().find((g) => g.id === group.id)!, ids, skips }
    },
    [MethodServersGet]: ({ id }) => {
      const server = findServer(id)
      return { server: { ...server }, profile: structuredClone(profiles.get(id)!) }
    },
    [MethodServersSave]: ({ id, groupId, profile }) => {
      if (!forms.forms.some((f) => f.type === profile.type)) throw failed(ReasonServerUnsupported, `unknown server type "${profile.type}"`)
      const problem = checkProfile(profile).problem
      if (problem) throw failed(ReasonInvalidValue, problem)
      const before = chosenGroup()
      let saved: Server
      if (id) {
        const old = findServer(id)
        saved = { ...serverOf(profile, id, old.groupId), latency: old.latency }
        profiles.set(id, profile)
        servers = servers.map((s) => (s.id === id ? saved : s))
      } else saved = addServer(profile, manual(groupId).id)
      changed(saved.groupId)
      keepServerChosen(before)
      return { ...saved }
    },
    [MethodServersRemove]: ({ ids }) => {
      const before = chosenGroup()
      const touched = [...new Set(servers.filter((s) => ids.includes(s.id)).map((s) => s.groupId))]
      servers = servers.filter((s) => !ids.includes(s.id))
      if (touched.length) {
        changed(...touched)
        keepServerChosen(before)
      }
      return null
    },
    [MethodServersClone]: ({ ids }) => {
      const copies = ids.map((id) => {
        const server = findServer(id)
        return addServer(structuredClone(profiles.get(id)!), server.groupId, servers.indexOf(server)).id
      })
      changed(...new Set(copies.map((id) => findServer(id).groupId)))
      return { ids: copies }
    },
    [MethodServersMove]: ({ ids, groupId }) => {
      const target = manual(groupId)
      const touched = new Set([target.id])
      const moved = ids.map((id) => {
        const server = findServer(id)
        if (server.groupId === target.id) return id
        if (find(server.groupId).kind === GroupManual) {
          touched.add(server.groupId)
          servers = [...servers.filter((s) => s.id !== id), { ...server, groupId: target.id }]
          return id
        }
        return addServer(structuredClone(profiles.get(id)!), target.id).id
      })
      changed(...touched)
      return { ids: moved }
    },
    [MethodServersLinks]: ({ ids }) => ({ links: ids.map((id) => (findServer(id), writeShareLink(profiles.get(id)!))) }),
    [MethodServersDeepLinks]: ({ ids }) => ({
      links: ids.map((id) => (findServer(id), 'poros://add/' + encodeBase64(JSON.stringify(profiles.get(id)!)))),
    }),
    [MethodServersForms]: () => structuredClone(forms),
    [MethodServersCheck]: ({ profile }) => checkProfile(profile),
    [MethodServersExport]: ({ id }) => {
      findServer(id)
      const { type, server, server_port, [type]: part } = profiles.get(id)! as unknown as Record<string, unknown> & { type: string }
      const outbound = { type, tag: 'proxy', server, server_port, ...(part as object) }
      return { core: CoreSingBox, json: JSON.stringify(outbound, null, 2) }
    },
    [MethodServersTest]: ({ groupId, ids }) => {
      const tested = servers.filter((s) => (ids?.length ? ids.includes(s.id) : groupId === '' || s.groupId === groupId))
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
    [MethodServersSpeedTest]: ({ groupId, ids }) => {
      const tested = servers.filter((s) => (ids?.length ? ids.includes(s.id) : groupId === '' || s.groupId === groupId))
      setTimeout(() => {
        const results = tested.map((s) => ({ serverId: s.id, download: '84.2 Mbps', upload: '21.7 Mbps', country: s.country, error: '' }))
        servers = servers.map((s) => (tested.includes(s) ? { ...s, speedDown: '84.2 Mbps', speedUp: '21.7 Mbps' } : s))
        emit(EventServersSpeedTested, { results, done: true })
      }, 0)
      return null
    },
    [MethodServersIPTest]: ({ groupId, ids }) => {
      const tested = servers.filter((s) => (ids?.length ? ids.includes(s.id) : groupId === '' || s.groupId === groupId))
      setTimeout(() => {
        const results = tested.map((s) => ({ serverId: s.id, ip: '203.0.113.' + (Number(s.id) % 250), country: s.country, error: '' }))
        servers = servers.map((s) => {
          const result = results.find((r) => r.serverId === s.id)
          return result ? { ...s, exitIp: result.ip, exitCountry: result.country } : s
        })
        emit(EventServersIPTested, { results, done: true })
      }, 0)
      return null
    },
    [MethodServersResolve]: () => ({ resolved: 0 }),
    [MethodServersClearResults]: ({ groupId, ids }) => {
      const picked = (s: Server) => (ids?.length ? ids.includes(s.id) : groupId === '' || s.groupId === groupId)
      servers = servers.map((s) => (picked(s) ? { ...s, latency: 0, speedDown: '', speedUp: '', exitIp: '', exitCountry: '' } : s))
      changed(...new Set(servers.filter(picked).map((s) => s.groupId)))
      return null
    },
    [MethodServersStopTests]: () => null,
    [MethodServersResetTraffic]: ({ groupId, ids }) => {
      const picked = (s: Server) => (ids?.length ? ids.includes(s.id) : groupId === '' || s.groupId === groupId)
      servers = servers.map((s) => (picked(s) ? { ...s, upload: 0, download: 0 } : s))
      changed(...new Set(servers.filter(picked).map((s) => s.groupId)))
      return null
    },
    [MethodServersClean]: ({ groupId, unavailable, insecure }) => {
      const gone = servers.filter(
        (s) => (groupId === '' || s.groupId === groupId) && ((unavailable && s.latency < 0) || (insecure && s.security !== '')),
      )
      servers = servers.filter((s) => !gone.includes(s))
      changed(...new Set(gone.map((s) => s.groupId)))
      return { ids: gone.map((s) => s.id) }
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
    [MethodPresetsExport]: ({ id }) => {
      const copy = copyOf(id)
      if (copy.builtin) throw failed(ReasonInvalidValue, 'a built-in preset is in every Poros already')
      return { link: encodeLink(copy) }
    },
    [MethodPresetsImport]: ({ link }) => {
      const copy = decodeLink(link)
      return saveCopy({ ...copy, name: freeName(copy.name) })
    },
    // A device made up as Cloudflare would give it: the stub asks no one.
    [MethodWARPRegister]: ({ mode }) => {
      if (![WARPModeWireGuard, WARPModeMASQUE].includes(mode)) throw failed(ReasonInvalidValue, `mode must be wireguard or masque, not "${mode}"`)
      const key = btoa(String.fromCharCode(...crypto.getRandomValues(new Uint8Array(32))))
      const common = { privateKey: key, addresses: '172.16.0.2/32, 2606:4700:110:8a2c::2/128', registeredAt: Date.now() }
      settings =
        mode === WARPModeWireGuard
          ? {
              ...settings,
              warpWireGuard: { ...noWarpDevice, ...common, endpoint: '162.159.192.1:2408', peerKey: 'bmQzcGxhY2Vob2xkZXJwbGFjZWhvbGRlcnBsYWNlaG8=', reserved: '12, 85, 201' },
            }
          : { ...settings, warpMasque: { ...settings.warpMasque, ...common, endpoint: '162.159.198.2:443', peerKey: 'MFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE' } }
      emit(EventSettingsChanged, { ...settings })
      return { ...settings }
    },
    [MethodDNSCheck]: ({ object }) => {
      if (!object.trim()) throw failed(ReasonInvalidValue, 'the dns object is empty')
      checkDnsObject(object)
      return null
    },
    [MethodPresetsCheck]: (rules) => ({ advanced: rules.advanced, rules: checkRules(rules), endpoints: rules.endpoints }),
    [MethodPresetsWriteLists]: (rules) => writeLists(rules),
    [MethodPresetsReadLists]: (input) => readLists(input),
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
        // The core asks only where a server takes the forged handshake; every stub server does.
        if (settings.spoofDefault && !status.elevated) {
          return { ...idle, error: ReasonSpoofNeedsElevation, errorText: 'a forged TLS handshake needs administrator rights' }
        }
        if (settings.killSwitch !== KillSwitchOff) arm()
        return {
          ...idle,
          state: ConnectionRunning,
          serverId: settings.server,
          servers: [
            settings.server,
            ...(profiles.get(settings.server)?.chain?.servers ?? []),
            ...(copies.find((c) => c.id === settings.preset)?.endpoints ?? []),
          ],
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
        if (settings.killSwitch === KillSwitchAlways) arm()
        else disarm()
        return { ...idle }
      })
      return { ...connection }
    },
    [MethodConnectionState]: () => ({ ...connection }),
    // Connections as on the mockup, their bytes growing with the time since the start.
    [MethodConnectionsList]: () => {
      if (connection.state !== ConnectionRunning || !settings.stats) return { connections: [], tracked: false }
      const seconds = (Date.now() - connection.since) / 1000
      return {
        tracked: true,
        connections: stubConnections
          .filter((c) => !closedConnections.has(c.id))
          .map((c) => ({ ...c, start: connection.since + c.start, upload: Math.round(c.upload * (1 + seconds)), download: Math.round(c.download * (1 + seconds)) })),
      }
    },
    [MethodConnectionsClose]: ({ id }) => {
      if (id && (!stubConnections.some((c) => c.id === id) || closedConnections.has(id))) throw failed(ReasonNotFound, `there is no connection "${id}"`)
      for (const c of stubConnections) if (!id || c.id === id) closedConnections.add(c.id)
      return null
    },
    [MethodNetworkCheck]: () => {
      if (!settings.checkIp) throw failed(ReasonCheckOff, 'the IP and DNS check is turned off')
      return checkNetwork()
    },
    [MethodNetworkState]: () => ({ ...network }),
    // The stub's byte of the log is the number of its lines.
    [MethodLogsGet]: ({ after }) => {
      const reset = after > stubLog.length
      return { lines: stubLog.slice(reset || after === 0 ? 0 : after), next: stubLog.length, reset }
    },
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
