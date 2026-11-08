import { describe, expect, it } from 'vitest'

import { routeInMode } from './useUiMode'

describe('the screen of the other mode', () => {
  it('keeps the screen, as far as the other mode has it', () => {
    expect(routeInMode('advanced', '/main')).toBe('/adv/profiles')
    expect(routeInMode('advanced', '/settings/subs')).toBe('/adv/settings/subs')
    expect(routeInMode('advanced', '/settings/presets')).toBe('/adv/routing')
    expect(routeInMode('simple', '/adv/settings/look')).toBe('/settings/look')
    expect(routeInMode('simple', '/adv/routing')).toBe('/settings/presets')
    expect(routeInMode('simple', '/adv/stats')).toBe('/main')
  })

  it('leaves alone what both modes share and what is already right', () => {
    for (const mode of ['simple', 'advanced'] as const) {
      expect(routeInMode(mode, '/wizard/2')).toBe('/wizard/2')
      expect(routeInMode(mode, '/preset/p1')).toBe('/preset/p1')
      expect(routeInMode(mode, '/')).toBe('/')
    }
    expect(routeInMode('advanced', '/adv/tools')).toBe('/adv/tools')
    expect(routeInMode('simple', '/settings/conn')).toBe('/settings/conn')
  })
})
