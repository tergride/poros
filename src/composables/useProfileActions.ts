import { useI18n } from 'vue-i18n'

import { useAttempt } from '@/composables/useAttempt'
import { useConnect } from '@/composables/useConnect'
import { useReason } from '@/composables/useReason'
import type { Group, Server, ServersAdded } from '@/platform/core-types'
import {
  GroupManual,
  MethodGroupsAdd,
  MethodServersAdd,
  MethodServersClearResults,
  MethodServersClone,
  MethodServersDeepLinks,
  MethodServersExport,
  MethodServersLinks,
  MethodServersMove,
  MethodServersRemove,
  MethodServersResetTraffic,
  MethodServersResolve,
} from '@/platform/core-types'
import { useCoreStore } from '@/stores/core'
import { useUiStore } from '@/stores/ui'

/** What the profiles of the advanced mode do with the selected rows: links, copies, moves, tests and their results. */
export function useProfileActions() {
  const core = useCoreStore()
  const ui = useUiStore()
  const connect = useConnect()
  const attempt = useAttempt()
  const reason = useReason()
  const { t } = useI18n()
  const call = core.core

  const tell = (text: string) => (ui.notice = { tone: 'info', text })

  async function copy(text: string, what: string): Promise<void> {
    await navigator.clipboard.writeText(text)
    tell(what)
  }

  return {
    copyText: (text: string) => copy(text, t('list.copied')),
    copyLinks: (ids: string[]) =>
      attempt(async () => {
        const { links } = await call().call(MethodServersLinks, { ids })
        await copy(links.join('\n'), t('profiles.copiedLinks', links.length))
      }),
    copyDeepLinks: (ids: string[]) =>
      attempt(async () => {
        const { links } = await call().call(MethodServersDeepLinks, { ids })
        await copy(links.join('\n'), t('profiles.copiedLinks', links.length))
      }),
    // One config stands alone; several come as a JSON list of them, each as its core reads it.
    exportConfigs: (ids: string[]) =>
      attempt(async () => {
        const configs = []
        for (const id of ids) configs.push(await call().call(MethodServersExport, { id }))
        const text = configs.length === 1 ? configs[0]!.json : `[\n${configs.map((c) => c.json).join(',\n')}\n]`
        await copy(text, t('profiles.copiedConfigs', configs.length))
      }),
    copyResult: (server: Server) =>
      attempt(async () => {
        const parts = [server.name]
        if (server.latency > 0) parts.push(t('ping.ms', { n: server.latency }))
        if (server.latency < 0) parts.push(t('ping.none'))
        if (server.speedDown) parts.push(`↓ ${server.speedDown}`)
        if (server.speedUp) parts.push(`↑ ${server.speedUp}`)
        if (server.exitIp) parts.push(`IP ${server.exitIp}${server.exitCountry ? ` ${server.exitCountry}` : ''}`)
        await copy(parts.join(' · '), t('profiles.copiedResult'))
      }),
    clone: (ids: string[]) => attempt(() => call().call(MethodServersClone, { ids })),
    moveTo: (ids: string[], groupId: string) => attempt(() => call().call(MethodServersMove, { ids, groupId })),
    remove: (ids: string[]) => attempt(() => call().call(MethodServersRemove, { ids })),
    resolve: (ids: string[]) =>
      attempt(async () => {
        const { resolved } = await call().call(MethodServersResolve, { groupId: '', ids })
        tell(t('profiles.resolved', resolved))
      }),
    clearResults: (ids: string[]) => attempt(() => call().call(MethodServersClearResults, { groupId: '', ids })),
    resetTraffic: (ids: string[]) => attempt(() => call().call(MethodServersResetTraffic, { groupId: '', ids })),
    start: (id: string) =>
      attempt(async () => {
        await connect.chooseServer(id)
        if (!core.running) await connect.start()
      }),
    /**
     * Adds the servers of a text: to a new group of newGroup's name, to the group if it is the user's, else to the
     * first group of the user's, else to a new «My servers». What did not become servers is told beside the count.
     */
    addText: (text: string, group: Group | null, newGroup = '') =>
      attempt(async () => {
        const target = newGroup ? undefined : group?.kind === GroupManual ? group : core.groups.find((g) => g.kind === GroupManual)
        let added: ServersAdded
        try {
          added = await call().call(
            MethodServersAdd,
            target ? { groupId: target.id, newGroup: '', text } : { groupId: '', newGroup: newGroup || t('profiles.myServers'), text },
          )
        } catch (error) {
          ui.notice = { tone: 'warn', text: t('profiles.addFailed', { reason: reason(error) }) }
          return
        }
        const skipped = added.skips.reduce((sum, skip) => sum + skip.count, 0)
        tell(skipped ? t('profiles.addedSkipped', { added: t('servers.count', added.ids.length), skipped }) : t('profiles.added', { added: t('servers.count', added.ids.length) }))
      }),
    newGroup: (name: string) => attempt(() => call().call(MethodGroupsAdd, { name })),
  }
}
