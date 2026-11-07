<script setup lang="ts">
import { computed } from 'vue'
import { useI18n } from 'vue-i18n'

import OptionRow from '@/components/settings/OptionRow.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Select } from '@/components/ui'
import { useSetting } from '@/composables/useSetting'

const { t } = useI18n()
const language = useSetting('language')
const textSize = useSetting('textSize')

// Each language in its own words, so it can be found whatever the current one is.
const languages = computed(() => [
  { value: '', label: t('settings.look.system') },
  { value: 'ru', label: 'Русский' },
  { value: 'en', label: 'English' },
])
const sizes = computed(() => [
  { value: 'normal', label: t('settings.look.normal') },
  { value: 'large', label: t('settings.look.large') },
])
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.look') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.look.lead') }}</p>
  <SettingsCard>
    <OptionRow :title="t('settings.look.language')">
      <Select v-model="language" :options="languages" :aria-label="t('settings.look.language')" />
    </OptionRow>
    <OptionRow :title="t('settings.look.textSize')">
      <Select v-model="textSize" :options="sizes" :aria-label="t('settings.look.textSize')" />
    </OptionRow>
  </SettingsCard>
</template>
