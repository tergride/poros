<script setup lang="ts">
import { TabsTrigger } from 'reka-ui'

// A label of the mockups' tabs: the chosen one is white and underlined by the road gradient.
defineProps<{ value: string }>()
</script>

<template>
  <TabsTrigger
    :value="value"
    class="relative flex h-10 cursor-pointer items-center gap-1.5 px-3 text-[13.5px] text-t2 hover:text-t1 data-[state=active]:text-white data-[state=active]:after:absolute data-[state=active]:after:inset-x-2 data-[state=active]:after:-bottom-px data-[state=active]:after:h-0.5 data-[state=active]:after:bg-[image:var(--road)] [&_svg]:size-3.5"
  >
    <slot />
  </TabsTrigger>
</template>
