import { createRouter, createWebHashHistory } from 'vue-router'

export const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    // Decides where to begin once the core answers: the wizard on a first start, the servers after it.
    { path: '/', component: () => import('./views/BootView.vue') },
    { path: '/wizard/:step(\\d)', component: () => import('./views/WizardView.vue') },
    { path: '/main', component: () => import('./views/MainView.vue') },
    { path: '/settings/:page', component: () => import('./views/SettingsView.vue') },
    { path: '/preset/:id', component: () => import('./views/PresetEditView.vue') },
    { path: '/:rest(.*)*', redirect: '/' },
  ],
})
