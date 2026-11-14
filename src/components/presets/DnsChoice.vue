<script setup lang="ts">
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import { Input, Select } from '@/components/ui'

// A DNS server as a settings row: the known choices, and «Custom…» with its address right under the row. The model is
// what the core keeps: "" for the default, a known address, or the user's own. Lazy takes the address once it is
// typed, for settings that save at once.
const props = defineProps<{ title: string; hint: string; choices: { value: string; label: string }[]; disabled?: boolean; lazy?: boolean }>()
const model = defineModel<string>({ required: true })
const { t } = useI18n()

const custom = '\u0000custom'
const known = (value: string) => props.choices.some((c) => c.value === value)
// «Custom…» with nothing typed yet is still a choice of its own, not the default.
const choice = ref(known(model.value) ? model.value : custom)
const own = ref(known(model.value) ? '' : model.value)
watch(model, (value) => {
  if (value === (choice.value === custom ? own.value : choice.value)) return
  choice.value = known(value) ? value : custom
  if (!known(value)) own.value = value
})

const options = computed(() => [...props.choices, { value: custom, label: t('presetEdit.dnsCustom') }])
const picked = computed({
  get: () => choice.value,
  set: (value: string) => {
    choice.value = value
    model.value = value === custom ? own.value : value
  },
})
const address = computed({
  get: () => own.value,
  set: (value: string) => {
    own.value = value
    if (!props.lazy) model.value = value
  },
})
function typed(): void {
  if (props.lazy && choice.value === custom) model.value = own.value.trim()
}
</script>

<template>
  <OptionRow :title="title" :hint="hint">
    <Select v-model="picked" :options="options" :disabled="disabled" class="w-[190px]" :aria-label="title" />
  </OptionRow>
  <div v-if="choice === custom" class="grid gap-1 pb-3">
    <Input
      v-model="address"
      mono
      :disabled="disabled"
      placeholder="https://dns.example.net/dns-query"
      :aria-label="t('presetEdit.dnsOwn', { title })"
      @change="typed"
      @keydown.enter="typed"
    />
    <span class="text-xs text-t3">{{ t('presetEdit.dnsCustomHint') }}</span>
  </div>
</template>
