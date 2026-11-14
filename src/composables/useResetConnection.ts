import { ref, watch } from 'vue'

import { useConnect } from '@/composables/useConnect'
import { useCoreStore } from '@/stores/core'

/** «Reset the connection»: a new core with the same rights, and the connection back if there was one. */
export function useResetConnection() {
  const core = useCoreStore()
  const connect = useConnect()
  const resetting = ref(false)

  async function reset(): Promise<void> {
    const wasRunning = core.running
    resetting.value = true
    try {
      await (core.status.elevated ? core.core().elevate() : core.core().restart())
      await new Promise<void>((resolve) => {
        const stop = watch(
          () => core.status.state === 'ready' && core.loaded,
          (ready) => {
            if (ready) {
              stop()
              resolve()
            }
          },
        )
      })
      if (wasRunning) await connect.start()
    } finally {
      resetting.value = false
    }
  }

  return { reset, resetting }
}
