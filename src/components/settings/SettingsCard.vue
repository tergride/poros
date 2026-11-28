<script setup lang="ts">
import { Check } from '@lucide/vue'
import { useI18n } from 'vue-i18n'

import { Button } from '@/components/ui'
import type { SettingsBlock } from '@/composables/useSettingsBlock'

// A card of a settings page. With a block its values wait for «Apply» at its foot, which shows once they change.
defineProps<{ title?: string; block?: SettingsBlock }>()
const { t } = useI18n()
</script>

<template>
  <section class="mb-3 rounded-xl border border-line bg-s2">
    <h3 v-if="title" class="px-4 pt-3 text-[13.5px] font-semibold">{{ title }}</h3>
    <div class="px-4 pt-1.5 pb-3"><slot /></div>
    <div v-if="block?.dirty" class="flex flex-wrap items-center justify-end gap-2 border-t border-line px-4 py-2.5" data-testid="block-bar">
      <p v-if="block.problem" class="min-w-[180px] flex-1 text-xs text-warn" role="alert" data-testid="block-problem">{{ block.problem }}</p>
      <Button variant="ghost" size="sm" :disabled="block.applying" data-testid="block-discard" @click="block.discard()">
        {{ t('settings.block.discard') }}
      </Button>
      <Button variant="primary" size="sm" :disabled="block.applying" data-testid="block-apply" @click="block.apply()">
        <Check />{{ t('settings.block.apply') }}
      </Button>
    </div>
  </section>
</template>
