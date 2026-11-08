import { ref, watch } from 'vue'
import { useI18n } from 'vue-i18n'
import { useRoute } from 'vue-router'

import { reasonOf } from '@/composables/useReason'
import { onLinks } from '@/platform'
import { LinkPreset, LinkServer, MethodLinksParse, ReasonInvalidLink } from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

/**
 * `poros://` links from the command line and from later starts. Each one opens a dialog and adds nothing on its
 * own. They wait for the core and for the end of the first setup, then come one at a time: the next one waits until
 * the dialog of the one before has opened and closed again.
 */
export function useLinks(): void {
  const core = useCoreStore()
  const ui = useUiStore()
  const route = useRoute()
  const { t } = useI18n()
  const queue = ref<string[]>([])
  // A link is being read by the core, its dialog not open yet: the queue must not hand out another meanwhile.
  const opening = ref(false)

  void onLinks((links) => queue.value.push(...links))

  async function open(link: string): Promise<void> {
    try {
      const info = await core.core().call(MethodLinksParse, { link })
      if (info.kind === LinkPreset && info.preset) {
        ui.linkPreset = { link, preset: info.preset }
        return
      }
      // The advanced mode offers to add the server; the simple mode adds none by hand: the add dialog tries the link
      // and says so.
      if (info.kind === LinkServer && info.server && core.settings?.uiMode === 'advanced') {
        ui.linkServer = { link, server: info.server }
        return
      }
      if (info.kind === LinkServer) {
        ui.addPrefill = { url: link, name: '', check: true }
        ui.addSubscription = true
        return
      }
      ui.addPrefill = { url: info.url, name: info.name, check: false }
      ui.addSubscription = true
    } catch (error) {
      // A poros:// link Poros cannot read is no subscription to try; any other link the add dialog tries and says
      // what is wrong with it, a link to one server included.
      if (/^(poros|throne):/i.test(link) && reasonOf(error) === ReasonInvalidLink) {
        ui.notice = { tone: 'warn', text: t('links.unknown', { link: link.length > 80 ? `${link.slice(0, 80)}…` : link }) }
        return
      }
      ui.addPrefill = { url: link, name: '', check: true }
      ui.addSubscription = true
    } finally {
      opening.value = false
    }
  }

  watch(
    () => [queue.value.length, core.loaded, route.path, ui.addSubscription, ui.linkPreset, ui.linkServer, opening.value] as const,
    ([waiting, loaded, path, adding, preset, server, busyOpening]) => {
      const busy = !loaded || path === '/' || path.startsWith('/wizard') || adding || preset || server || busyOpening
      if (!waiting || busy) return
      const link = queue.value.shift()
      if (!link) return
      opening.value = true
      void open(link)
    },
  )
}
