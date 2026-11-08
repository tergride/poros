<script setup lang="ts">
import { ArrowLeft, ArrowRight, Check } from '@lucide/vue'
import { computed, ref } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute, useRouter } from 'vue-router'

import portal from '@/assets/img/portal.jpg'
import wordmark from '@/assets/img/wordmark.png'
import SubscriptionLink from '@/components/main/SubscriptionLink.vue'
import { Button, Checkbox, RadioGroup, RadioItem } from '@/components/ui'
import { urlScheme } from '@/platform'
import type { SubscriptionUpdate } from '@/platform/core-types'
import { MethodGroupsRemove } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'

const core = useCoreStore()
const route = useRoute()
const router = useRouter()
const { t, te } = useI18n()

const steps = ['subscription', 'start', 'routing'] as const
const step = computed(() => Math.min(Math.max(Number(route.params.step) || 1, 1), steps.length))
const link = ref<InstanceType<typeof SubscriptionLink> | null>(null)

const startOptions = [
  { key: 'autostart', label: 'wizard.start.autostart', hint: 'wizard.start.autostartHint' },
  { key: 'autoConnect', label: 'wizard.start.autoConnect', hint: 'wizard.start.autoConnectHint' },
] as const
// poros:// has no setting: the wizard takes the links for this copy once, when it finishes.
const takeLinks = ref(true)

const builtins = computed(() => core.presets.filter((p) => p.builtin))
const presetName = (id: string, name: string) => (te(`presets.name.${id}`) ? t(`presets.name.${id}`) : name)

const preset = computed({
  get: () => core.settings?.preset ?? 'all',
  set: (id: string) => void core.updateSettings({ preset: id }).catch(() => undefined),
})

// The wizard shows one subscription: a link put in after another one that worked replaces it, so that a mistaken
// link does not stay behind unseen.
let earlier = ''
async function replaceEarlier(update: SubscriptionUpdate): Promise<void> {
  const id = update.group.id
  if (earlier && earlier !== id) await core.core().call(MethodGroupsRemove, { id: earlier }).catch(() => undefined)
  earlier = id
}

async function next(): Promise<void> {
  // A link typed but not yet confirmed counts as given.
  if (step.value === 1) await link.value?.add()
  await router.push(`/wizard/${step.value + 1}`)
}

async function finish(): Promise<void> {
  if (takeLinks.value) await urlScheme.set(true).catch(() => undefined)
  await core.updateSettings({ onboardingDone: true })
  await router.push('/main')
}
</script>

