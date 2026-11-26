import { describe, expect, it } from 'vitest'

import type { Endpoint, Group, OTPCode, Settings, WARPDevice } from '@/platform/core-types'

import { type BackupData, backupHasSecrets } from './backup'

const device = (privateKey = '') => ({ privateKey }) as WARPDevice
const settings = (s: Partial<Settings> = {}) =>
  ({ proxyAuth: false, proxyPassword: '', warpWireGuard: device(), warpMasque: device(), ...s }) as Settings
const empty: BackupData = { settings: settings(), groups: [], endpoints: [], otp: [] }
const all = ['profiles', 'routing', 'settings', 'otp']

describe('the secrets of a backup', () => {
  it('are in the parts that hold links, keys and passwords', () => {
    expect(backupHasSecrets(all, empty)).toBe(false)
    expect(backupHasSecrets(['profiles'], { ...empty, groups: [{ url: 'https://sub.example.com/u/1', servers: 0 } as Group] })).toBe(true)
    expect(backupHasSecrets(['profiles'], { ...empty, groups: [{ url: '', servers: 2 } as Group] })).toBe(true)
    expect(backupHasSecrets(['profiles'], { ...empty, groups: [{ url: '', servers: 0 } as Group] })).toBe(false)
    expect(backupHasSecrets(['routing'], { ...empty, endpoints: [{} as Endpoint] })).toBe(true)
    expect(backupHasSecrets(['otp'], { ...empty, otp: [{} as OTPCode] })).toBe(true)
  })

  it('are in the settings with a password of the local proxy, on or off, or a key of WARP', () => {
    expect(backupHasSecrets(['settings'], { ...empty, settings: settings({ proxyAuth: true, proxyPassword: 'pw' }) })).toBe(true)
    expect(backupHasSecrets(['settings'], { ...empty, settings: settings({ proxyAuth: false, proxyPassword: 'pw' }) })).toBe(true)
    expect(backupHasSecrets(['settings'], { ...empty, settings: settings({ proxyAuth: true }) })).toBe(false)
    expect(backupHasSecrets(['settings'], { ...empty, settings: settings({ warpMasque: device('key') }) })).toBe(true)
    expect(backupHasSecrets(['settings'], { ...empty, settings: null })).toBe(false)
  })

  it('are only in the chosen parts', () => {
    const data = { ...empty, endpoints: [{} as Endpoint] }
    expect(backupHasSecrets(['profiles', 'settings', 'otp'], data)).toBe(false)
    expect(backupHasSecrets([], data)).toBe(false)
  })
})
