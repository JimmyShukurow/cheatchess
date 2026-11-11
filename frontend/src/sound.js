// Tiny synthesized sounds (no audio files needed).
let ctx = null
function ac() {
  if (!ctx) ctx = new (window.AudioContext || window.webkitAudioContext)()
  if (ctx.state === 'suspended') ctx.resume()
  return ctx
}

function knock(freq, dur, gain = 0.5, type = 'triangle') {
  try {
    const c = ac()
    const t = c.currentTime
    const o = c.createOscillator()
    const g = c.createGain()
    o.type = type
    o.frequency.setValueAtTime(freq, t)
    o.frequency.exponentialRampToValueAtTime(freq * 0.5, t + dur)
    g.gain.setValueAtTime(gain, t)
    g.gain.exponentialRampToValueAtTime(0.001, t + dur)
    o.connect(g).connect(c.destination)
    o.start(t)
    o.stop(t + dur)
  } catch {}
}

export const sounds = {
  move: () => knock(420, 0.08, 0.6),
  capture: () => { knock(300, 0.1, 0.7); setTimeout(() => knock(520, 0.06, 0.4), 40) },
  check: () => { knock(660, 0.12, 0.5, 'square'); },
  start: () => { knock(520, 0.15, 0.3, 'sine'); setTimeout(() => knock(780, 0.2, 0.3, 'sine'), 120) },
  end: () => { knock(780, 0.15, 0.3, 'sine'); setTimeout(() => knock(520, 0.25, 0.3, 'sine'), 140) },
  lowTime: () => knock(880, 0.08, 0.25, 'sine'),
}
