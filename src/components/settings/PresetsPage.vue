<script setup lang="ts">
import { ClipboardPaste, Copy, Lock, MoreHorizontal, Pencil, Plus, Trash2, X } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRouter } from 'vue-router'

import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Banner, Button, Dialog, Menu, MenuItem, MenuSeparator, RadioGroup, RadioItem } from '@/components/ui'
import { useReason } from '@/composables/useReason'
import type { Preset } from '@/platform/core-types'
import { MethodPresetsExport, MethodPresetsImport, MethodPresetsRemove } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const router = useRouter()
const reason = useReason()
const { t, te } = useI18n()
const notice = ref<{ tone: 'info' | 'warn'; text: string } | null>(null)
const removing = ref<Preset | null>(null)

const builtins = computed(() => core.presets.filter((p) => p.builtin))
const mine = computed(() => core.presets.filter((p) => !p.builtin))

const nameOf = (preset: Preset | undefined) =>
  !preset ? '' : preset.builtin && te(`presets.name.${preset.id}`) ? t(`presets.name.${preset.id}`) : preset.name
// A preset made from scratch has no base; removing it while chosen goes back to the standard one, all traffic.
const baseOf = (preset: Preset) => nameOf(core.presets.find((p) => p.id === (preset.basedOn || 'all')))
const aboutMine = (preset: Preset) => {
  const added = { sites: preset.sites, apps: preset.apps }
  return preset.basedOn
    ? `${t('presets.copyOf', { base: baseOf(preset) })}: ${t('presets.added', added)}`
    : t('presets.addedAlone', added)
}

const selected = computed({
  get: () => core.settings?.preset ?? '',
  set: (id: string) => void core.updateSettings({ preset: id }),
})

async function exportLink(preset: Preset): Promise<void> {
  const { link } = await core.core().call(MethodPresetsExport, { id: preset.id })
  await navigator.clipboard.writeText(link)
  notice.value = { tone: 'info', text: t('settings.presets.exported') }
}

async function importLink(): Promise<void> {
  try {
    const link = (await navigator.clipboard.readText()).trim()
    const preset = await core.core().call(MethodPresetsImport, { link })
    notice.value = { tone: 'info', text: t('settings.presets.imported', { name: preset.name }) }
  } catch (error) {
    notice.value = { tone: 'warn', text: reason(error) }
  }
}

async function remove(): Promise<void> {
  if (!removing.value) return
  await core.core().call(MethodPresetsRemove, { id: removing.value.id })
  removing.value = null
}
</script>

<template>
  <h2 class="mb-1 font-disp text-[19px] font-normal">{{ t('settings.nav.presets') }}</h2>
  <p class="mb-4 max-w-[640px] text-t2">{{ t('settings.presets.lead') }}</p>
  <Banner v-if="notice" :tone="notice.tone" class="mb-3">
    {{ notice.text }}
    <template #actions>
      <Button variant="ghost" size="icon-sm" :aria-label="t('common.close')" @click="notice = null"><X /></Button>
    </template>
  </Banner>
  <RadioGroup v-model="selected">
    <SettingsCard :title="t('settings.presets.builtin')">
      <div
        v-for="preset in builtins"
        :key="preset.id"
        class="grid grid-cols-[20px_1fr_auto] items-center gap-3 border-t border-line py-3 first:border-t-0"
        data-testid="preset"
      >
        <RadioItem :value="preset.id" :aria-label="nameOf(preset)" />
        <div>
          <strong class="flex items-center gap-2 font-medium">
            {{ nameOf(preset) }}
            <span class="inline-flex h-[22px] items-center gap-[5px] rounded-md bg-s4 px-2 text-[11.5px] text-t2">
              <Lock class="size-3" />{{ t('presets.builtin') }}
            </span>
          </strong>
          <span class="text-xs text-t3">{{ te(`presets.about.${preset.id}`) ? t(`presets.about.${preset.id}`) : '' }}</span>
        </div>
        <Button size="sm" @click="router.push(`/preset/${preset.id}`)"><Copy />{{ t('settings.presets.makeCopy') }}</Button>
      </div>
    </SettingsCard>
    <SettingsCard :title="t('settings.presets.mine')">
      <p v-if="mine.length === 0" class="py-3 text-t3">{{ t('settings.presets.none') }}</p>
      <div
        v-for="preset in mine"
        :key="preset.id"
        class="grid grid-cols-[20px_1fr_auto] items-center gap-3 border-t border-line py-3 first:border-t-0"
        data-testid="preset"
      >
        <RadioItem :value="preset.id" :aria-label="preset.name" />
        <div>
          <strong class="font-medium">{{ preset.name }}</strong>
          <span class="block text-xs text-t3">{{ aboutMine(preset) }}</span>
        </div>
        <div class="flex gap-1.5">
          <Button size="sm" @click="router.push(`/preset/${preset.id}`)"><Pencil />{{ t('settings.presets.edit') }}</Button>
          <Menu align="end" min-width="220px">
            <template #trigger>
              <Button variant="ghost" size="icon-sm" :aria-label="t('common.more')"><MoreHorizontal /></Button>
            </template>
            <MenuItem @select="exportLink(preset)"><Copy />{{ t('settings.presets.export') }}</MenuItem>
            <MenuSeparator />
            <MenuItem danger @select="removing = preset"><Trash2 />{{ t('common.remove') }}</MenuItem>
          </Menu>
        </div>
      </div>
    </SettingsCard>
  </RadioGroup>
  <div class="flex gap-2">
    <Button data-testid="preset-create" @click="router.push('/preset/new')"><Plus />{{ t('settings.presets.create') }}</Button>
    <Button @click="importLink"><ClipboardPaste />{{ t('settings.presets.import') }}</Button>
  </div>

  <Dialog :open="removing !== null" :title="t('settings.presets.removeTitle')" @update:open="(o) => !o && (removing = null)">
    <p class="text-t2">
      {{ t('settings.presets.removeText', { name: removing?.name ?? '', base: removing ? baseOf(removing) : '' }) }}
    </p>
    <template #footer>
      <Button variant="ghost" class="sp" @click="removing = null">{{ t('common.cancel') }}</Button>
      <Button variant="danger" @click="remove">{{ t('common.remove') }}</Button>
    </template>
  </Dialog>
</template>
