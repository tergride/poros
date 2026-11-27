<script setup lang="ts">
import { Plus, Route, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, RadioGroup, RadioItem } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { MethodPresetsGet, MethodPresetsSave } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A new preset, or a copy of any preset, asks its kind first and is made at once: «New preset» or «… — copy», with a
// free name. A copy of the user's preset takes its base, rules, DNS and default as saved. A simple preset can become
// advanced later, never back, so a copy of an advanced one is advanced too.
// source is the preset to copy: "" for a new one from scratch, null for no dialog.
const props = defineProps<{ source: string | null }>()
const emit = defineEmits<{ close: []; created: [id: string] }>()
const core = useCoreStore()
const failureOf = useFailure()
const { t, te } = useI18n()
const kind = ref<string>('simple')
const busy = ref(false)
const failure = ref('')

const source = computed(() => core.presets.find((p) => p.id === props.source))
const open = computed({
  get: () => props.source !== null,
  set: (shown: boolean) => {
    if (!shown) emit('close')
  },
})
watch(open, (shown) => {
  if (!shown) return
  kind.value = source.value?.advanced ? 'advanced' : 'simple'
  failure.value = ''
})

const baseName = computed(() => {
  const id = props.source ?? ''
  return te(`presets.name.${id}`) ? t(`presets.name.${id}`) : (source.value?.name ?? id)
})
const title = computed(() => (props.source ? t('newPreset.copyTitle', { name: baseName.value }) : t('newPreset.title')))
const locked = (k: string) => k === 'simple' && !!source.value?.advanced

// The names of the presets as the user sees them: the built-in ones in the interface's language.
function freeName(name: string): string {
  const taken = core.presets.map((p) => (p.builtin && te(`presets.name.${p.id}`) ? t(`presets.name.${p.id}`) : p.name).trim().toLowerCase())
  let free = name
  for (let n = 2; taken.includes(free.toLowerCase()); n++) free = `${name} ${n}`
  return free
}

async function create(): Promise<void> {
  if (busy.value || props.source === null) return
  busy.value = true
  failure.value = ''
  try {
    const name = freeName(props.source ? t('presetEdit.copyName', { name: baseName.value }) : t('newPreset.name'))
    const advanced = kind.value === 'advanced'
    const mine = source.value && !source.value.builtin ? await core.core().call(MethodPresetsGet, { id: source.value.id }) : null
    const preset = await core.core().call(
      MethodPresetsSave,
      mine
        ? { ...mine, id: '', name, advanced: advanced || mine.advanced }
        : { id: '', builtin: false, basedOn: props.source, name, advanced, rules: [], dns: '', directDns: '', default: '', inherited: [] },
    )
    emit('created', preset.id)
  } catch (error) {
    failure.value = failureOf(error)
  } finally {
    busy.value = false
  }
}
</script>

<template>
  <Dialog v-model:open="open" :title="title">
    <template #icon><Route /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <RadioGroup v-model="kind" class="grid gap-1">
      <label
        v-for="k in ['simple', 'advanced'] as const"
        :key="k"
        :class="['grid grid-cols-[20px_1fr] items-center gap-3 border-t border-line py-2.5 first:border-t-0', locked(k) ? 'opacity-50' : 'cursor-pointer']"
      >
        <RadioItem :value="k" :disabled="locked(k)" :aria-label="t(`newPreset.${k}`)" :data-testid="`new-preset-${k}`" />
        <span>
          <strong class="block font-medium">{{ t(`newPreset.${k}`) }}</strong>
          <span class="text-xs text-t3">{{ t(`newPreset.${k}Text`) }}</span>
        </span>
      </label>
    </RadioGroup>
    <p class="mt-1.5 text-xs text-t3">{{ t('newPreset.note') }}</p>
    <p v-if="failure" class="mt-2 text-xs text-bad">{{ failure }}</p>
    <template #footer>
      <Button variant="ghost" class="sp" @click="open = false">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="busy" data-testid="new-preset-create" @click="create"><Plus />{{ t('newPreset.create') }}</Button>
    </template>
  </Dialog>
</template>
