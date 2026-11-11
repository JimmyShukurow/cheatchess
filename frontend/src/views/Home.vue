<template>
  <div class="home">
    <div class="board-col">
      <ChessBoard :fen="START" />
    </div>

    <!-- logged out: landing -->
    <div v-if="!state.user" class="landing">
      <h1>Play Chess Online<br />on CheatChess</h1>
      <p class="muted">Free, fast, no nonsense. Grab a time control and get paired in seconds.</p>
      <router-link to="/register" class="btn btn-green btn-big">
        <span>
          <div>Get Started</div>
          <div class="sub">Sign up with email or Google</div>
        </span>
      </router-link>
      <router-link to="/login" class="btn btn-big">I already have an account</router-link>
    </div>

    <!-- logged in: play panel -->
    <div v-else class="play card">
      <h2>♟ Play Online</h2>

      <div v-for="cat in categories" :key="cat.name" class="cat">
        <div class="cat-name">
          <span class="cat-ico">{{ cat.icon }}</span> {{ cat.name }}
          <span class="muted rating">{{ state.user[cat.key] }}</span>
        </div>
        <div class="tcs">
          <button
            v-for="tc in cat.tcs"
            :key="tc"
            class="tc-btn"
            :class="{ sel: selected === tc }"
            @click="selected = tc"
          >
            {{ label(tc) }}
          </button>
        </div>
      </div>

      <button v-if="state.seeking" class="btn btn-big seeking" @click="send({ t: 'cancelSeek' })">
        <span class="spinner" />
        <span>
          <div>Searching {{ label(state.seeking) }}…</div>
          <div class="sub">click to cancel</div>
        </span>
      </button>
      <button v-else class="btn btn-green btn-big" :disabled="!state.connected" @click="play">Play</button>

      <div class="stats muted">
        <span><b>{{ state.online.players }}</b> online</span>
        <span><b>{{ state.online.games }}</b> games in play</span>
      </div>

      <div v-if="games.length" class="recent">
        <h3>Recent games</h3>
        <router-link v-for="gm in games" :key="gm.id" :to="`/game/${gm.id}`" class="grow">
          <span class="res" :class="resClass(gm)">{{ resIcon(gm) }}</span>
          <span class="opp">vs {{ opponent(gm).username }} <span class="muted">({{ opponent(gm).rating }})</span></span>
          <span class="muted">{{ gm.tc }}</span>
          <span class="muted">{{ Math.ceil(gm.moves.length / 2) }} mv</span>
        </router-link>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, watch } from 'vue'
import ChessBoard from '../components/ChessBoard.vue'
import { state, send, api } from '../store.js'

const START = 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1'
const categories = [
  { name: 'Bullet', icon: '⚡', key: 'ratingBullet', tcs: ['1+0', '1+1', '2+1'] },
  { name: 'Blitz', icon: '🔥', key: 'ratingBlitz', tcs: ['3+0', '3+2', '5+0', '5+3'] },
  { name: 'Rapid', icon: '⏱', key: 'ratingRapid', tcs: ['10+0', '10+5', '15+10', '30+0'] },
]
const selected = ref(localStorage.getItem('tc') || '10+0')
const games = ref([])

function label(tc) {
  const [m, i] = tc.split('+')
  return i === '0' ? `${m} min` : `${m} | ${i}`
}

function play() {
  localStorage.setItem('tc', selected.value)
  send({ t: 'seek', tc: selected.value })
}

const opponent = (gm) => (gm.white.id === state.user.id ? gm.black : gm.white)
function resClass(gm) {
  if (gm.result === '1/2-1/2') return 'draw'
  const iWhite = gm.white.id === state.user.id
  return (gm.result === '1-0') === iWhite ? 'win' : 'loss'
}
const resIcon = (gm) => ({ win: '+', loss: '−', draw: '=' })[resClass(gm)]

watch(
  () => state.user?.id,
  async (id) => {
    if (!id) return
    try {
      games.value = (await api('/api/me/games')).games
    } catch {}
  },
  { immediate: true },
)
</script>

<style scoped>
.home {
  display: flex; gap: 32px; padding: 24px; justify-content: center; align-items: flex-start;
  --board: min(calc(100vh - 48px), calc(100vw - 168px - 48px - 32px - 380px));
}
.board-col { width: var(--board); min-width: 280px; }
.landing { width: 380px; display: flex; flex-direction: column; gap: 18px; padding-top: 40px; }
.landing h1 { font-size: 44px; line-height: 1.1; margin: 0; text-align: center; }
.landing p { text-align: center; font-size: 17px; margin: 0 0 12px; }
.sub { font-size: 13px; font-weight: 600; opacity: .85; }
.play { width: 380px; display: flex; flex-direction: column; gap: 14px; }
.play h2 { margin: 0 0 4px; text-align: center; font-size: 24px; }
.cat-name { font-weight: 700; font-size: 15px; margin-bottom: 8px; display: flex; align-items: center; gap: 6px; }
.cat-ico { width: 20px; text-align: center; }
.rating { margin-left: auto; font-size: 13px; }
.tcs { display: grid; grid-template-columns: repeat(4, 1fr); gap: 8px; }
.tc-btn {
  background: var(--panel-2); color: var(--text); border: 2px solid transparent; border-radius: 6px;
  padding: 12px 0; font-weight: 700; font-size: 15px; cursor: pointer; box-shadow: 0 3px 0 #1e1d1a;
}
.tc-btn:hover { background: #4a4845; }
.tc-btn.sel { border-color: var(--green); background: #3a4a2e; }
.seeking { gap: 16px; }
.stats { display: flex; justify-content: center; gap: 18px; font-size: 13px; }
.stats b { color: var(--text); }
.recent h3 { font-size: 15px; margin: 6px 0 8px; }
.grow { display: grid; grid-template-columns: 24px 1fr auto auto; gap: 10px; align-items: center; padding: 8px 6px; border-radius: 4px; font-size: 14px; }
.grow:hover { background: #2f2d2a; }
.res { width: 20px; height: 20px; border-radius: 4px; display: grid; place-items: center; font-weight: 800; font-size: 14px; }
.res.win { background: var(--green); }
.res.loss { background: #c0392b; }
.res.draw { background: #777; }
.opp { overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }

@media (max-width: 1000px) {
  .home { flex-direction: column; align-items: center; padding: 12px; --board: min(calc(100vw - 24px), 520px); }
  .play, .landing { width: var(--board); }
  .landing { padding-top: 0; }
}
</style>
