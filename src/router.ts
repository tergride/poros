import { createRouter, createWebHashHistory } from 'vue-router'

import { installModeGuard } from './composables/useUiMode'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    // Decides where to begin once the core answers: the wizard on a first start, the servers after it.
    { path: '/', component: () => import('./views/BootView.vue') },
    { path: '/wizard/:step(\\d)', component: () => import('./views/WizardView.vue') },
    { path: '/main', component: () => import('./views/MainView.vue') },
    { path: '/settings/:page', component: () => import('./views/SettingsView.vue') },
    { path: '/preset/:id', component: () => import('./views/PresetEditView.vue') },
    // The advanced mode: its sections share the rail and the connection bar.
    {
      path: '/adv',
      component: () => import('./views/adv/AdvShell.vue'),
      children: [
        { path: '', redirect: '/adv/profiles' },
        { path: 'profiles', component: () => import('./views/adv/AdvProfilesView.vue') },
        { path: 'settings', redirect: '/adv/settings/conn' },
        { path: 'settings/:page', component: () => import('./views/adv/AdvSettingsView.vue') },
        { path: 'routing', redirect: '/adv/routing/rules' },
        { path: 'routing/:tab(rules|warp|common)/:id?', component: () => import('./views/adv/AdvRoutingView.vue') },
        { path: 'logs', component: () => import('./views/adv/AdvLogsView.vue') },
        { path: 'connections', component: () => import('./views/adv/AdvConnectionsView.vue') },
        { path: 'stats', redirect: '/adv/stats/now' },
        { path: 'stats/:tab(now|traffic)', component: () => import('./views/adv/AdvStatsView.vue') },
        { path: 'tools', redirect: '/adv/tools/otp' },
        { path: 'tools/:tab(otp|misc)', component: () => import('./views/adv/AdvToolsView.vue') },
      ],
    },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
})

installModeGuard(router)
