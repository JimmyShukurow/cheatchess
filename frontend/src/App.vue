<template>
  <div class="layout">
    <aside class="sidebar">
      <router-link to="/" class="logo">
        <img src="/pieces/cburnett/wN.svg" alt="" />
        <span>CheatChess</span>
      </router-link>
      <nav>
        <router-link to="/" class="nav-item">
          <span class="ico">♟</span><span class="lbl">Play</span>
        </router-link>
        <router-link v-if="state.activeGame" :to="`/game/${state.activeGame}`" class="nav-item live">
          <span class="ico">●</span><span class="lbl">Current game</span>
        </router-link>
      </nav>
      <div class="spacer" />
      <div v-if="state.user" class="me">
        <div class="avatar">{{ state.user.username[0].toUpperCase() }}</div>
        <div class="lbl">
          <div class="name">{{ state.user.username }}</div>
          <div class="muted small">
            <span :class="['dot', state.connected ? 'on' : 'off']" />
            {{ state.connected ? 'online' : 'connecting…' }}
          </div>
        </div>
      </div>
      <button v-if="state.user" class="nav-item logout" @click="doLogout">
        <span class="ico">⎋</span><span class="lbl">Log out</span>
      </button>
      <template v-else>
        <router-link to="/register" class="btn btn-green side-btn">Sign Up</router-link>
        <router-link to="/login" class="btn side-btn">Log In</router-link>
      </template>
    </aside>
    <main class="content">
      <router-view />
    </main>
  </div>
</template>

<script setup>
import { onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { state, onMessage, logout } from './store.js'

const router = useRouter()
const route = useRoute()

const off = onMessage((msg) => {
  if (msg.t === 'start' || msg.t === 'redirect') {
    const id = msg.t === 'start' ? msg.game.id : msg.game
    if (route.path !== `/game/${id}`) router.push(`/game/${id}`)
  }
  if (msg.t === 'hello' && msg.activeGame && route.path === '/') {
    router.push(`/game/${msg.activeGame}`)
  }
})
onUnmounted(off)

async function doLogout() {
  await logout()
  router.push('/')
}
</script>

<style scoped>
.layout { display: flex; min-height: 100vh; }
.sidebar {
  width: 168px; flex-shrink: 0; background: var(--bg-dark); display: flex; flex-direction: column;
  padding: 10px 8px; position: sticky; top: 0; height: 100vh;
}
.logo { display: flex; align-items: center; gap: 6px; padding: 6px 8px 16px; font-weight: 800; font-size: 20px; letter-spacing: -.5px; }
.logo img { width: 34px; height: 34px; }
.nav-item {
  display: flex; align-items: center; gap: 12px; padding: 10px 10px; border-radius: 6px;
  font-weight: 700; font-size: 15px; color: var(--text); background: none; border: none; cursor: pointer; width: 100%; text-align: left;
}
.nav-item:hover { background: #1c1b19; }
.nav-item.router-link-exact-active { background: #1c1b19; }
.ico { width: 24px; text-align: center; font-size: 20px; }
.live .ico { color: var(--green); font-size: 14px; }
.spacer { flex: 1; }
.me { display: flex; align-items: center; gap: 10px; padding: 10px; }
.avatar { width: 34px; height: 34px; border-radius: 4px; background: #5d9948; display: grid; place-items: center; font-weight: 800; flex-shrink: 0; }
.name { font-weight: 700; font-size: 14px; overflow: hidden; text-overflow: ellipsis; }
.small { font-size: 12px; }
.dot { display: inline-block; width: 7px; height: 7px; border-radius: 50%; margin-right: 3px; }
.dot.on { background: var(--green); }
.dot.off { background: #c33; }
.side-btn { margin: 6px 4px; }
.content { flex: 1; min-width: 0; }

@media (max-width: 800px) {
  .layout { flex-direction: column; }
  .sidebar { width: 100%; height: auto; flex-direction: row; align-items: center; position: static; padding: 6px; gap: 4px; }
  .sidebar nav { display: flex; }
  .logo { padding: 4px; font-size: 16px; }
  .logo span, .nav-item .lbl, .me .lbl { display: none; }
  .me { padding: 4px; }
  .nav-item { width: auto; }
  .side-btn { padding: 8px 12px; font-size: 13px; }
}
</style>
