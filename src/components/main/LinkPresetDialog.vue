<script setup lang="ts">
import { Info, Layers, Plus } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import SameNamePresetDialog from '@/components/presets/SameNamePresetDialog.vue'
import { Button, Dialog } from '@/components/ui'
import { usePresetSummary } from '@/composables/usePresetSummary'
import { useFailure } from '@/composables/useReason'
import type { Preset } from '@/platform/core-types'
import { MethodPresetsImport } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// A poros://route/ link from a browser or a messenger: the preset is added only after a yes, and under the name of
// the user's preset it is the question of SameNamePresetDialog.
const core = useCoreStore()
const ui = useUiStore()
const reason = useFailure()
const summaryOf = usePresetSummary()
const { t } = useI18n()
const busy = ref(false)
const failure = ref('')

const open = computed({
  get: () => ui.linkPreset !== null && !ui.linkPreset.same,
  set: (shown: boolean) => {
    if (!shown) ui.linkPreset = null
  },
})

watch(open, () => (failure.value = ''))

const preset = computed(() => ui.linkPreset?.preset)
const summary = computed(() => (preset.value ? summaryOf(preset.value) : ''))

function answered(saved: Preset, replaced: boolean): void {
  ui.linkPreset = null
  if (replaced) ui.notice = { tone: 'info', text: t('settings.presets.replaced', { name: saved.name }) }
}

async function add(): Promise<void> {
  const link = ui.linkPreset?.link
  if (!link || busy.value) return
  busy.value = true
  failure.value = ''
  try {
    await core.core().call(MethodPresetsImport, { link })
    open.value = false
  } catch (error) {
    failure.value = reason(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('linkPreset.title')">
    <template #icon><Layers /></template>
    <div class="grid gap-3">
      <p class="text-t2">{{ t('linkPreset.text', { name: preset?.name ?? '' }) }}</p>
      <div class="flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-3" data-testid="link-preset-summary">
        <Info class="size-4 flex-none text-ok" /><span>{{ summary }}</span>
      </div>
      <p v-if="failure" class="text-xs text-bad">{{ failure }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="busy" data-testid="link-preset-add" @click="add"><Plus />{{ t('linkPreset.add') }}</Button>
    </template>
  </Dialog>
  <SameNamePresetDialog :incoming="ui.linkPreset" @cancel="ui.linkPreset = null" @refreshed="(read) => (ui.linkPreset = read)" @done="answered" />
</template>
