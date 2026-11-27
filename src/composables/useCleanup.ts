import { ref, watch } from 'vue'

import { useConnect } from '@/composables/useConnect'
import { appControls, shellCleanup } from '@/platform'
import { type CleanupItem, CleanupProfile, MethodCleanupRun, MethodCleanupScan } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * «Remove Poros traces»: what this copy of Poros left outside its folder, from the core and the shell, and its removal.
 * A trace that takes administrator rights waits for them, so the run asks for them once while one stays. A core with
 * the rights of another account does not see the user's profile: a core without rights then looks there once more.
 * The shell goes last, after the core turned the start with Windows off. With nothing left Poros exits.
 */
export function useCleanup() {
  const core = useCoreStore()
  const connect = useConnect()
  /** What the scan found, null while it looks. */
  const found = ref<CleanupItem[] | null>(null)
  /** What a run left, null before one. */
  const left = ref<CleanupItem[] | null>(null)
  const busy = ref(false)

  async function scan(): Promise<void> {
    found.value = left.value = null
    const [ours, shell] = await Promise.all([core.core().call(MethodCleanupScan), shellCleanup.scan()])
    found.value = [...shell, ...ours.items]
  }

  const runCore = async () => (await core.core().call(MethodCleanupRun)).items
  const waitsForProfile = (items: CleanupItem[]) => items.some((item) => item.kind === CleanupProfile)

  /** A new core without rights, the one that sees the user's profile. */
  async function restartWithoutRights(): Promise<void> {
    await core.core().restart()
    await new Promise<void>((resolve) => {
      const stop = watch(
        () => core.status.state === 'ready' && !core.status.elevated && core.loaded,
        (ready) => {
          if (!ready) return
          stop()
          resolve()
        },
        { immediate: true },
      )
    })
  }

  /** Removes the traces; Poros exits when none stays, otherwise `left` tells which. */
  async function run(): Promise<void> {
    busy.value = true
    try {
      let items = await runCore()
      const profileSeen = !waitsForProfile(items)
      if (items.some((item) => item.rights && !item.error) && !core.status.elevated && (await connect.gainRights())) {
        items = await runCore()
      }
      if (!profileSeen && waitsForProfile(items)) {
        await restartWithoutRights()
        items = await runCore()
      }
      // A core of another account after the rights does not see the profile a core of the user has cleaned already.
      if (profileSeen) items = items.filter((item) => item.kind !== CleanupProfile)
      const stays = [...(await shellCleanup.run()), ...items]
      if (stays.length === 0) {
        await appControls.exit()
        return
      }
      left.value = stays
    } finally {
      busy.value = false
    }
  }

  return { found, left, busy, scan, run }
}
