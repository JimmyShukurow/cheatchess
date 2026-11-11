# CheatChess

Online chess: sign up with email or Google, choose a time control, get paired, play.
No anti-cheat by design.

- **Backend:** Go: one binary with REST, WebSocket, SQLite and the built frontend embedded
- **Frontend:** Vue 3 + Vite, a custom drag-and-drop board, chess.js for move hints
- **Resources:** a single ~12 MB static binary using about 3 MB RAM idle, run by systemd behind Caddy (no containers). A 1 vCPU / 512 MB VPS handles hundreds of players.

## Features
- Register/login with email+password or Google OAuth (cookie sessions)
- Matchmaking queues: bullet (1+0, 1+1, 2+1), blitz (3+0, 3+2, 5+0, 5+3), rapid (10+0, 10+5, 15+10, 30+0)
- Server-authoritative moves and clocks. Timeout, checkmate, stalemate, threefold, 50-move, insufficient material
- Resign, draw offer/accept, abort before move 2 (also automatic after 30 s without a first move)
- Elo ratings per category, recent games, game replay (←/→ keys), board flip, right-click square marks
- Reconnect-safe: a refresh or dropped connection resyncs the game, and clocks keep running on the server
- Rematch with the same opponent (colors swapped), available for 15 minutes after a game
- **Engine "Best move" button** (Stockfish, admin accounts only, checked by the server): arrow on the board plus eval and line.
  Click again (or press `H`) to recalculate deeper: 1.2 s → 3 s → 6 s → 10 s.

## Admin / engine button
Set `ADMIN_EMAILS=you@example.com` in `.env`. The email also has to be **verified**, otherwise anyone could register with it:
- sign in with Google once (this verifies it automatically), or
- run `./cheatchess verify-email you@example.com` locally, or `sudo cheatchess-cli verify-email you@example.com` on the VPS, then log in again.

Stockfish has to be installed (`sudo apt install stockfish`; the provisioning playbook does this). Set `STOCKFISH_PATH` if it's somewhere unusual.

## Develop
```bash
make dev-backend    # terminal 1 → http://localhost:8080 (API/WS)
make dev-frontend   # terminal 2 → http://localhost:5173 (hot reload)
```

## Deploy (native on the VPS: systemd + Caddy, no Docker)
```bash
make provision   # once: Caddy, Stockfish, systemd units, firewall, swap, daily backups
make deploy      # build dist/cheatchess and ship it (or just `git push` with GitHub Actions)
```
Secrets come from an encrypted Ansible Vault. Full guide: **[deploy/README.md](deploy/README.md)**.

## Google login
In Google Cloud Console → APIs & Services → Credentials → *Create OAuth client ID* (Web application):
- Authorized redirect URI: `https://YOUR_DOMAIN/api/auth/google/callback`
- Put the client ID and secret in `.env`. If they're missing, the Google button is hidden.

## Config (env vars)
| var | default | |
|---|---|---|
| `ADDR` | `:8080` | listen address |
| `DB_PATH` | `cheatchess.db` | SQLite file |
| `BASE_URL` | `http://localhost:8080` | public URL (OAuth redirect; `https://` enables secure cookies) |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | – | enables Google login |
| `TRUST_PROXY` | – | `1` = use `X-Forwarded-For` for rate limiting |
| `ADMIN_EMAILS` | – | comma-separated emails that get the engine button (must be verified) |
| `STOCKFISH_PATH` | `stockfish` | engine binary (also checks `/usr/games/stockfish`) |

The server also reads a `.env` file from the current or parent directory.

## Credits
Piece set "cburnett" by Colin M.L. Burnett, CC BY-SA 3.0 (`frontend/public/pieces/cburnett/LICENSE.txt`).
To use another set, add `frontend/public/pieces/<name>/{wK,wQ,…,bP}.svg` and change `pieceSet` in `ChessBoard.vue`.
