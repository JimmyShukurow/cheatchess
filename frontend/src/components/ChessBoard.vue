<template>
  <div
    ref="boardEl"
    class="board"
    :class="{ dragging: !!drag?.moved }"
    @pointerdown="onDown"
    @pointermove="onMove"
    @pointerup="onUp"
    @pointercancel="cancelDrag"
    @contextmenu.prevent="onRightClick"
  >
    <!-- squares + coordinates -->
    <div
      v-for="s in squares"
      :key="s.name"
      class="sq"
      :class="s.light ? 'light' : 'dark'"
      :style="posStyle(s.name)"
    >
      <span v-if="s.x === 0" class="coord rank">{{ s.name[1] }}</span>
      <span v-if="s.y === 7" class="coord file">{{ s.name[0] }}</span>
    </div>

    <!-- highlights -->
    <div v-for="sq in lastMove || []" :key="'lm' + sq" class="hl last" :style="posStyle(sq)" />
    <div v-if="selected" class="hl last" :style="posStyle(selected)" />
    <div v-for="sq in marked" :key="'mk' + sq" class="hl marked" :style="posStyle(sq)" />
    <div v-if="check" class="hl check" :style="posStyle(check)" />
    <div v-if="hoverSq && drag?.moved" class="hl hover" :style="posStyle(hoverSq)" />
    <div
      v-for="d in selectedDests"
      :key="'d' + d"
      class="hint"
      :class="{ capture: pieceAt(d) }"
      :style="posStyle(d)"
    />

    <!-- pieces -->
    <div
      v-for="p in pieces"
      :key="p.id"
      class="piece"
      :class="{ 'no-anim': noAnim === p.sq, lifted: drag?.moved && drag.sq === p.sq }"
      :style="pieceStyle(p)"
    />

    <!-- arrows (engine suggestion) -->
    <svg v-if="arrowLines.length" class="arrows" viewBox="0 0 8 8">
      <defs>
        <marker v-for="c in arrowColors" :id="'ah-' + c.id" :key="c.id" markerWidth="3.2" markerHeight="3.2" refX="1.6" refY="1.6" orient="auto">
          <path d="M0,0 L3.2,1.6 L0,3.2 z" :fill="c.color" />
        </marker>
      </defs>
      <line
        v-for="(a, i) in arrowLines"
        :key="i"
        :x1="a.x1" :y1="a.y1" :x2="a.x2" :y2="a.y2"
        :stroke="a.color" stroke-width="0.2" stroke-linecap="round" :opacity="a.opacity"
        :marker-end="`url(#ah-${a.cid})`"
      />
    </svg>

    <!-- promotion chooser -->
    <div v-if="promo" class="promo-backdrop" @pointerdown.stop="promo = null">
      <div class="promo" :style="promoStyle" @pointerdown.stop>
        <div v-for="t in promoOrder" :key="t" class="promo-opt" @click="choosePromo(t)">
          <div class="piece static" :style="{ backgroundImage: `url(${img(promo.color + t.toUpperCase())})` }" />
        </div>
        <div class="promo-x" @click="promo = null">✕</div>
      </div>
    </div>
  </div>
</template>

<script setup>
import { ref, computed, watch } from 'vue'

const props = defineProps({
  fen: { type: String, required: true },
  orientation: { type: String, default: 'white' },
  // color allowed to move ('w' | 'b'), or null for a view-only board
  movable: { type: String, default: null },
  // Map<from, to[]>
  dests: { type: Object, default: () => new Map() },
  lastMove: { type: Array, default: null },
  check: { type: String, default: null },
  pieceSet: { type: String, default: 'cburnett' },
  // [{ from, to, color?, opacity? }]
  arrows: { type: Array, default: () => [] },
})
const emit = defineEmits(['move'])

const FILES = 'abcdefgh'
const boardEl = ref(null)
const img = (p) => `/pieces/${props.pieceSet}/${p}.svg`

const flipped = computed(() => props.orientation === 'black')
function xy(sq) {
  const f = FILES.indexOf(sq[0])
  const r = +sq[1] - 1
  return flipped.value ? { x: 7 - f, y: r } : { x: f, y: 7 - r }
}
function sqFromXY(x, y) {
  if (x < 0 || x > 7 || y < 0 || y > 7) return null
  return flipped.value ? FILES[7 - x] + (y + 1) : FILES[x] + (8 - y)
}
const posStyle = (sq) => {
  const { x, y } = xy(sq)
  return { transform: `translate(${x * 100}%, ${y * 100}%)` }
}

