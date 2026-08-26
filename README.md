# Grok Build WebUI

Run persistent **Grok Build CLI** (`grok`) sessions in your browser — tabs and
split panes per project, built to sit 24/7 behind a Cloudflare Tunnel.

Each pane is a live PTY running `grok` inside a project directory. Sessions
survive refreshes, logouts, project switches and network blips; they end only
when you close the pane or stop the service.

**Highlights**

- Terminal-per-tab with horizontal/vertical splits (xterm.js); replays the last 256 KB on reconnect
- Auth: password (bcrypt) + passkeys/WebAuthn + JWT httpOnly cookie
- Projects are absolute paths on your machine — anything you can reach, you can grok
- Single static Go binary + SQLite — no Node, no external services
- Ships with a service installer: **systemd or docker compose**, up on boot, restarts on crash

---

## Install

```sh
git clone https://github.com/karutoil/grok-build-webui.git
cd grok-build-webui
./scripts/install.sh
```

An interactive wizard takes it from there — every question shows its default,
so pressing **Enter** through the prompts gives you a working install. It
installs the **Grok Build CLI** for you if it's missing, then asks how the
service should run:

| Mode | What you get |
|------|--------------|
| **native** *(recommended)* | Plain systemd unit running a static binary as your user — fewest moving parts, nothing extra to install. No Go needed: prebuilt binaries download from GitHub Releases automatically. |
| **docker** | Compose stack running as *you* (`UID`/`GID`), with your `$HOME` bind-mounted **at the same path** so project dirs and the host `grok` CLI (`~/.grok/bin/grok`) work exactly as natively. Handy if you want isolation or already run everything in containers. |

Either way you get: enabled on boot, automatic restart on crash, logs in
`journalctl`, and updates that never touch your database.

No Go installed? No problem — release binaries are built by CI on every tag
and verified against checksums at download time. Compiling locally is still
possible with `install --from-source`.

### After installing

1. Open `http://localhost:8080` (or the port/port-forward you configured)
2. First visit shows **Initial Setup** → create the admin user
3. Sidebar → **`+ New`** → name + an absolute project path → **`+ Tab`** → grok starts

Useful keys: <kbd>Ctrl</kbd>+<kbd>\`</kbd> new tab · `+H` / `+V` split a pane · drag resizers · `×` closes a pane.

---

## Managing the service

Re-run the wizard any time, or call actions directly — every command is also
usable non-interactively (`--mode`, `--port`, `-y`, …):

```sh
./scripts/install.sh            # interactive menu
./scripts/install.sh update     # fetch latest code/release + restart
./scripts/install.sh config     # change port / public URL / grok binary
./scripts/install.sh status     # detailed service state
./scripts/install.sh logs       # follow journalctl live
./scripts/install.sh doctor     # environment sanity check
./scripts/install.sh remove     # uninstall (data deletion asks twice)
```

---

## Going public (Cloudflare Tunnel)

```sh
cloudflared tunnel --url http://localhost:8080
# → https://random-words-1234.trycloudflare.com
```

Set that URL during install (the *Public URL* prompt expects the full form,
e.g. `https://grok.example.com`) or later via `config`. It drives CORS, secure
cookies and WebAuthn, so set it **before** registering passkeys. For named
tunnels swap `--url` for `tunnel run --url http://localhost:8080 grok-webui`.

> **Security note:** anyone who can log in can run a grok agent in any visible
> project directory. Use strong credentials, keep it off the open internet
> without a tunnel/firewall, and prefer passkeys once your URL is final.

---

<details>
<summary><strong>Appendix — configuration, internals & development</strong></summary>

### Runtime flags / environment (set automatically by the installer)

| Flag | Env | Default |
|------|-----|---------|
| `--addr` | `GROK_WEBUI_ADDR` | `:8080` |
| `--data` | `GROK_WEBUI_DATA` | `./data` |
| `--public-url` | `GROK_WEBUI_PUBLIC_URL` | *(empty)* |
| `--grok-bin` | `GROK_WEBUI_GROK_BIN` | `grok` |
| `--jwt-secret` | `GROK_WEBUI_JWT_SECRET` | auto-generated, persisted |
| `--max-sessions` | `GROK_WEBUI_MAX_SESSIONS` | `16` |

Binary supply order for native installs: existing local binary → local
`go build` (if clone + toolchain present) → **download newest CI release**
(default when Go is absent; override repo via `--repo OWNER/NAME` or
`GROK_WEBUI_REPO`, optional token via `GITHUB_TOKEN`).

### How it works

```
Browser (vanilla JS + bundled xterm.js, single SPA)
  │  HTTPS/WSS  (direct, or via Cloudflare Tunnel)
  └─► Go server (net/http + embed.FS)
        ├─ REST   /api/{auth,projects,sessions,settings}
        ├─ WS     /api/sessions/:id/ws ──► manager ──► pty(grok)
        ├─ Auth   JWT cookie + WebAuthn
        └─ SQLite (WAL) users, credentials, projects, sessions, settings
```

PTYs stream into a 256 KB ring buffer fanned out over WebSockets; deleting a
pane sends SIGINT, then SIGKILL after a grace period.

### Security notes

- Login rate-limited in memory; extend or proxy-limit if exposed publicly
- Project paths must exist; `/`, `/etc`, `/root` are rejected
- `Secure` cookies activate automatically when traffic arrives over https
- Passkeys registered on one origin don't transfer to another

### Developing

```sh
make vet       # static checks
make test      # go test ./...
make build     # stamped dev binary
make run       # :8080, data in ./data
```

Frontend is vanilla JS in `web/` (embedded at compile time — rebuild after
edits). CI builds tagged releases via `.github/workflows/release.yml`.

</details>

---

MIT — do as you like, keep the notice.
