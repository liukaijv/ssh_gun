import { createRouter, createWebHashHistory } from 'vue-router'

const router = createRouter({
  history: createWebHashHistory(),
  routes: [
    { path: '/', redirect: '/servers' },
    {
      path: '/servers',
      name: 'servers',
      component: () => import('@/views/Servers.vue'),
    },
    {
      path: '/mappings',
      name: 'mappings',
      component: () => import('@/views/Mappings.vue'),
    },
    {
      path: '/mappings/:id/files',
      name: 'mapping-files',
      component: () => import('@/views/MappingFiles.vue'),
    },
    {
      path: '/forwards',
      name: 'forwards',
      component: () => import('@/views/Forwards.vue'),
    },
    {
      path: '/processes',
      name: 'processes',
      component: () => import('@/views/Processes.vue'),
    },
    {
      path: '/logs',
      name: 'logs',
      component: () => import('@/views/Logs.vue'),
    },
    {
      path: '/settings',
      name: 'settings',
      component: () => import('@/views/Settings.vue'),
    },
  ],
})

export default router
