<template>
  <div class="game-page">
    <div class="board-col">
      <PlayerBar
        :player="topColor === 'w' ? g?.white : g?.black"
        :color="topColor"
        :ms="clockMs(topColor)"
        :running="clockActive(topColor)"
        :captured="material.captured[topColor]"
        :advantage="material.adv[topColor]"
        :diff="ended ? g?.ratingDiff?.[topColor] : 0"
      />
      <div class="board-wrap">
        <ChessBoard
          :fen="boardFen"
          :orientation="orientation"
          :movable="canInteract ? myColor : null"
          :dests="dests"
          :last-move="viewLastMove"
          :check="viewCheck"
          :arrows="hintArrows"
          :premove="premoveSquares"
          @move="onBoardMove"
          @cancel="premoves = []"
        />
        <div v-if="premoves.length" class="pm-badge" title="Right-click the board to cancel">
          ⏩ {{ premoves.length }} premove{{ premoves.length > 1 ? 's' : '' }} · right-click to cancel
        </div>
        <div v-if="firstMoveBanner" class="first-move">
          {{ firstMoveBanner }}
        </div>
      </div>
      <PlayerBar
        :player="bottomColor === 'w' ? g?.white : g?.black"
        :color="bottomColor"
        :ms="clockMs(bottomColor)"
        :running="clockActive(bottomColor)"
        :captured="material.captured[bottomColor]"
        :advantage="material.adv[bottomColor]"
        :diff="ended ? g?.ratingDiff?.[bottomColor] : 0"
      />
    </div>

    <div class="side card">
      <div class="side-head">
        <span class="tc">⏱ {{ g?.tc }}</span>
        <span class="muted">{{ category }}</span>
        <span class="spacer" />
        <span v-if="!ended && g" class="live-dot">● Live</span>
        <span v-else-if="g" class="muted">{{ resultLine }}</span>
      </div>

      <div ref="movesEl" class="moves">
        <div v-for="row in moveRows" :key="row.n" class="mrow" :class="{ odd: row.n % 2 === 1 }">
          <span class="mnum">{{ row.n }}.</span>
          <span class="mv" :class="{ cur: displayPly === row.wi + 1 }" @click="viewPly = row.wi + 1">{{ row.w }}</span>
          <span v-if="row.b" class="mv" :class="{ cur: displayPly === row.wi + 2 }" @click="viewPly = row.wi + 2">{{ row.b }}</span>
        </div>
        <div v-if="!history.length" class="muted empty">{{ g ? 'Waiting for the first move…' : 'Loading game…' }}</div>
        <div v-if="ended" class="result-row">
          <b>{{ g.result === '*' ? '–' : g.result }}</b>
          <span class="muted">{{ resultLine }}</span>
        </div>
      </div>

      <div v-if="drawOfferedToMe" class="draw-banner">
        <span>½ Your opponent offers a draw</span>
        <div>
          <button class="btn btn-green sm" @click="send({ t: 'drawAnswer', game: gameId, accept: true })">Accept</button>
          <button class="btn sm" @click="send({ t: 'drawAnswer', game: gameId, accept: false })">Decline</button>
        </div>
      </div>

      <div v-if="hintAllowed" class="engine">
        <button v-if="!state.autoHint" class="btn engine-btn" title="Show the best move on every one of your turns (H)" @click="startAuto">
          💡 Best move
        </button>
        <div v-else class="engine-row">
          <button class="btn engine-btn" :class="{ thinking: hintThinking }" :disabled="!myTurnLive" title="Wrong suggestion? Recalculate deeper (H)" @click="recalc">
            <span v-if="hintThinking" class="spinner sm" />
            <span v-else>⟳</span>
            {{ hintThinking ? 'Calculating…' : myTurnLive ? 'Recalculate' : 'Waiting for your turn…' }}
          </button>
          <button class="btn stop-btn" title="Stop suggestions" @click="stopAuto">■ Stop</button>
        </div>
        <div v-if="hintVisible && hint.info?.best" class="engine-info">
          <div class="eline">
            <span class="ebest">{{ hintSan[0] }}</span>
            <span class="eval" :class="hintEvalClass">{{ hintEval }}</span>
            <span class="muted">d{{ hint.info.depth }} · {{ (hint.ms / 1000).toFixed(1) }}s · #{{ hint.attempt }}</span>
          </div>
          <div class="pv muted">{{ hintSan.join(' ') }}</div>
        </div>
      </div>

      <div class="nav-btns">
        <button class="btn icon" title="First" @click="viewPly = 0">⏮</button>
        <button class="btn icon" title="Previous (←)" @click="step(-1)">◀</button>
        <button class="btn icon" title="Next (→)" @click="step(1)">▶</button>
        <button class="btn icon" title="Last" @click="viewPly = null">⏭</button>
        <button class="btn icon" title="Flip board" @click="flipped = !flipped">⇅</button>
      </div>

      <div v-if="isPlayer && !ended" class="actions">
        <button v-if="history.length < 2" class="btn btn-block" @click="send({ t: 'abort', game: gameId })">✕ Abort</button>
        <template v-else>
          <button class="btn" :disabled="drawOfferedByMe" @click="send({ t: 'draw', game: gameId })">
            ½ {{ drawOfferedByMe ? 'Draw offered' : 'Draw' }}
          </button>
          <button class="btn" :class="{ danger: confirmResign }" @click="resign">
            ⚑ {{ confirmResign ? 'Confirm?' : 'Resign' }}
          </button>
        </template>
      </div>

      <div v-if="ended && isPlayer && rematchOfferedToMe" class="draw-banner">
        <span>⟳ {{ opponentName }} wants a rematch</span>
        <div>
          <button class="btn btn-green sm" @click="rematch(true)">Accept</button>
          <button class="btn sm" @click="rematch(false)">Decline</button>
        </div>
      </div>
      <div v-if="ended" class="actions">
        <button v-if="isPlayer && !rematchOfferedToMe" class="btn" :disabled="rematchOfferedByMe" @click="rematch(true)">
          ⟳ {{ rematchOfferedByMe ? 'Rematch sent…' : 'Rematch' }}
        </button>
        <button v-if="!state.seeking" class="btn btn-green" @click="newGame">New {{ g.tc }}</button>
        <button v-else class="btn" @click="send({ t: 'cancelSeek' })"><span class="spinner" /> cancel</button>
      </div>
    </div>

    <!-- game over modal -->
    <div v-if="showModal" class="modal-bg" @click.self="showModal = false">
      <div class="modal">
        <button class="close" @click="showModal = false">✕</button>
        <div class="modal-head" :class="outcomeClass">
          <h2>{{ modalTitle }}</h2>
          <div>{{ reasonText }}</div>
        </div>
        <div class="modal-body">
          <div class="players">
            <div class="p">
              <div class="avatar aw">{{ g.white.username[0].toUpperCase() }}</div>
              <div class="pn">{{ g.white.username }}</div>
              <div v-if="g.ratingDiff.w" :class="g.ratingDiff.w > 0 ? 'up' : 'down'">{{ g.white.rating + g.ratingDiff.w }} ({{ fmtDiff(g.ratingDiff.w) }})</div>
            </div>
            <div class="vs">{{ g.result === '*' ? 'vs' : g.result }}</div>
            <div class="p">
              <div class="avatar ab">{{ g.black.username[0].toUpperCase() }}</div>
              <div class="pn">{{ g.black.username }}</div>
              <div v-if="g.ratingDiff.b" :class="g.ratingDiff.b > 0 ? 'up' : 'down'">{{ g.black.rating + g.ratingDiff.b }} ({{ fmtDiff(g.ratingDiff.b) }})</div>
            </div>
          </div>
          <div v-if="isPlayer && rematchOfferedToMe" class="modal-rematch">
            {{ opponentName }} wants a rematch!
          </div>
          <div class="modal-btns">
            <button v-if="isPlayer" class="btn" :disabled="rematchOfferedByMe" @click="rematch(true)">
              ⟳ {{ rematchOfferedToMe ? 'Accept rematch' : rematchOfferedByMe ? 'Rematch sent…' : 'Rematch' }}
            </button>
            <button v-if="!state.seeking" class="btn btn-green" @click="newGame">New {{ g.tc }}</button>
            <button v-else class="btn" @click="send({ t: 'cancelSeek' })"><span class="spinner" /> cancel</button>
          </div>
          <router-link to="/" class="btn btn-block" style="margin-top: 12px">Home</router-link>
        </div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, reactive, computed, watch, onMounted, onUnmounted, nextTick, shallowRef } from 'vue'
