// Game sounds: original synthesized set in /public/sounds (see frontend/sounds-src/generate.py).
// Decoded once and played through Web Audio, so rapid moves/premoves overlap without lag.
let ctx = null
function ac() {
  if (!ctx) ctx = new (window.AudioContext || window.webkitAudioContext)()
  if (ctx.state === 'suspended') ctx.resume()
  return ctx
}

// Browsers only allow audio after a user gesture: unlock on the first click/key.
if (typeof window !== 'undefined') {
  const unlock = () => {
    ac()
    window.removeEventListener('pointerdown', unlock)
    window.removeEventListener('keydown', unlock)
  }
  window.addEventListener('pointerdown', unlock)
  window.addEventListener('keydown', unlock)
}

const NAMES = ['move1', 'move2', 'move3', 'capture', 'castle', 'check', 'start', 'end', 'lowtime', 'notify']
const buffers = {}
for (const n of NAMES) {
  buffers[n] = fetch(`/sounds/${n}.mp3`)
    .then((r) => r.arrayBuffer())
    .then((b) => ac().decodeAudioData(b))
    .catch(() => null)
}

const VOLUME = 0.9

async function play(name, gain = 1) {
  try {
    const buffer = await buffers[name]
    if (!buffer) return
    const c = ac()
    const src = c.createBufferSource()
    src.buffer = buffer
    const g = c.createGain()
    g.gain.value = gain * VOLUME
    src.connect(g).connect(c.destination)
    src.start()
  } catch {}
}

let lastMove = 0
export const sounds = {
  // rotate between 3 slightly different clacks so repeated moves sound natural
  move: () => play(`move${(lastMove = (lastMove % 3) + 1)}`),
  capture: () => play('capture'),
  castle: () => play('castle'),
  check: () => play('check'),
  start: () => play('start'),
  end: () => play('end'),
  lowTime: () => play('lowtime'),
  notify: () => play('notify'),
}
