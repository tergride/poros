<script setup lang="ts">
import { nextTick, onBeforeUnmount, onMounted, ref, watch } from 'vue'

import { listen, position, shown, tooltipId } from './tooltip'

// The one tooltip of the window, for every element with v-tooltip. It is placed once its size is known; it lies outside
// the app so that the large text zooms only its box, as with the popups of Reka.
const box = ref<HTMLElement | null>(null)
const place = ref<{ left: number; top: number } | null>(null)

let stop = () => {}
onMounted(() => (stop = listen(document)))
onBeforeUnmount(() => stop())

watch(
  () => [shown.target, shown.text] as const,
  async ([target]) => {
    place.value = null
    if (!target) return
    await nextTick()
    if (!box.value || shown.target !== target) return
    const view = { width: window.innerWidth, height: window.innerHeight }
    place.value = position(target.getBoundingClientRect(), box.value.getBoundingClientRect(), target.hasAttribute('data-tooltip-end'), view)
  },
)
</script>

<template>
  <Teleport to="body">
    <div
      v-if="shown.target"
      data-tooltip-wrapper
      class="pointer-events-none fixed z-[60]"
      :style="place ? { left: `${place.left}px`, top: `${place.top}px` } : { left: '0', top: '0', visibility: 'hidden' }"
    >
      <div
        :id="tooltipId"
        ref="box"
        role="tooltip"
        class="max-w-[400px] rounded-[10px] border border-line2 bg-s3 px-3.5 py-2.5 text-xs leading-relaxed [overflow-wrap:anywhere] text-t2 shadow-[0_18px_44px_rgba(0,0,0,.55)]"
        data-testid="tooltip"
      >
        {{ shown.text }}
      </div>
    </div>
  </Teleport>
</template>
