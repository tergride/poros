<script setup lang="ts">
import { ArrowLeft, Lock, Plus, TextAlignStart, X } from '@lucide/vue'
import { computed, onMounted, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import OptionRow from '@/components/settings/OptionRow.vue'
import PresetListsDialog from '@/components/settings/PresetListsDialog.vue'
import SettingsCard from '@/components/settings/SettingsCard.vue'
import { Banner, Button, Input, Select } from '@/components/ui'
import { useConnect } from '@/composables/useConnect'
import { sentence, useFailure } from '@/composables/useReason'
import { useRestartNeeded } from '@/composables/useRestartNeeded'
import type { PresetCopy, PresetEntries } from '@/platform/core-types'
import {
  ActionBlock,
  ActionDirect,
  ActionProxy,
  MethodPresetsGet,
  MethodPresetsReadLists,
  MethodPresetsSave,
  MethodPresetsWriteLists,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

// The editor works on its own draft: leaving without saving drops it (note 5), and the next visit starts from what
// the core holds.
const core = useCoreStore()
const connect = useConnect()
const restart = useRestartNeeded()
const route = useRoute()
const router = useRouter()
const reason = useFailure()
const { t, te } = useI18n()

const draft = ref<PresetCopy | null>(null)
const failure = ref('')
const newSite = ref('')
const newApp = ref('')
const saving = ref(false)
const listsOpen = ref(false)

const dnsChoices = ['', 'https://1.1.1.1/dns-query', 'https://8.8.8.8/dns-query', 'https://9.9.9.9/dns-query'] as const
const dnsChoice = ref<string>('')
const customDns = ref('')

onMounted(async () => {
  try {
    // `new` is a preset from scratch: the core gives its draft for an empty ID.
    const id = route.params.id === 'new' ? '' : String(route.params.id)
    const copy = await core.core().call(MethodPresetsGet, { id })
    // A copy of a built-in preset is named in the interface's language, and apart from the presets there are.
    if (!copy.id && copy.basedOn) copy.name = freeName(t('presetEdit.copyName', { name: shownName(copy.basedOn, copy.name) }))
    draft.value = copy
    if ((dnsChoices as readonly string[]).includes(copy.dns)) dnsChoice.value = copy.dns
    else {
      dnsChoice.value = 'custom'
      customDns.value = copy.dns
    }
  } catch (error) {
    failure.value = reason(error)
  }
})

const shownName = (id: string, name: string) => (te(`presets.name.${id}`) ? t(`presets.name.${id}`) : name)

// The names of the other presets as the user sees them: the built-in ones in the interface's language.
function namesTaken(): string[] {
  return core.presets.filter((p) => p.id !== draft.value?.id).map((p) => (p.builtin ? shownName(p.id, p.name) : p.name).trim().toLowerCase())
}

function freeName(name: string): string {
  const taken = namesTaken()
  let free = name
  for (let n = 2; taken.includes(free.toLowerCase()); n++) free = `${name} ${n}`
  return free
}

const baseName = computed(() => {
  const id = draft.value?.basedOn ?? ''
  return te(`presets.name.${id}`) ? t(`presets.name.${id}`) : (core.presets.find((p) => p.id === id)?.name ?? id)
})

const actions = computed(() =>
  [ActionProxy, ActionDirect, ActionBlock].map((value) => ({ value: value as string, label: t(`action.${value}`) })),
)
const dnsOptions = computed(() => [
  { value: '', label: t(draft.value?.basedOn ? 'presetEdit.dnsPreset' : 'presetEdit.dnsStandard') },
  { value: dnsChoices[1], label: 'Cloudflare' },
  { value: dnsChoices[2], label: 'Google' },
  { value: dnsChoices[3], label: 'Quad9' },
  { value: 'custom', label: t('presetEdit.dnsCustom') },
])

/**
 * Puts an entry into the draft once the core reads it as one: the window of lists writes and reads it the way the
 * core will, so a wrong entry is told now and not only at saving. A repeated one is not added twice, and a program
 * named without its extension gets the .exe that Windows gives its process.
 */
async function add(kind: 'sites' | 'apps', text: string): Promise<boolean> {
  const list = draft.value?.[kind]
  let value = text.trim()
  if (!list || !value) return false
  if (kind === 'apps' && !/[\\/]/.test(value) && !/\.[a-z0-9]+$/i.test(value)) value = `${value}.exe`
  failure.value = ''
  if (list.some((entry) => entry.value.toLowerCase() === value.toLowerCase())) {
    failure.value = t('presetEdit.duplicate', { value })
    return false
  }
  try {
    const entry = { value, action: ActionProxy }
    const lists = await core.core().call(MethodPresetsWriteLists, kind === 'sites' ? { sites: [entry], apps: [] } : { sites: [], apps: [entry] })
    const read = await core.core().call(MethodPresetsReadLists, lists)
    if (read.problems.length > 0 || read[kind].length !== 1) {
      failure.value = sentence(`«${value}» — ${t('reason.invalid_entry')}`)
      return false
    }
  } catch (error) {
    failure.value = reason(error)
    return false
  }
  list.push({ value, action: ActionProxy })
  return true
}

async function addSite(): Promise<void> {
  if (await add('sites', newSite.value)) newSite.value = ''
}

async function addApp(): Promise<void> {
  if (await add('apps', newApp.value)) newApp.value = ''
}

// The lists window replaces the entries of the draft; the preset is saved as before.
function applyLists(entries: PresetEntries): void {
  if (!draft.value) return
  draft.value.sites = entries.sites
  draft.value.apps = entries.apps
}

const ruleName = (name: string) => (te(`presets.rule.${name}`) ? t(`presets.rule.${name}`) : name)

async function save(): Promise<void> {
  if (!draft.value) return
  failure.value = ''
  // The core knows the built-in presets by their English names; the ones the user sees are checked here.
  if (namesTaken().includes(draft.value.name.trim().toLowerCase())) {
    failure.value = sentence(t('reason.preset_name_taken'))
    return
  }
  saving.value = true
  try {
    const dns = dnsChoice.value === 'custom' ? customDns.value.trim() : dnsChoice.value
    const saved = await core.core().call(MethodPresetsSave, { ...draft.value, dns })
    // Saving applies: the copy becomes the preset in use, and a running connection takes it at once.
    await core.updateSettings({ preset: saved.id })
    if (core.running) {
      await connect.connect()
      restart.presetEdited = false
    }
    await router.push('/settings/presets')
  } catch (error) {
    failure.value = reason(error)
  } finally {
    saving.value = false
  }
}
</script>

<template>
  <div class="flex min-w-0 flex-1 flex-col">
    <div class="min-h-0 flex-1 overflow-auto px-7 pt-[22px] pb-7">
      <div class="mb-3 flex items-center gap-2">
        <Button variant="ghost" size="sm" @click="router.push('/settings/presets')"><ArrowLeft />{{ t('presetEdit.back') }}</Button>
        <span
          v-if="draft?.basedOn"
          class="inline-flex h-[22px] items-center rounded-md bg-s4 px-2 text-[11.5px] text-t2"
          data-testid="preset-base"
        >
          {{ t('presetEdit.copyOf', { name: baseName }) }}
        </span>
        <Button size="sm" class="ml-auto" :disabled="!draft" data-testid="preset-lists" @click="listsOpen = true">
          <TextAlignStart />{{ t('presetLists.open') }}
        </Button>
      </div>
      <Banner v-if="failure" class="mb-3" data-testid="preset-failure">{{ failure }}</Banner>
      <template v-if="draft">
        <label class="mb-3.5 flex max-w-[420px] flex-col gap-[5px]">
          <span class="text-[12.5px] text-t2">{{ t('presetEdit.name') }}</span>
          <Input v-model="draft.name" :placeholder="t('presetEdit.namePlaceholder')" data-testid="preset-name" />
        </label>
        <div class="grid grid-cols-2 items-start gap-3">
          <SettingsCard :title="t('presetEdit.sites')">
            <div class="mt-1.5 mb-2 flex gap-2">
              <Input
                v-model="newSite"
                :placeholder="t('presetEdit.sitePlaceholder')"
                :aria-label="t('presetEdit.sitePlaceholder')"
                @keydown.enter="addSite"
              />
              <Button @click="addSite">
                <Plus />{{ t('common.add') }}
              </Button>
            </div>
            <div
              v-for="(entry, i) in draft.sites"
              :key="`s${i}`"
              class="grid grid-cols-[1fr_200px_30px] items-center gap-2 border-t border-line py-[7px]"
              data-testid="site"
            >
              <span class="truncate font-mono text-[12.5px]">{{ entry.value }}</span>
              <Select v-model="entry.action" :options="actions" class="h-8 w-full" :aria-label="t('presetEdit.action')" />
              <Button variant="ghost" size="icon-sm" :aria-label="t('common.remove')" @click="draft.sites.splice(i, 1)"><X /></Button>
            </div>
            <div
              v-for="rule in draft.inherited"
              :key="rule.name"
              class="grid grid-cols-[1fr_200px_30px] items-center gap-2 border-t border-line py-[7px] text-t3"
            >
              <span>{{ ruleName(rule.name) }}</span>
              <span class="pl-[11px] text-[12.5px]">{{ t(`action.${rule.action}`) }}</span>
              <span class="grid place-items-center"><Lock class="size-3.5" /></span>
            </div>
          </SettingsCard>
          <div>
            <SettingsCard :title="t('presetEdit.apps')">
              <div class="mt-1.5 mb-2 flex gap-2">
                <Input
                  v-model="newApp"
                  :placeholder="t('presetEdit.appPlaceholder')"
                  :aria-label="t('presetEdit.appPlaceholder')"
                  @keydown.enter="addApp"
                />
                <Button @click="addApp">
                  <Plus />{{ t('common.add') }}
                </Button>
              </div>
              <div
                v-for="(entry, i) in draft.apps"
                :key="`a${i}`"
                class="grid grid-cols-[1fr_200px_30px] items-center gap-2 border-t border-line py-[7px]"
              >
                <span class="truncate font-mono text-[12.5px]">{{ entry.value }}</span>
                <Select v-model="entry.action" :options="actions" class="h-8 w-full" :aria-label="t('presetEdit.action')" />
                <Button variant="ghost" size="icon-sm" :aria-label="t('common.remove')" @click="draft.apps.splice(i, 1)"><X /></Button>
              </div>
            </SettingsCard>
            <SettingsCard :title="t('presetEdit.dns')">
              <OptionRow :title="t('presetEdit.dnsServers')">
                <Select v-model="dnsChoice" :options="dnsOptions" class="w-[170px]" :aria-label="t('presetEdit.dnsServers')" />
              </OptionRow>
              <Input v-if="dnsChoice === 'custom'" v-model="customDns" mono class="mt-1" :placeholder="t('presetEdit.dnsCustomPlaceholder')" />
            </SettingsCard>
          </div>
        </div>
      </template>
    </div>
    <div class="flex items-center gap-2 border-t border-line px-5 py-3">
      <Button variant="ghost" class="mr-auto" @click="router.push('/settings/presets')">{{ t('common.cancel') }}</Button>
      <Button variant="primary" :disabled="!draft || saving" data-testid="preset-save" @click="save">{{ t('presetEdit.save') }}</Button>
    </div>
    <PresetListsDialog
      v-if="draft"
      v-model:open="listsOpen"
      :name="draft.name"
      :entries="{ sites: draft.sites, apps: draft.apps }"
      @apply="applyLists"
    />
  </div>
</template>
