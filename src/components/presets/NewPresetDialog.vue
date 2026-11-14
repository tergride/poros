<script setup lang="ts">
import { Plus, Route, X } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Button, Dialog, RadioGroup, RadioItem } from '@/components/ui'
import { useFailure } from '@/composables/useReason'
import { MethodPresetsSave } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// A new preset, or a copy of a built-in one, asks its kind first and is made at once: «New preset» or «… — copy»,
// with a free name. A simple preset can become advanced later, never back.
const props = defineProps<{ basedOn: string | null }>()
const emit = defineEmits<{ close: []; created: [id: string] }>()
const core = useCoreStore()
const failureOf = useFailure()
const { t, te } = useI18n()
const kind = ref<string>('simple')
const busy = ref(false)
const failure = ref('')

const open = computed({
  get: () => props.basedOn !== null,
  set: (shown: boolean) => {
    if (!shown) emit('close')
  },
})
watch(open, (shown) => {
  if (!shown) return
  kind.value = 'simple'
  failure.value = ''
})

const baseName = computed(() => {
  const id = props.basedOn ?? ''
  return te(`presets.name.${id}`) ? t(`presets.name.${id}`) : (core.presets.find((p) => p.id === id)?.name ?? id)
})
const title = computed(() => (props.basedOn ? t('newPreset.copyTitle', { name: baseName.value }) : t('newPreset.title')))

// The names of the presets as the user sees them: the built-in ones in the interface's language.
function freeName(name: string): string {
  const taken = core.presets.map((p) => (p.builtin && te(`presets.name.${p.id}`) ? t(`presets.name.${p.id}`) : p.name).trim().toLowerCase())
  let free = name
  for (let n = 2; taken.includes(free.toLowerCase()); n++) free = `${name} ${n}`
  return free
}

async function create(): Promise<void> {
  if (busy.value || props.basedOn === null) return
  busy.value = true
  failure.value = ''
  try {
    const name = freeName(props.basedOn ? t('presetEdit.copyName', { name: baseName.value }) : t('newPreset.name'))
    const preset = await core.core().call(MethodPresetsSave, {
      id: '',
      builtin: false,
      basedOn: props.basedOn,
      name,
      advanced: kind.value === 'advanced',
      rules: [],
      dns: '',
      directDns: '',
      default: '',
      inherited: [],
      endpoints: [],
    })
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
      <label v-for="k in ['simple', 'advanced'] as const" :key="k" class="grid cursor-pointer grid-cols-[20px_1fr] items-center gap-3 border-t border-line py-2.5 first:border-t-0">
        <RadioItem :value="k" :aria-label="t(`newPreset.${k}`)" :data-testid="`new-preset-${k}`" />
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