import { useRoute } from 'vue-router'
import { Chess } from 'chess.js'
import ChessBoard from '../components/ChessBoard.vue'
import PlayerBar from '../components/PlayerBar.vue'
import { state, send, onMessage, unwatch } from '../store.js'
import { sounds } from '../sound.js'
import { premoveDests, applyPremoves } from '../premove.js'

const START = 'rnbqkbnr/pppppppp/8/8/8/8/PPPPPPPP/RNBQKBNR w KQkq - 0 1'
const route = useRoute()
const gameId = computed(() => route.params.id)

const g = ref(null) // last server state
let chess = new Chess()
const moves = ref([]) // UCI list (local, may be one ahead of server)
const history = shallowRef([]) // [{ fen, san, from, to, check, capture }]
const viewPly = ref(null) // null = live
const flipped = ref(false)
const showModal = ref(false)
const confirmResign = ref(false)
const movesEl = ref(null)

const clockBase = reactive({ w: 0, b: 0 })
const clockAt = ref(performance.now())
const clockRunning = ref(false)
const firstMoveLeft = ref(0)
const now = ref(performance.now())
let ticker = null
let lowTimePlayed = false

// ---------- derived ----------
const myColor = computed(() => {
  if (!g.value || !state.user) return null
  if (g.value.white.id === state.user.id) return 'w'
  if (g.value.black.id === state.user.id) return 'b'
  return null
})
const isPlayer = computed(() => !!myColor.value)
const ended = computed(() => g.value?.status === 'ended')
const turn = computed(() => (moves.value.length % 2 === 0 ? 'w' : 'b'))
const orientation = computed(() => {
  const base = myColor.value === 'b' ? 'black' : 'white'
  return flipped.value ? (base === 'white' ? 'black' : 'white') : base
})
const bottomColor = computed(() => (orientation.value === 'white' ? 'w' : 'b'))
const topColor = computed(() => (bottomColor.value === 'w' ? 'b' : 'w'))
const displayPly = computed(() => viewPly.value ?? history.value.length)
const isLive = computed(() => displayPly.value === history.value.length)
const canMove = computed(() => g.value && !ended.value && isPlayer.value && isLive.value && turn.value === myColor.value)
// can touch pieces: on my turn = real moves, otherwise = premoves
const canInteract = computed(() => g.value && !ended.value && isPlayer.value && isLive.value)
// queued premoves [{ from, to, promotion }], played one per turn as the opponent moves
const premoves = ref([])
// what the board shows: the live position with queued premoves applied visually
const boardFen = computed(() => (isLive.value && premoves.value.length ? applyPremoves(viewFen.value, premoves.value) : viewFen.value))
const premoveSquares = computed(() => (premoves.value.length ? premoves.value.flatMap((p) => [p.from, p.to]) : null))