const arrowColors = computed(() => {
  const seen = new Map()
  for (const a of props.arrows) {
    const color = a.color || 'rgba(255,170,0,0.85)'
    if (!seen.has(color)) seen.set(color, { id: seen.size, color })
  }
  return [...seen.values()]
})
const arrowLines = computed(() =>
  props.arrows.map((a) => {
    const color = a.color || 'rgba(255,170,0,0.85)'
    const p1 = xy(a.from), p2 = xy(a.to)
    const x1 = p1.x + 0.5, y1 = p1.y + 0.5, x2 = p2.x + 0.5, y2 = p2.y + 0.5
    const len = Math.hypot(x2 - x1, y2 - y1)
    const k = (len - 0.32) / len // stop short so the arrow head ends inside the square
    return {
      x1, y1, x2: x1 + (x2 - x1) * k, y2: y1 + (y2 - y1) * k, color,
      opacity: a.opacity ?? 1, cid: arrowColors.value.find((c) => c.color === color).id,
    }
  }),
)

const squares = computed(() => {
  const out = []
  for (let y = 0; y < 8; y++)
    for (let x = 0; x < 8; x++) {
      const name = sqFromXY(x, y)
      const f = FILES.indexOf(name[0])
      const r = +name[1] - 1
      out.push({ name, x, y, light: (f + r) % 2 === 1 })
    }
  return out
})

// ---------- interaction state ----------
const selected = ref(null)
const drag = ref(null) // { sq, x, y, startX, startY, moved, wasSelected }
const hoverSq = ref(null)
const noAnim = ref(null)
const promo = ref(null)
const marked = ref([])
const promoOrder = ['q', 'n', 'r', 'b']

// ---------- pieces with stable ids (for move animation) ----------
let nextId = 1
const pieces = ref([])

function parseFen(fen) {
  const out = []
  const rows = fen.split(' ')[0].split('/')
  rows.forEach((row, i) => {
    let f = 0
    for (const ch of row) {
      if (/\d/.test(ch)) f += +ch
      else {
        const color = ch === ch.toUpperCase() ? 'w' : 'b'
        out.push({ sq: FILES[f] + (8 - i), p: color + ch.toUpperCase() })
        f++
      }
    }
  })
  return out
}

function dist(a, b) {
  return Math.abs(FILES.indexOf(a[0]) - FILES.indexOf(b[0])) + Math.abs(+a[1] - +b[1])
}

watch(
  () => props.fen,
  (fen) => {
    const next = parseFen(fen)
    const old = pieces.value.map((p) => ({ ...p, used: false }))
    const bySq = Object.fromEntries(old.map((p) => [p.sq, p]))
    const result = []
    const unmatched = []
    for (const n of next) {
      const o = bySq[n.sq]
      if (o && o.p === n.p) {
        o.used = true
        result.push({ id: o.id, sq: n.sq, p: n.p })
      } else unmatched.push(n)
    }
    for (const n of unmatched) {
      let best = null
      for (const o of old) {
        if (o.used || o.p !== n.p) continue
        if (!best || dist(o.sq, n.sq) < dist(best.sq, n.sq)) best = o
      }
      if (best) {
        best.used = true
        result.push({ id: best.id, sq: n.sq, p: n.p })
      } else result.push({ id: nextId++, sq: n.sq, p: n.p })
    }
    pieces.value = result
    selected.value = null
  },
  { immediate: true },
)

const pieceAt = (sq) => pieces.value.find((p) => p.sq === sq)

// ---------- interaction ----------

const selectedDests = computed(() => (selected.value && props.dests.get(selected.value)) || [])

function canPick(sq) {
  const p = pieceAt(sq)
  return props.movable && p && p.p[0] === props.movable && props.dests.has(sq)
}

function pointerSquare(e) {
  const r = boardEl.value.getBoundingClientRect()
  const size = r.width / 8
  return {
    sq: sqFromXY(Math.floor((e.clientX - r.left) / size), Math.floor((e.clientY - r.top) / size)),
    x: e.clientX - r.left,
    y: e.clientY - r.top,
  }
}

function onDown(e) {
  if (e.button !== 0) return
  marked.value = []
  const { sq, x, y } = pointerSquare(e)
  if (!sq) return
  // click-to-move
  if (selected.value && selectedDests.value.includes(sq)) {
    tryMove(selected.value, sq, false)
    return
  }
  if (canPick(sq)) {
    const wasSelected = selected.value === sq
    selected.value = sq
    drag.value = { sq, x, y, startX: x, startY: y, moved: false, wasSelected }
    boardEl.value.setPointerCapture(e.pointerId)
    e.preventDefault()
  } else {
    selected.value = null
  }
}

function onMove(e) {
  if (!drag.value) return
  const { sq, x, y } = pointerSquare(e)
  drag.value.x = x
  drag.value.y = y
  if (!drag.value.moved && Math.hypot(x - drag.value.startX, y - drag.value.startY) > 4) drag.value.moved = true
  hoverSq.value = sq
}

function onUp(e) {
  const d = drag.value
  if (!d) return
  drag.value = null
  hoverSq.value = null
  const { sq } = pointerSquare(e)
  if (d.moved) {
    if (sq && sq !== d.sq && (props.dests.get(d.sq) || []).includes(sq)) tryMove(d.sq, sq, true)
    else if (sq !== d.sq) selected.value = null
  } else if (d.wasSelected) {
    selected.value = null
  }
}

