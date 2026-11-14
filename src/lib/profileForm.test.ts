import { describe, expect, it } from 'vitest'

import type { Profile, ProfileForm } from '@/platform/profile-types'
import stubForms from '@/platform/stubForms.json'

import { cleanProfile, fieldState, getAt, jsonErrorLine, parseBytes, parseHeaders, securityOf, setAt, setSecurity } from './profileForm'

const vlessForm = (stubForms.forms as ProfileForm[]).find((f) => f.type === 'vless')!
const field = (path: string, at = 0) => vlessForm.fields.filter((f) => f.path === path)[at]!

describe('the profile editor', () => {
  it('writes a value by its path and leaves no empty objects below the part of the protocol', () => {
    const p: Record<string, unknown> = { type: 'vless', vless: {} }
    setAt(p, 'vless.tls.reality.enabled', true)
    expect(getAt(p, 'vless.tls.reality.enabled')).toBe(true)
    setAt(p, 'vless.tls.reality.enabled', false)
    expect(p).toEqual({ type: 'vless', vless: {} })
    setAt(p, 'name', '')
    setAt(p, 'vless.tls.alpn', [])
    expect(p).toEqual({ type: 'vless', vless: {} })
  })

  it('turns TLS and REALITY on and off as one choice', () => {
    const p: Record<string, unknown> = { type: 'vless', vless: {} }
    setSecurity(p, 'vless.tls', 'reality')
    expect(securityOf(p, 'vless.tls')).toBe('reality')
    setSecurity(p, 'vless.tls', 'tls')
    expect(p).toEqual({ type: 'vless', vless: { tls: { enabled: true } } })
    setSecurity(p, 'vless.tls', '')
    expect(securityOf(p, 'vless.tls')).toBe('')
  })

  it('hides the fields another choice rules out and greys out those a setting turns off', () => {
    const ws = { type: 'vless', vless: { transport: { type: 'ws' }, tls: { enabled: true } } }
    expect(fieldState(field('vless.transport.service_name'), ws)).toBe('hidden')
    expect(fieldState(field('vless.flow'), ws)).toBe('hidden')
    expect(fieldState(field('vless.transport.host', 1), ws)).toBe('active')
    expect(fieldState(field('vless.tls.server_name'), ws)).toBe('active')
    expect(fieldState(field('vless.tls.server_name'), { type: 'vless', vless: {} })).toBe('inactive')
    const reality = { type: 'vless', vless: { tls: { enabled: true, reality: { enabled: true } } } }
    expect(fieldState(field('vless.tls.ech.enabled'), reality)).toBe('inactive')
    expect(fieldState(field('vless.tls.reality.public_key'), reality)).toBe('active')
  })

  it('saves no value of a field that does not apply, and keeps a path another field uses', () => {
    const p = {
      type: 'vless',
      server: 'a.example',
      server_port: 443,
      vless: {
        uuid: 'id',
        flow: 'xtls-rprx-vision',
        transport: { type: 'ws', host: 'cdn.example', service_name: 'grpc' },
        tls: { enabled: true, server_name: 'a.example', ech: { enabled: true }, reality: { enabled: true, public_key: 'key' } },
      },
    } as unknown as Profile
    expect(cleanProfile(vlessForm, p)).toEqual({
      type: 'vless',
      server: 'a.example',
      server_port: 443,
      vless: {
        uuid: 'id',
        transport: { type: 'ws', host: 'cdn.example' },
        tls: { enabled: true, server_name: 'a.example', reality: { enabled: true, public_key: 'key' } },
      },
    })
  })

  it('reads headers, bytes and where JSON broke', () => {
    expect(parseHeaders('Host: a.example\n\nX-Id:1:2')).toEqual({ Host: 'a.example', 'X-Id': '1:2' })
    expect(parseBytes('1, 2 255')).toEqual([1, 2, 255])
    expect(parseBytes('1, 256')).toBeUndefined()
    const text = '{\n  "type": "vless",\n  oops\n}'
    let error: unknown
    try {
      JSON.parse(text)
    } catch (e) {
      error = e
    }
    expect(jsonErrorLine(text, error)).toBe(3)
  })
})
