<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import SpeedChart from '@/components/adv/SpeedChart.vue'
import TrafficHistory from '@/components/adv/TrafficHistory.vue'
import { Tabs, TabsList, TabsTrigger } from '@/components/ui'
import { flagOf, formatAgo, formatBytes, formatDuration, serverTitle } from '@/lib/format'
import { ConnectionRunning, MethodStatsNow, type StatsNow } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Statistics» of the advanced mode. «Now»: the core, the connection and the network, the speed of the last minute
// and the next update of subscriptions, asked of the core every two seconds while the tab is open. «Traffic»: the
// history of traffic.
const core = useCoreStore()
const route = useRoute()
const router = useRouter()
const { t, locale } = useI18n()
const now = ref<StatsNow | null>(null)
const clock = ref(Date.now())
let timer: ReturnType<typeof setInterval> | undefined

const tab = computed({
  get: () => String(route.params.tab ?? 'now'),
  set: (next: string) => void router.push(`/adv/stats/${next}`),
})
const running = computed(() => core.connection.state === ConnectionRunning)
const server = computed(() => (running.value ? (core.servers.find((s) => s.id === core.connection.serverId) ?? null) : null))
const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))
const none = '—'

const cards = computed(() => {
  const n = now.value
  const s = server.value
  const proxy = core.network.proxy
  return [
    {
      id: 'process',
      rows: [
        ['core', n?.core || none],
        ['memory', n ? bytes(n.memory) : none],
        ['uptime', n ? formatDuration(n.uptime) : none],
        ['databases', n ? bytes(n.databases) : none],
      ],
    },
    {
      id: 'config',
      rows: [
        ['profile', s ? serverTitle(s.name, s.country) : none],
        ['country', flagOf(s?.country || proxy.countryCode) || none],
        ['ip', (running.value && (proxy.ip || proxy.ipv6)) || none],
        ['latency', s && s.latency > 0 ? t('adv.stats.ms', { n: s.latency }) : none],
        ['endpoint', s?.host ? `${s.host}:${s.port}` : none],
      ],
    },
    {
      id: 'network',
      rows: [
        ['state', n ? t(n.online ? 'adv.stats.online' : 'adv.stats.offline') : none],
        ['adapter', n?.network || none],
        ['throughServer', n && running.value ? String(n.throughServer) : none],
        ['direct', n && running.value ? String(n.direct) : none],
        ['other', n && running.value ? String(n.other) : none],
      ],
    },
  ]
})
const nextUpdate = computed(() => {
  const next = now.value?.nextSubscriptionUpdate ?? 0
  if (!next) return none
  return next <= clock.value ? t('adv.stats.soon') : formatAgo(next, clock.value, locale.value)
})

async function poll(): Promise<void> {
  if (tab.value !== 'now') return
  now.value = await core.core().call(MethodStatsNow)
  clock.value = Date.now()
}
onMounted(() => {
  void poll().catch(() => undefined)
  timer = setInterval(() => void poll().catch(() => undefined), 2000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <Tabs v-model="tab">
      <TabsList>
        <TabsTrigger value="now">{{ t('adv.stats.now') }}</TabsTrigger>
        <TabsTrigger value="traffic">{{ t('adv.stats.traffic') }}</TabsTrigger>
      </TabsList>
    </Tabs>
    <div v-if="tab === 'now'" class="min-h-0 flex-1 overflow-auto px-3.5 py-3.5" data-testid="stats-now">
      <div class="grid grid-cols-3 gap-2.5">
        <section v-for="card in cards" :key="card.id" class="rounded-xl border border-line bg-s2 px-4 py-3" :data-testid="`stats-${card.id}`">
          <h3 class="mb-1.5 text-[13px] font-semibold">{{ t(`adv.stats.${card.id}`) }}</h3>
          <div v-for="[name, value] in card.rows" :key="name" class="flex items-baseline justify-between gap-3 py-[3px] text-[13px]">
            <span class="text-t2">{{ t(`adv.stats.${name}`) }}</span>
            <span class="truncate font-mono text-xs text-t1">{{ value }}</span>
          </div>
        </section>
      </div>
      <section class="mt-2.5 rounded-xl border border-line bg-s2 px-4 py-3">
        <h3 class="mb-1.5 text-[13px] font-semibold">{{ t('adv.stats.speed') }}</h3>
        <div class="h-[132px]"><SpeedChart :samples="core.speedHistory" /></div>
      </section>
      <section class="mt-2.5 flex items-baseline justify-between rounded-xl border border-line bg-s2 px-4 py-3 text-[13px]">
        <span class="text-t2">{{ t('adv.stats.nextUpdate') }}</span>
        <span class="font-mono text-xs" data-testid="stats-next-update">{{ nextUpdate }}</span>
      </section>
    </div>
    <TrafficHistory v-else />
  </div>
</template>
