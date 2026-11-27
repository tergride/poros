<script setup lang="ts">
import { computed, ref, useId } from 'vue'
import { useI18n } from 'vue-i18n'

import { formatBytes } from '@/lib/format'
import { speedPoints, speedSeconds, speedTicks } from '@/lib/speedChart'
import type { Traffic } from '@/platform/core-types'

// The speed of the last minute: download and upload on one axis of bytes a second, a sample a second, the newest at
// the right edge. The legend names the lines with their speed now; the pointer or the arrow keys pick any second and
// show both speeds of it. Without samples only the baseline and the time axis stay. A title in the slot goes before
// the legend.
const props = defineProps<{ samples: Traffic[] }>()
const { t, locale } = useI18n()
const id = useId()

const width = 600
const height = 100
const series = [
  { key: 'download', color: 'var(--blue)', of: (s: Traffic) => s.download },
  { key: 'upload', color: 'var(--ok)', of: (s: Traffic) => s.upload },
] as const
const timeTicks = [1, 0.75, 0.5, 0.25, 0].map((part) => part * speedSeconds)

const recent = computed(() => props.samples.slice(-(speedSeconds + 1)))
const latest = computed(() => recent.value.at(-1) ?? null)
const ticks = computed(() => speedTicks(Math.max(0, ...recent.value.map((s) => Math.max(s.download, s.upload)))))
const top = computed(() => ticks.value.at(-1)!)
const lines = computed(() =>
  series.map((line) => {
    const points = speedPoints(recent.value.map(line.of), top.value, width, height)
    const path = points.map(([x, y]) => `${x.toFixed(1)},${y.toFixed(1)}`).join(' ')
    const area = points.length > 1 ? `M${points[0]![0].toFixed(1)},${height} L${path} L${width},${height} Z` : ''
    return { ...line, path: points.length > 1 ? path : '', area }
  }),
)
// The download line is drawn last, over the upload one where they meet.
const strokes = computed(() => [...lines.value].reverse())

// The second the pointer or the keys picked, in seconds before the newest sample.
const picked = ref<number | null>(null)
const shown = computed(() => {
  const count = recent.value.length
  if (picked.value === null || count === 0) return null
  const ago = Math.min(picked.value, count - 1)
  const sample = recent.value[count - 1 - ago]!
  return {
    ago,
    x: (speedSeconds - ago) / speedSeconds,
    values: series.map((line) => ({ ...line, value: line.of(sample), y: 1 - Math.min(line.of(sample), top.value) / top.value })),
  }
})

const bytes = (value: number) => formatBytes(value, locale.value, (unit) => t(`units.${unit}`))
const speed = (value: number) => t('units.perSecond', { value: bytes(value) })
const tickLabel = (value: number) => (value === 0 ? '0' : speed(value))
const timeLabel = (ago: number) => (ago === 0 ? t('adv.speed.now') : t('adv.speed.tick', { n: ago }))

function point(event: PointerEvent): void {
  const box = (event.currentTarget as HTMLElement).getBoundingClientRect()
  const fraction = Math.min(1, Math.max(0, (event.clientX - box.left) / box.width))
  picked.value = Math.round((1 - fraction) * speedSeconds)
}

function focus(event: FocusEvent): void {
  if ((event.currentTarget as HTMLElement).matches(':focus-visible')) picked.value = 0
}

function key(event: KeyboardEvent): void {
  const oldest = recent.value.length - 1
  if (oldest < 0) return
  const now = Math.min(picked.value ?? 0, oldest)
  const moves: Record<string, number> = { ArrowLeft: now + 1, ArrowRight: now - 1, Home: oldest, End: 0 }
  const next = moves[event.key]
  if (next === undefined) return
  event.preventDefault()
  picked.value = Math.min(oldest, Math.max(0, next))
}
</script>

