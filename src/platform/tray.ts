import { Menu, type MenuOptions } from '@tauri-apps/api/menu'
import { TrayIcon } from '@tauri-apps/api/tray'

import type { TrayItem } from '@/lib/trayMenu'

type MenuEntry = NonNullable<MenuOptions['items']>[number]

function entry(item: TrayItem): MenuEntry {
  switch (item.kind) {
    case 'separator':
      return { item: 'Separator' }
    case 'submenu':
      return { text: item.text, items: item.items.map(entry) }
    case 'check':
      return { text: item.text, checked: item.checked, action: item.action }
    case 'item':
      return { text: item.text, enabled: item.enabled ?? true, action: item.action }
  }
}

let icon: Promise<TrayIcon | null> | undefined
let shown: Menu | undefined

/** Replaces the menu of the tray icon the shell created, and frees the previous one. */
export async function setTrayMenu(items: TrayItem[], tooltip: string): Promise<void> {
  icon ??= TrayIcon.getById('main')
  const tray = await icon
  if (!tray) return
  const menu = await Menu.new({ items: items.map(entry) })
  await tray.setMenu(menu)
  await tray.setTooltip(tooltip)
  const previous = shown
  shown = menu
  await previous?.close()
}
