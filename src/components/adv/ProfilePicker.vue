<script setup lang="ts">
import { Plus } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'

import { Banner, Dialog } from '@/components/ui'
import { useProfileForms } from '@/composables/useProfileForms'
import { useReason } from '@/composables/useReason'
import { protocolName } from '@/lib/format'
import type { NewProfile } from '@/platform/profile-types'
import { useUiStore } from '@/stores/ui'

// «New profile»: what the main sing-box runs, by kind; a choice opens the editor on the profile it starts from.
const ui = useUiStore()
const reason = useReason()
const { forms, load } = useProfileForms()
const { t } = useI18n()

const failure = ref('')
watch(
  () => ui.profilePicker,
  (target) => {
    if (!target) return
    failure.value = ''
    load().catch((error: unknown) => (failure.value = reason(error)))
  },
  { immediate: true },
)

const kinds = ['proxy', 'quic', 'vpn', 'compound']
const groups = computed(() => kinds.map((kind) => ({ kind, choices: (forms.value?.new ?? []).filter((n) => n.kind === kind) })))

function pick(choice: NewProfile): void {
  const target = ui.profilePicker
  if (!target) return
  ui.profilePicker = null
  ui.profileEdit = { id: '', profile: JSON.parse(JSON.stringify(choice.profile)), target }
}
</script>

<template>
  <Dialog :open="ui.profilePicker !== null" :title="t('profileEditor.newProfile')" class="w-[640px]" @update:open="(open) => !open && (ui.profilePicker = null)">
    <template #icon><Plus /></template>
    <Banner v-if="failure" class="mb-3">{{ failure }}</Banner>
    <template v-for="g in groups" :key="g.kind">
      <div class="pt-1.5 pb-1 text-[11.5px] text-t3">{{ t(`profileEditor.kind.${g.kind}`) }}</div>
      <div class="mb-2 grid grid-cols-4 gap-1.5">
        <button
          v-for="choice in g.choices"
          :key="choice.id"
          type="button"
          class="flex h-9 cursor-pointer items-center rounded-lg border border-line bg-s3 px-2.5 text-left text-[13px] hover:border-blue"
          :data-testid="`new-${choice.id}`"
          @click="pick(choice)"
        >
          {{ protocolName(choice.id, t) }}
        </button>
      </div>
    </template>
  </Dialog>
</template>
