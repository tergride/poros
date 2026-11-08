<script setup lang="ts" generic="T extends string | number">
import { Check, ChevronDown } from '@lucide/vue'
import { cn } from 'cn'
import {
  SelectContent,
  SelectItem,
  SelectItemIndicator,
  SelectItemText,
  SelectPortal,
  SelectRoot,
  SelectTrigger,
  SelectViewport,
} from 'reka-ui'
import { type HTMLAttributes, computed } from 'vue'

// A list of choices drawn like the menus of the app. The attributes, aria-label among them, go to the button.
defineOptions({ inheritAttrs: false })
const props = defineProps<{ options: { value: T; label: string }[]; class?: HTMLAttributes['class']; disabled?: boolean }>()
const model = defineModel<T>({ required: true })

// reka-ui keeps "" for no choice, and some choices here are "" (as the system's language): the items go by their
// place in the list.
const index = computed({
  get: () => String(props.options.findIndex((option) => option.value === model.value)),
  set: (next: string) => {
    const option = props.options[Number(next)]
    if (option) model.value = option.value
  },
})
const label = computed(() => props.options.find((option) => option.value === model.value)?.label ?? '')
</script>

<template>
  <SelectRoot v-model="index" :disabled="disabled">
    <SelectTrigger
      v-bind="$attrs"
      :class="
        cn(
          'inline-flex h-9 w-[220px] min-w-0 cursor-pointer items-center justify-between gap-2 rounded-lg border border-line2 bg-s0 pr-2 pl-[11px] text-left text-[13px] text-t1 outline-none hover:border-t3 focus-visible:border-blue data-[disabled]:cursor-default data-[disabled]:opacity-50 data-[disabled]:hover:border-line2 data-[state=open]:border-blue',
          props.class,
        )
      "
    >
      <span class="truncate">{{ label }}</span>
      <ChevronDown class="size-4 flex-none text-t2" />
    </SelectTrigger>
    <SelectPortal>
      <SelectContent
        position="popper"
        :side-offset="4"
        class="z-50 max-h-[var(--reka-select-content-available-height)] min-w-[var(--reka-select-trigger-width)] overflow-hidden rounded-[10px] border border-line2 bg-s3 p-1 shadow-[0_18px_44px_rgba(0,0,0,.55)]"
      >
        <SelectViewport>
          <SelectItem
            v-for="(option, i) in options"
            :key="String(option.value)"
            :value="String(i)"
            class="relative flex min-h-[32px] cursor-pointer items-center rounded-md pr-8 pl-2.5 text-[13px] text-t1 outline-none select-none data-[highlighted]:bg-s5"
          >
            <SelectItemText>{{ option.label }}</SelectItemText>
            <SelectItemIndicator class="absolute right-2 inline-flex"><Check class="size-4 text-blue" /></SelectItemIndicator>
          </SelectItem>
        </SelectViewport>
      </SelectContent>
    </SelectPortal>
  </SelectRoot>
</template>
