<script setup lang="ts">
import { Search, Settings2, X } from '@lucide/vue'
import { computed, onBeforeUnmount, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import SpeedChart from '@/components/adv/SpeedChart.vue'
import { Button, ContextMenu, ContextMenuItem, Input, Segmented } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { type ConnectionRow, filterConnections, groupByProcess, withSpeeds } from '@/lib/connections'
import { formatBytes, serverTitle } from '@/lib/format'
import { ConnectionRunning, MethodConnectionsClose, MethodConnectionsList } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Connections» of the advanced mode: the speed of the last minute and the open connections through Poros, by
// program or as one list, polled once a second while the screen is open.
const core = useCoreStore()
const router = useRouter()
const { t, locale } = useI18n()
const attempt = useAttempt()
const rows = ref<ConnectionRow[]>([])
const tracked = ref(true)
const view = ref('process')
const filter = ref('')
// The connection the menu of the right button was opened over, or the program's when it was over a program.
const target = ref<{ id?: string; process?: string } | null>(null)
let before = new Map<string, number>()
let polledAt = 0
let timer: ReturnType<typeof setInterval> | undefined

const running = computed(() => core.connection.state === ConnectionRunning)
const views = computed(() => [
  { value: 'process', label: t('adv.connections.byProcess') },
  { value: 'list', label: t('adv.connections.list') },
])

const serverName = computed(() => {
  const server = core.servers.find((s) => s.id === core.connection.serverId)
  return server ? serverTitle(server.name, server.country) : t('adv.connections.server')
})
function outboundName(tag: string): string {
  if (tag === 'proxy') return serverName.value
  if (tag === 'direct') return t('adv.connections.direct')
  if (tag === 'warp') return 'WARP'
  const endpoint = /^endpoint-(\d+)$/.exec(tag)
  if (endpoint) return t('adv.connections.endpoint', { n: endpoint[1] })
  return tag
}
const processName = (process: string) => process || t('adv.connections.system')
const protocolOf = (c: ConnectionRow) => (c.protocol ? `${c.network}/${c.protocol}` : c.network)
const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))
const speed = (value: number) => t('units.perSecond', { value: bytes(value) })

const shown = computed(() => filterConnections(rows.value, filter.value, outboundName))
const groups = computed(() => groupByProcess(shown.value))
const listed = computed(() => [...shown.value].sort((a, b) => b.start - a.start))

async function poll(): Promise<void> {
  if (!running.value) {
    rows.value = []
    before = new Map()
    return
  }
  const list = await core.core().call(MethodConnectionsList)
  const now = Date.now()
  tracked.value = list.tracked
  rows.value = withSpeeds(list.connections, before, polledAt ? (now - polledAt) / 1000 : 0)
  before = new Map(list.connections.map((c) => [c.id, c.upload + c.download]))
  polledAt = now
}

function close(): void {
  const chosen = target.value
  if (!chosen) return
  const ids = chosen.id ? [chosen.id] : rows.value.filter((c) => c.process === chosen.process).map((c) => c.id)
  void attempt(async () => {
    for (const id of ids) await core.core().call(MethodConnectionsClose, { id }).catch(() => undefined)
    await poll()
  })
}
const closeAll = () => attempt(async () => {
  await core.core().call(MethodConnectionsClose, { id: '' })
  await poll()
})

onMounted(() => {
  void poll().catch(() => undefined)
  timer = setInterval(() => void poll().catch(() => undefined), 1000)
})
onBeforeUnmount(() => clearInterval(timer))
</script>

