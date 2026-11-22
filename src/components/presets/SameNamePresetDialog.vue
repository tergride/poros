<script setup lang="ts">
import { Layers, Plus, RefreshCw } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import type { IncomingPreset } from '@/composables/usePresetImport'
import { usePresetSummary } from '@/composables/usePresetSummary'
import { useFailure } from '@/composables/useReason'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import type { Preset } from '@/platform/core-types'
import { MethodPresetsImport, MethodPresetsRead, MethodPresetsReplace } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// An import under the name of the user's preset, from a link of the browser or from «Import a preset»: the imported
// preset takes its place or comes as a new one, and nothing changes without an answer. Cancel has the focus first.
const props = defineProps<{ incoming: IncomingPreset | null }>()
const emit = defineEmits<{ cancel: []; done: [preset: Preset, replaced: boolean]; refreshed: [incoming: IncomingPreset] }>()
const core = useCoreStore()
const ui = useUiStore()
const restart = useRestartNeeded()
const failed = useFailure()
const summary = usePresetSummary()
const { t } = useI18n()
const busy = ref(false)
const failure = ref('')

watch(
  () => props.incoming,
  () => (failure.value = ''),
)

const same = computed(() => props.incoming?.same)
const open = computed({
  get: () => !!same.value,
  set: (shown: boolean) => {
    if (!shown && !busy.value) emit('cancel')
  },
})
// The preset of the connection, chosen or not: replaced, it waits for a restart by the chip.
const inUse = computed(() => !!same.value && restart.presetInUse(same.value.id))

// A question under another reads again what its answer changed. Synchronously, while busy still tells the change of
// the answer given here: that one has nothing more to ask.
let reading = 0
watch(
  () => core.presets,
  async () => {
    const link = props.incoming?.link
    if (!link || !same.value || busy.value) return
    const turn = ++reading
    try {
      const read = await core.core().call(MethodPresetsRead, { link })
      if (turn === reading && !busy.value && props.incoming?.link === link) emit('refreshed', { ...read, link })
    } catch (error) {
      failure.value = failed(error)
    }
  },
  { flush: 'sync' },
)

async function answer(replace: boolean): Promise<void> {
  const incoming = props.incoming
  const old = incoming?.same
  if (!incoming || !old || busy.value) return
  busy.value = true
  failure.value = ''
  try {
    const { link } = incoming
    const preset = replace
      ? await core.core().call(MethodPresetsReplace, { id: old.id, link })
      : await core.core().call(MethodPresetsImport, { link })
    if (replace) {
      ui.replacedPreset = { id: preset.id }
      restart.presetSaved(preset.id)
    }
    emit('done', preset, replace)
  } catch (error) {
    failure.value = failed(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="t('samePreset.title')">
    <template #icon><Layers /></template>
    <div v-if="incoming && same" class="grid gap-3" data-testid="same-preset">
      <p class="break-words text-t2">{{ t('samePreset.text', { name: same.name }) }}</p>
      <dl class="grid grid-cols-[auto_minmax(0,1fr)] gap-x-3.5 gap-y-2 rounded-[10px] border border-line bg-s2 px-3.5 py-3">
        <dt class="text-t3">{{ t('samePreset.current') }}</dt>
        <dd data-testid="same-preset-current">{{ summary(same) }}</dd>
        <dt class="text-t3">{{ t('samePreset.incoming') }}</dt>
        <dd data-testid="same-preset-incoming">{{ summary(incoming.preset) }}</dd>
      </dl>
      <ul class="grid gap-1.5 text-[13px] text-t2">
        <li>{{ t('samePreset.replaceHint') }}</li>
        <li class="break-words">{{ t('samePreset.createHint', { name: incoming.newName }) }}</li>
      </ul>
      <p v-if="same.advanced && !incoming.preset.advanced" class="text-xs text-t3" data-testid="same-preset-stays-advanced">
        {{ t('samePreset.staysAdvanced') }}
      </p>
      <p v-if="inUse" class="text-xs text-t3" data-testid="same-preset-in-use">{{ t('samePreset.inUse') }}</p>
      <p v-if="failure" class="text-xs text-bad" role="alert" data-testid="same-preset-failure">{{ failure }}</p>
    </div>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button :disabled="busy" data-testid="same-preset-create" @click="answer(false)"><Plus />{{ t('samePreset.create') }}</Button>
      <Button variant="primary" :disabled="busy" data-testid="same-preset-replace" @click="answer(true)">
        <RefreshCw />{{ t('samePreset.replace') }}
      </Button>
    </template>
  </Dialog>
</template>
