import { useI18n } from 'vue-i18n'

import type { PresetCopy } from '@/platform/core-types'
import { ActionProxy } from '@/platform/core-types'

/** What a preset is, in a line, for the questions before an import. */
export function usePresetSummary(): (preset: PresetCopy) => string {
  const { t, te } = useI18n()
  return (preset) => {
    const rules = t('presets.rules', { n: preset.rules.length }, preset.rules.length)
    const base = te(`presets.name.${preset.basedOn}`) ? t(`presets.name.${preset.basedOn}`) : preset.basedOn
    const parts = [`${preset.basedOn ? t('presets.copyOf', { base }) : t('presets.alone')}: ${rules}.`]
    if (preset.advanced) parts.push(t('linkPreset.advanced'))
    // A preset that sends the rest past the server or blocks it says so: the user should know before adding it.
    if (preset.default && preset.default !== ActionProxy) parts.push(t(`linkPreset.default.${preset.default}`))
    if (preset.dns || preset.directDns) parts.push(t('linkPreset.dns'))
    return parts.join(' ')
  }
}