<template>
  <div class="flex h-full min-h-0 flex-col gap-3">
    <div class="flex flex-none flex-wrap items-center gap-x-4 gap-y-1 text-xs">
      <slot />
      <span v-for="line in series" :key="line.key" class="inline-flex items-center gap-1.5" :data-testid="`speed-legend-${line.key}`">
        <span class="h-0.5 w-3.5 rounded-full" :style="{ background: line.color }" />
        <span class="text-t2">{{ t(`portal.${line.key}`) }}</span>
        <span class="min-w-[10ch] font-mono text-t1" data-testid="speed">{{ latest ? speed(line.of(latest)) : '—' }}</span>
      </span>
    </div>
    <div class="grid min-h-0 flex-1 grid-cols-[auto_minmax(0,1fr)] grid-rows-[minmax(0,1fr)_auto] gap-x-2 gap-y-1">
      <div class="relative text-right font-mono text-[11px] leading-none text-t3" :class="{ invisible: recent.length === 0 }" aria-hidden="true">
        <div class="invisible h-0">
          <div v-for="tick in ticks" :key="tick" class="whitespace-nowrap">{{ tickLabel(tick) }}</div>
        </div>
        <span
          v-for="tick in ticks"
          :key="tick"
          class="absolute right-0 -translate-y-1/2 whitespace-nowrap"
          :style="{ top: `${(1 - tick / top) * 100}%` }"
        >{{ tickLabel(tick) }}</span>
      </div>
      <div
        class="relative min-h-0 rounded-sm"
        tabindex="0"
        role="group"
        :aria-label="t('adv.speed.label')"
        data-testid="speed-plot"
        @pointermove="point"
        @pointerleave="picked = null"
        @focus="focus"
        @blur="picked = null"
        @keydown="key"
      >
        <svg :viewBox="`0 0 ${width} ${height}`" preserveAspectRatio="none" class="absolute inset-0 size-full overflow-visible" aria-hidden="true">
          <defs>
            <linearGradient v-for="line in series" :id="`${id}-${line.key}`" :key="line.key" x1="0" x2="0" y1="0" y2="1">
              <stop offset="0" :stop-color="line.color" stop-opacity="0.2" />
              <stop offset="1" :stop-color="line.color" stop-opacity="0" />
            </linearGradient>
          </defs>
          <line
            v-for="tick in recent.length ? ticks : [0]"
            :key="tick"
            x1="0"
            :x2="width"
            :y1="height - (tick / top) * height"
            :y2="height - (tick / top) * height"
            :stroke="tick === 0 ? 'var(--line2)' : 'var(--line)'"
            stroke-width="1"
            vector-effect="non-scaling-stroke"
          />
          <path v-for="line in lines" :key="`${line.key}-area`" :d="line.area" :fill="`url(#${id}-${line.key})`" />
          <line
            v-if="shown"
            :x1="shown.x * width"
            :x2="shown.x * width"
            y1="0"
            :y2="height"
            stroke="var(--t3)"
            stroke-opacity="0.6"
            stroke-width="1"
            vector-effect="non-scaling-stroke"
          />
          <polyline
            v-for="line in strokes"
            :key="line.key"
            :points="line.path"
            fill="none"
            :stroke="line.color"
            stroke-width="2"
            stroke-linejoin="round"
            stroke-linecap="round"
            vector-effect="non-scaling-stroke"
          />
        </svg>
        <p v-if="recent.length === 0" class="absolute inset-0 grid place-items-center px-4 text-center text-xs text-t3">{{ t('adv.speed.empty') }}</p>
        <template v-if="shown">
          <span
            v-for="value in shown.values"
            :key="value.key"
            class="pointer-events-none absolute size-2.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-s1"
            :style="{ left: `${shown.x * 100}%`, top: `${value.y * 100}%`, background: value.color }"
          />
          <div
            class="pointer-events-none absolute top-0 z-10 rounded-[10px] border border-line2 bg-s3 px-3 py-2 text-xs whitespace-nowrap shadow-[0_18px_44px_rgba(0,0,0,.55)]"
            :style="shown.x > 0.6 ? { right: `calc(${(1 - shown.x) * 100}% + 12px)` } : { left: `calc(${shown.x * 100}% + 12px)` }"
            data-testid="speed-tip"
          >
            <p class="mb-1 text-t3">{{ shown.ago === 0 ? t('adv.speed.current') : t('adv.speed.ago', { n: shown.ago }) }}</p>
            <div class="grid grid-cols-[auto_auto_auto] items-center gap-x-2 gap-y-0.5">
              <template v-for="value in shown.values" :key="value.key">
                <span class="h-0.5 w-3 rounded-full" :style="{ background: value.color }" />
                <span class="text-right font-mono font-medium text-t1">{{ speed(value.value) }}</span>
                <span class="text-t2">{{ t(`portal.${value.key}`) }}</span>
              </template>
            </div>
          </div>
        </template>
      </div>
      <div />
      <div class="relative h-3.5 text-[11px] leading-none text-t3" aria-hidden="true">
        <span
          v-for="ago in timeTicks"
          :key="ago"
          class="absolute top-0 whitespace-nowrap"
          :class="ago === speedSeconds ? '' : ago === 0 ? '-translate-x-full' : '-translate-x-1/2'"
          :style="{ left: `${((speedSeconds - ago) / speedSeconds) * 100}%` }"
        >{{ timeLabel(ago) }}</span>
      </div>
    </div>
  </div>
</template>