function cancelDrag() {
  drag.value = null
  hoverSq.value = null
}

function tryMove(from, to, dragged) {
  const p = pieceAt(from)
  selected.value = null
  if (p && p.p[1] === 'P' && (to[1] === '8' || to[1] === '1')) {
    promo.value = { from, to, color: p.p[0], dragged }
    return
  }
  doMove(from, to, undefined, dragged)
}

function doMove(from, to, promotion, dragged) {
  noAnim.value = dragged ? to : null
  emit('move', { from, to, promotion })
  setTimeout(() => (noAnim.value = null), 50)
}

function choosePromo(t) {
  const { from, to, dragged } = promo.value
  promo.value = null
  doMove(from, to, t, dragged)
}

const promoStyle = computed(() => {
  if (!promo.value) return {}
  const { x, y } = xy(promo.value.to)
  const top = y === 0
  return {
    left: `${x * 12.5}%`,
    top: top ? '0' : 'auto',
    bottom: top ? 'auto' : '0',
    flexDirection: top ? 'column' : 'column-reverse',
  }
})

function onRightClick(e) {
  const { sq } = pointerSquare(e)
  if (!sq) return
  marked.value = marked.value.includes(sq) ? marked.value.filter((s) => s !== sq) : [...marked.value, sq]
}

function pieceStyle(p) {
  const style = { backgroundImage: `url(${img(p.p)})` }
  if (drag.value?.moved && drag.value.sq === p.sq) {
    const size = boardEl.value.getBoundingClientRect().width / 8
    style.transform = `translate(${drag.value.x - size / 2}px, ${drag.value.y - size / 2}px)`
  } else {
    Object.assign(style, posStyle(p.sq))
  }
  return style
}

// The parent can call this to clear the selection (e.g. after a server reject).
defineExpose({ clear: () => (selected.value = null) })
</script>

<style scoped>
.board {
  position: relative; width: 100%; aspect-ratio: 1; user-select: none; touch-action: none;
  border-radius: 3px; overflow: hidden; container-type: inline-size;
}
.sq, .hl, .hint, .piece {
  position: absolute; left: 0; top: 0; width: 12.5%; height: 12.5%;
}
.sq.light { background: var(--light-sq); }
.sq.dark { background: var(--dark-sq); }
.coord { position: absolute; font-size: max(10px, 2.6cqw); font-weight: 700; line-height: 1; }
.coord.rank { top: 4%; left: 5%; }
.coord.file { bottom: 4%; right: 6%; }
.sq.light .coord { color: var(--dark-sq); }
.sq.dark .coord { color: var(--light-sq); }

.hl.last { background: rgba(255, 255, 51, 0.5); }
.hl.marked { background: rgba(235, 97, 80, 0.8); }
.hl.check { background: radial-gradient(ellipse at center, rgba(255, 0, 0, 1) 0%, rgba(231, 0, 0, 1) 25%, rgba(169, 0, 0, 0) 89%, rgba(158, 0, 0, 0) 100%); }
.hl.hover { box-shadow: inset 0 0 0 max(3px, .55cqw) rgba(255, 255, 255, 0.65); }

.hint { pointer-events: none; z-index: 3; }
.hint::after {
  content: ''; position: absolute; left: 50%; top: 50%; width: 33%; height: 33%;
  transform: translate(-50%, -50%); border-radius: 50%; background: rgba(0, 0, 0, 0.14);
}
.hint.capture::after {
  width: 100%; height: 100%; background: none; box-sizing: border-box;
  border: max(4px, .9cqw) solid rgba(0, 0, 0, 0.14);
}

.piece {
  background-size: 100%; background-repeat: no-repeat; z-index: 2; cursor: pointer;
  transition: transform .14s ease-out; will-change: transform;
}
.piece.no-anim, .piece.lifted { transition: none; }
.piece.lifted { z-index: 10; cursor: grabbing; }
.board.dragging { cursor: grabbing; }
.piece.static { position: static; width: 100%; height: 100%; transition: none; }

.arrows { position: absolute; inset: 0; width: 100%; height: 100%; z-index: 5; pointer-events: none; }
.promo-backdrop { position: absolute; inset: 0; z-index: 20; background: rgba(0, 0, 0, .35); }
.promo {
  position: absolute; width: 12.5%; display: flex; background: #fff; border-radius: 4px;
  box-shadow: 0 4px 16px rgba(0,0,0,.5); overflow: hidden;
}
.promo-opt { aspect-ratio: 1; cursor: pointer; }
.promo-opt:hover { background: #e9e9e9; }
.promo-x { text-align: center; color: #777; background: #f1f1f1; padding: 4px; cursor: pointer; font-size: 13px; }
</style>
