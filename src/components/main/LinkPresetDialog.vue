<script setup lang="ts">
import { Info, Layers, Plus } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { MethodPresetsImport } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// A poros://route/ link from a browser or a messenger: the preset is added only after a yes.
const core = useCoreStore()
const ui = useUiStore()
const reason = useFailure()
const { t, te } = useI18n()
const busy = ref(false)
const failure = ref('')

const open = computed({
  get: () => ui.linkPreset !== null,
  set: (shown: boolean) => {
    if (!shown) ui.linkPreset = null
  },
})

watch(open, () => (failure.value = ''))

const preset = computed(() => ui.linkPreset?.preset)
const summary = computed(() => {
  const p = preset.value
  if (!p) return ''
  const added = { sites: p.sites.length, apps: p.apps.length }
  const base = te(`presets.name.${p.basedOn}`) ? t(`presets.name.${p.basedOn}`) : p.basedOn
  const parts = [p.basedOn ? `${t('presets.copyOf', { base })}: ${t('presets.added', added)}.` : `${t('presets.addedAlone', added)}.`]
  if (p.dns) parts.push(t('linkPreset.dns'))
  return parts.join(' ')
})

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
</template>
