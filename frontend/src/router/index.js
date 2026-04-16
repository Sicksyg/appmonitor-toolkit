import { createRouter, createWebHashHistory } from 'vue-router'
import IOS from '../views/IOS.vue'
import Android from '../views/Android.vue'
import Utilities from '../views/Utilities.vue'
import Settings from '../views/Settings.vue'

const routes = [
    { path: '/', redirect: '/ios' },
    { path: '/ios', name: 'iOS', component: IOS },
    { path: '/android', name: 'Android', component: Android },
    { path: '/utilities', name: 'Utilities', component: Utilities },
    { path: '/settings', name: 'Settings', component: Settings },
]

const router = createRouter({
    history: createWebHashHistory(),
    routes,
})

export default router
