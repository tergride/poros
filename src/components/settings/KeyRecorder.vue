<script setup lang="ts">
import { X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'

import { shortcutOf } from '@/lib/shortcuts'

// A field that records a shortcut: a click waits for the keys, Escape gives up, Backspace or Delete alone, and the
// cross, clear it. bad marks a shortcut that cannot be taken.
const model = defineModel<string>({ required: true })
defineProps<{ bad?: boolean; label: string }>()
const { t } = useI18n()
const recording = ref(false)
const parts = computed(() => (model.value ? model.value.split('+') : []))

function onKey(event: KeyboardEvent): void {
  if (!recording.value) return
  event.preventDefault()
  event.stopPropagation()
  const bare = !event.ctrlKey && !event.altKey && !event.shiftKey && !event.metaKey
  if (bare && event.code === 'Escape') {
    recording.value = false
    return
  }
  if (bare && (event.code === 'Backspace' || event.code === 'Delete')) {
    model.value = ''
    recording.value = false
    return
  }
  const shortcut = shortcutOf(event)
  if (!shortcut) return
  model.value = shortcut
  recording.value = false
}
</script>

<template>
  <!-- The field shows the focus of the keyboard as a field of text does. -->
  <span
    :class="[
      'inline-flex h-[30px] min-w-[180px] items-center gap-1 rounded-lg border bg-s0 pr-1 pl-2.5 font-mono text-xs text-t1 has-[:focus-visible]:border-blue has-[:focus-visible]:shadow-[0_0_0_3px_color-mix(in_srgb,var(--blue)_18%,transparent)]',
      recording ? 'border-blue shadow-[0_0_0_3px_color-mix(in_srgb,var(--blue)_18%,transparent)]' : bad ? 'border-warn' : 'border-line2',
    ]"
  >
    <button
      type="button"
      class="flex h-full min-w-0 flex-1 cursor-pointer items-center gap-[3px] outline-none"
      :aria-label="label"
      data-testid="key-recorder"
      @click="recording = !recording"
      @keydown="onKey"
      @blur="recording = false"
    >
      <span v-if="recording" class="font-sans text-t2">{{ t('settings.hotkeys.press') }}</span>
      <template v-else-if="parts.length">
        <template v-for="(part, i) in parts" :key="i">
          <span v-if="i > 0" class="text-t3">+</span>
          <kbd class="rounded-[5px] bg-s4 px-1.5 py-[3px] text-[11.5px] font-medium">{{ part }}</kbd>
        </template>
      </template>
      <span v-else class="mx-auto font-sans text-t3">{{ t('settings.hotkeys.none') }}</span>
    </button>
    <button
      v-if="model && !recording"
      type="button"
      class="grid size-[22px] flex-none cursor-pointer place-items-center rounded-[5px] text-t3 hover:bg-s4 hover:text-t1 [&_svg]:size-3.5"
      :aria-label="t('settings.hotkeys.clear')"
      data-testid="key-clear"
      @click="model = ''"
    >
      <X />
    </button>
  </span>
</template>
