<script setup lang="ts">
import { Lock, TriangleAlert } from '@lucide/vue'
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Segmented } from '@/components/ui'
import { usePresetDraft } from '@/composables/usePresetDraft'
import { ruleActions } from '@/lib/rules'
import { ActionBlock } from '@/platform/core-types'

// Where the preset sends what none of its rules takes. It comes first: it decides the most. A built-in preset shows
// its choice under a lock.
const { draft } = usePresetDraft()
const { t } = useI18n()
const actions = computed(() => ruleActions.map((value) => ({ value, label: t(`action.${value}`) })))
const choice = computed({
  get: () => draft.value?.default ?? '',
  set: (value: string) => {
    if (draft.value && !draft.value.builtin) draft.value.default = value
  },
})
</script>

<template>
  <SettingsCard v-if="draft" :title="t('presetEdit.default')" data-testid="preset-default">
    <p class="mb-2 text-xs text-t3">{{ t('presetEdit.defaultHint') }}</p>
    <div class="flex items-center gap-2">
      <fieldset :disabled="draft.builtin" class="contents">
        <Segmented v-model="choice" :options="actions" :aria-label="t('presetEdit.default')" />
      </fieldset>
      <Lock v-if="draft.builtin" class="size-3.5 text-t3" />
    </div>
    <div
      v-if="!draft.builtin && draft.default === ActionBlock"
      class="mt-2 flex items-center gap-2.5 rounded-[10px] border border-line bg-s2 px-3.5 py-2.5 text-warn"
    >
      <TriangleAlert class="size-4 flex-none" /><span>{{ t('presetEdit.defaultBlocked') }}</span>
    </div>
  </SettingsCard>
</template>
