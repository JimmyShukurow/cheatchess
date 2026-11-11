<template>
  <div class="pbar">
    <div class="avatar" :class="color === 'w' ? 'aw' : 'ab'">{{ (player?.username || '?')[0].toUpperCase() }}</div>
    <div class="info">
      <div class="top">
        <span class="name">{{ player?.username || '…' }}</span>
        <span class="rating">({{ player?.rating ?? '' }})</span>
        <span v-if="diff" class="diff" :class="diff > 0 ? 'up' : 'down'">{{ diff > 0 ? '+' + diff : diff }}</span>
      </div>
      <div class="captured">
        <img v-for="(p, i) in captured" :key="i" :src="`/pieces/cburnett/${p}.svg`" :class="{ gap: i > 0 && captured[i - 1][1] !== p[1] }" />
        <span v-if="advantage > 0" class="adv">+{{ advantage }}</span>
      </div>
    </div>
    <div class="clock" :class="{ active: running, low: ms < 20000, white: color === 'w' }">
      {{ fmt }}
    </div>
  </div>
</template>

<script setup>
import { computed } from 'vue'

const props = defineProps({
  player: Object,
  color: String, // 'w' | 'b'
  ms: { type: Number, default: 0 },
  running: Boolean,
  captured: { type: Array, default: () => [] }, // piece codes this player has captured, e.g. ['bP','bN']
  advantage: { type: Number, default: 0 },
  diff: { type: Number, default: 0 },
})

const fmt = computed(() => {
  const ms = Math.max(0, props.ms)
  const total = Math.floor(ms / 1000)
  const h = Math.floor(total / 3600)
  const m = Math.floor((total % 3600) / 60)
  const s = total % 60
  if (h) return `${h}:${String(m).padStart(2, '0')}:${String(s).padStart(2, '0')}`
  if (ms < 20000) return `${m}:${String(s).padStart(2, '0')}.${Math.floor((ms % 1000) / 100)}`
  return `${m}:${String(s).padStart(2, '0')}`
})
</script>

<style scoped>
.pbar { display: flex; align-items: center; gap: 10px; padding: 6px 0; height: 56px; }
.avatar { width: 40px; height: 40px; border-radius: 4px; display: grid; place-items: center; font-weight: 800; font-size: 18px; flex-shrink: 0; }
.aw { background: #e8e6e3; color: #333; }
.ab { background: #4b4847; color: #ddd; }
.info { flex: 1; min-width: 0; }
.top { display: flex; align-items: baseline; gap: 6px; }
.name { font-weight: 700; font-size: 15px; overflow: hidden; text-overflow: ellipsis; white-space: nowrap; }
.rating { color: var(--muted); font-size: 14px; }
.diff { font-weight: 700; font-size: 13px; }
.diff.up { color: var(--green); }
.diff.down { color: #e05b4b; }
.captured { display: flex; align-items: center; height: 20px; }
.captured img { width: 18px; height: 18px; margin-right: -9px; }
.captured img.gap { margin-left: 12px; }
.adv { margin-left: 14px; font-size: 13px; color: var(--muted); font-weight: 600; }
.clock {
  font-family: 'Roboto Mono', ui-monospace, Menlo, Consolas, monospace; font-size: 26px; font-weight: 600;
  min-width: 128px; text-align: right; padding: 6px 12px; border-radius: 4px;
  background: #262421; color: #6b6966;
}
.clock.white { background: #989795; color: #4f4d4a; }
.clock.active { background: #262421; color: #fff; }
.clock.white.active { background: #fff; color: #262421; }
.clock.active.low { background: #a72e24; color: #fff; }
</style>
