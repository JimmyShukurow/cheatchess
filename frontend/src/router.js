import { createRouter, createWebHistory } from 'vue-router'
import { state } from './store.js'
import Home from './views/Home.vue'
import Login from './views/Login.vue'
import Register from './views/Register.vue'
import Game from './views/Game.vue'

export const router = createRouter({
  history: createWebHistory(),
  routes: [
    { path: '/', component: Home },
    { path: '/login', component: Login, meta: { guest: true } },
    { path: '/register', component: Register, meta: { guest: true } },
    { path: '/game/:id', component: Game, meta: { auth: true } },
    { path: '/:pathMatch(.*)*', redirect: '/' },
  ],
})

router.beforeEach((to) => {
  if (to.meta.auth && !state.user) return { path: '/login', query: { next: to.fullPath } }
  if (to.meta.guest && state.user) return '/'
})
