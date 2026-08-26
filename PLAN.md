# Grok Build WebUI — Plan

## Goal
Lightweight Go frontend that multiplexes persistent Grok Build CLI ( `grok` TUI ) sessions across projects with tab + split-pane UX. Sessions survive refresh/logout/project-switch; only explicit tab/pane close kills the underlying PTY.

Cloudflare Tunnel friendly: configurable public URL drives CORS, cookie, and WebAuthn RP origin.

## Non-goals
- Multi-node clustering
- Built-in git hosting
- Heavy frontend framework / build step

## Architecture
```
Browser (xterm.js + vanilla JS, single SPA)
   │  HTTPS / WSS
   ├─► Go HTTP Server (single binary, embed.FS)
   │     ├─ Static SPA (index.html, app.js, style.css)
   │     ├─ REST API  (/api/*)
   │     ├─ WebSocket (/api/sessions/:id/ws)  ──► PTY Manager ──► pty (grok)
   │     ├─ Auth (JWT httpOnly cookie + WebAuthn)
   │     ├─ CORS middleware (derived from PUBLIC_URL setting)
   │     └─ SQLite (modernc.org/sqlite, pure Go) ──► users, webauthn creds, projects, sessions, settings
   └─► Cloudflare Tunnel (optional, points at :8080)
```

Single binary. No Node build. Frontend assets embedded via `embed.FS`.

## Data Model (SQLite)
```sql
users(id TEXT PK, username TEXT UNIQUE, password_hash TEXT, created_at DATETIME)
webauthn_credentials(id TEXT PK, user_id FK, credential BLOB, created_at DATETIME)
projects(id TEXT PK, name TEXT, path TEXT UNIQUE, created_at DATETIME)
sessions(id TEXT PK, project_id FK, title TEXT, status TEXT, pid INT, cols INT, rows INT, created_at DATETIME, last_active DATETIME)
settings(key TEXT PK, value TEXT) -- e.g. public_url
```

Runtime-only (not persisted as running pid):
- `manager.sessions` map[id]*PTYSession { cmd, pty, buffer (ring 64KB), clients map[ws], cols/rows, mutex }

On server restart: all previous PTYs are dead; sessions rows marked `exited` on load, user can recreate.

