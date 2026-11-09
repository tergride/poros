<script setup lang="ts">
import { Check, ClipboardPaste, Copy, Hammer, Layers, Link, Lock, Plus, Route, Trash2 } from '@lucide/vue'
import { computed, ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { onBeforeRouteLeave, onBeforeRouteUpdate, useRoute, useRouter } from 'vue-router'

import NewPresetDialog from '@/components/presets/NewPresetDialog.vue'
import PresetEditor from '@/components/presets/PresetEditor.vue'
import { Banner, Button, Dialog, Input, Tabs, TabsList, TabsTrigger } from '@/components/ui'
import { useAttempt } from '@/composables/useAttempt'
import { providePresetDraft } from '@/composables/usePresetDraft'
import { useFailure } from '@/composables/useReason'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import type { Preset } from '@/platform/core-types'
import { MethodPresetsExport, MethodPresetsImport, MethodPresetsRemove } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

// «Routing» of the advanced mode. «Presets and rules» lists the presets and edits the chosen one beside the list:
// a change waits for «Save», and leaving with one asks first, as on the settings pages. WARP and «General» come with
// later parts of the stage.
const core = useCoreStore()
const ui = useUiStore()
const restart = useRestartNeeded()
const route = useRoute()
const router = useRouter()
const attempt = useAttempt()
const failureOf = useFailure()
const { t } = useI18n()
const editor = providePresetDraft()
const { draft, failure, dirty, toAdvancedOpen } = editor

const tab = computed({
  get: () => String(route.params.tab ?? 'rules'),
  set: (next: string) => void router.push(`/adv/routing/${next}`),
})
// The preset to edit: the one in the address, else the one in use.
const selected = computed(() => String(route.params.id || core.settings?.preset || 'all'))
watch(selected, (id) => void editor.load(id), { immediate: true })

const builtins = computed(() => core.presets.filter((p) => p.builtin))
const mine = computed(() => core.presets.filter((p) => !p.builtin))
const current = computed(() => core.presets.find((p) => p.id === selected.value))
const baseName = computed(() => editor.shownName(core.presets.find((p) => p.id === (draft.value?.basedOn || 'all'))))
const making = ref<string | null>(null)
const removing = ref<Preset | null>(null)

function open(id: string): void {
  making.value = null
  void router.push(`/adv/routing/rules/${id}`)
}

const saving = ref(false)
async function save(): Promise<boolean> {
  saving.value = true
  try {
    const saved = await editor.save()
    // A saved preset of the running connection waits for a restart by the chip.
    if (saved && core.running && saved.id === core.settings?.preset) restart.presetEdited = true
    return saved !== null
  } finally {
    saving.value = false
  }
}

function exportLink(): Promise<void> {
  return attempt(async () => {
    const { link } = await core.core().call(MethodPresetsExport, { id: selected.value })
    await navigator.clipboard.writeText(link)
    ui.notice = { tone: 'info', text: t(current.value?.endpoints ? 'settings.presets.exportedLocal' : 'settings.presets.exported') }
  })
}

async function importLink(): Promise<void> {
  try {
    const link = (await navigator.clipboard.readText()).trim()
    const preset = await core.core().call(MethodPresetsImport, { link })
    ui.notice = { tone: 'info', text: t('settings.presets.imported', { name: preset.name }) }
    open(preset.id)
  } catch (error) {
    ui.notice = { tone: 'warn', text: failureOf(error) }
  }
}

async function remove(): Promise<void> {
  const preset = removing.value
  removing.value = null
  if (!preset) return
  await attempt(async () => {
    await core.core().call(MethodPresetsRemove, { id: preset.id })
    // Nothing is left to save of a removed preset.
    editor.discard()
    draft.value = null
    await router.push('/adv/routing/rules')
  })
}

// The question before leaving with changes: the answer resolves the navigation it holds.
const asking = ref<((go: boolean) => void) | null>(null)
function ask(): Promise<boolean> | boolean {
  if (!dirty.value) return true
  return new Promise((resolve) => (asking.value = resolve))
}
function answer(choice: 'save' | 'leave' | 'stay'): void {
  const resolve = asking.value
  asking.value = null
  if (!resolve) return
  if (choice === 'leave') {
    editor.discard()
    resolve(true)
  } else if (choice === 'save') {
    void save().then(resolve)
  } else resolve(false)
}
onBeforeRouteLeave(ask)
onBeforeRouteUpdate(ask)
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1 flex-col">
    <Tabs v-model="tab">
      <TabsList>
        <TabsTrigger value="rules">{{ t('routing.tabs.rules') }}</TabsTrigger>
        <TabsTrigger value="warp">WARP</TabsTrigger>
        <TabsTrigger value="common">{{ t('routing.tabs.common') }}</TabsTrigger>
      </TabsList>
    </Tabs>
    <div v-if="tab === 'rules'" class="flex min-h-0 flex-1">
      <aside class="flex w-[230px] flex-none flex-col border-r border-line bg-s1">
        <div class="px-3.5 pt-3 pb-1 text-[12.5px] font-semibold">{{ t('routing.presets') }}</div>
        <nav class="min-h-0 flex-1 overflow-auto px-2 pb-2">
          <template v-for="group in [{ label: t('settings.presets.builtin'), list: builtins }, { label: t('routing.mine'), list: mine }]" :key="group.label">
            <div v-if="group.list.length > 0" class="px-2 pt-2 pb-0.5 text-[11.5px] text-t3">{{ group.label }}</div>
            <RouterLink
              v-for="preset in group.list"
              :key="preset.id"
              :to="`/adv/routing/rules/${preset.id}`"
              :class="[
                'flex min-h-9 items-center gap-2.5 rounded-lg px-2.5 text-[13px]',
                preset.id === selected ? 'bg-s4 text-white' : 'text-t2 hover:bg-s3 hover:text-t1',
              ]"
              data-testid="routing-preset"
            >
              <component :is="preset.builtin ? Lock : Route" class="size-4 flex-none" />
              <span class="min-w-0 flex-1 truncate">{{ editor.shownName(preset) }}</span>
              <span class="text-[11.5px] text-t3">{{ preset.rules }}</span>
            </RouterLink>
          </template>
        </nav>
        <div class="grid gap-1.5 p-2">
          <Button size="sm" data-testid="preset-create" @click="making = ''"><Plus />{{ t('settings.presets.create') }}</Button>
          <Button size="sm" @click="importLink"><ClipboardPaste />{{ t('settings.presets.import') }}</Button>
        </div>
      </aside>
      <div class="flex min-w-0 flex-1 flex-col">
        <div v-if="draft" class="flex flex-none items-center gap-2 border-b border-line px-5 py-2.5">
          <template v-if="draft.builtin">
            <strong class="text-[14px]">{{ editor.shownName(draft) }}</strong>
            <span class="inline-flex h-[22px] items-center gap-[5px] rounded-md bg-s4 px-2 text-[11.5px] text-t2"><Lock class="size-3" />{{ t('presets.builtin') }}</span>
            <Button size="sm" class="ml-auto" data-testid="preset-copy" @click="making = draft.id"><Copy />{{ t('settings.presets.makeCopy') }}</Button>
          </template>
          <template v-else>
            <Input v-model="draft.name" class="h-[30px] w-[200px]" :aria-label="t('presetEdit.name')" data-testid="preset-name" />
            <span v-if="draft.basedOn" class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] text-t2" data-testid="preset-base">
              {{ t('presetEdit.copyOf', { name: baseName }) }}
            </span>
            <span v-if="draft.advanced" class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] text-t2" data-testid="preset-advanced">
              {{ t('presets.advanced') }}
            </span>
            <span class="flex-1" />
            <Button v-if="!draft.advanced" size="sm" data-testid="preset-to-advanced" @click="toAdvancedOpen = true"><Layers />{{ t('toAdvanced.button') }}</Button>
            <Button size="sm" data-testid="preset-export" @click="exportLink"><Link />{{ t('settings.presets.export') }}</Button>
            <Button variant="ghost" size="icon-sm" :aria-label="t('presetEdit.remove')" data-testid="preset-remove" @click="removing = current ?? null">
              <Trash2 />
            </Button>
          </template>
        </div>
        <div class="min-h-0 flex-1 overflow-auto px-7 pt-4 pb-7">
          <Banner v-if="failure" class="mb-3" data-testid="preset-failure">{{ failure }}</Banner>
          <p v-if="draft?.builtin" class="mb-2.5 flex items-center gap-1.5 text-xs text-t3"><Lock class="size-3.5" />{{ t('presetEdit.builtinNote') }}</p>
          <PresetEditor lists-button />
        </div>
        <div v-if="dirty" class="flex flex-none items-center gap-3 border-t border-line bg-s2 px-7 py-2.5" role="status" data-testid="savebar">
          <span class="flex-1 text-t2">{{ t('presetEdit.unsaved', { name: draft?.name.trim() ?? '' }) }}</span>
          <Button variant="primary" size="sm" :disabled="saving" data-testid="preset-save" @click="save()"><Check />{{ t('common.save') }}</Button>
        </div>
      </div>
    </div>
    <section v-else class="grid flex-1 place-items-center p-10 text-center" data-testid="section-soon">
      <div class="grid max-w-[420px] justify-items-center gap-3">
        <Hammer class="size-8 text-t3" />
        <p class="text-t2">{{ t('adv.soon') }}</p>
      </div>
    </section>

    <NewPresetDialog :based-on="making" @close="making = null" @created="open" />
    <Dialog :open="removing !== null" :title="t('settings.presets.removeTitle')" @update:open="(o) => !o && (removing = null)">
      <p class="text-t2">
        {{ t('settings.presets.removeText', { name: removing?.name ?? '', base: baseName }) }}
      </p>
      <template #footer>
        <Button variant="ghost" class="sp" @click="removing = null">{{ t('common.cancel') }}</Button>
        <Button variant="danger" data-testid="preset-remove-confirm" @click="remove">{{ t('common.remove') }}</Button>
      </template>
    </Dialog>
    <Dialog :open="asking !== null" :title="t('adv.unsaved.title')" @update:open="(o) => !o && answer('stay')">
      <p class="text-t2">{{ t('adv.unsaved.text') }}</p>
      <template #footer>
        <Button variant="ghost" class="sp" data-testid="unsaved-leave" @click="answer('leave')">{{ t('adv.unsaved.leave') }}</Button>
        <Button variant="primary" data-testid="unsaved-save" @click="answer('save')"><Check />{{ t('common.save') }}</Button>
      </template>
    </Dialog>
  </div>
</template>
