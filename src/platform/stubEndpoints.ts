import type { CoreError } from './core'
import type { Endpoint, EndpointEntry, EndpointExport, EndpointFile, EndpointMove, EndpointText } from './core-types'
import {
  ReasonEndpointNameTaken,
  ReasonInvalidLink,
  ReasonInvalidValue,
  ReasonNotFound,
  ReasonSignInMissing,
  SignInCA,
  SignInCert,
  SignInKey,
  SignInKeyPassword,
  SignInOTP,
  SignInPassword,
  SignInUsername,
  SignInWrapKey,
} from './core-types'
import type { Profile } from './profile-types'
import { checkRules } from './stubPresets'

// The endpoints of the stub core as core/engine/endpoints.go keeps them: the list in the user's order, the data for
// signing in a file leaves out, and the files themselves.

const signInOrder = [SignInUsername, SignInPassword, SignInCA, SignInCert, SignInKey, SignInKeyPassword, SignInWrapKey, SignInOTP]
const endpointTypes = ['openvpn', 'openvpn-by', 'openconnect']
const btlsSuites = ['DHE-BIGN-WITH-BELT-DWP-HBELT', 'DHT-BIGN-WITH-BELT-DWP-HBELT', 'DHE-BIGN-WITH-BELT-CTR-MAC-HBELT', 'DHT-BIGN-WITH-BELT-CTR-MAC-HBELT']

function failed(reason: string, message: string): CoreError {
  return { code: 1, message, data: { reason } }
}

type Part = Record<string, unknown> & { control_wrap?: { type?: string; key?: string } }

/** The key of a type's own part, as PartOf of core/profile names it. */
export const partKey = (type: string) => (type === 'openvpn-by' ? 'openvpn' : type)

/** The fields of the data for signing in of a profile, as get and set over its part. */
function signInFields(profile: Profile): Map<string, { get: () => string; set: (value: string) => void }> {
  const part = (profile as unknown as Record<string, Part | undefined>)[partKey(profile.type)]
  const fields = new Map<string, { get: () => string; set: (value: string) => void }>()
  if (!part || !endpointTypes.includes(profile.type)) return fields
  const field = (name: string, key: string) =>
    fields.set(name, { get: () => String(part[key] ?? ''), set: (value) => (value ? (part[key] = value) : delete part[key]) })
  field(SignInUsername, 'username')
  field(SignInPassword, 'password')
  field(SignInCert, 'cert')
  field(SignInKey, 'key')
  field(SignInOTP, 'otp')
  if (profile.type !== 'openconnect') field(SignInCA, 'ca')
  if (profile.type !== 'openvpn') field(SignInKeyPassword, 'key_password')
  if (profile.type !== 'openconnect' && part.control_wrap?.type) {
    const wrap = part.control_wrap
    fields.set(SignInWrapKey, { get: () => wrap.key ?? '', set: (value) => (wrap.key = value) })
  }
  return fields
}

// A code of OTP is needed only while {otp} may sign in: the name or the password has it or is still missing. The
// password of the key only while the key has one or is not there to tell.
function stillNeeded(need: string[], profile: Profile): string[] {
  const fields = signInFields(profile)
  const missing = (name: string) => need.includes(name) && fields.get(name)?.get() === ''
  const otp = [SignInUsername, SignInPassword].some((name) => missing(name) || fields.get(name)?.get().includes('{otp}'))
  const key = fields.get(SignInKey)?.get() ?? ''
  const locked = key === '' || key.includes('ENCRYPTED')
  return signInOrder.filter((name) => missing(name) && (name !== SignInOTP || otp) && (name !== SignInKeyPassword || locked))
}

// The user's own data for signing in, which a file without it leaves out: not the CA and the keys of tls-auth and
// tls-crypt, the server's, nor the code of OTP, this computer's.
function personal(profile: Profile, name: string): boolean {
  if (name === SignInOTP || name === SignInCA) return false
  return name !== SignInWrapKey || (profile as unknown as Record<string, Part | undefined>).openvpn?.control_wrap?.type === 'tls_crypt_v2'
}