const viewFen = computed(() => (displayPly.value === 0 ? START : history.value[displayPly.value - 1].fen))
const viewLastMove = computed(() => {
  const h = history.value[displayPly.value - 1]
  return h ? [h.from, h.to] : null
})
const viewCheck = computed(() => {
  const h = history.value[displayPly.value - 1]
  if (!h || !h.check) return null
  return findKing(h.fen, h.fen.split(' ')[1])
})

const dests = computed(() => {
  const m = new Map()
  if (canInteract.value && !canMove.value) return premoveDests(boardFen.value, myColor.value)
  if (!canMove.value) return m
  // eslint-disable-next-line no-unused-expressions
  moves.value.length // dependency
  for (const mv of chess.moves({ verbose: true })) {
    if (!m.has(mv.from)) m.set(mv.from, [])
    if (!m.get(mv.from).includes(mv.to)) m.get(mv.from).push(mv.to)
  }
  return m
})

const inc = computed(() => (g.value ? +g.value.tc.split('+')[1] * 1000 : 0))
const category = computed(() => {
  if (!g.value) return ''
  const [m, i] = g.value.tc.split('+').map(Number)
  const est = m * 60 + 40 * i
  return est < 180 ? 'Bullet' : est < 600 ? 'Blitz' : 'Rapid'
})

