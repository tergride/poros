<script setup lang="ts">
import { CircleQuestionMark, X } from '@lucide/vue'
import { computed } from 'vue'
import { I18nT, useI18n } from 'vue-i18n'

import { Button, Dialog } from '@/components/ui'
import { listKinds, protocols } from '@/lib/rules'

// How the lines of the lists window are written: a prefix per kind with what it takes and a line the core reads.
const props = defineProps<{ advanced: boolean }>()
const open = defineModel<boolean>('open', { default: false })
const { t } = useI18n()

const examples: Record<string, string> = {
  suffix: 'example.com',
  ip: '192.168.1.0/24',
  processName: 'steam.exe',
  processPath: 'C:\\Games\\game.exe',
  domain: 'www.example.com',
  keyword: 'video',
  regex: '^ads\\.',
  port: '8000-8080',
  processPathRegex: '(?i)\\\\games\\\\',
  protocol: 'quic',
  network: 'udp',
  ipVersion: '6',
  sourceIP: '192.168.1.20',
}

const kinds = computed(() => listKinds(props.advanced))
const kbd = 'rounded-[5px] border border-line2 bg-s3 px-1.5 py-px font-mono text-[11px] text-t2'
</script>

<template>
  <Dialog v-model:open="open" :title="t('presetLists.help.title')" class="w-[640px]">
    <template #icon><CircleQuestionMark /></template>
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="open = false"><X /></Button>
    </template>
    <p class="text-t2">{{ t('presetLists.help.intro') }}</p>
    <table class="mt-3 w-full border-collapse text-[13px]" data-testid="lists-help-kinds">
      <thead class="text-left text-xs text-t3">
        <tr>
          <th class="py-1.5 pr-3 font-medium">{{ t('presetLists.help.prefix') }}</th>
          <th class="px-3 py-1.5 font-medium">{{ t('presetLists.help.what') }}</th>
          <th class="py-1.5 pl-3 font-medium">{{ t('presetLists.help.example') }}</th>
        </tr>
      </thead>
      <tbody>
        <tr v-for="kind in kinds" :key="kind" class="border-t border-line">
          <td class="py-1.5 pr-3 font-mono text-[12.5px] whitespace-nowrap text-blue">{{ kind }}:</td>
          <td class="px-3 py-1.5">{{ t(`presetLists.help.kinds.${kind}`, { list: protocols.join(', ') }) }}</td>
          <td class="py-1.5 pl-3 font-mono text-[12.5px] whitespace-nowrap text-t2">{{ examples[kind] }}</td>
        </tr>
      </tbody>
    </table>
    <ul class="mt-3 grid gap-1.5 border-t border-line pt-3 text-xs text-t3">
      <li>{{ t('presetLists.help.notes') }}</li>
      <li>{{ t('presetLists.help.tabs') }}</li>
      <I18nT keypath="presetLists.help.keys" tag="li" scope="global">
        <template #up><kbd :class="kbd">↑</kbd></template>
        <template #down><kbd :class="kbd">↓</kbd></template>
        <template #enter><kbd :class="kbd">Enter</kbd></template>
        <template #tab><kbd :class="kbd">Tab</kbd></template>
        <template #esc><kbd :class="kbd">Esc</kbd></template>
      </I18nT>
      <li v-if="!advanced">{{ t('presetLists.help.more') }}</li>
    </ul>
  </Dialog>
</template>
