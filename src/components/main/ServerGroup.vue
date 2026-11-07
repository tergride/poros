<script setup lang="ts">
import { ChevronDown, ChevronRight, Copy, Gauge, MoreHorizontal, Pencil, RefreshCw, Trash2 } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { useClock } from '@/composables/useClock'
import { Button, Menu, MenuItem, MenuSeparator } from '@/components/ui'
import { formatAgo, formatBytes, formatDate, latencyTone, serverTitle } from '@/lib/format'
import type { Server, Subscription } from '@/platform/core-types'

const props = defineProps<{ subscription: Subscription; servers: Server[]; selected: string; open: boolean; updating: boolean }>()
const emit = defineEmits<{
  toggle: []
  pick: [id: string]
  update: []
  test: []
  copyLink: []
  rename: []
  remove: []
}>()

const { t, locale } = useI18n()
const now = useClock(60_000)

const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))
const used = computed(() => props.subscription.upload + props.subscription.download)
const percent = computed(() => (props.subscription.total > 0 ? Math.min(100, (used.value / props.subscription.total) * 100) : 0))
// A subscription that ran out works no more at the provider's: it says so instead of looking like the others.
const expired = computed(() => props.subscription.expire > 0 && props.subscription.expire <= now.value)
const exhausted = computed(() => props.subscription.total > 0 && used.value >= props.subscription.total)
const quotaText = computed(() => {
  const s = props.subscription
  if (s.total > 0) return exhausted.value ? t('list.exhausted') : t('list.quota', { used: bytes(used.value), total: bytes(s.total) })
  return used.value > 0 ? t('list.used', { used: bytes(used.value) }) : t('list.unlimited')
})
const untilText = computed(() => {
  const date = formatDate(props.subscription.expire, locale.value, now.value)
  return expired.value ? t('list.expired', { date }) : t('list.until', { date })
})
const summary = computed(() => {
  const servers = t('servers.count', props.subscription.servers)
  if (!props.subscription.updatedAt) return t('list.groupNever', { servers })
  return t('list.group', { servers, when: formatAgo(props.subscription.updatedAt, now.value, locale.value) })
})

const toneClass = { good: 'text-ok', mid: 'text-warn', bad: 'text-bad', none: 'text-t3' }
function latencyText(latency: number): string {
  if (latency === 0) return t('ping.untested')
  return latency < 0 ? t('ping.none') : t('ping.ms', { n: latency })
}
</script>

<template>
  <section class="flex-none overflow-hidden rounded-xl border border-line bg-s2" data-testid="server-group">
    <div class="flex items-center gap-2 py-[9px] pr-2.5 pl-3">
      <button
        type="button"
        class="flex min-w-0 flex-1 cursor-pointer items-center gap-2.5 text-left"
        :aria-expanded="open"
        @click="emit('toggle')"
      >
        <ChevronDown v-if="open" class="size-4 flex-none text-t2" /><ChevronRight v-else class="size-4 flex-none text-t2" />
        <span class="min-w-0">
          <strong class="block truncate font-semibold" :title="subscription.name">{{ subscription.name }}</strong>
          <small class="block truncate text-xs text-t3">{{ summary }}</small>
        </span>
      </button>
      <!-- A long line, as in English, moves the date below whole instead of breaking it inside. -->
      <div
        v-if="subscription.total > 0 || subscription.expire > 0 || used > 0"
        class="grid w-[196px] flex-none gap-1 text-[11.5px] text-t2"
        data-testid="quota"
      >
        <div class="flex flex-wrap justify-between gap-x-2 whitespace-nowrap">
          <span :class="exhausted && 'text-warn'">{{ quotaText }}</span>
          <span v-if="subscription.expire > 0" :class="expired ? 'text-warn' : 'text-t3'">{{ untilText }}</span>
        </div>
        <div v-if="subscription.total > 0" class="h-[5px] overflow-hidden rounded-[3px] bg-s5">
          <i
            :class="['block h-full rounded-[3px]', exhausted || expired ? 'bg-warn' : 'bg-[image:var(--road)]']"
            :style="{ width: `${percent}%` }"
          />
        </div>
      </div>
      <Button variant="ghost" size="icon-sm" :aria-label="t('list.update')" :disabled="updating" @click="emit('update')">
        <RefreshCw :class="updating && 'animate-spin'" />
      </Button>
      <Menu align="end" min-width="240px">
        <template #trigger>
          <Button variant="ghost" size="icon-sm" :aria-label="t('list.actions')"><MoreHorizontal /></Button>
        </template>
        <MenuItem @select="emit('update')"><RefreshCw />{{ t('list.update') }}</MenuItem>
        <MenuItem @select="emit('test')"><Gauge />{{ t('list.test') }}</MenuItem>
        <MenuItem @select="emit('copyLink')"><Copy />{{ t('list.copyLink') }}</MenuItem>
        <MenuItem @select="emit('rename')"><Pencil />{{ t('list.rename') }}</MenuItem>
        <MenuSeparator />
        <MenuItem danger @select="emit('remove')"><Trash2 />{{ t('list.removeSubscription') }}</MenuItem>
      </Menu>
    </div>
    <template v-if="open">
      <button
        v-for="server in servers"
        :key="server.id"
        type="button"
        :class="[
          'grid h-[42px] w-full cursor-pointer grid-cols-[30px_1fr_auto_20px] items-center gap-2.5 border-t border-line px-3 text-left',
          server.id === selected ? 'bg-s4' : 'hover:bg-s3',
        ]"
        data-testid="server"
        :aria-pressed="server.id === selected"
        @click="emit('pick', server.id)"
      >
        <span class="inline-grid h-[22px] w-[30px] place-items-center rounded-[5px] bg-s4 font-mono text-[11px] font-medium">
          {{ server.country || '··' }}
        </span>
        <span class="truncate">{{ serverTitle(server.name, server.country) }}</span>
        <span :class="['font-mono text-[12.5px] font-medium whitespace-nowrap', toneClass[latencyTone(server.latency)]]">
          {{ latencyText(server.latency) }}
        </span>
        <span
          :class="[
            'size-4 justify-self-end rounded-full border-[1.5px]',
            server.id === selected ? 'border-[5px] border-blue' : 'border-line2',
          ]"
        />
      </button>
    </template>
  </section>
</template>