function clockMs(color) {
  let v = clockBase[color]
  if (clockActive(color)) v -= now.value - clockAt.value
  return Math.max(0, v)
}
function clockActive(color) {
  return !!g.value && !ended.value && clockRunning.value && turn.value === color
}

// ---------- rematch ----------
const opponentName = computed(() => (myColor.value === 'w' ? g.value?.black.username : g.value?.white.username))
const rematchOfferedToMe = computed(() => ended.value && g.value?.rematch && g.value.rematch !== myColor.value)
const rematchOfferedByMe = computed(() => ended.value && g.value?.rematch === myColor.value)
function rematch(accept) {
  send({ t: accept ? 'rematch' : 'rematchDecline', game: gameId.value })
}

// ---------- engine hint (admin only, enforced by the server) ----------
const hint = ref(null) // { ply, status, attempt, ms, info }
const hintAllowed = computed(() => !!state.user?.isAdmin && isPlayer.value)
const hintThinking = computed(() => hint.value?.status === 'thinking' && hint.value.ply === history.value.length)
const hintVisible = computed(() => !!hint.value && hint.value.ply === history.value.length && isLive.value)
const hintArrows = computed(() => {
  const best = hintVisible.value && hint.value.info?.best
  if (!best) return []
  const arrows = [{ from: best.slice(0, 2), to: best.slice(2, 4), opacity: hintThinking.value ? 0.55 : 1 }]
  const reply = hint.value.info.pv?.[1]
  if (reply && !hintThinking.value) arrows.push({ from: reply.slice(0, 2), to: reply.slice(2, 4), color: 'rgba(80,160,255,0.55)' })
  return arrows
})
const hintSan = computed(() => {
  if (!hintVisible.value || !hint.value.info?.pv) return []
  const c = new Chess(chess.fen())
  const out = []
  try {
    for (const u of hint.value.info.pv) out.push(c.move({ from: u.slice(0, 2), to: u.slice(2, 4), promotion: u[4] }).san)
  } catch {}
  return out
})
const hintEval = computed(() => {
  const inf = hint.value?.info
  if (!inf) return ''
  if (inf.mate) return (inf.mate > 0 ? '+M' : '-M') + Math.abs(inf.mate)
  const v = inf.score / 100
  return (v > 0 ? '+' : '') + v.toFixed(2)
})
const hintEvalClass = computed(() => {
  const inf = hint.value?.info
  if (!inf) return ''
  const v = inf.mate ? inf.mate * 10000 : inf.score
  return v > 0 ? 'ew' : v < 0 ? 'eb' : ''
})
// Auto mode: once started, a suggestion is calculated on every one of my turns until Stop.
const myTurnLive = computed(() => !!g.value && !ended.value && isPlayer.value && turn.value === myColor.value)
let requestedPly = -1
function requestHint() {
  requestedPly = history.value.length
  send({ t: 'hint', game: gameId.value })
}
function startAuto() {
  if (!hintAllowed.value) return
  state.autoHint = true
  viewPly.value = null
  if (myTurnLive.value) requestHint()
}
function stopAuto() {
  state.autoHint = false
  hint.value = null
  requestedPly = -1
  premoves.value = []
  send({ t: 'hintStop' })
}
function recalc() {
  if (!hintAllowed.value || !myTurnLive.value) return
  viewPly.value = null
  requestHint() // same position again -> server searches deeper
}
watch([() => history.value.length, myTurnLive, () => state.autoHint, hintAllowed], () => {
  if (state.autoHint && hintAllowed.value && myTurnLive.value && requestedPly !== history.value.length) requestHint()
})