<template>
  <div class="flex min-h-0 min-w-0 flex-1">
    <aside class="relative w-[clamp(360px,40%,560px)] flex-none bg-cover bg-center" :style="{ backgroundImage: `url(${portal})` }">
      <div class="absolute inset-0 bg-[linear-gradient(to_bottom,transparent_55%,color-mix(in_srgb,var(--s0)_92%,transparent)_100%)]" />
      <div class="absolute right-8 bottom-[30px] left-8 z-10">
        <img :src="wordmark" alt="Poros" class="mb-3 block h-[18px] w-auto" />
        <p class="max-w-[320px] text-sm text-t2">{{ t('wizard.tagline') }}</p>
      </div>
    </aside>
    <section class="flex min-w-0 flex-1 flex-col bg-s1">
      <nav class="flex items-center px-11 pt-7" :aria-label="t('menu.wizard')">
        <template v-for="(name, i) in steps" :key="name">
          <div
            :class="['flex items-center gap-[9px] text-[13px] font-medium', i + 1 <= step ? 'text-t1' : 'text-t3']"
            :aria-current="i + 1 === step ? 'step' : undefined"
          >
            <b
              :class="[
                'grid size-[26px] place-items-center rounded-full border font-mono text-xs font-medium',
                i + 1 < step && 'border-act bg-act text-white',
                i + 1 === step && 'border-transparent bg-[image:var(--grad)] text-white',
                i + 1 > step && 'border-line2 bg-s3 text-t2',
              ]"
            >
              <Check v-if="i + 1 < step" class="size-3.5" /><template v-else>{{ i + 1 }}</template>
            </b>
            {{ t(`wizard.steps.${name}`) }}
          </div>
          <span
            v-if="i < steps.length - 1"
            :class="['mx-2.5 h-0.5 flex-1 rounded-sm', i + 1 < step ? 'bg-[image:var(--road)]' : 'bg-line2']"
          />
        </template>
      </nav>

      <div class="flex min-h-0 flex-1 flex-col gap-4 overflow-auto px-11 pt-10 pb-5">
        <template v-if="step === 1">
          <h1 class="font-disp text-2xl leading-snug font-normal">{{ t('wizard.subscription.title') }}</h1>
          <p class="max-w-[560px] text-t2">{{ t('wizard.subscription.text') }}</p>
          <SubscriptionLink ref="link" large @added="replaceEarlier" />
        </template>

        <template v-else-if="step === 2 && core.settings">
          <h1 class="font-disp text-2xl leading-snug font-normal">{{ t('wizard.start.title') }}</h1>
          <div class="grid gap-2.5">
            <label
              v-for="option in startOptions"
              :key="option.key"
              class="flex cursor-pointer gap-3.5 rounded-xl border border-line bg-s2 px-4 py-[15px] hover:border-line2"
            >
              <Checkbox
                :model-value="core.settings[option.key]"
                @update:model-value="(on: boolean) => core.updateSettings({ [option.key]: on })"
              />
              <span>
                <strong class="mb-0.5 block font-medium">{{ t(option.label) }}</strong>
                <span class="text-xs text-t3">{{ t(option.hint) }}</span>
              </span>
            </label>
            <label class="flex cursor-pointer gap-3.5 rounded-xl border border-line bg-s2 px-4 py-[15px] hover:border-line2">
              <Checkbox v-model="takeLinks" data-testid="wizard-links" />
              <span>
                <strong class="mb-0.5 block font-medium">{{ t('wizard.start.urlScheme') }}</strong>
                <span class="text-xs text-t3">{{ t('wizard.start.urlSchemeHint') }}</span>
              </span>
            </label>
          </div>
          <p class="text-xs text-t3">{{ t('wizard.start.note') }}</p>
        </template>

        <template v-else-if="step === 3">
          <h1 class="font-disp text-2xl leading-snug font-normal">{{ t('wizard.routing.title') }}</h1>
          <RadioGroup v-model="preset" class="grid gap-2.5" :aria-label="t('wizard.steps.routing')">
            <label
              v-for="p in builtins"
              :key="p.id"
              class="flex cursor-pointer items-start gap-3.5 rounded-xl border border-line bg-s2 px-4 py-[15px] hover:border-line2"
            >
              <RadioItem :value="p.id" class="mt-0.5" />
              <span>
                <strong class="mb-0.5 block font-medium">{{ presetName(p.id, p.name) }}</strong>
                <span class="text-xs text-t3">{{ te(`presets.about.${p.id}`) ? t(`presets.about.${p.id}`) : '' }}</span>
              </span>
            </label>
          </RadioGroup>
          <p class="text-xs text-t3">{{ t('wizard.routing.note') }}</p>
        </template>
      </div>

      <div class="flex items-center gap-2 border-t border-line px-11 py-4">
        <Button variant="ghost" class="mr-auto" @click="finish">{{ t('common.skip') }}</Button>
        <Button v-if="step > 1" variant="ghost" @click="router.push(`/wizard/${step - 1}`)">
          <ArrowLeft />{{ t('common.back') }}
        </Button>
        <Button v-if="step < steps.length" variant="primary" :disabled="link?.busy" @click="next">
          {{ t('common.next') }}<ArrowRight />
        </Button>
        <Button v-else variant="primary" @click="finish">{{ t('common.done') }}<Check /></Button>
      </div>
    </section>
  </div>
</template>