## Auth Design
- **First run**: `GET /api/auth/setup-required` → if zero users, frontend shows Setup: create first admin (username+password). No auth required for that endpoint only.
- **Password**: bcrypt hash, `POST /api/auth/login` → verify → issue JWT (HS256, 7d) in `HttpOnly; Secure?; SameSite=Lax` cookie `grok_token`. `Secure` auto-enabled when `public_url` is https or `X-Forwarded-Proto=https`.
- **Passkey**: `go-webauthn/webauthn` (RPID = hostname of public_url or request host fallback; RPOrigins = [public_url, http://localhost:*]). Flows:
  - Registration: `POST /api/auth/webauthn/register/begin` (auth required) → `POST .../finish`
  - Login begin: `POST /api/auth/webauthn/login/begin` (no auth) with username optional (discoverable) → `POST .../finish` issues same JWT cookie.
- **Middleware**: extracts JWT from cookie `grok_token` or `Authorization: Bearer`. Sets `user` in context. WS upgrade also checks cookie + Origin.

## Public URL & CORS
- Setting: `public_url` stored in `settings` + env override `GROK_WEBUI_PUBLIC_URL` + flag `--public-url`. Env wins if set, else DB value, else request host.
- Controls:
  - `RPID` = hostname(public_url) without port
  - `RPOrigins` = [public_url, https://public_url, http://localhost:*, http://127.0.0.1:*]
  - CORS: `AllowOrigin` = public_url's origin + localhost dev origins; `AllowCredentials=true`; `Vary: Origin`. Handles preflight.
  - Cookie: `Domain` not set (host-only). `SameSite=Lax` (works for same-site tunnel). If public_url https => `Secure=true`. Behind Cloudflare Tunnel, `X-Forwarded-Proto` respected.
  - WS: origin check against same allowlist.

- Settings UI: input for public URL, test button, save → updates DB + live reloads WebAuthn config + CORS.

## PTY / Session Lifecycle
- Create: `POST /api/projects/:id/sessions` { cols, rows, title? } → spawn `pty.Start(cmd)` where `cmd = exec.Command("grok")` with `Dir=project.path`, `Env` + `TERM=xterm-256color` `COLORTERM=truecolor`. Assign `id=uuid`, store row, start goroutine copying pty→ring buffer + broadcast to attached WS clients.
- Attach: `GET /api/sessions/:id/ws?cols=&rows=` → upgrade, replay ring buffer, then stream. On client `resize` msg → `pty.Setsize`. On client `data` msg → write to pty.
- Detach: WS close → remove client from session.clients; **do NOT kill pty**.
- Kill: `DELETE /api/sessions/:id` → `cmd.Process.Signal(SIGHUP)` then `Kill`, close pty, delete row, notify clients.
- Resize: `POST /api/sessions/:id/resize` or WS `{"type":"resize",cols,rows}`
- History: ring buffer 128KB per session, replay on attach so refresh restores view.
- Idle: no timeout; sessions live until explicit close or server shutdown (graceful SIGHUP).

## API Contract (REST + WS)
```
Auth:
  GET  /api/auth/setup-required
  POST /api/auth/setup            {username,password}
  POST /api/auth/login            {username,password}
  POST /api/auth/logout
  GET  /api/auth/me
  POST /api/auth/webauthn/register/begin
  POST /api/auth/webauthn/register/finish
  POST /api/auth/webauthn/login/begin   {username?}
  POST /api/auth/webauthn/login/finish
  GET  /api/auth/webauthn/credentials
  DELETE /api/auth/webauthn/credentials/:id

Projects:
  GET    /api/projects
  POST   /api/projects              {name,path}
  GET    /api/projects/:id
  PUT    /api/projects/:id          {name,path}
  DELETE /api/projects/:id

Sessions:
  GET    /api/projects/:id/sessions
  POST   /api/projects/:id/sessions {title?,cols?,rows?}
  GET    /api/sessions/:id
  DELETE /api/sessions/:id
  POST   /api/sessions/:id/resize   {cols,rows}
  GET    /api/sessions/:id/ws       (WS)

Settings (admin):
  GET  /api/settings
  PUT  /api/settings                {public_url}

WS protocol JSON:
  client → server: {"type":"data","data":"..."} | {"type":"resize","cols":120,"rows":30}
  server → client: {"type":"data","data":"..."} | {"type":"exit","code":0}
```

## Frontend (vanilla, lightweight)
- `web/index.html`: layout shell: header (project selector, settings, logout), sidebar (project list + new project), main (tab bar + pane container + status bar), login/setup overlay, settings modal.
- `web/app.js` (~900 LOC, no bundler): router (hash or history), state { user, projects, activeProjectId, tabs: Map<projectId, {sessions, activeSessionId, panes: PaneTree}>, wsConnections }. PaneTree = binary tree leaf=sessionId or split {dir:'row'|'col', a,b, sizes}.
  - Tab handling: createTab → POST session → add to tabs, render.
  - Split: splitPane(paneId, direction) → create new session → replace leaf with split node.
  - Persistence: tabs/panes in localStorage per project + server session list reconciliation. Sessions survive even if tabs state lost (user can re-attach via session list).
  - xterm.js 5.x via CDN (cdn.jsdelivr.net) with `addon-fit` and `addon-web-links`. Each pane has its own Terminal instance attached to WS.
  - Reconnect on WS drop with exponential backoff; on refresh replay buffer handles continuity.
- `web/style.css`: dark theme matching Grok aesthetic, minimal, CSS Grid/Flex, resizable panes via drag handle.

## File Layout
```
grok-build-webui/
  go.mod
  cmd/server/main.go
  internal/
    config/config.go
    db/db.go
    auth/auth.go
    middleware/cors.go
    project/project.go
    session/manager.go
    handlers/
      auth.go
      projects.go
      sessions.go
      ws.go
      settings.go
  web/
    index.html
    app.js
    style.css
```

## Dependencies (Go)
- `github.com/go-webauthn/webauthn/webauthn`
- `github.com/golang-jwt/jwt/v5`
- `github.com/gorilla/websocket`
- `github.com/creack/pty`
- `modernc.org/sqlite` (pure Go sqlite)
- `golang.org/x/crypto/bcrypt`
- `github.com/google/uuid`

## Security
- Passwords bcrypt cost 10.
- JWT HS256 with 32+ byte secret (generated on first run if not provided via `GROK_WEBUI_JWT_SECRET`, persisted in settings).
- Rate limit login attempts (in-memory 5/min/IP).
- Project `path` traversal check: must be absolute, must exist, must not be `/` or `/etc` etc. Optional allowlist root.
- WS origin validation strict.

## Cloudflare Tunnel Notes
- Run: `cloudflared tunnel --url http://localhost:8080`
- Set `public_url` to `https://your-tunnel.trycloudflare.com` in Settings → ensures WebAuthn RPID/Origin + CORS + Secure cookies align.
- No code change needed; just URL config.

## Build & Run
```
go build -o grok-webui ./cmd/server
./grok-webui --addr :8080 --data ./data --public-url https://example.com
# env: GROK_WEBUI_PUBLIC_URL, GROK_WEBUI_JWT_SECRET, GROK_WEBUI_ADDR
```

## Verification
- `go vet` + `go build`
- Manual: create project → open tab → run grok → refresh → still attached, logout/login → still alive, split → two groks, close tab → session killed.
- Auth: setup → login password → register passkey → logout → login via passkey
- CORS: set public_url to tunnel URL → fetch from tunnel origin succeeds, other origin blocked.