function fileName(name: string): string {
  // eslint-disable-next-line no-control-regex
  return name.replace(/[<>:"/\\|?*\x00-\x1f]/g, '_').trim().replace(/[. ]+$/, '') || 'preset'
}

/**
 * Enough of an .ovpn file for the screens: the first remote, the protocol, the name, the CA, auth-user-pass, the
 * cryptography of OpenVPN|BY and the keys read from files, which it needs.
 */
export function readOpenVPN(text: string): EndpointEntry | null {
  const remote = /^\s*remote\s+(\S+)(?:\s+(\d+))?/m.exec(text)
  if (!remote || !/^\s*(client|dev)\b/m.test(text)) return null
  const value = (directive: string) => new RegExp(`^\\s*${directive}\\s+(\\S+)`, 'm').exec(text)?.[1]
  const name = /^\s*setenv\s+FRIENDLY_NAME\s+(.+)$/m.exec(text)?.[1]?.trim() ?? ''
  const ca = /<ca>\s*([\s\S]*?)\s*<\/ca>/i.exec(text)?.[1] ?? ''
  const signIn = /^\s*auth-user-pass\s*$/m.test(text)
  const by = /\b(BIGN|belt-|bash\d)/i.test(text)
  const part: Record<string, unknown> = { network: value('proto') ?? 'udp', ca }
  if (by) Object.assign(part, { tls_cipher: value('tls-cipher')?.split(':') ?? btlsSuites, data_ciphers: value('data-ciphers')?.split(':'), auth: value('auth') })
  const profile = { type: by ? 'openvpn-by' : 'openvpn', name: name || remote[1]!, server: remote[1]!, server_port: Number(remote[2] ?? 1194), openvpn: part }
  const files = [
    [SignInCA, 'ca'],
    [SignInCert, 'cert'],
    [SignInKey, 'key'],
  ].filter(([, directive]) => value(directive!) !== undefined && value(directive!) !== '[inline]')
  const need = [...(signIn ? [SignInUsername, SignInPassword] : []), ...files.map(([n]) => n!), ...(by ? [SignInKeyPassword] : [])]
  return { id: '', on: false, profile: profile as unknown as Profile, rules: [], need }
}

export function createStubEndpoints(changed: (list: Endpoint[]) => void, nextId: () => string) {
  let records: EndpointEntry[] = []
  const view = (r: EndpointEntry): Endpoint => ({ id: r.id, name: r.profile.name ?? '', type: r.profile.type, on: r.on, rules: r.rules.length, need: [...r.need] })
  const list = () => records.map(view)
  const announce = () => changed(list())
  const index = (id: string) => {
    const i = records.findIndex((r) => r.id === id)
    if (i < 0) throw failed(ReasonNotFound, `there is no endpoint "${id}"`)
    return i
  }
  const taken = (name: string, id: string) => records.some((r) => r.id !== id && (r.profile.name ?? '').toLowerCase() === name.toLowerCase())

  function check(entry: EndpointEntry): EndpointEntry {
    const profile = structuredClone(entry.profile)
    profile.name = (profile.name ?? '').trim()
    if (!endpointTypes.includes(profile.type)) throw failed(ReasonInvalidValue, 'an endpoint is an OpenVPN, OpenVPN|BY or OpenConnect connection')
    if (!profile.name) throw failed(ReasonInvalidValue, 'the endpoint needs a name')
    if (!profile.server?.trim() || !(profile.server_port >= 1 && profile.server_port <= 65535)) throw failed(ReasonInvalidValue, `${profile.name}: the server is missing`)
    const rules = checkRules({ advanced: true, endpoint: true, rules: entry.rules })
    const need = stillNeeded(entry.need, profile)
    if (entry.on && need.length > 0) throw failed(ReasonSignInMissing, `${profile.name} lacks data for signing in: ${need.join(', ')}`)
    return { id: entry.id, on: entry.on, profile, rules, need }
  }

  function save(entry: EndpointEntry): Endpoint {
    const record = check(entry)
    const name = record.profile.name ?? ''
    if (taken(name, record.id)) throw failed(ReasonEndpointNameTaken, `there is an endpoint named "${name}" already`)
    if (record.id) records[index(record.id)] = record
    else record.id = nextId()
    if (!records.includes(record)) records = [...records.filter((r) => r.id !== record.id), record]
    announce()
    return view(record)
  }

  return {
    list,
    get: (id: string): EndpointEntry => structuredClone(records[index(id)]!),
    save,
    remove(id: string): null {
      records.splice(index(id), 1)
      announce()
      return null
    },
    move({ id, offset }: EndpointMove): null {
      const i = index(id)
      const to = i + offset
      if (to >= 0 && to < records.length) [records[i], records[to]] = [records[to]!, records[i]!]
      announce()
      return null
    },
    exportFile({ id, signIn }: EndpointExport): EndpointFile {
      const entry = structuredClone(records[index(id)]!)
      const fields = signInFields(entry.profile)
      const need = [...entry.need]
      if (fields.get(SignInOTP)?.get()) {
        fields.get(SignInOTP)!.set('')
        need.push(SignInOTP)
      }
      if (!signIn) {
        for (const name of signInOrder) {
          const field = fields.get(name)
          if (personal(entry.profile, name) && field?.get()) {
            field.set('')
            need.push(name)
          }
        }
      }
      const file = { kind: 'poros-endpoint', v: 1, profile: entry.profile, rules: entry.rules, need: stillNeeded(need, entry.profile) }
      return { name: `${fileName(entry.profile.name ?? '')}.json`, text: `${JSON.stringify(file, null, 2)}\n` }
    },
    importFile({ text }: EndpointText): Endpoint {
      let entry: EndpointEntry | null = null
      if (text.trim().startsWith('{')) {
        try {
          const data = JSON.parse(text) as Record<string, unknown>
          const known = ['kind', 'v', 'profile', 'rules', 'need']
          if (data.kind === 'poros-endpoint' && data.v === 1 && Object.keys(data).every((key) => known.includes(key))) {
            entry = { id: '', on: false, profile: data.profile as Profile, rules: (data.rules as EndpointEntry['rules']) ?? [], need: (data.need as string[]) ?? [] }
          }
        } catch {
          entry = null
        }
      } else {
        entry = readOpenVPN(text)
      }
      if (!entry) throw failed(ReasonInvalidLink, 'it holds no Poros endpoint and no OpenVPN config')
      const fields = signInFields(entry.profile)
      if (fields.has(SignInOTP)) {
        fields.get(SignInOTP)!.set('')
        if ([SignInUsername, SignInPassword].some((name) => fields.get(name)?.get().includes('{otp}'))) entry.need.push(SignInOTP)
      }
      const base = (entry.profile.name ?? '').trim() || entry.profile.server
      let name = base
      for (let n = 2; taken(name, ''); n++) name = `${base} ${n}`
      entry.profile.name = name
      return save({ ...entry, on: false })
    },
    /** The endpoints a connection runs with, in their order. */
    on: (): EndpointEntry[] => records.filter((r) => r.on && r.need.length === 0),
    replace(next: EndpointEntry[]): void {
      records = structuredClone(next)
      announce()
    },
    all: (): EndpointEntry[] => structuredClone(records),
  }
}
