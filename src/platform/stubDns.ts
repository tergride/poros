import type { CoreError } from './core'
import type { Settings } from './core-types'
import { ReasonInvalidValue } from './core-types'

// The DNS settings as core/engine/dns.go and core/config/dns.go check them, close enough for screens and tests.

function invalid(message: string): CoreError {
  return { code: 1, message, data: { reason: ReasonInvalidValue } }
}

const ip = /^(\d{1,3}(\.\d{1,3}){3}|[0-9a-f:]*:[0-9a-f:.]*)$/i
const host = /^[\p{L}\p{N}.-]+$/u

/** As config.CheckDNSServer: https://host[:port][/path], tls://host[:port] or an address. */
function dnsServerProblem(text: string): string {
  const [scheme, rest] = text.includes('://') ? [text.slice(0, text.indexOf('://')).toLowerCase(), text.slice(text.indexOf('://') + 3)] : ['udp', text]
  const [authority = '', ...path] = rest.split('/')
  const name = authority.replace(/:\d+$/, '').replace(/^\[|\]$/g, '')
  const bad = `"${text}" is not a DNS server: write https://host/path, tls://host or an address`
  if (!name || !(ip.test(name) || host.test(name))) return bad
  if (scheme === 'https') return ''
  if (scheme === 'tls') return path.length === 0 ? '' : bad
  if (scheme === 'udp') return path.length === 0 && ip.test(name) ? '' : bad
  return bad
}

/** As config.ParseAnswers: a line each, a name and its addresses. */
function answersProblem(text: string): string {
  const names = new Set<string>()
  for (const [i, line] of text.split('\n').entries()) {
    const fields = line.trim().split(/\s+/).filter(Boolean)
    if (fields.length === 0 || fields[0]!.startsWith('#')) continue
    const name = fields[0]!.toLowerCase().replace(/\.$/, '')
    if (fields.length < 2) return `line ${i + 1}: a name and its addresses`
    const wrong = fields.slice(1).find((field) => !ip.test(field))
    if (wrong) return `line ${i + 1}: "${wrong}" is no address`
    if (names.has(name)) return `line ${i + 1}: ${name} is there already`
    names.add(name)
  }
  return ''
}

/** As checkDNSReferences in core/config/dns.go, after the JSON is read. */
export function checkDnsObject(text: string): void {
  let object: { servers?: Record<string, unknown>[]; rules?: Record<string, unknown>[]; final?: string }
  try {
    object = JSON.parse(text) as typeof object
  } catch (error) {
    throw invalid(`sing-box cannot read the dns object: ${String(error)}`)
  }
  if (typeof object !== 'object' || object === null || Array.isArray(object)) throw invalid('sing-box cannot read the dns object')
  const tags = new Set((object.servers ?? []).map((s) => String(s.tag ?? '')).filter(Boolean))
  for (const [i, server] of (object.servers ?? []).entries()) {
    const detour = String(server.detour ?? '')
    if (detour && !['proxy', 'direct'].includes(detour) && !/^endpoint-[1-9]\d*$/.test(detour)) {
      throw invalid(`servers[${i}]: unknown outbound "${detour}"; a server goes through the server with "detour": "proxy"`)
    }
  }
  for (const [i, rule] of (object.rules ?? []).entries()) {
    const server = String(rule.server ?? '')
    if (server && !tags.has(server)) throw invalid(`rules[${i}]: unknown DNS server "${server}"`)
  }
  if (object.final && !tags.has(object.final)) throw invalid(`final: unknown DNS server "${object.final}"`)
}

