import { BackupOTP, BackupProfiles, BackupRouting, BackupSettings } from '@/platform/core-types'
import type { Endpoint, Group, OTPCode, Settings } from '@/platform/core-types'

/** What the parts of a backup are made of, as the interface knows it. */
export interface BackupData {
  settings: Settings | null
  groups: Group[]
  endpoints: Endpoint[]
  otp: OTPCode[]
}

/**
 * Whether the chosen parts of a backup hold secrets: subscription links and the keys of servers, endpoints, OTP
 * secrets, the password of the local proxy and the keys of WARP.
 */
export function backupHasSecrets(parts: readonly string[], data: BackupData): boolean {
  const s = data.settings
  return parts.some((part) => {
    switch (part) {
      case BackupProfiles:
        return data.groups.some((g) => g.url !== '' || g.servers > 0)
      case BackupRouting:
        return data.endpoints.length > 0
      case BackupSettings:
        return !!s && (s.proxyPassword !== '' || s.warpWireGuard.privateKey !== '' || s.warpMasque.privateKey !== '')
      case BackupOTP:
        return data.otp.length > 0
      default:
        return false
    }
  })
}
