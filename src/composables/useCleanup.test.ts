import { createPinia, setActivePinia } from 'pinia'
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest'

import { CodeElevationCancelled, type CoreClient, type CoreError, appControls, shellCleanup } from '@/platform'
import {
  type CleanupItem,
  CleanupFirewall,
  CleanupProfile,
  CleanupShortcuts,
  MethodCleanupRun,
  MethodCleanupScan,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

import { useCleanup } from './useCleanup'

const settle = () => new Promise((resolve) => setTimeout(resolve, 20))

/** A core whose cleanup finds `found` and leaves what `runs` gives, one run after another. */
function fakeCleanup(client: CoreClient, found: CleanupItem[], runs: CleanupItem[][]) {
  const call = client.call.bind(client)
  const calls = { runs: 0 }
  vi.spyOn(client, 'call').mockImplementation(((method: string, ...params: never[]) => {
    if (method === MethodCleanupScan) return Promise.resolve({ items: found })
    if (method === MethodCleanupRun) return Promise.resolve({ items: runs[calls.runs++] ?? [] })
    return (call as (m: string, ...p: never[]) => Promise<unknown>)(method, ...params)
  }) as CoreClient['call'])
  return calls
}

async function ready(elevated = false) {
  const core = useCoreStore()
  await core.start()
  await (elevated ? core.core().elevate() : core.core().restart())
  await settle()
  return core
}

const shortcuts: CleanupItem = { kind: CleanupShortcuts, count: 2 }
const firewall: CleanupItem = { kind: CleanupFirewall, count: 1, rights: true }

describe('the removal of Poros traces', () => {
  beforeEach(() => {
    setActivePinia(createPinia())
    vi.spyOn(appControls, 'exit').mockResolvedValue()
  })
  afterEach(() => vi.restoreAllMocks())

  it('lists the traces of the core after those of the shell', async () => {
    const core = await ready()
    fakeCleanup(core.core(), [shortcuts, firewall], [])
    vi.spyOn(shellCleanup, 'scan').mockResolvedValue([{ kind: 'autostart', count: 1 }])
    const cleanup = useCleanup()
    await cleanup.scan()
    expect(cleanup.found.value?.map((item) => item.kind)).toEqual(['autostart', CleanupShortcuts, CleanupFirewall])
  })

  it('asks for rights once for what takes them, then exits with nothing left', async () => {
    const core = await ready()
    const calls = fakeCleanup(core.core(), [shortcuts, firewall], [[firewall], []])
    const elevate = vi.spyOn(core.core(), 'elevate')
    const shell = vi.spyOn(shellCleanup, 'run').mockResolvedValue([])
    const cleanup = useCleanup()
    await cleanup.run()
    expect(elevate).toHaveBeenCalledOnce()
    expect(calls.runs).toBe(2)
    // The shell goes after the core: the autostart is off by then.
    expect(shell).toHaveBeenCalledOnce()
    expect(appControls.exit).toHaveBeenCalledOnce()
    expect(cleanup.left.value).toBeNull()
  })

  it('tells what stays when the rights are refused, and does not exit', async () => {
    const core = await ready()
    fakeCleanup(core.core(), [firewall], [[firewall]])
    const cancelled: CoreError = { code: CodeElevationCancelled, message: 'administrator rights were not given' }
    vi.spyOn(core.core(), 'elevate').mockRejectedValue(cancelled)
    vi.spyOn(shellCleanup, 'run').mockResolvedValue([{ kind: 'urlScheme', count: 1, error: 'access denied' }])
    const cleanup = useCleanup()
    await cleanup.run()
    expect(cleanup.left.value).toEqual([{ kind: 'urlScheme', count: 1, error: 'access denied' }, firewall])
    expect(appControls.exit).not.toHaveBeenCalled()
  })

  it('does not ask for rights for a trace that failed with them', async () => {
    const core = await ready(true)
    const failed = { ...firewall, error: 'netsh failed' }
    fakeCleanup(core.core(), [firewall], [[failed]])
    const elevate = vi.spyOn(core.core(), 'elevate')
    vi.spyOn(shellCleanup, 'run').mockResolvedValue([])
    const cleanup = useCleanup()
    await cleanup.run()
    expect(elevate).not.toHaveBeenCalled()
    expect(cleanup.left.value).toEqual([failed])
  })

  it('looks at the user profile with a core without rights after one of another account', async () => {
    const core = await ready(true)
    const calls = fakeCleanup(core.core(), [], [[{ kind: CleanupProfile, count: 0 }], []])
    const restart = vi.spyOn(core.core(), 'restart')
    vi.spyOn(shellCleanup, 'run').mockResolvedValue([])
    const cleanup = useCleanup()
    await cleanup.run()
    expect(restart).toHaveBeenCalledOnce()
    expect(calls.runs).toBe(2)
    expect(appControls.exit).toHaveBeenCalledOnce()
  })

  it('takes the profile as cleaned when a core of another account comes with the rights', async () => {
    const core = await ready()
    fakeCleanup(core.core(), [firewall], [[firewall], [{ kind: CleanupProfile, count: 0 }]])
    const restart = vi.spyOn(core.core(), 'restart')
    vi.spyOn(shellCleanup, 'run').mockResolvedValue([])
    const cleanup = useCleanup()
    await cleanup.run()
    expect(restart).not.toHaveBeenCalled()
    expect(appControls.exit).toHaveBeenCalledOnce()
  })
})
