<script setup lang="ts">
import { RefreshCw, Settings2 } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import { Button, Select } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { formatBytes, serverTitle } from '@/lib/format'
import { MethodStatsTraffic, type TrafficHistory } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// «Statistics → Traffic»: the traffic of a period by profile and by program, from the history the core keeps.
const core = useCoreStore()
const router = useRouter()
const { t, locale } = useI18n()
const attempt = useAttempt()
const hours = ref(24)
const history = ref<TrafficHistory | null>(null)
const loading = ref(false)

const periods = computed(() => [
  { value: 24, label: t('adv.traffic.day') },
  { value: 24 * 7, label: t('adv.traffic.week') },
  { value: 24 * 30, label: t('adv.traffic.month') },
  { value: 24 * 90, label: t('adv.traffic.quarter') },
])
const kept = computed(() => !!core.settings?.stats && !!core.settings?.trafficHistory)
const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))

const profiles = computed(() =>
  (history.value?.profiles ?? []).map((item) => {
    const server = core.servers.find((s) => s.id === item.key)
    return {
      ...item,
      name: server ? serverTitle(server.name, server.country) : t('adv.traffic.removed'),
      group: server ? (core.groups.find((g) => g.id === server.groupId)?.name ?? '') : '',
    }
  }),
)
const apps = computed(() => (history.value?.apps ?? []).map((item) => ({ ...item, name: item.key || t('adv.connections.system') })))

async function load(): Promise<void> {
  loading.value = true
  try {
    history.value = await core.core().call(MethodStatsTraffic, { hours: hours.value })
  } finally {
    loading.value = false
  }
}
watch(hours, () => void attempt(load), { immediate: true })
</script>

<template>
  <div class="flex min-h-0 flex-1 flex-col">
    <div class="flex flex-none items-center gap-2 border-b border-line px-3.5 py-2.5">
      <Select v-model="hours" :options="periods" :aria-label="t('adv.traffic.period')" data-testid="traffic-period" />
      <Button size="sm" :disabled="loading" data-testid="traffic-refresh" @click="attempt(load)"><RefreshCw />{{ t('adv.traffic.refresh') }}</Button>
    </div>
    <div v-if="!kept" class="flex flex-none items-center gap-3 border-b border-line bg-s2 px-3.5 py-2.5 text-t2" data-testid="traffic-off">
      <span class="flex-1">{{ t('adv.traffic.off') }}</span>
      <Button size="sm" @click="router.push('/adv/settings/logging')"><Settings2 />{{ t('adv.connections.toSettings') }}</Button>
    </div>
    <div class="grid min-h-0 flex-1 grid-cols-2 gap-3 overflow-auto px-3.5 py-3">
      <section>
        <h3 class="mb-1.5 text-[13px] font-semibold">{{ t('adv.traffic.byProfile') }}</h3>
        <table class="w-full border-collapse text-[13px]" data-testid="traffic-profiles">
          <thead class="text-left text-xs text-t3">
            <tr>
              <th class="px-2.5 py-2 font-medium">{{ t('adv.traffic.profile') }}</th>
              <th class="px-2.5 py-2 font-medium">{{ t('adv.traffic.group') }}</th>
              <th class="px-2.5 py-2 text-right font-medium">{{ t('adv.traffic.download') }}</th>
              <th class="px-2.5 py-2 text-right font-medium">{{ t('adv.traffic.upload') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in profiles" :key="item.key" class="border-t border-line">
              <td class="truncate px-2.5 py-2">{{ item.name }}</td>
              <td class="truncate px-2.5 py-2 text-t2">{{ item.group }}</td>
              <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(item.download) }}</td>
              <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(item.upload) }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="profiles.length === 0" class="px-2.5 py-4 text-t3">{{ t('adv.traffic.empty') }}</p>
      </section>
      <section>
        <h3 class="mb-1.5 text-[13px] font-semibold">{{ t('adv.traffic.byApp') }}</h3>
        <table class="w-full border-collapse text-[13px]" data-testid="traffic-apps">
          <thead class="text-left text-xs text-t3">
            <tr>
              <th class="px-2.5 py-2 font-medium">{{ t('adv.traffic.app') }}</th>
              <th class="px-2.5 py-2 text-right font-medium">{{ t('adv.traffic.download') }}</th>
              <th class="px-2.5 py-2 text-right font-medium">{{ t('adv.traffic.upload') }}</th>
              <th class="px-2.5 py-2 text-right font-medium">{{ t('adv.traffic.total') }}</th>
            </tr>
          </thead>
          <tbody>
            <tr v-for="item in apps" :key="item.key" class="border-t border-line">
              <td class="truncate px-2.5 py-2">{{ item.name }}</td>
              <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(item.download) }}</td>
              <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(item.upload) }}</td>
              <td class="px-2.5 py-2 text-right font-mono text-xs">{{ bytes(item.download + item.upload) }}</td>
            </tr>
          </tbody>
        </table>
        <p v-if="apps.length === 0" class="px-2.5 py-4 text-t3">{{ t('adv.traffic.empty') }}</p>
      </section>
    </div>
  </div>
</template>