const firstMoveBanner = computed(() => {
  if (!g.value || ended.value || moves.value.length >= 2 || !isPlayer.value || turn.value !== myColor.value) return ''
  const left = Math.max(0, Math.ceil((firstMoveLeft.value - (now.value - clockAt.value)) / 1000))
  return `Make your first move – ${left}s left or the game will be aborted`
})

const drawOfferedToMe = computed(() => !ended.value && isPlayer.value && g.value?.drawOffer && g.value.drawOffer !== myColor.value)
const drawOfferedByMe = computed(() => !ended.value && g.value?.drawOffer === myColor.value)

const moveRows = computed(() => {
  const rows = []
  const h = history.value
  for (let i = 0; i < h.length; i += 2) rows.push({ n: i / 2 + 1, wi: i, w: h[i].san, b: h[i + 1]?.san })
  return rows
})

const VALUES = { P: 1, N: 3, B: 3, R: 5, Q: 9 }
const START_COUNT = { P: 8, N: 2, B: 2, R: 2, Q: 1 }
const material = computed(() => {
  const count = { w: { P: 0, N: 0, B: 0, R: 0, Q: 0 }, b: { P: 0, N: 0, B: 0, R: 0, Q: 0 } }
  for (const ch of viewFen.value.split(' ')[0]) {
    const up = ch.toUpperCase()
    if (VALUES[up] !== undefined) count[ch === up ? 'w' : 'b'][up]++
  }
  const captured = { w: [], b: [] }
  let score = { w: 0, b: 0 }
  for (const c of ['w', 'b']) {
    const opp = c === 'w' ? 'b' : 'w'
    for (const t of ['P', 'N', 'B', 'R', 'Q']) {
      score[c] += count[c][t] * VALUES[t]
      for (let i = 0; i < Math.max(0, START_COUNT[t] - count[opp][t]); i++) captured[c].push(opp + t)
    }
  }
  return { captured, adv: { w: score.w - score.b, b: score.b - score.w } }
})

const outcomeClass = computed(() => {
  if (!g.value || g.value.result === '*' || g.value.result === '1/2-1/2' || !myColor.value) return 'neutral'
  const won = (g.value.result === '1-0') === (myColor.value === 'w')
  return won ? 'won' : 'lost'
})
const modalTitle = computed(() => {
  const r = g.value?.result
  if (r === '*') return 'Game Aborted'
  if (r === '1/2-1/2') return 'Draw'
  if (myColor.value) return outcomeClass.value === 'won' ? 'You Won!' : 'You Lost'
  return r === '1-0' ? 'White Won' : 'Black Won'
})
const REASONS = {
  checkmate: 'by checkmate', resignation: 'by resignation', timeout: 'on time', agreement: 'by agreement',
  stalemate: 'by stalemate', repetition: 'by repetition', '50-move rule': 'by 50-move rule',
  'insufficient material': 'by insufficient material', 'timeout vs insufficient material': 'timeout vs insufficient material',
  aborted: 'Game aborted',
}
const reasonText = computed(() => REASONS[g.value?.reason] || g.value?.reason || '')
const resultLine = computed(() => {
  if (!g.value || !ended.value) return ''
  if (g.value.result === '*') return 'Aborted'
  const who = g.value.result === '1-0' ? 'White won' : g.value.result === '0-1' ? 'Black won' : 'Draw'
  return `${who} ${reasonText.value}`
})
const fmtDiff = (d) => (d > 0 ? `+${d}` : `${d}`)

// ---------- helpers ----------
function findKing(fen, color) {
  const rows = fen.split(' ')[0].split('/')
  const k = color === 'w' ? 'K' : 'k'
  for (let i = 0; i < 8; i++) {
    let f = 0
    for (const ch of rows[i]) {
      if (/\d/.test(ch)) f += +ch
      else {
        if (ch === k) return 'abcdefgh'[f] + (8 - i)
        f++
      }
    }
  }
  return null
}