/** As validateDNS in core/engine/dns.go. */
export function checkDnsSettings(s: Settings): void {
  for (const [name, server] of [
    ['dnsRemote', s.dnsRemote],
    ['dnsDirect', s.dnsDirect],
  ] as const) {
    const problem = server ? dnsServerProblem(server) : ''
    if (problem) throw invalid(`${name}: ${problem}`)
  }
  if (!['auto', 'proxy', 'direct'].includes(s.dnsRouting)) throw invalid(`dnsRouting must be auto, proxy or direct, not "${s.dnsRouting}"`)
  if (!['', 'ipv4', 'ipv6'].includes(s.addressFirst)) throw invalid(`addressFirst must be empty, ipv4 or ipv6, not "${s.addressFirst}"`)
  const answers = answersProblem(s.dnsAnswers)
  if (answers) throw invalid(`dnsAnswers: ${answers}`)
  if (!Number.isInteger(s.dnsCacheSize) || s.dnsCacheSize < 1 || s.dnsCacheSize > 1 << 20) throw invalid('dnsCacheSize is out of range')
  if (s.dnsIgnoreTtl && s.dnsOptimistic) throw invalid('an optimistic cache needs answers that get outdated: not with dnsIgnoreTtl')
  if (!Number.isInteger(s.dnsTimeout) || s.dnsTimeout < 1 || s.dnsTimeout > 60) throw invalid('dnsTimeout must be from 1 to 60 seconds')
  const subnet = s.dnsClientSubnet.trim()
  if (subnet && !ip.test(subnet.split('/')[0]!)) throw invalid(`dnsClientSubnet: "${subnet}" is no subnet`)
  if (s.dnsObject) checkDnsObject(s.dnsObject)
  // As validateTun in core/engine/tun.go, the formats left to the core.
  if (!Number.isInteger(s.tunMtu) || s.tunMtu < 1280 || s.tunMtu > 9000) throw invalid(`tunMtu must be from 1280 to 9000, not ${s.tunMtu}`)
  if (!/^\d{1,3}(\.\d{1,3}){3}\/\d{1,2}$/.test(s.tunIpv4.trim())) throw invalid(`tunIpv4: "${s.tunIpv4}" is no address of the TUN with its subnet`)
  if (s.tunBypass.split('\n').some((line) => ['0.0.0.0/0', '::/0'].includes(line.trim()))) throw invalid('tunBypass covers every address')
  // As validateInbound in core/engine/inbound.go.
  if (!Number.isInteger(s.proxyPort) || s.proxyPort < 0 || s.proxyPort > 65535) throw invalid(`proxyPort must be from 1 to 65535, not ${s.proxyPort}`)
  if (s.proxyAuth && (!s.proxyUsername.trim() || !s.proxyPassword)) throw invalid('a local proxy with authorization needs a username and a password')
  if (s.proxyUsername.includes(':')) throw invalid('the username of the local proxy cannot hold a colon')
  if (s.latencyUrl && !/^https?:\/\//.test(s.latencyUrl)) throw invalid(`the delay test asks for an http:// or https:// address, not "${s.latencyUrl}"`)
  if (!Number.isInteger(s.latencyTimeout) || s.latencyTimeout < 500 || s.latencyTimeout > 30000) throw invalid('latencyTimeout must be from 500 to 30000 ms')
  if (!Number.isInteger(s.latencyConcurrency) || s.latencyConcurrency < 1 || s.latencyConcurrency > 64) throw invalid('latencyConcurrency must be from 1 to 64')
  // As validateProtocols in core/engine/protocols.go, the formats of QUIC left to the core.
  if (!['smux', 'yamux', 'h2mux'].includes(s.muxProtocol)) throw invalid(`muxProtocol must be smux, yamux or h2mux, not "${s.muxProtocol}"`)
  if (!Number.isInteger(s.muxConcurrency) || s.muxConcurrency < 1 || s.muxConcurrency > 128) throw invalid(`muxConcurrency must be from 1 to 128, not ${s.muxConcurrency}`)
  if (s.spoofDefault && !s.spoofSni.trim()) throw invalid('a forged handshake by default needs the name to forge')
  const packet = s.quicInitialPacketSize
  if (s.quicMaxStreams < 0 || packet < 0 || (packet !== 0 && (packet < 1200 || packet > 65000))) throw invalid('the streams are none or more, the initial packet from 1200 to 65000 bytes')
  // As validateNTP in core/engine/ntp.go.
  if (!['direct', 'proxy'].includes(s.ntpOutbound)) throw invalid(`ntpOutbound must be direct or proxy, not "${s.ntpOutbound}"`)
  if ((s.ntp && !s.ntpServer.trim()) || /[\s/\\@]/.test(s.ntpServer.trim())) throw invalid(`the NTP server "${s.ntpServer}" is no host name or address`)
  if (!Number.isInteger(s.ntpPort) || s.ntpPort < 1 || s.ntpPort > 65535) throw invalid(`ntpPort must be from 1 to 65535, not ${s.ntpPort}`)
  if (!Number.isInteger(s.ntpInterval) || s.ntpInterval < 1 || s.ntpInterval > 1440) throw invalid(`ntpInterval must be from 1 to 1440 minutes, not ${s.ntpInterval}`)
  // As validateWARP in core/engine/warp.go, the formats left to the core.
  if (!['wireguard', 'masque'].includes(s.warpMode)) throw invalid(`warpMode must be wireguard or masque, not "${s.warpMode}"`)
  const device = s.warpMode === 'masque' ? s.warpMasque : s.warpWireGuard
  if (s.warp && (!device.endpoint || !device.privateKey || !device.peerKey || !device.addresses)) throw invalid('the device of WARP is not whole')
  const reserved = s.warpWireGuard.reserved.split(',').map((b) => b.trim()).filter(Boolean)
  if (reserved.length !== 0 && (reserved.length !== 3 || reserved.some((b) => !/^\d+$/.test(b) || Number(b) > 255))) throw invalid('WARP takes three reserved bytes')
}
