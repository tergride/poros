import { defineStore } from 'pinia'
import { computed, ref } from 'vue'

import { appControls } from '@/platform'
import { MethodUpdatesCheck, UpdateReady } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/**
 * The updates of Poros as the screens show them: the core checks and downloads, nothing installs until the user
 * presses "Restart and update". "Later" puts off the offer of that version for this run.
 */
export const useUpdates = defineStore('updates', () => {
  const core = useCoreStore()
  const later = ref('')
  const failureClosed = ref(false)
  // A check the user asked for, so that the help page can say the latest version is installed.
  const checkedByHand = ref(false)

  const offer = computed(() => core.updates.state === UpdateReady && later.value !== core.updates.version)
  const failure = computed(() => !!core.updates.installError && !failureClosed.value)

  async function check(): Promise<void> {
    checkedByHand.value = true
    await core.core().call(MethodUpdatesCheck)
  }

  /** The updater takes over and Poros exits; a running connection comes back after the restart. */
  async function install(): Promise<void> {
    await appControls.update(core.running)
  }

  function putOff(): void {
    later.value = core.updates.version
  }

  return { later, failureClosed, checkedByHand, offer, failure, check, install, putOff }
})
