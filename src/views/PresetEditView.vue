<script setup lang="ts">
import { ArrowLeft, Layers, Lock, TextAlignStart } from '@lucide/vue'
import { computed, onMounted } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import PresetEditor from '@/components/presets/PresetEditor.vue'
import { Banner, Button, Input } from '@/components/ui'
import { providePresetDraft } from '@/composables/usePresetDraft'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import { useCoreStore } from '@/stores/core'

// The editor of a preset in the simple mode, a page of its own. It works on a draft: leaving without saving drops it
// (note 5), and the next visit starts from what the core holds. Saving applies the preset too.
const core = useCoreStore()
const restart = useRestartNeeded()
const route = useRoute()
const router = useRouter()
const { t } = useI18n()
const editor = providePresetDraft()
const { draft, failure, listsOpen, toAdvancedOpen } = editor

onMounted(() => void editor.load(String(route.params.id)))

const baseName = computed(() => editor.shownName(core.presets.find((p) => p.id === draft.value?.basedOn)))
const back = () => router.push('/settings/presets')

async function save(): Promise<void> {
  const saved = await editor.save()
  if (!saved) return
  // The preset in use changes; a running connection takes it when the user restarts it from the chip.
  await core.updateSettings({ preset: saved.id })
  if (core.running) restart.presetEdited = true
  await back()
}
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col">
    <div class="min-h-0 flex-1 overflow-auto px-7 pt-[22px] pb-7">
      <div class="mb-3 flex items-center gap-2">
        <Button variant="ghost" size="sm" @click="back"><ArrowLeft />{{ t('presetEdit.back') }}</Button>
        <span v-if="draft?.builtin" class="inline-flex h-[22px] items-center gap-[5px] rounded-md bg-s4 px-2 text-[11.5px] text-t2">
          <Lock class="size-3" />{{ t('presets.builtin') }}
        </span>
        <span v-if="draft?.basedOn" class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] text-t2" data-testid="preset-base">
          {{ t('presetEdit.copyOf', { name: baseName }) }}
        </span>
        <span v-if="draft?.advanced" class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] text-t2" data-testid="preset-advanced">
          {{ t('presets.advanced') }}
        </span>
        <template v-if="draft && !draft.builtin && !draft.advanced">
          <Button size="sm" class="ml-auto" data-testid="preset-to-advanced" @click="toAdvancedOpen = true"><Layers />{{ t('toAdvanced.button') }}</Button>
          <Button size="sm" data-testid="preset-lists" @click="listsOpen = true"><TextAlignStart />{{ t('presetLists.open') }}</Button>
        </template>
      </div>
      <Banner v-if="failure" class="mb-3" data-testid="preset-failure">{{ failure }}</Banner>
      <template v-if="draft">
        <p v-if="draft.builtin" class="mb-2.5 flex items-center gap-1.5 text-xs text-t3"><Lock class="size-3.5" />{{ t('presetEdit.builtinNote') }}</p>
        <label class="mb-3.5 flex max-w-[420px] flex-col gap-[5px]">
          <span class="text-[12.5px] text-t2">{{ t('presetEdit.name') }}</span>
          <Input v-if="draft.builtin" :model-value="editor.shownName(draft)" disabled data-testid="preset-name" />
          <Input v-else v-model="draft.name" :placeholder="t('presetEdit.namePlaceholder')" data-testid="preset-name" />
        </label>
        <PresetEditor />
      </template>
    </div>
    <div class="flex items-center gap-2 border-t border-line px-5 py-3">
      <Button variant="ghost" class="mr-auto" @click="back">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="!draft || draft.builtin" data-testid="preset-save" @click="save">{{ t('presetEdit.save') }}</Button>
    </div>
  </div>
</template>
