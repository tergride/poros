import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { CodeElevationCancelled } from '@/platform'
import { ConnectionIdle, KillSwitchOff, MethodSubscriptionsAdd } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

import { useConnect } from './useConnect'

const settle = () => new Promise((resolve) => setTimeout(resolve, 20))

/** A core without rights over a subscription, with the given settings. */
async function prepare(settings: Parameters<ReturnType<typeof useCoreStore>['updateSettings']>[0]) {
  const core = useCoreStore()
  await core.start()
  await core.core().restart()
  await settle()
  if (!core.servers.length) {
    await core.core().call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    await core.load()
  }
  await core.updateSettings({
    onboardingDone: true,
    server: core.servers[0]!.id,
    tun: false,
    systemProxy: true,
    killSwitch: KillSwitchOff,
    autoConnect: true,
    runAsAdmin: false,
    adminAutostart: false,
    spoofDefault: false,
    ...settings,
  })
  return core
}

/** The UAC prompt answered no while `run` runs. */
async function declining(core: ReturnType<typeof useCoreStore>, run: () => Promise<void>) {
  const client = core.core()
  const elevate = client.elevate
  client.elevate = () => Promise.reject({ code: CodeElevationCancelled, message: 'administrator rights were not given' })
  try {
    await run()
  } finally {
    client.elevate = elevate
  }
}

describe('TLS spoofing', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('offers rights after a start that forges a handshake without them, and connects with them', async () => {
    const core = await prepare({ spoofDefault: true, spoofSni: 'www.example.com' })
    const connect = useConnect()
    await connect.start()
    await settle()
    expect(connect.spoofFailed).toBe(true)
    expect(connect.tunFailed).toBe(false)

    await declining(core, () => connect.elevateForSpoof())
    expect(connect.spoofFailed).toBe(true)
    expect(core.connection.state).toBe(ConnectionIdle)

    await connect.elevateForSpoof()
    await settle()
    expect(core.status.elevated).toBe(true)
    expect(core.connection.state).not.toBe(ConnectionIdle)
    expect(connect.spoofFailed).toBe(false)
  })
})

describe('rights at launch', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('runs with rights from the launch and connects in the new core', async () => {
    const core = await prepare({ runAsAdmin: true })
    await useConnect().autoStart()
    await settle()
    expect(core.status.elevated).toBe(true)
    expect(core.connection.state).not.toBe(ConnectionIdle)
  })

  it('connects without rights when the prompt is declined and the connection needs none', async () => {
    const core = await prepare({ runAsAdmin: true })
    const connect = useConnect()
    await declining(core, () => connect.autoStart())
    await settle()
    expect(core.status.elevated).toBe(false)
    expect(core.connection.state).not.toBe(ConnectionIdle)
    expect(connect.tunFailed).toBe(false)
  })

  it('asks for rights before the sign-in task and leaves it off when declined', async () => {
    const core = await prepare({})
    const connect = useConnect()
    await declining(core, () => connect.setAdminAutostart(true))
    expect(core.settings?.adminAutostart).toBe(false)

    await connect.setAdminAutostart(true)
    expect(core.status.elevated).toBe(true)
    expect(core.settings?.adminAutostart).toBe(true)
  })
})
