import { useRouter } from 'vue-router'

import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useProfileActions } from '@/composables/useProfileActions'
import { useResetConnection } from '@/composables/useResetConnection'
import { useUpdates } from '@/composables/useUpdates'
import { ConnectionIdle, GroupManual, KillSwitchConnected, MethodSubscriptionsUpdate } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

/** The actions of the advanced mode that the search and the shortcuts of the window share; a failure is told. */
export function useAppActions() {
  const core = useCoreStore()
  const router = useRouter()
  const attempt = useAttempt()
  const connect = useConnect()
  const profiles = useProfileActions()
  const updates = useUpdates()
  const { reset } = useResetConnection()

  return {
    newProfile: () => {
      void router.push('/adv/profiles')
      profiles.newProfile(null)
    },
    // One subscription that fails does not keep the others from their update.
    updateAll: async () => {
      for (const g of core.groups.filter((g) => g.kind !== GroupManual)) await attempt(() => core.core().call(MethodSubscriptionsUpdate, { id: g.id }))
    },
    testAll: () => attempt(() => core.testServers('')),
    // As the first item of the tray: Stop while a connection runs or the kill switch holds after a lost one.
    startStop: () =>
      attempt(() => {
        const idle = core.connection.state === ConnectionIdle
        const held = core.killSwitch.on && idle && core.settings?.killSwitch === KillSwitchConnected
        return held || !idle ? connect.stop() : connect.start()
      }),
    reset: () => attempt(reset),
    checkUpdates: () => attempt(() => updates.check()),
    wizard: () => router.push('/wizard/1'),
    settings: () => router.push('/adv/settings'),
    logs: () => router.push('/adv/logs'),
  }
}
