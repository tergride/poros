import { readFileSync } from 'node:fs'

import { type Page, expect } from '@playwright/test'

import type { Group, OTPEntry, Server, Settings } from '../src/platform/core-types'

// A backup of the core in memory as a data folder brought from another computer reads: the links of the subscriptions
// and the secrets of the servers lost, and a code of OTP without its secret. Restored, it puts Poros in that state.

interface Backup {
  groups?: Group[]
  servers?: { server: Server; profile: Record<string, unknown> }[]
  settings?: Settings
  otp?: OTPEntry[]
}

const secretOf: Record<string, string> = { vless: 'uuid', vmess: 'uuid', trojan: 'password' }

/** The text of a backup with its secrets lost; chosen is the name of the server the settings choose. */
function fromAnotherComputer(text: string, chosen: string): Buffer {
  const file = JSON.parse(text) as Backup
  for (const group of file.groups ?? []) {
    if (group.kind === 'subscription') Object.assign(group, { url: '', need: ['url'] })
  }
  for (const { server, profile } of file.servers ?? []) {
    const field = secretOf[server.protocol]
    if (!field) continue
    delete (profile[server.protocol] as Record<string, unknown> | undefined)?.[field]
    server.need = [`${server.protocol}.${field}`]
  }
  const pick = file.servers?.find((s) => s.server.name === chosen)
  if (pick && file.settings) file.settings.server = pick.server.id
  const lost: OTPEntry = { id: 'lost', name: 'Office', account: 'me', secret: '', kind: 'totp', algorithm: 'SHA1', digits: 6, period: 30, counter: 0, lost: true }
  file.otp = [...(file.otp ?? []), lost]
  return Buffer.from(JSON.stringify(file))
}

/** On the open backup page of either mode: a backup of everything, restored whole as if from another computer. */
export async function restoreFromAnotherComputer(page: Page, chosen = ''): Promise<void> {
  const saving = page.waitForEvent('download')
  await page.getByTestId('backup-create').click()
  const text = readFileSync(await (await saving).path(), 'utf8')
  const buffer = fromAnotherComputer(text, chosen)
  await page.getByTestId('backup-file').setInputFiles({ name: 'poros-backup.json', mimeType: 'application/json', buffer })
  await page.getByTestId('restore-confirm').click()
  await expect(page.getByTestId('backup-restored')).toBeVisible()
}
