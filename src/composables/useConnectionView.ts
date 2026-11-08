import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { useAttempt } from '@/composables/useAttempt'
import { useClock } from '@/composables/useClock'
import { useConnect } from '@/composables/useConnect'
import { formatBytes, formatDuration, serverTitle } from '@/lib/format'
import { ConnectionRunning, ConnectionStarting, ConnectionStopping, KillSwitchConnected } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A start that takes long may be cancelled; a quick one has no time to show the choice.
const cancelAfter = 3000

/**
 * The connection as both modes show it: the portal of the simple mode and the connection bar of the advanced one say
 * the same about the state, the kill switch, the address and the switches.
 */
export function useConnectionView() {
  const core = useCoreStore()
  const connect = useConnect()
  const attempt = useAttempt()
  const { t, te, locale } = useI18n()
  const now = useClock(1000)

  const state = computed(() => core.connection.state)
  const on = computed(() => state.value === ConnectionRunning)
  const busy = computed(() => state.value === ConnectionStarting || state.value === ConnectionStopping)
  const group = computed(() => core.groups.find((g) => g.id === core.server?.groupId))
  const empty = computed(() => core.servers.length === 0)
  // The kill switch keeps the traffic of the server from going out directly; after a lost connection it holds until
  // Stop. Full is Windows blocking everything while Poros's TUN is down.
  const blocked = computed(() => core.killSwitch.on)
  const full = computed(() => core.killSwitch.full)
  const held = computed(() => blocked.value && !on.value && !busy.value && core.settings?.killSwitch === KillSwitchConnected)
  // The kill switch in Always holds without a connection too: VPN traffic is closed, not merely disconnected.
  const blockedIdle = computed(() => blocked.value && !on.value && !busy.value && !held.value)

  const startingSince = ref(0)
  watch(state, (next) => (startingSince.value = next === ConnectionStarting ? Date.now() : 0), { immediate: true })
  const cancellable = computed(() => startingSince.value > 0 && now.value - startingSince.value >= cancelAfter)

  const stateText = computed(() => {
    if (on.value) return t('portal.on', { time: formatDuration(now.value - core.connection.since) })
    if (held.value) return t(full.value ? 'portal.blockedFull' : 'portal.blocked')
    if (state.value === ConnectionStarting) return t('portal.starting')
    if (state.value === ConnectionStopping) return t('portal.stopping')
    if (blockedIdle.value) return t(full.value ? 'portal.blockedAlwaysFull' : 'portal.blockedAlways')
    return t('portal.off')
  })

  // The address the services see: through the VPN while connected, the user's own otherwise; IPv4 first, as the
  // shorter one. Behind the block the services cannot be reached, so no address is waited for.
  const address = computed(() => (on.value ? core.network.proxy : core.network.direct))
  const shownIp = computed(() => address.value.ip || address.value.ipv6)
  const ipText = computed(() => {
    if (blocked.value && !on.value) return ''
    if (core.network.checking && !shownIp.value) return t('portal.ipChecking')
    if (!shownIp.value) return ''
    return t('portal.ip', { ip: shownIp.value })
  })

  function latencyText(latency: number): string {
    if (latency === 0) return ''
    return latency < 0 ? t('ping.none') : t('ping.ms', { n: latency })
  }

  const speed = (bytes: number) => t('units.perSecond', { value: formatBytes(bytes, locale.value, (unit) => t(`units.${unit}`)) })

  const presetName = computed(() => {
    const preset = core.preset
    if (!preset) return ''
    return preset.builtin && te(`presets.name.${preset.id}`) ? t(`presets.name.${preset.id}`) : preset.name
  })

  const tun = computed({
    get: () => core.settings?.tun ?? false,
    set: (value: boolean) => void attempt(() => connect.setTun(value)),
  })
  const system = computed({
    get: () => core.settings?.systemProxy ?? false,
    set: (value: boolean) => void attempt(() => connect.setSystemProxy(value)),
  })
  const exitCountry = computed(() => core.network.proxy.countryCode || core.server?.country || '')
  const serverName = computed(() => (core.server ? serverTitle(core.server.name, core.server.country) : t('portal.noServer')))

  return {
    state,
    on,
    busy,
    group,
    empty,
    blocked,
    full,
    held,
    blockedIdle,
    cancellable,
    stateText,
    address,
    shownIp,
    ipText,
    latencyText,
    speed,
    presetName,
    tun,
    system,
    exitCountry,
    serverName,
  }
}