<template>
  <section class="flex min-h-0 min-w-0 flex-1 flex-col">
    <div class="h-[132px] flex-none border-b border-line px-3.5 pt-3 pb-2" data-testid="speed-chart">
      <SpeedChart :samples="core.speedHistory" />
    </div>
    <div class="flex flex-none items-center gap-2 border-b border-line px-3.5 py-2.5">
      <div class="relative min-w-0 flex-1">
        <Search class="absolute top-2.5 left-2.5 size-4 text-t3" />
        <Input v-model="filter" class="pl-8" :placeholder="t('adv.connections.filter')" :aria-label="t('adv.connections.filter')" data-testid="conn-filter" />
      </div>
      <Segmented v-model="view" :options="views" :aria-label="t('adv.connections.view')" />
      <span class="rounded-md bg-s3 px-2 py-1 text-xs text-t2" data-testid="conn-active">{{ t('adv.connections.active', { n: rows.length }) }}</span>
    </div>

    <div v-if="!running" class="grid flex-1 place-items-center p-10 text-center text-t2" data-testid="conn-idle">{{ t('adv.connections.idle') }}</div>
    <div v-else-if="!tracked" class="grid flex-1 place-items-center p-10 text-center" data-testid="conn-untracked">
      <div class="grid justify-items-center gap-3">
        <p class="max-w-[420px] text-t2">{{ t('adv.connections.untracked') }}</p>
        <Button size="sm" @click="router.push('/adv/settings/logging')"><Settings2 />{{ t('adv.connections.toSettings') }}</Button>
      </div>
    </div>
    <ContextMenu v-else min-width="240px">
      <template #trigger>
        <div class="min-h-0 flex-1 overflow-auto">
          <table class="w-full border-collapse text-[13px]" data-testid="conn-table">
            <thead class="sticky top-0 bg-s1 text-left text-xs text-t3">
              <tr>
                <th class="px-3.5 py-2 font-medium">{{ t('adv.connections.what') }}</th>
                <th class="px-2.5 py-2 font-medium">{{ t('adv.connections.source') }}</th>
                <th class="px-2.5 py-2 font-medium">{{ t('adv.connections.protocol') }}</th>
                <th class="px-2.5 py-2 font-medium">{{ t('adv.connections.outbound') }}</th>
                <th class="px-2.5 py-2 text-right font-medium">{{ t('adv.connections.traffic') }}</th>
                <th class="px-3.5 py-2 text-right font-medium">{{ t('adv.connections.speed') }}</th>
              </tr>
            </thead>
            <tbody v-if="view === 'process'">
              <template v-for="group in groups" :key="group.process">
                <tr class="border-t border-line hover:bg-s2" data-testid="conn-process" @contextmenu="target = { process: group.process }">
                  <td class="px-3.5 py-2 font-semibold" :title="group.connections[0]?.processPath">{{ processName(group.process) }}</td>
                  <td class="px-2.5 py-2" />
                  <td class="px-2.5 py-2 font-mono text-xs text-t2">{{ group.networks.join(', ') }}</td>
                  <td class="px-2.5 py-2 text-t2">{{ group.outbounds.map(outboundName).join(', ') }}</td>
                  <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(group.traffic) }}</td>
                  <td class="px-3.5 py-2 text-right font-mono text-xs">{{ speed(group.speed) }}</td>
                </tr>
                <tr
                  v-for="c in group.connections"
                  :key="c.id"
                  class="border-t border-line text-t2 hover:bg-s2"
                  data-testid="conn-row"
                  @contextmenu="target = { id: c.id }"
                >
                  <td class="truncate py-2 pr-2.5 pl-7 text-t1" :title="c.rule">{{ c.destination }}</td>
                  <td class="px-2.5 py-2 font-mono text-xs">{{ c.source }}</td>
                  <td class="px-2.5 py-2 font-mono text-xs">{{ protocolOf(c) }}</td>
                  <td class="px-2.5 py-2">{{ outboundName(c.outbound) }}</td>
                  <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(c.upload + c.download) }}</td>
                  <td class="px-3.5 py-2 text-right font-mono text-xs">{{ speed(c.speed) }}</td>
                </tr>
              </template>
            </tbody>
            <tbody v-else>
              <tr
                v-for="c in listed"
                :key="c.id"
                class="border-t border-line text-t2 hover:bg-s2"
                data-testid="conn-row"
                @contextmenu="target = { id: c.id }"
              >
                <td class="px-3.5 py-2">
                  <div class="truncate text-t1" :title="c.rule">{{ c.destination }}</div>
                  <div class="truncate text-xs text-t3" :title="c.processPath">{{ processName(c.process) }}</div>
                </td>
                <td class="px-2.5 py-2 font-mono text-xs">{{ c.source }}</td>
                <td class="px-2.5 py-2 font-mono text-xs">{{ protocolOf(c) }}</td>
                <td class="px-2.5 py-2">{{ outboundName(c.outbound) }}</td>
                <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(c.upload + c.download) }}</td>
                <td class="px-3.5 py-2 text-right font-mono text-xs">{{ speed(c.speed) }}</td>
              </tr>
            </tbody>
          </table>
          <p v-if="shown.length === 0" class="px-3.5 py-6 text-center text-t3">{{ t(rows.length ? 'adv.connections.nothing' : 'adv.connections.none') }}</p>
        </div>
      </template>
      <ContextMenuItem :disabled="!target" @select="close"><X />{{ t(target?.id ? 'adv.connections.close' : 'adv.connections.closeProcess') }}</ContextMenuItem>
      <ContextMenuItem :disabled="rows.length === 0" @select="closeAll"><X />{{ t('adv.connections.closeAll') }}</ContextMenuItem>
    </ContextMenu>
  </section>
</template>
