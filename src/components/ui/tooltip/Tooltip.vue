<script setup lang="ts">
import { TooltipContent, TooltipPortal, TooltipProvider, TooltipRoot, TooltipTrigger } from 'reka-ui'

// A longer explanation that shows while the pointer rests on what it explains, or the keyboard focuses it, the way
// games explain their settings. The slot is the trigger; short hints stay in the text. Without text the slot stands
// alone, so that a row with and without an explanation is the same markup.
withDefaults(defineProps<{ text: string; side?: 'top' | 'right' | 'bottom' | 'left'; align?: 'start' | 'center' | 'end' }>(), {
  side: 'bottom',
  align: 'start',
})
</script>

<template>
  <slot v-if="!text" />
  <TooltipProvider v-else :delay-duration="300">
    <TooltipRoot>
      <TooltipTrigger as-child><slot /></TooltipTrigger>
      <TooltipPortal>
        <TooltipContent
          :side="side"
          :align="align"
          :side-offset="6"
          :collision-padding="12"
          class="z-[60] max-w-[400px] rounded-[10px] border border-line2 bg-s3 px-3.5 py-2.5 text-xs leading-relaxed text-t2 shadow-[0_18px_44px_rgba(0,0,0,.55)]"
          data-testid="tooltip"
        >
          {{ text }}
        </TooltipContent>
      </TooltipPortal>
    </TooltipRoot>
  </TooltipProvider>
</template>
