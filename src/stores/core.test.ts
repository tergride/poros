import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { MethodSubscriptionsAdd } from '@/platform/core-types'

import { useCoreStore } from './core'

const settle = () => new Promise((resolve) => setTimeout(resolve, 10))

describe('the core store over a new core', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('ends a test the old core ran and keeps what was measured until the new one measures again', async () => {
    const core = useCoreStore()
    await core.start()
    await core.core().call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    await core.load()
    expect(core.servers.length).toBeGreaterThan(0)
    // What the old core measured, and a test it never finished.
    core.servers = core.servers.map((server, i) => ({ ...server, latency: 40 + i }))
    core.network = { ...core.network, checkedAt: 1, direct: { ip: '192.0.2.1', ipv6: '', countryCode: 'DE', city: '', isp: '', asn: 0 } }
    core.testing = true

    await core.core().restart()
    await settle()
    expect(core.status.state).toBe('ready')
    expect(core.testing).toBe(false)
    expect(core.servers.every((server, i) => server.latency === 40 + i)).toBe(true)
    expect(core.network.direct.ip).toBe('192.0.2.1')
  })
})
