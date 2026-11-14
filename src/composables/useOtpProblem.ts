import { useI18n } from 'vue-i18n'

import { sentence } from '@/composables/useReason'
import type { OTPProblem } from '@/platform/core-types'

/** Why a code of OTP or a line of pasted codes cannot be taken, in words; the line and the name lead when known. */
export function useOtpProblem() {
  const { t, te } = useI18n()
  return (p: OTPProblem): string => {
    const key = `adv.tools.problem.${p.reason}`
    const text = te(key) ? t(key, { value: p.value }) : p.reason
    if (p.line > 0) return t('adv.tools.problemLine', { line: p.line, text })
    return p.name ? t('adv.tools.problemNamed', { name: p.name, text }) : sentence(text)
  }
}