function applyUci(uci) {
  const mv = chess.move({ from: uci.slice(0, 2), to: uci.slice(2, 4), promotion: uci[4] })
  return { fen: chess.fen(), san: mv.san, from: mv.from, to: mv.to, check: chess.inCheck(), capture: !!mv.captured }
}

function playMoveSound(h) {
  if (!h) return
  if (h.check) sounds.check()
  else if (h.capture) sounds.capture()
  else sounds.move()
}

function reset() {
  g.value = null
  chess = new Chess()
  moves.value = []
  history.value = []
  viewPly.value = null
  showModal.value = false
  flipped.value = false
  hint.value = null
  lowTimePlayed = false
}

function syncMoves(list, withSound) {
  const local = moves.value
  const isPrefix = local.length <= list.length && local.every((m, i) => m === list[i])
  let h = history.value
  let have = local.length
  if (!isPrefix) {
    chess = new Chess()
    h = []
    have = 0
  } else if (have === list.length) return
  const added = []
  try {
    for (let i = have; i < list.length; i++) added.push(applyUci(list[i]))
  } catch (e) {
    console.error('move replay failed', e)
  }
  history.value = [...h, ...added]
  moves.value = list.slice()
  if (withSound && isPrefix && added.length === 1) playMoveSound(added[0])
  scrollMoves()
}

function applyState(st) {
  const first = !g.value
  const wasActive = g.value && g.value.status === 'active'
  g.value = st
  syncMoves(st.moves, !first)
  clockBase.w = st.clock.w
  clockBase.b = st.clock.b
  clockAt.value = performance.now()
  now.value = clockAt.value
  clockRunning.value = st.running
  firstMoveLeft.value = st.firstMoveLeft || 0
  if (wasActive && st.status === 'ended') {
    showModal.value = true
    sounds.end()
  }
  if (st.status === 'ended') premoves.value = []
  else playPremove()
}

// My turn: play the first queued premove if it is legal now. If it isn't, the rest of the
// line no longer makes sense, so the whole queue is dropped (same as chess.com).
function playPremove() {
  if (!premoves.value.length || !canMove.value) return
  const [p, ...rest] = premoves.value
  const legal = chess.moves({ verbose: true }).find((m) => m.from === p.from && m.to === p.to)
  if (!legal) {
    premoves.value = []
    return
  }
  premoves.value = rest
  onBoardMove({ from: p.from, to: p.to, promotion: legal.promotion ? p.promotion || 'q' : undefined })
}

function scrollMoves() {
  nextTick(() => {
    if (movesEl.value) movesEl.value.scrollTop = movesEl.value.scrollHeight
  })
}

// ---------- actions ----------
function onBoardMove({ from, to, promotion }) {
  if (canInteract.value && !canMove.value) {
    if (premoves.value.length < 100) premoves.value = [...premoves.value, { from, to, promotion }]
    return
  }
  if (!canMove.value) return
  const uci = from + to + (promotion || '')
  let h
  try {
    h = applyUci(uci)
  } catch {
    return
  }
  const color = myColor.value
  const wasRunning = moves.value.length >= 2
  if (wasRunning) {
    clockBase[color] = clockMs(color) + inc.value
  }
  clockAt.value = performance.now()
  moves.value = [...moves.value, uci]
  history.value = [...history.value, h]
  clockRunning.value = moves.value.length >= 2
  playMoveSound(h)
  scrollMoves()
  send({ t: 'move', game: gameId.value, move: uci })
}

function resign() {
  if (!confirmResign.value) {
    confirmResign.value = true
    setTimeout(() => (confirmResign.value = false), 3000)
    return
  }
  confirmResign.value = false
  send({ t: 'resign', game: gameId.value })
}

function newGame() {
  send({ t: 'seek', tc: g.value.tc })
}

