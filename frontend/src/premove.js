// Squares a piece could premove to: its movement pattern on the board, ignoring
// blockers (the position will change before the premove is played). Like lichess/chess.com,
// a premove onto your own piece is allowed (e.g. to recapture).
const FILES = 'abcdefgh'

function parseBoard(fen) {
  const board = {}
  fen.split(' ')[0].split('/').forEach((row, i) => {
    let f = 0
    for (const ch of row) {
      if (/\d/.test(ch)) f += +ch
      else {
        board[FILES[f] + (8 - i)] = { color: ch === ch.toUpperCase() ? 'w' : 'b', type: ch.toLowerCase() }
        f++
      }
    }
  })
  return board
}

const sq = (f, r) => (f >= 0 && f < 8 && r >= 0 && r < 8 ? FILES[f] + (r + 1) : null)

function targets(piece, from, board) {
  const f = FILES.indexOf(from[0])
  const r = +from[1] - 1
  const out = []
  const add = (s) => s && out.push(s)
  const rays = (dirs) => {
    for (const [df, dr] of dirs) for (let k = 1; k < 8; k++) add(sq(f + df * k, r + dr * k))
  }
  const ORTHO = [[1, 0], [-1, 0], [0, 1], [0, -1]]
  const DIAG = [[1, 1], [1, -1], [-1, 1], [-1, -1]]
  switch (piece.type) {
    case 'p': {
      const dir = piece.color === 'w' ? 1 : -1
      add(sq(f, r + dir))
      if ((piece.color === 'w' && r === 1) || (piece.color === 'b' && r === 6)) add(sq(f, r + 2 * dir))
      add(sq(f - 1, r + dir))
      add(sq(f + 1, r + dir))
      break
    }
    case 'n':
      for (const [df, dr] of [[1, 2], [2, 1], [-1, 2], [-2, 1], [1, -2], [2, -1], [-1, -2], [-2, -1]]) add(sq(f + df, r + dr))
      break
    case 'b':
      rays(DIAG)
      break
    case 'r':
      rays(ORTHO)
      break
    case 'q':
      rays([...ORTHO, ...DIAG])
      break
    case 'k': {
      for (const [df, dr] of [...ORTHO, ...DIAG]) add(sq(f + df, r + dr))
      const home = piece.color === 'w' ? '1' : '8'
      if (from === 'e' + home) {
        const rook = (s) => board[s]?.type === 'r' && board[s]?.color === piece.color
        if (rook('h' + home)) add('g' + home)
        if (rook('a' + home)) add('c' + home)
      }
      break
    }
  }
  return out
}

// Map<from, to[]> for every piece of `color`.
export function premoveDests(fen, color) {
  const board = parseBoard(fen)
  const m = new Map()
  for (const [from, piece] of Object.entries(board)) {
    if (piece.color !== color) continue
    const t = targets(piece, from, board)
    if (t.length) m.set(from, t)
  }
  return m
}
