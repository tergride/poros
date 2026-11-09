<script setup lang="ts">
import { ChevronDown, ChevronRight, Copy, Gauge, MoreHorizontal, Pencil, RefreshCw, Trash2 } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SubscriptionQuota from '@/components/main/SubscriptionQuota.vue'
import { useClock } from '@/composables/useClock'
import { Button, Flag, Menu, MenuItem, MenuSeparator } from '@/components/ui'
import { formatAgo, latencyTone, serverTitle } from '@/lib/format'
import type { Group, Server } from '@/platform/core-types'
import { GroupManual } from '@/platform/core-types'

const props = defineProps<{ group: Group; servers: Server[]; selected: string; open: boolean; updating: boolean }>()
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

// A group of the user's has no provider: no quota, no update and no link.
const manual = computed(() => props.group.kind === GroupManual)
const used = computed(() => props.group.upload + props.group.download)
const summary = computed(() => {
  const servers = t('servers.count', props.group.servers)
  if (manual.value) return t('list.manualGroup', { servers })
  if (!props.group.updatedAt) return t('list.groupNever', { servers })
  return t('list.group', { servers, when: formatAgo(props.group.updatedAt, now.value, locale.value) })
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
          <strong class="block truncate font-semibold" :title="group.name">{{ group.name }}</strong>
          <small class="block truncate text-xs text-t3">{{ summary }}</small>
        </span>
      </button>
      <SubscriptionQuota v-if="!manual && (group.total > 0 || group.expire > 0 || used > 0)" :group="group" class="w-[196px]" />
      <Button v-if="!manual" variant="ghost" size="icon-sm" :aria-label="t('list.update')" :disabled="updating" @click="emit('update')">
        <RefreshCw :class="updating && 'animate-spin'" />
      </Button>
      <Menu align="end" min-width="240px">
        <template #trigger>
          <Button variant="ghost" size="icon-sm" :aria-label="t(manual ? 'list.groupActions' : 'list.actions')"><MoreHorizontal /></Button>
        </template>
        <MenuItem v-if="!manual" @select="emit('update')"><RefreshCw />{{ t('list.update') }}</MenuItem>
        <MenuItem @select="emit('test')"><Gauge />{{ t('list.test') }}</MenuItem>
        <MenuItem v-if="!manual" @select="emit('copyLink')"><Copy />{{ t('list.copyLink') }}</MenuItem>
        <MenuItem @select="emit('rename')"><Pencil />{{ t('list.rename') }}</MenuItem>
        <MenuSeparator />
        <MenuItem danger @select="emit('remove')"><Trash2 />{{ t(manual ? 'list.removeGroup' : 'list.removeSubscription') }}</MenuItem>
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
        <span class="inline-grid h-[22px] w-[30px] place-items-center">
          <Flag v-if="server.country" :code="server.country" class="text-[17px]" />
          <span v-else class="font-mono text-[11px] text-t3">··</span>
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
