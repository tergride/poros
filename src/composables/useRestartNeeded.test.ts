import { createPinia, setActivePinia } from 'pinia'
import { beforeEach, describe, expect, it } from 'vitest'

import { ConnectionRunning, ConnectionStarting, KillSwitchAlways, MethodPresetsSave, MethodSubscriptionsAdd } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

import { useConnect } from './useConnect'
import { useRestartNeeded } from './useRestartNeeded'

const settle = () => new Promise((resolve) => setTimeout(resolve, 20))

/** A running connection over System on the stub core. */
async function running() {
  const core = useCoreStore()
  await core.start()
  if (!core.servers.length) {
    await core.core().call(MethodSubscriptionsAdd, { url: 'https://sub.example.com/u/1', name: '' })
    await core.load()
  }
  await core.updateSettings({ onboardingDone: true, server: core.servers[0]!.id, tun: false, systemProxy: true })
  await useConnect().connect()
  await settle()
  expect(core.connection.state).toBe(ConnectionRunning)
  return core
}

describe('settings saved under a running connection', () => {
  beforeEach(() => setActivePinia(createPinia()))

  it('wait for a restart the user asks for, and the connection keeps running', async () => {
    const core = await running()
    const restart = useRestartNeeded()
    expect(restart.needed).toBe(false)

    // On the fly: no restart is needed for the kill switch or System.
    await core.updateSettings({ killSwitch: KillSwitchAlways, systemProxy: false })
    expect(restart.needed).toBe(false)

    await core.updateSettings({ allowLan: true })
    expect(restart.needed).toBe(true)
    expect(core.connection.state).toBe(ConnectionRunning)

    restart.dialogOpen = true
    await restart.apply()
    await settle()
    expect(restart.dialogOpen).toBe(false)
    expect(restart.needed).toBe(false)
  })

  it('count a preset saved while it runs, and leave nothing to apply once the connection stops', async () => {
    const core = await running()
    const restart = useRestartNeeded()
    restart.presetSaved(core.connection.preset!)
    expect(restart.needed).toBe(true)
    restart.dialogOpen = true

    await useConnect().stop()
    await settle()
    expect(restart.needed).toBe(false)
    expect(restart.dialogOpen).toBe(false)
    expect(core.running).toBe(false)
  })

  it('go by the preset the connection was built of, not by the chosen one', async () => {
    const core = await running()
    const restart = useRestartNeeded()
    const make = (name: string) =>
      core.core().call(MethodPresetsSave, { id: '', builtin: false, basedOn: '', name, advanced: false, rules: [], dns: '', directDns: '', default: '', inherited: [] })
    const [first, second] = [await make('First'), await make('Second')]
    await core.updateSettings({ preset: first.id })
    // Saved before the start reads it: the connection has it.
    restart.presetSaved(first.id)
    await useConnect().connect()
    await settle()
    expect(restart.needed).toBe(false)

    await core.updateSettings({ preset: second.id })
    expect(restart.presetInUse(first.id)).toBe(true)
    expect(restart.presetInUse(second.id)).toBe(false)
    restart.presetSaved(second.id)
    await core.updateSettings({ preset: first.id })
    expect(restart.needed).toBe(false)

    await core.updateSettings({ preset: second.id })
    restart.presetSaved(first.id)
    await core.updateSettings({ preset: first.id })
    expect(restart.needed).toBe(true)

    await useConnect().connect()
    expect(core.connection.state).toBe(ConnectionStarting)
    expect(restart.presetInUse(first.id)).toBe(true)
    restart.presetSaved(first.id)
    await settle()
    expect(core.connection.state).toBe(ConnectionRunning)
    expect(restart.needed).toBe(true)

    // Chosen while it starts: the connection still comes up with the one before.
    await useConnect().connect()
    await core.updateSettings({ preset: second.id })
    await settle()
    expect(core.connection.preset).toBe(first.id)
    expect(restart.needed).toBe(true)
    await core.updateSettings({ preset: first.id })
    expect(restart.needed).toBe(false)
  })
})