function step(d) {
  const n = Math.min(history.value.length, Math.max(0, displayPly.value + d))
  viewPly.value = n === history.value.length ? null : n
}

function onKey(e) {
  if (e.target.tagName === 'INPUT') return
  if ((e.key === 'h' || e.key === 'H') && hintAllowed.value) state.autoHint ? recalc() : startAuto()
  else if (e.key === 'ArrowLeft') step(-1)
  else if (e.key === 'ArrowRight') step(1)
  else if (e.key === 'ArrowUp') viewPly.value = 0
  else if (e.key === 'ArrowDown') viewPly.value = null
  else return
  e.preventDefault()
}

const off = onMessage((msg) => {
  if ((msg.t === 'game' || msg.t === 'start') && msg.game.id === gameId.value) {
    if (msg.t === 'start') sounds.start()
    applyState(msg.game)
  } else if (msg.t === 'hint' && msg.game === gameId.value) {
    // ignore late results from positions / attempts that are no longer current
    if (!state.autoHint || msg.ply !== history.value.length) return
    if (hint.value && hint.value.ply === msg.ply && msg.attempt < hint.value.attempt) return
    hint.value = msg
  }
})

watch(
  gameId,
  (id) => {
    if (!id) return
    reset()
    send({ t: 'watch', game: id })
  },
  { immediate: true },
)

// low-time warning
watch(
  () => (myColor.value ? clockMs(myColor.value) : Infinity),
  (ms) => {
    if (!lowTimePlayed && clockActive(myColor.value) && ms < 10000 && ms > 0) {
      lowTimePlayed = true
      sounds.lowTime()
    }
  },
)

onMounted(() => {
  ticker = setInterval(() => (now.value = performance.now()), 100)
  window.addEventListener('keydown', onKey)
})
onUnmounted(() => {
  clearInterval(ticker)
  window.removeEventListener('keydown', onKey)
  off()
  unwatch()
})
</script>

