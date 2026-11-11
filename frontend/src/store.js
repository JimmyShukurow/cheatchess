import { reactive } from 'vue'
import { sounds } from './sound.js'

export const state = reactive({
  user: null,
  googleEnabled: false,
  connected: false,
  online: { players: 0, games: 0 },
  seeking: '',
  activeGame: '',
  autoHint: false, // engine auto-suggest mode (admin)
  players: [], // online players [{ id, username, ratingBullet, ratingBlitz, ratingRapid, status }]
  incoming: [], // challenges sent to me
  outgoing: null, // my pending challenge
  notice: '', // short message (e.g. "X declined your challenge")
})

let noticeTimer = null
export function notify(msg) {
  state.notice = msg
  clearTimeout(noticeTimer)
  noticeTimer = setTimeout(() => (state.notice = ''), 4000)
}

export async function api(path, { method = 'GET', body } = {}) {
  const res = await fetch(path, {
    method,
    credentials: 'same-origin',
    headers: body ? { 'Content-Type': 'application/json' } : {},
    body: body ? JSON.stringify(body) : undefined,
  })
  const data = await res.json().catch(() => ({}))
  if (!res.ok) throw new Error(data.error || res.statusText)
  return data
}

export async function init() {
  try {
    const cfg = await api('/api/config')
    state.googleEnabled = cfg.googleEnabled
  } catch {}
  try {
    const { user } = await api('/api/auth/me')
    state.user = user
    connect()
  } catch {}
}

export async function login(loginName, password) {
  const { user } = await api('/api/auth/login', { method: 'POST', body: { login: loginName, password } })
  state.user = user
  connect()
}

export async function register(email, username, password) {
  const { user } = await api('/api/auth/register', { method: 'POST', body: { email, username, password } })
  state.user = user
  connect()
}

export async function logout() {
  await api('/api/auth/logout', { method: 'POST' }).catch(() => {})
  disconnect()
  state.user = null
  state.seeking = ''
  state.activeGame = ''
  state.incoming = []
  state.outgoing = null
}

// ---------------- WebSocket ----------------

let ws = null
let retry = 0
let wanted = false
let watching = ''
const queue = []
const listeners = new Set()

export function onMessage(fn) {
  listeners.add(fn)
  return () => listeners.delete(fn)
}

export function send(msg) {
  if (msg.t === 'watch') watching = msg.game
  if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(msg))
  else if (msg.t !== 'watch') queue.push(msg)
}

export function unwatch() {
  watching = ''
}

export function connect() {
  wanted = true
  if (ws) return
  const proto = location.protocol === 'https:' ? 'wss' : 'ws'
  ws = new WebSocket(`${proto}://${location.host}/ws`)
  ws.onopen = () => {
    retry = 0
    state.connected = true
    if (watching) ws.send(JSON.stringify({ t: 'watch', game: watching }))
    while (queue.length) ws.send(JSON.stringify(queue.shift()))
  }
  ws.onmessage = (ev) => {
    let msg
    try { msg = JSON.parse(ev.data) } catch { return }
    switch (msg.t) {
      case 'hello':
        state.user = msg.user
        state.activeGame = msg.activeGame || ''
        state.seeking = msg.seeking || ''
        break
      case 'user':
        state.user = msg.user
        break
      case 'seeking':
        state.seeking = msg.tc
        break
      case 'seekCanceled':
        state.seeking = ''
        break
      case 'start':
        state.seeking = ''
        state.activeGame = msg.game.id
        state.incoming = []
        state.outgoing = null
        break
      case 'challenges':
        state.incoming = msg.incoming || []
        state.outgoing = msg.outgoing || null
        break
      case 'challenge':
        sounds.notify()
        state.incoming = [...state.incoming.filter((c) => c.id !== msg.challenge.id), msg.challenge]
        break
      case 'challengeSent':
        state.outgoing = msg.challenge
        break
      case 'challengeGone': {
        state.incoming = state.incoming.filter((c) => c.id !== msg.id)
        if (state.outgoing?.id === msg.id) {
          state.outgoing = null
          const why = { declined: `${msg.by} declined your challenge`, expired: `${msg.by} didn't answer`, offline: `${msg.by} went offline`, busy: `${msg.by} started another game` }
          if (why[msg.reason]) notify(why[msg.reason])
        }
        break
      }
      case 'game':
        if (msg.game.status === 'ended' && state.activeGame === msg.game.id) state.activeGame = ''
        break
      case 'error':
        notify(msg.msg)
        break
      case 'online':
        state.online = { players: msg.players, games: msg.games }
        if (msg.list) state.players = msg.list
        break
    }
    listeners.forEach((fn) => fn(msg))
  }
  ws.onclose = () => {
    ws = null
    state.connected = false
    if (!wanted) return
    const delay = Math.min(1000 * 2 ** retry, 10000)
    retry++
    setTimeout(() => wanted && connect(), delay)
  }
}

export function disconnect() {
  wanted = false
  if (ws) ws.close()
  ws = null
}
