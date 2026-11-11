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
      <router-link v-if="state.user" to="/settings" class="nav-item">
        <span class="ico">⚙</span><span class="lbl">Settings</span>
      </router-link>
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

    <!-- incoming challenges + notices (any page) -->
    <div class="challenges">
      <div v-for="ch in state.incoming" :key="ch.id" class="ch-wrap">
        <div class="fire" aria-hidden="true"><span v-for="i in 44" :key="i" :style="flameStyle(i)" /></div>
        <div class="popup challenge">
        <div class="ch-head">
          <div class="avatar sm">{{ ch.from.username[0].toUpperCase() }}</div>
          <div>
            <div><b>{{ ch.from.username }}</b> <span class="muted">({{ ch.from.rating }})</span></div>
            <div class="muted small">challenges you · <b class="tc">{{ tcLabel(ch.tc) }}</b> · {{ secondsLeft(ch) }}s</div>
          </div>
        </div>
        <div class="ch-btns">
          <button class="btn btn-green" @click="answer(ch, true)">Accept</button>
          <button class="btn" @click="answer(ch, false)">Decline</button>
        </div>
        </div>
      </div>
    </div>
    <div class="popups">
      <div v-if="state.notice" class="popup notice">{{ state.notice }}</div>
    </div>
  </div>
</template>

<script setup>
import { ref, onMounted, onUnmounted } from 'vue'
import { useRouter, useRoute } from 'vue-router'
import { state, onMessage, logout, send } from './store.js'

const now = ref(Date.now())
let tick = null
onMounted(() => (tick = setInterval(() => (now.value = Date.now()), 1000)))
onUnmounted(() => clearInterval(tick))
const secondsLeft = (ch) => Math.max(0, Math.round((ch.expires - now.value) / 1000))
const tcLabel = (tc) => {
  const [m, i] = tc.split('+')
  return i === '0' ? `${m} min` : `${m} | ${i}`
}
// Deterministic spread of flame particles along the bottom of the challenge card.
const flameStyle = (i) => ({
  left: `${6 + ((i * 37) % 89)}%`,
  '--s': `${60 + ((i * 29) % 50)}px`,
  '--d': `${0.9 + ((i * 13) % 9) / 10}s`,
  animationDelay: `-${((i * 7) % 18) / 10}s`,
})
function answer(ch, accept) {
  send({ t: 'challengeAnswer', id: ch.id, accept })
  state.incoming = state.incoming.filter((c) => c.id !== ch.id)
}

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
.popups { position: fixed; right: 16px; bottom: 16px; z-index: 200; display: flex; flex-direction: column; gap: 10px; width: 320px; max-width: calc(100vw - 32px); }
.popup { background: #3c3a37; border-radius: 10px; padding: 14px; box-shadow: 0 10px 30px rgba(0,0,0,.5); animation: pop .18s ease-out; }
.popup.notice { background: #1f1e1b; border-left: 4px solid var(--green); font-weight: 600; font-size: 14px; }
.challenges {
  position: fixed; inset: 0; z-index: 210; display: flex; flex-direction: column; align-items: center; justify-content: center;
  gap: 16px; padding: 16px; pointer-events: none;
}
.ch-wrap { position: relative; pointer-events: auto; width: 420px; max-width: 100%; animation: pop-in .22s ease-out; }
.popup.challenge {
  position: relative; z-index: 1; padding: 22px; font-size: 17px; border-radius: 14px;
  border: 2px solid var(--green); animation: glow 1.6s ease-in-out infinite;
}
/* fire behind the card: particles rise from the card's lower half and burn out above it */
.fire { position: absolute; left: -50px; right: -50px; bottom: 20px; height: 300px; pointer-events: none; filter: blur(9px); }
.fire::before {
  content: ''; position: absolute; left: 0; right: 0; bottom: -30px; height: 190px; border-radius: 50% 50% 40% 40%;
  background: radial-gradient(ellipse 50% 70% at 50% 85%, rgba(255, 200, 60, .9), rgba(255, 110, 20, .7) 40%, rgba(220, 40, 0, .35) 65%, transparent 80%);
  animation: flicker .2s ease-in-out infinite alternate;
}
.fire span {
  position: absolute; bottom: 0; width: calc(var(--s) * .75); height: calc(var(--s) * 1.4); margin-left: calc(var(--s) * -.375);
  border-radius: 50% 50% 45% 45% / 60% 60% 40% 40%; opacity: 0; mix-blend-mode: screen;
  background: radial-gradient(ellipse at 50% 75%, #fff2a8 0%, #ffc22e 25%, #ff7616 50%, rgba(210, 30, 0, .6) 68%, transparent 80%);
  animation: flame var(--d) ease-in infinite;
}
@keyframes flame {
  0% { transform: translateY(20px) scale(1, 1); opacity: 0; }
  10% { opacity: 1; }
  50% { transform: translateY(-110px) scale(.75, 1.1) translateX(6px); opacity: .9; }
  100% { transform: translateY(-260px) scale(.15, .5) translateX(-8px); opacity: 0; }
}
@keyframes flicker { from { opacity: .75; transform: scaleX(.96); } to { opacity: 1; transform: scaleX(1.04); } }
@media (prefers-reduced-motion: reduce) {
  .fire span, .fire::before, .popup.challenge { animation: none; }
  .fire span { opacity: .5; }
}
.challenge .avatar.sm { width: 48px; height: 48px; font-size: 22px; }
.challenge .small { font-size: 14px; }
.challenge .ch-btns { gap: 12px; margin-top: 18px; }
.challenge .ch-btns .btn { padding: 12px; font-size: 17px; }
@keyframes glow {
  0%, 100% { box-shadow: 0 0 14px 2px rgba(129, 182, 76, .45), 0 10px 30px rgba(0,0,0,.6); }
  50% { box-shadow: 0 0 34px 8px rgba(129, 182, 76, .8), 0 10px 30px rgba(0,0,0,.6); }
}
@keyframes pop-in { from { transform: scale(.85); opacity: 0; } }
.ch-head { display: flex; gap: 12px; align-items: center; }
.avatar.sm { width: 36px; height: 36px; font-size: 16px; }
.tc { color: var(--text); }
.ch-btns { display: flex; gap: 8px; margin-top: 12px; }
.ch-btns .btn { flex: 1; padding: 8px; }
@keyframes pop { from { transform: translateY(10px); opacity: 0; } }

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