<style scoped>
.game-page {
  display: flex; gap: 24px; padding: 12px 24px; justify-content: center; align-items: flex-start;
  --board: min(calc(100vh - 140px), calc(100vw - 168px - 48px - 24px - 360px));
}
.board-col { width: var(--board); min-width: 280px; }
.board-wrap { position: relative; }
.pm-badge {
  position: absolute; left: 8px; bottom: 8px; z-index: 15; pointer-events: none;
  background: rgba(214, 79, 0, .92); color: #fff; font-size: 13px; font-weight: 700; padding: 4px 10px; border-radius: 12px;
}
.first-move {
  position: absolute; left: 50%; top: 50%; transform: translate(-50%, -50%); z-index: 15;
  background: rgba(0,0,0,.78); padding: 10px 16px; border-radius: 8px; font-weight: 600; pointer-events: none; text-align: center;
}
.side {
  width: 360px; height: calc(var(--board) + 124px); display: flex; flex-direction: column; padding: 0; overflow: hidden; position: relative;
}
.side-head { display: flex; align-items: center; gap: 10px; padding: 14px 16px; background: #211f1c; font-weight: 700; }
.spacer { flex: 1; }
.live-dot { color: var(--green); font-size: 13px; }
.moves { flex: 1; overflow-y: auto; font-size: 14px; }
.mrow { display: grid; grid-template-columns: 44px 1fr 1fr; padding: 3px 12px; align-items: center; }
.mrow.odd { background: #2a2926; }
.mnum { color: var(--muted); }
.mv { cursor: pointer; padding: 2px 6px; border-radius: 3px; font-weight: 600; width: fit-content; }
.mv:hover { background: #3c3a37; }
.mv.cur { background: #4b4847; box-shadow: 0 2px 0 #1e1d1a; }
.empty { padding: 16px; text-align: center; }
.result-row { padding: 12px 16px; display: flex; gap: 12px; justify-content: center; }
.nav-btns { display: grid; grid-template-columns: repeat(5, 1fr); gap: 6px; padding: 10px 12px; background: #211f1c; }
.btn.icon { padding: 8px 0; font-size: 15px; }
.actions { display: flex; gap: 8px; padding: 0 12px 14px; background: #211f1c; }
.actions .btn { flex: 1; }
.btn.danger { background: #a72e24; box-shadow: 0 4px 0 0 #6f1e18; }
.btn.sm { padding: 6px 12px; font-size: 13px; }
.draw-banner { padding: 10px 12px; background: #3c3a37; display: flex; flex-direction: column; gap: 8px; font-weight: 600; }
.draw-banner div { display: flex; gap: 8px; }
.engine { padding: 10px 12px; background: #211f1c; border-top: 1px solid #3c3a37; }
.engine-row { display: flex; gap: 8px; }
.engine-row .engine-btn { flex: 1; }
.stop-btn { background: #a72e24; box-shadow: 0 4px 0 0 #6f1e18; }
.stop-btn:hover { background: #c0392b; }
.engine-btn { width: 100%; background: #3d3a5c; box-shadow: 0 4px 0 0 #24223a; }
.engine-btn:hover { background: #4a4770; }
.engine-btn.thinking { background: #4a4770; }
.spinner.sm { width: 16px; height: 16px; border-width: 2px; }
.engine-info { margin-top: 8px; font-size: 13px; }
.eline { display: flex; align-items: center; gap: 8px; }
.ebest { font-size: 18px; font-weight: 800; color: #ffb733; }
.eval { font-weight: 800; padding: 1px 6px; border-radius: 3px; background: #4b4847; }
.eval.ew { background: #eee; color: #222; }
.eval.eb { background: #111; color: #eee; }
.pv { margin-top: 4px; white-space: nowrap; overflow: hidden; text-overflow: ellipsis; }
.modal-btns { display: flex; gap: 8px; }
.modal-btns .btn { flex: 1; padding: 14px 8px; font-size: 16px; }
.modal-rematch { text-align: center; font-weight: 700; margin-bottom: 12px; color: var(--green); }
.toast { position: absolute; left: 12px; right: 12px; bottom: 120px; background: #a72e24; color: #fff; padding: 10px; border-radius: 6px; text-align: center; font-weight: 600; }

.modal-bg { position: fixed; inset: 0; background: rgba(0,0,0,.55); display: grid; place-items: center; z-index: 100; }
.modal { width: 340px; background: var(--panel); border-radius: 12px; overflow: hidden; position: relative; box-shadow: 0 20px 60px rgba(0,0,0,.6); }
.close { position: absolute; right: 10px; top: 8px; background: none; border: none; color: #fff; font-size: 18px; cursor: pointer; opacity: .7; }
.modal-head { padding: 22px 16px 16px; text-align: center; }
.modal-head h2 { margin: 0 0 4px; font-size: 28px; }
.modal-head.won { background: linear-gradient(#5d9948, #45753c); }
.modal-head.lost { background: linear-gradient(#5a5754, #3c3a37); }
.modal-head.neutral { background: linear-gradient(#5a5754, #3c3a37); }
.modal-body { padding: 20px; }
.players { display: flex; align-items: center; justify-content: space-around; margin-bottom: 20px; text-align: center; }
.players .p { display: flex; flex-direction: column; align-items: center; gap: 4px; width: 110px; font-size: 13px; }
.players .pn { font-weight: 700; overflow: hidden; text-overflow: ellipsis; max-width: 100%; }
.avatar { width: 56px; height: 56px; border-radius: 6px; display: grid; place-items: center; font-weight: 800; font-size: 24px; }
.aw { background: #e8e6e3; color: #333; }
.ab { background: #4b4847; color: #ddd; }
.vs { font-weight: 800; font-size: 18px; color: var(--muted); }
.up { color: var(--green); font-weight: 700; }
.down { color: #e05b4b; font-weight: 700; }
.big { font-size: 20px; padding: 14px; }

@media (max-width: 1000px) {
  .game-page { flex-direction: column; align-items: center; padding: 8px; --board: min(calc(100vw - 16px), calc(100vh - 170px)); }
  .side { width: var(--board); height: 360px; }
}
</style>
