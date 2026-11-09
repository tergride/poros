<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import { Tooltip } from '@/components/ui'
import { useClock } from '@/composables/useClock'
import { formatAgo, formatBytes, formatDate } from '@/lib/format'
import { quotaOf } from '@/lib/quota'
import type { Group } from '@/platform/core-types'

// The traffic and the end date of a subscription in its header, the same in both modes. What runs out or ran out turns
// red, and the hint on it says why. The width comes from the header.
defineOptions({ inheritAttrs: false })
const props = defineProps<{ group: Group }>()
const { t, locale } = useI18n()
const now = useClock(60_000)

const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))
const quota = computed(() => quotaOf(props.group, now.value))
const trafficRed = computed(() => quota.value.traffic === 'low' || quota.value.traffic === 'out')
const timeRed = computed(() => quota.value.time === 'soon' || quota.value.time === 'over')
const date = computed(() => formatDate(props.group.expire, locale.value, now.value))

const trafficText = computed(() => {
  const q = quota.value
  if (q.traffic === 'out') return t('list.exhausted')
  if (q.total > 0) return t('list.quota', { used: bytes(q.used), total: bytes(q.total) })
  return q.used > 0 ? t('list.used', { used: bytes(q.used) }) : t('list.unlimited')
})
const timeText = computed(() => t(quota.value.time === 'over' ? 'list.expired' : 'list.until', { date: date.value }))
const why = computed(() => {
  const q = quota.value
  const reasons: string[] = []
  if (q.traffic === 'out') reasons.push(t('list.trafficOut'))
  if (q.traffic === 'low') reasons.push(t('list.trafficLow', { left: bytes(q.total - q.used), total: bytes(q.total) }))
  if (q.time === 'over') reasons.push(t('list.ended', { date: date.value }))
  if (q.time === 'soon') reasons.push(t('list.endingSoon', { when: formatAgo(props.group.expire, now.value, locale.value), date: date.value }))
  return reasons.join(' ')
})
</script>

<template>
  <Tooltip :text="why" align="end">
    <!-- A long line, as in English, moves the date below whole instead of breaking it inside. -->
    <div
      v-bind="$attrs"
      class="grid flex-none gap-1 text-[11.5px] text-t2"
      :class="why && 'cursor-help rounded-md'"
      :tabindex="why ? 0 : undefined"
      :data-running-out="why ? '' : undefined"
      data-testid="quota"
    >
      <div class="flex flex-wrap justify-between gap-x-2 whitespace-nowrap">
        <span :class="trafficRed && 'text-bad'" data-testid="quota-traffic">{{ trafficText }}</span>
        <span v-if="group.expire > 0" :class="timeRed ? 'text-bad' : 'text-t3'" data-testid="quota-time">{{ timeText }}</span>
      </div>
      <div v-if="group.total > 0" class="h-[5px] overflow-hidden rounded-[3px] bg-s5">
        <i
          :class="['block h-full rounded-[3px]', trafficRed || quota.time === 'over' ? 'bg-bad' : 'bg-[image:var(--road)]']"
          :style="{ width: `${quota.percent}%` }"
        />
      </div>
    </div>
  </Tooltip>
</template>
