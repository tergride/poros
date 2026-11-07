import { ref, watch } from 'vue'
import { useRoute } from 'vue-router'

import { onLinks } from '@/platform'
import { LinkPreset, MethodLinksParse } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

/**
 * `poros://` links from the command line and from later starts. Each one opens a dialog and adds nothing on its
 * own. They wait for the core and for the end of the first setup, then come one at a time.
 */
export function useLinks(): void {
  const core = useCoreStore()
  const ui = useUiStore()
  const route = useRoute()
  const queue = ref<string[]>([])

  void onLinks((links) => queue.value.push(...links))

  async function open(link: string): Promise<void> {
    try {
      const info = await core.core().call(MethodLinksParse, { link })
      if (info.kind === LinkPreset && info.preset) {
        ui.linkPreset = { link, preset: info.preset }
        return
      }
      ui.addPrefill = { url: info.url, name: info.name, check: false }
    } catch {
      // The add dialog tries the link and says what is wrong with it, a link to one server included.
      ui.addPrefill = { url: link, name: '', check: true }
    }
    ui.addSubscription = true
  }

  watch(
    () => [queue.value.length, core.loaded, route.path, ui.addSubscription, ui.linkPreset] as const,
    ([waiting, loaded, path, adding, preset]) => {
      const busy = !loaded || path === '/' || path.startsWith('/wizard') || adding || preset
      if (!waiting || busy) return
      const link = queue.value.shift()
      if (link) void open(link)
    },
  )
}
