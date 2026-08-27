#!/usr/bin/env bash
#
# grok-build-webui installer / manager
#
# An interactive TUI (whiptail when available, plain-text fallback) that
# installs, updates, configures and removes Grok Build WebUI as a systemd
# service — either as a NATIVE binary service or via DOCKER COMPOSE with a
# thin systemd wrapper, so `systemctl status grok-webui` always works.
#
# Native mode does NOT require Go on the target machine: by default it
# downloads a prebuilt static binary from this project's GitHub Releases
# (built automatically by CI on every v* tag). Pass --from-source if you
# want to compile locally instead. Every prompt accepts its default with
# plain Enter.
#
# Usage:
#   ./scripts/install.sh                 # launch the interactive TUI
#   ./scripts/install.sh install|update|remove|status|logs|config|doctor
#
# Zero-setup install (no clone needed):
#   curl -fsSL https://raw.githubusercontent.com/karutoil/grok-build-webui/main/scripts/install.sh | bash
#
# Non-interactive examples:
#   ./scripts/install.sh install --mode docker --port 8080 -y
#   ./scripts/install.sh install --mode native --port 9090 -y
#   ./scripts/install.sh install --mode native --from-source            # build with local go
#   ./scripts/install.sh install --repo OWNER/NAME                      # override GitHub repo
#   ./scripts/install.sh remove          # asks before destroying anything
#
set -euo pipefail

# ============================================================================
# constants & pretty output
# ============================================================================

APP_NAME="grok-webui"
SERVICE="$APP_NAME"
UNIT="/etc/systemd/system/${SERVICE}.service"
COMPOSE_FILE_DEFAULT="docker-compose.yml"

# This project on GitHub — used for release downloads and raw one-liners.
# (--repo flag > GROK_WEBUI_REPO env > git remote origin > this default)
REPO_DEFAULT="karutoil/grok-build-webui"
RAW_BASE="https://raw.githubusercontent.com/${REPO_DEFAULT}/main"

# Official Grok Build CLI bootstrap script (embedded in the grok binary's
# own help text). Used to provision grok when it is missing.
GROK_INSTALL_URL="https://x.ai/cli/install.sh"
#
# Run directly from a clone, or with zero setup:
#   curl -fsSL ${RAW_BASE}/scripts/install.sh | bash
# Piped runs have no repo context, so we work out of ~/grok-build-webui
# and fetch a source snapshot only when actually needed.

# NOTE: under `set -u`, BASH_SOURCE[0] is unset when piped (`curl ... | bash`),
# so both references below need an explicit ${...:-} default.
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]:-$0}")" >/dev/null 2>&1 && pwd)" || SCRIPT_DIR=""
if [[ -n "$SCRIPT_DIR" && -f "$SCRIPT_DIR/../go.mod" ]]; then
	APP_DIR="$(cd -- "$SCRIPT_DIR/.." >/dev/null 2>&1 && pwd)"
	STANDALONE=0
else
	APP_DIR="${GROK_WEBUI_APP_DIR:-$HOME/grok-build-webui}"
	STANDALONE=1
	mkdir -p "$APP_DIR"
fi

# Detect a piped run (`curl -fsSL <url> | bash`) once, here at top level:
# BASH_SOURCE[0] is genuinely unset only at top level for a stdin script.
# (Inside functions bash fills it with the function name — verified — so
# checks deferred to main() would never detect the pipe.)
# For such runs stdin is the download pipe rather than the keyboard, so we
# also open a dedicated handle (prompt_read → GROK_UI_FD) to the real
# terminal. We deliberately do NOT rebind stdin itself: the parser may still
# need unread bytes from the pipe. Deliberate redirections like
# `./install.sh < answers-file` have a real BASH_SOURCE[0] and are unaffected.
PIPED_RUN=0
GROK_UI_FD=""
if [[ -z "${BASH_SOURCE[0]:-}" ]]; then
	PIPED_RUN=1
	if { exec {GROK_UI_FD}</dev/tty; } 2>/dev/null; then
		:   # terminal handle ready — prompt_read will use it
	else
		GROK_UI_FD=""   # no controlling terminal (CI etc.)
	fi
fi

if [[ -t 1 ]]; then
	C_BOLD=$'\033[1m'; C_DIM=$'\033[2m'; C_RED=$'\033[31m'
	C_GRN=$'\033[32m'; C_YLW=$'\033[33m'; C_BLU=$'\033[34m'; C_RST=$'\033[0m'
else
	C_BOLD=""; C_DIM=""; C_RED=""; C_GRN=""; C_YLW=""; C_BLU=""; C_RST=""
fi

log()  { printf '%s\n' "  ${C_BLU}[i]${C_RST} $*"; }
ok()   { printf '%s\n' "  ${C_GRN}[✓]${C_RST} $*"; }
warn() { printf '%s\n' "  ${C_YLW}[!]${C_RST} $*"; }
err()  { printf '%s\n' "${C_BOLD}${C_RED}[✗]${C_RST} $*" >&2; }
die()  { err "$*"; exit 1; }

# ============================================================================
# configuration / state  (persisted in <app>/.env — shared with docker compose)
# ============================================================================

MODE=""; PORT=""; PUBLIC_URL=""; DATA_DIR=""
CONTAINER_DATA="/app/data"; GROK_BIN=""; SVC_USER=""; SVC_GROUP=""; TZ_VAL=""
BINARY_SOURCE=""; INSTALLED_VERSION=""; REPO="${GROK_WEBUI_REPO:-$REPO_DEFAULT}"
FROM_SOURCE="" FLAG_FORCE=0

STATE_ENV="$APP_DIR/.env"
CUSTOM_MARKER="# ---------------- your custom settings below ----------------"

# Read persisted settings. Parsed manually (no sourcing) because compose wants
# `UID=` in the file and bash keeps UID readonly.
load_state() {
	[[ -f "$STATE_ENV" ]] || return 0
	local line key val stripped
	while IFS= read -r line; do
		case "$line" in '#'*|"") continue ;; esac
		key="${line%%=*}"; val="${line#*=}"
		val="${val%%#*}"          # strip inline comments (they must not leak into values)
		val="$(rtrim "$val")"
		stripped="${val%\"}"; stripped="${stripped#\"}"
		stripped="${stripped%\'}"; stripped="${stripped#\'}"
		case "$key" in
			MODE)                       MODE="${stripped:-$MODE}" ;;
			WEBUI_PORT|PORT)            PORT="${stripped:-$PORT}" ;;
			PUBLIC_URL)                 PUBLIC_URL="${stripped}" ;;
			DATA_DIR_HOST)              DATA_DIR="${stripped:-$DATA_DIR}" ;;
			DATA_DIR)                   CONTAINER_DATA="${stripped:-$CONTAINER_DATA}" ;;
			GROK_BIN)                   GROK_BIN="${stripped:-$GROK_BIN}" ;;
			SVC_USER)                   SVC_USER="${stripped:-$SVC_USER}" ;;
			SVC_GROUP)                  SVC_GROUP="${stripped:-$SVC_GROUP}" ;;
			TZ)                         TZ_VAL="${stripped:-$TZ_VAL}" ;;
			BINARY_SOURCE)              BINARY_SOURCE="$stripped" ;;
			INSTALLED_VERSION)          INSTALLED_VERSION="$stripped" ;;
			REPO)                       if [[ -z "$REPO" && -n "$stripped" ]]; then REPO="$stripped"; fi ;;
			HOME_DIR)                   : ;; # derived below, never trusted from file
		esac
	done < "$STATE_ENV"
}

# Append the caller's custom block back onto a freshly written .env.
custom_tail() {
	if [[ -f "$STATE_ENV" ]]; then
		sed -n "/^${CUSTOM_MARKER}$/,\$p" "$STATE_ENV" | tail -n +2 || true
	fi
}

save_state() {
	# SVC_USER may be unset on follow-up runs (config/update) — heal from env
	local user="${SVC_USER:-${SUDO_USER:-$(id -un)}}"
	SVC_USER="$user"
	SVC_GROUP="${SVC_GROUP:-$(id -gn "$user")}"
	local uid gid home_dir
	uid="$(id -u "$user")"
	gid="$(id -g "$user")"
	home_dir="$(get_home "$user")"
	mkdir -p "$DATA_DIR"

	cat > "$STATE_ENV" <<EOF
############ grok-build-webui — deployment settings ############
# Managed by scripts/install.sh. Tweak and re-run "config" or reinstall.
# This file is ALSO consumed by \`docker compose\`.

# --- deployment ---
# MODE: native | docker
MODE=$MODE
WEBUI_PORT=$PORT
PUBLIC_URL=$PUBLIC_URL

# --- paths ---
# DATA_DIR_HOST lives on the host; DATA_DIR is the same dir inside the container.
DATA_DIR_HOST=$DATA_DIR
DATA_DIR=$CONTAINER_DATA

# --- grok cli ---
GROK_BIN=$GROK_BIN
MAX_SESSIONS=16

# --- binary supply (how native-mode got its binary; update reuses it) ---
BINARY_SOURCE=${BINARY_SOURCE:-source}
INSTALLED_VERSION=$INSTALLED_VERSION

# --- release download source ---
REPO=$REPO

# --- runtime identity (service user; reused by config/update runs) ---
SVC_USER=$SVC_USER
SVC_GROUP=$SVC_GROUP

# --- container identity (consumed by docker compose) ---
UID=$uid
GID=$gid
HOME_DIR=$home_dir
TZ=$TZ_VAL

$CUSTOM_MARKER
EOF
	custom_tail >> "$STATE_ENV"
	ok "settings written to ${STATE_ENV#$APP_DIR/}"
}

# ============================================================================
# small utilities
# ============================================================================

have()   { command -v "$1" >/dev/null 2>&1; }
is_root() { [[ $(id -u) -eq 0 ]]; }

get_home() {
	local u="${1:-}"
	if [[ -z "$u" ]]; then echo ""; return; fi
	getent passwd "$u" | cut -d: -f6
}

SUDO=""
as_root() {
	# lazily pick an elevation tool once
	if [[ -z "$SUDO" ]]; then
		if is_root; then SUDO="direct"
		elif have sudo; then SUDO="sudo"
		elif have doas; then SUDO="doas"
		else die "root privileges required to manage the systemd unit, and no sudo/doas found."
		fi
	fi
	if [[ "$SUDO" == "direct" ]]; then "$@"; else "$SUDO" "$@"; fi
}

# rtrim whitespace
rtrim() {
	local s="$1"
	while [[ "$s" == *[[:space:]] ]]; do s="${s%?}"; done
	printf '%s' "$s"
}

detect_tz() {
	TZ_VAL="$(cat /etc/timezone 2>/dev/null || true)"
	if [[ -z "$TZ_VAL" ]] && have timedatectl; then
		TZ_VAL="$(timedatectl show -p Timezone --value 2>/dev/null || true)"
	fi
	echo "${TZ_VAL:-UTC}"
}

probe_http() {
	# prints "200" style status code, or 000
	local url="$1"
	if have curl; then
		curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$url" 2>/dev/null || echo 000
	elif have wget; then
		wget -q -O /dev/null --timeout=2 "$url" 2>/dev/null && echo 200 || echo 000
	else
		echo 000
	fi
}

port_taken() {
	if have ss; then ss -ltnH 2>/dev/null | awk '{print $4}' | grep -Eq "[:.]${1}\$";
	elif have netstat; then netstat -ltn 2>/dev/null | grep -Eq "[:.]${1}[[:space:]]";
	else false; fi
}

service_active() { systemctl is-active --quiet "$SERVICE" 2>/dev/null; }
service_exists() { [[ -e "$UNIT" ]]; }

stop_running_service() {
	if service_exists && service_active; then
		log "stopping running ${SERVICE}..."
		as_root systemctl stop "$SERVICE"
	fi
}

# ============================================================================
# Grok Build CLI — detect, and bootstrap via the official installer if missing
# ============================================================================

detect_grok() { # sets GROK_PATH + GROK_VERSION globals ("" when absent)
	local cand
	for cand in "${HOME:-}/.grok/bin/grok" "$(command -v grok 2>/dev/null || true)"; do
		if [[ -n "$cand" && -x "$cand" ]]; then
			GROK_PATH="$cand"
			GROK_VERSION="$("$cand" --version 2>/dev/null | head -1 || true)"
			return 0
		fi
	done
	GROK_PATH=""; GROK_VERSION=""
	return 1
}

ensure_grok_cli() {
	detect_grok && { ok "grok cli found: ${GROK_VERSION:-<unknown version>} at $GROK_PATH"; return 0; }

	warn "the Grok Build CLI ('grok') is not installed — PTY sessions need it."
	local proceed=1
	if [[ "$FLAG_YES" == "1" ]]; then
		log "auto-installing via official script (--yes mode)."
	else
		ui_yesno "Install Grok Build CLI?" \
"'grok' is required to run sessions. Install it now via the official
bootstrap script?

    curl -fsSL ${GROK_INSTALL_URL} | bash" || proceed=0
	fi

	if [[ "$proceed" != "1" ]]; then
		warn "skipping grok install — sessions will fail until you install it manually."
		warn "later: curl -fsSL ${GROK_INSTALL_URL} | bash   then re-run 'config'."
		return 1
	fi

	log "running official grok installer..."
	have curl || die "curl is required to fetch the grok installer but was not found."
	curl -fsSL "$GROK_INSTALL_URL" | bash || die "grok installer failed — install manually: see https://x.ai/cli"

	# PATH may not include ~/.local/bin in this shell yet; detect explicitly
	detect_grok || die "installer ran but 'grok' still not found at ~/.grok/bin/grok or on PATH.
     Open a new shell and re-run this installer, or set GROK_BIN manually via 'config'."
	ok "installed grok: ${GROK_VERSION:-ok} at $GROK_PATH"
	return 0
}

# ============================================================================
# TUI layer — whiptail when available, numbered plain-text menus otherwise
# ============================================================================

WT=""
if have whiptail; then WT="whiptail"; elif have dialog; then WT="dialog"; fi

# Every interactive read goes through prompt_read: under a piped run,
# main() sets GROK_UI_FD to an fd on /dev/tty so prompts talk to the real
# terminal even though stdin is the download pipe.
prompt_read() {
	if [[ -n "${GROK_UI_FD:-}" ]]; then
		read "$@" <&"$GROK_UI_FD"
	else
		read "$@"
	fi
}
W=76

ui_title() { printf '\n%s=== %s ===%s\n' "$C_BOLD" "$1" "$C_RST" >&2; }

ui_msg() {
	local title="$1" text="$2"
	if [[ -n "$WT" ]]; then
		"$WT" --title "$title" --msgbox "$text" 18 "$W" >&2 || true
	else
		ui_title "$title"; printf '%b\n' "$text"
		printf '%s' "${C_DIM}press enter to continue...${C_RST}" >&2; prompt_read -r _ || true
		printf '\n'
	fi
}

ui_yesno() {
	local title="$1" text="$2"
	if [[ -n "$WT" ]]; then
		"$WT" --title "$title" --yesno "$text" 12 "$W" >&2
	else
		while true; do
			printf '%s [%syes%s/%sno%s]: ' "$text" "$C_GRN" "$C_RST" "$C_RED" "$C_RST" >&2
			prompt_read -r answer || { printf '\n' >&2; return 1; }   # EOF → treat as "no"
			# shellcheck disable=SC2154  # assigned via prompt_read's $@ passthrough
			case "$answer" in y|Y|yes|Yes|YES) return 0;; n|N|no|No|NO|"") return 1;; esac
			printf 'please answer yes or no.\n' >&2
		done
	fi
}

ui_input() {
	local title="$1" text="$2" def="${3:-}" out
	if [[ -n "$WT" ]]; then
		out="$("$WT" --title "$title" --inputbox "$text" 12 "$W" "$def" 3>&1 1>&2 2>&3)" || return 1
	else
		ui_title "$title"
		printf '%s %s[%s]%s: ' "$text" "$C_DIM" "$def" "$C_RST" >&2
		prompt_read -r out || out=""   # EOF → fall back to the default
		out="${out:-$def}"
	fi
	printf '%s' "$out"
}

# ui_menu title text tag1 desc1 [tag2 desc2]... → echoes chosen tag
ui_menu() {
	local title="$1" text="$2"; shift 2
	local args=() tag desc i=1
	if [[ -n "$WT" ]]; then
		while (($#)); do
			tag="$1"; desc="$2"; shift 2
			args+=( "$tag" "$desc" )
		done
		"$WT" --title "$title" --menu "$text" 20 "$W" "${#args[@]}" \
			"${args[@]}" 3>&1 1>&2 2>&3
	else
		ui_title "$title"; printf '%s\n' "$text"
		local tags=()
		while (($#)); do
			tag="$1"; desc="$2"; shift 2
			tags+=("$tag")
			printf '  %s%2d)%s %-10s %s\n' "$C_BOLD" "$i" "$C_RST" "$tag" "$desc"
			i=$((i+1))
		done
		while true; do
			printf 'choice [#]: ' >&2; prompt_read -r sel || { printf '\n' >&2; return 1; }   # EOF → cancel menu
			# shellcheck disable=SC2154  # assigned via prompt_read's $@ passthrough
			[[ "$sel" =~ ^[0-9]+$ ]] && (( sel >= 1 && sel <= ${#tags[@]} )) || continue
			printf '%s\n' "${tags[$((sel-1))]}"
			return 0
		done
	fi
}

# ============================================================================
# dependency checks
# ============================================================================

require_systemd() {
	[[ -d /run/systemd/system ]] || die "systemd not detected (/run/systemd/system missing). This installer manages a systemd service."
}

deps_native_ok() {
	have go || { warn "native mode needs the Go toolchain (>= 1.25) — 'go' not found."; return 1; }
	return 0
}

deps_docker_ok() {
	have docker || { warn "'docker' not found — install Docker first (https://docs.docker.com/engine/install/)."; return 1; }
	docker compose version >/dev/null 2>&1 || { warn "'docker compose' plugin missing (need Docker Compose v2)."; return 1; }
	return 0
}

build_binary() {
	log "building ${APP_NAME} with go ..."
	( cd "$APP_DIR" && CGO_ENABLED=0 go build -o "${APP_NAME}.new" ./cmd/server )
	mv -f "$APP_DIR/${APP_NAME}.new" "$APP_DIR/$APP_NAME"   # atomic swap; safe if service running (ETXTBSY)
	BINARY_SOURCE="source"
	INSTALLED_VERSION="$(git -C "$APP_DIR" describe --tags --always 2>/dev/null || echo dev)"
	ok "built $APP_DIR/$APP_NAME (${INSTALLED_VERSION})"
}

ensure_docker_image() { # pull CI image; build locally only if that fails
	( cd "$APP_DIR" || exit 1
	  if docker compose --env-file "$STATE_ENV" -f "$COMPOSE_FILE_DEFAULT" pull >/dev/null 2>&1; then
		  echo "  ok pulled latest image from ghcr.io"
	  else
		  echo "  [i] registry unavailable or offline — building image locally..."
		  docker compose --env-file "$STATE_ENV" -f "$COMPOSE_FILE_DEFAULT" build
	  fi
	  docker compose --env-file "$STATE_ENV" -f "$COMPOSE_FILE_DEFAULT" up -d )
}

# ============================================================================
# binary supply — prebuilt release download (default) or local go build
# ============================================================================

is_repo()        { git -C "$APP_DIR" rev-parse --is-inside-work-tree >/dev/null 2>&1; }

bootstrap_app_dir() { # download a source snapshot into APP_DIR (no clone needed)
	detect_repo
	[[ -n "$REPO" ]] || die "don't know which GitHub repo to fetch sources from."
	log "fetching project snapshot from github.com/${REPO} (main)..."
	local tmp
	tmp="$(mktemp -d)"
	if ! gh_fetch "https://github.com/${REPO}/archive/refs/heads/main.tar.gz" > "$tmp/src.tar.gz" 2>/dev/null; then
		rm -rf "$tmp"; die "could not download source snapshot — check network/repo name."
	fi
	if ! tar -xzf "$tmp/src.tar.gz" -C "$APP_DIR" --strip-components=1 2>/dev/null; then
		rm -rf "$tmp"; die "failed to unpack source snapshot into $APP_DIR."
	fi
	rm -rf "$tmp"
	ok "project files in place at $APP_DIR (settings & data untouched)"
}

ensure_sources_present() { # standalone installs: docker mode / --from-source need real sources
	if [[ -f "$APP_DIR/go.mod" ]]; then return 0; fi
	bootstrap_app_dir
}

detect_repo() {
	[[ -n "$REPO" ]] && return 0
	if is_repo; then
		local url
		url="$(git -C "$APP_DIR" remote get-url origin 2>/dev/null || true)"
		case "$url" in
			git@github.com:*) REPO="${url#git@github.com:}" ;;
			*github.com*)     REPO="$(printf '%s' "$url" | sed -E 's#.*github\.com[:/]##; s/\.git$//')" ;;
		esac
		REPO="${REPO%/}"
	fi
}

arch_tag() {
	case "$(uname -m)" in
		x86_64)        echo amd64 ;;
		aarch64|arm64) echo arm64 ;;
		*)             echo "$(uname -m)" ;;
	esac
}

gh_fetch() { # GET a URL, honoring an optional token (avoids API rate limits)
	local url="$1"
	local auth=()
	if [[ -n "${GITHUB_TOKEN:-}" ]]; then      auth=(-H "Authorization: Bearer ${GITHUB_TOKEN}")
	elif [[ -n "${GROK_WEBUI_TOKEN:-}" ]]; then auth=(-H "Authorization: Bearer ${GROK_WEBUI_TOKEN}")
	fi
	curl -fsSL ${auth[@]+"${auth[@]}"} "$url"
}

latest_release_version() {
	detect_repo
	[[ -n "$REPO" ]] || return 1
	gh_fetch "https://api.github.com/repos/${REPO}/releases/latest" 2>/dev/null |
		sed -n 's/.*"tag_name"[[:space:]]*:[[:space:]]*"\([^"]*\)".*/\1/p' | head -1
}

release_tarball_url() { # <version> → asset URL for this machine's arch
	local ver="$1" arch
	arch="$(arch_tag)"
	local json
	json="$(gh_fetch "https://api.github.com/repos/${REPO}/releases/tags/${ver}")" || return 1
	printf '%s\n' "$json" |
		grep -o '"browser_download_url": *"[^"]*"' | cut -d '"' -f4 |
		grep "/${APP_NAME}-linux-${arch}\.tar\.gz\$" | head -1
}

install_release_binary() { # <version-tag>
	local ver="$1"
	detect_repo
	if [[ -z "$REPO" ]]; then
		die "don't know which GitHub repo to download from. Provide one via:
     --repo OWNER/NAME          (this run, saved for future runs)
     export GROK_WEBUI_REPO=OWNER/NAME
     git remote add origin git@github.com:OWNER/NAME.git"
	fi
	local arch tarball url tmp
	arch="$(arch_tag)"
	tarball="${APP_NAME}-linux-${arch}.tar.gz"
	log "fetching ${REPO} release ${ver} (${arch})..."
	if ! url="$(release_tarball_url "$ver")" || [[ -z "$url" ]]; then
		die "asset '${tarball}' not found in release ${ver} of ${REPO}.
     (Has CI finished for that tag? Does it match your architecture?)"
	fi
	tmp="$(mktemp -d)"
	if ! gh_fetch "$url" > "$tmp/$tarball" 2>/dev/null; then
		rm -rf "$tmp"; die "download failed: $url"
	fi
	# verify against checksums.txt when the release publishes one
	if have sha256sum && gh_fetch "${url%/*}/checksums.txt" > "$tmp/checksums.txt" 2>/dev/null; then
		local want got
		want="$(grep -E "[[:space:]][^[:space:]/]*${tarball}\$" "$tmp/checksums.txt" | awk '{print $1}' | head -1)"
		got="$(sha256sum "$tmp/$tarball" | awk '{print $1}')"
		if [[ -z "$want" ]]; then
			warn "no entry for ${tarball} in checksums.txt — skipped verification."
		elif [[ "$want" != "$got" ]]; then
			rm -rf "$tmp"; die "SHA256 mismatch for ${tarball} — aborting (got ${got}, want ${want})."
		else
			ok "checksum verified"
		fi
	fi
	if ! tar -xzf "$tmp/$tarball" -C "$tmp" 2>/dev/null || [[ ! -x "$tmp/$APP_NAME" ]]; then
		rm -rf "$tmp"; die "archive did not contain an executable named ${APP_NAME}."
	fi
	mv -f "$tmp/$APP_NAME" "$APP_DIR/$APP_NAME.new"
	rm -rf "$tmp"
	chmod +x "$APP_DIR/$APP_NAME.new"
	mv -f "$APP_DIR/$APP_NAME.new" "$APP_DIR/$APP_NAME"     # atomic swap
	BINARY_SOURCE="release"
	INSTALLED_VERSION="$ver"
	ok "installed prebuilt binary ${ver} (no Go needed)"
}

# Decide how native mode obtains its binary: existing-local binary wins as
# fastest path; else local build when we're in a git clone with Go; else
# download the newest CI release.
ensure_runtime_binary() {
	if [[ -n "$FROM_SOURCE" ]]; then
		deps_native_ok || die "--from-source selected but the Go toolchain (>= 1.25) is missing."
		build_binary
		BINARY_SOURCE="source"
		INSTALLED_VERSION="$(git -C "$APP_DIR" describe --tags --always 2>/dev/null || echo dev)"
		return 0
	fi
	if [[ -x "$APP_DIR/$APP_NAME" ]]; then
		warn "reusing existing binary at $APP_DIR/$APP_NAME — run with --from-source or reinstall to replace it."
		return 0
	fi
	if is_repo && have go; then
		log "git clone + Go detected — building locally (pass --from-source to force)."
		build_binary
		BINARY_SOURCE="source"
		INSTALLED_VERSION="$(git -C "$APP_DIR" describe --tags --always 2>/dev/null || echo dev)"
		return 0
	fi
	local latest
	if ! latest="$(latest_release_version)"; then
		die "no Go toolchain and no way to fetch a binary.
     Options: --from-source (needs go), or --repo OWNER/NAME / GROK_WEBUI_REPO to use CI releases."
	fi
	[[ -n "$latest" ]] || die "GitHub repo ${REPO:-<unset>} has no releases yet — push a v* tag or use --from-source."
	install_release_binary "$latest"
}

# ============================================================================
# interactive setting collection
# ============================================================================

collect_common_settings() {
	# --- mode ---
	if [[ "$FLAG_YES" != "1" && -z "$FORCE_MODE" && -z "$MODE" ]]; then
		MODE="$(ui_menu "Install mode" "How should ${APP_NAME} run as a service?" \
			"native"  "RECOMMENDED — plain systemd unit + static binary; simplest, fewest moving parts" \
			"docker"  "containerized via compose; host home mounted same-path; needs Docker + Compose v2")" || die "cancelled."
	fi
	[[ -n "$FORCE_MODE" ]] && MODE="$FORCE_MODE"

	# --- non-interactive fast path (--yes): accept current/derived defaults ---
	if [[ "$FLAG_YES" == "1" ]]; then
		[[ -z "$MODE" ]] && MODE="native"
	fi

	if [[ "$MODE" != "docker" && "$MODE" != "native" ]]; then
		die "invalid mode '$MODE' (expected docker|native)."
	fi
	if [[ "$MODE" == "docker" ]] && ! deps_docker_ok; then die "docker mode selected but Docker requirements missing."
	fi
	if [[ "$MODE" == "native" ]] && ! deps_native_ok; then die "native mode selected but the Go toolchain is missing."
	fi

	# --- service user (file ownership + native-mode runtime identity) ---
	if [[ -z "$SVC_USER" ]]; then
		SVC_USER="${SUDO_USER:-$USER}"
	fi
	id "$SVC_USER" >/dev/null 2>&1 || die "user '$SVC_USER' does not exist."

	# --- port ---
	local def_port="${PORT:-8080}"
	if [[ "$FLAG_YES" == "1" ]]; then
		PORT="${PORT:-8080}"
	else
		while true; do
			PORT="$(ui_input "Port" "TCP port to listen on:" "$def_port")" || die "cancelled."
			PORT="${PORT:-$def_port}"   # Enter accepts the default
			if [[ "$PORT" =~ ^[0-9]+$ ]] && (( PORT >= 1 && PORT <= 65535 )); then break; fi
			ui_msg "Invalid port" "Please enter a number between 1 and 65535."
		done
	fi
	if port_taken "$PORT"; then
		warn "port ${PORT} appears busy — that may be a previous ${SERVICE} instance (it will be replaced)."
	fi

	# --- public URL ---
	PUBLIC_URL="${PUBLIC_URL:-}"
	if [[ "$FLAG_YES" != "1" ]]; then
		while true; do
			PUBLIC_URL="$(ui_input "Public URL (optional)" \
"Full base URL WITH scheme — e.g. https://grok.example.com or
https://myapp.trycloudflare.com. A bare hostname (grok.example.com)
is NOT valid here. Used behind a tunnel for CORS, Secure cookies and
WebAuthn RPID. Leave empty (just Enter) for LAN-only access:" "${PUBLIC_URL:-}")" || die "cancelled."
			PUBLIC_URL="${PUBLIC_URL%/}"
			[[ -z "$PUBLIC_URL" || "$PUBLIC_URL" =~ ^https?://[^[:space:]]+$ ]] && break
			ui_msg "Invalid public URL" "Must start with http:// or https:// (or be left empty)."
		done
	else
		PUBLIC_URL="${PUBLIC_URL%/}"
		if [[ -n "$PUBLIC_URL" && ! "$PUBLIC_URL" =~ ^https?://[^[:space:]]+$ ]]; then
			die "--public-url must start with http:// or https://"
		fi
	fi

	# --- data dir ---
	local def_data="${DATA_DIR:-$APP_DIR/data}"
	if [[ "$FLAG_YES" == "1" ]]; then
		DATA_DIR="$def_data"
	else
		DATA_DIR="$(ui_input "Data directory" "Where should the SQLite database live?" "$def_data")" || die "cancelled."
		DATA_DIR="${DATA_DIR:-$def_data}"   # Enter accepts the default
	fi
	DATA_DIR="${DATA_DIR%/}"

	# --- grok cli (required by sessions in every mode — auto-installed if missing) ---
	ensure_grok_cli || true   # declining is allowed; sessions will fail until installed
	local def_grok="${GROK_BIN:-${GROK_PATH:-grok}}"
	if [[ "$FLAG_YES" == "1" ]]; then
		GROK_BIN="$def_grok"
	else
		GROK_BIN="$(ui_input "Grok CLI binary" \
"Executable that PTY sessions will spawn inside project dirs.
Detected: ${def_grok}" "$def_grok")" || die "cancelled."
		GROK_BIN="${GROK_BIN:-$def_grok}"   # Enter accepts the default
	fi
	if [[ ! -x "${GROK_BIN/#\~/$HOME}" && ! -x "$GROK_BIN" && "$GROK_BIN" != /* ]]; then
		warn "'$GROK_BIN' not found right now — sessions will fail until it exists (fine inside containers relying on \$PATH)."
	fi

	TZ_VAL="${TZ_VAL:-$(detect_tz)}"
	SVC_GROUP="$(id -gn "$SVC_USER")"
}

# ============================================================================
# systemd unit generation
# ============================================================================

render_template() { # render_template <template-string>
	# escape values for use as sed replacements (& and \ are special)
	local esc_service="${SERVICE//\\/\\\\}";  esc_service="${esc_service//&/\\&}"
	local esc_appdir="${APP_DIR//\\/\\\\}";   esc_appdir="${esc_appdir//&/\\&}"
	local esc_bin="${APP_DIR//\\/\\\\}/${APP_NAME//\\/\\\\}"; esc_bin="${esc_bin//&/\\&}"
	local esc_port="$PORT"
	local esc_data="${DATA_DIR//\\/\\\\}";    esc_data="${esc_data//&/\\&}"
	local esc_url="${PUBLIC_URL//\\/\\\\}";   esc_url="${esc_url//&/\\&}"
	local esc_user="${SVC_USER//\\/\\\\}";    esc_user="${esc_user//&/\\&}"
	local esc_grok="${GROK_BIN//\\/\\\\}";    esc_grok="${esc_grok//&/\\&}"
	sed -e "s|@SERVICE@|$esc_service|g" \
		-e "s|@APP_DIR@|$esc_appdir|g" \
		-e "s|@BIN@|$esc_bin|g" \
		-e "s|@PORT@|$esc_port|g" \
		-e "s|@DATA_DIR@|$esc_data|g" \
		-e "s|@PUBLIC_URL@|$esc_url|g" \
		-e "s|@SVC_USER@|$esc_user|g" \
		-e "s|@GROK_BIN@|$esc_grok|g" \
		-e "s|@DOCKER@|$(command -v docker 2>/dev/null || echo docker)|g" <<< "$1"
}

gen_native_unit() {
	cat <<'UNIT'
[Unit]
Description=Grok Build WebUI (@SERVICE@)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=@SVC_USER@
WorkingDirectory=@APP_DIR@
ExecStart=@BIN@ --addr :@PORT@ --data @DATA_DIR@ --grok-bin "@GROK_BIN@"
Environment=GROK_WEBUI_PUBLIC_URL=@PUBLIC_URL@
Restart=on-failure
RestartSec=3
TimeoutStopSec=20
KillSignal=SIGTERM
LimitNOFILE=65536
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
UNIT
}

gen_docker_wrapper_unit() {
	cat <<'UNIT'
[Unit]
Description=Grok Build WebUI (@SERVICE@, docker compose)
Requires=docker.service
After=docker.service network-online.target
Wants=network-online.target

[Service]
Type=oneshot
RemainAfterExit=yes
WorkingDirectory=@APP_DIR@
# best-effort image refresh ("-": ignore failures when offline); local
# build fallback still exists in the compose file for first installs
ExecStartPre=-@DOCKER@ compose --env-file @APP_DIR@/.env -f @APP_DIR@/docker-compose.yml pull --quiet --ignore-buildable
ExecStart=@DOCKER@ compose --env-file @APP_DIR@/.env -f @APP_DIR@/docker-compose.yml up -d --wait
ExecStop=@DOCKER@ compose --env-file @APP_DIR@/.env -f @APP_DIR@/docker-compose.yml stop
ExecReload=-@DOCKER@ compose --env-file @APP_DIR@/.env -f @APP_DIR@/docker-compose.yml pull --quiet --ignore-buildable
ExecReload=@DOCKER@ compose --env-file @APP_DIR@/.env -f @APP_DIR@/docker-compose.yml up -d --wait
TimeoutStartSec=10min

[Install]
WantedBy=multi-user.target
UNIT
}

install_unit() { # install_unit <rendered-unit-text>
	local rendered="$1" tmp
	tmp="$(mktemp)"
	printf '%s\n' "$rendered" > "$tmp"
	as_root install -m 644 "$tmp" "$UNIT"
	rm -f "$tmp"
	as_root systemctl daemon-reload
	ok "installed $UNIT"
}

# ============================================================================
# actions
# ============================================================================

FORCE_MODE="" FLAG_YES=0 FLAG_PURGE=0

banner() {
	printf '\n%s╔══════════════════════════════════════════════╗%s\n' "$C_BLU" "$C_RST"
	printf '%s║%s   grok-build-webui :: service installer     %s║%s\n' "$C_BLU" "$C_RST" "" "$C_RST"
	printf '%s╚══════════════════════════════════════════════╝%s\n\n' "$C_BLU" "$C_RST"
}

summary_print() {
	local url_hint="http://localhost:${PORT}"
	[[ -n "$PUBLIC_URL" ]] && url_hint="$PUBLIC_URL (http://localhost:${PORT} locally)"
	cat <<EOF

  ${C_BOLD}${C_GRN}${APP_NAME} is installed and running.${C_RST}
  URL:     $url_hint
  Mode:    $MODE
  Data:    $DATA_DIR

  Manage the service:
    systemctl status  $SERVICE
    sudo systemctl restart $SERVICE     # after updates
    journalctl -u $SERVICE -f          # live logs

  First visit shows "Initial Setup" — create the admin user there.
  Open tabs survive refreshes/logout; they end when the service stops.

EOF
}

confirm_plan() {
	[[ "$FLAG_YES" == "1" ]] && return 0
	local method url_disp
	url_disp="${PUBLIC_URL:-(none — LAN only)}"
	case "$MODE" in
		docker)
			method="docker compose image build" ;;
		native)
			if [[ -n "$FROM_SOURCE" ]]; then
				method="go build (source, as requested)"
			elif [[ -x "$APP_DIR/$APP_NAME" ]]; then
				method="reuse existing binary"
			elif is_repo && have go; then
				method="go build (local clone + toolchain detected)"
			else
				detect_repo
				method="download CI release from ${REPO:-<REPO UNSET — will fail>}"
			fi ;;
	esac
	ui_yesno "Confirm plan" "
  mode        : $MODE
  listen      : http://0.0.0.0:$PORT
  public URL  : $url_disp
  data dir    : $DATA_DIR
  grok binary : $GROK_BIN
  binary via  : $method
  run as      : $SVC_USER ($(id -u "$SVC_USER"):$(id -g "$SVC_USER"))
  unit        : $UNIT

Proceed with install?" || die "cancelled."
}

do_install() {
	require_systemd
	# snapshot CLI-supplied flags, then adopt previous-install values as the
	# prompt defaults — flags still win over what's stored in .env
	local f_port="${PORT:-}" f_url="${PUBLIC_URL:-}" f_data="${DATA_DIR:-}"
	load_state
	[[ -n "$f_port" ]] && PORT="$f_port"
	[[ -n "$f_url" ]] && PUBLIC_URL="$f_url"
	[[ -n "$f_data" ]] && DATA_DIR="$f_data"

	stop_running_service          # idempotent; replaces older instances

	ui_msg "Install ${APP_NAME}" \
"This wizard will:\n\
 • build the app (${C_DIM}docker compose image${C_RST} or ${C_DIM}go build${C_RST})\n\
 • write /etc/systemd/system/${SERVICE}.service\n\
 • enable + start it (restarts on boot/crash)\n\
 • persist settings in .env next to the sources"

	collect_common_settings
	confirm_plan

	if [[ "$MODE" == "docker" || -n "$FROM_SOURCE" ]]; then
		ensure_sources_present
	fi

	if [[ "$MODE" == "docker" ]]; then
		log "acquiring container image (${MODE} mode)..."
		ensure_docker_image
	else
		log "acquiring binary (${MODE} mode)..."
		ensure_runtime_binary
	fi

	# persist AFTER acquisition so BINARY_SOURCE/INSTALLED_VERSION are real
	save_state

	if [[ "$MODE" == "docker" ]]; then
		install_unit "$(render_template "$(gen_docker_wrapper_unit)")"
	else
		install_unit "$(render_template "$(gen_native_unit)")"
	fi

	as_root systemctl enable --now "$SERVICE"

	wait_healthy
	summary_print
}

wait_healthy() {
	local url="http://127.0.0.1:${PORT}/api/auth/setup-required" code="" i
	log "waiting for ${url} ..."
	for i in $(seq 1 30); do
		code="$(probe_http "$url")"
		[[ "$code" != "000" ]] && break
		sleep 1
	done
	if [[ "$code" == "000" ]]; then
		warn "server did not answer yet — check: journalctl -u $SERVICE -n 50"
	else
		ok "healthy (HTTP $code)"
	fi
}

git_pull_latest() {
	if ! is_repo; then
		warn "$APP_DIR is not a git repo — skipping pull."
		return 0
	fi
	local old_sha new_sha
	old_sha="$(git -C "$APP_DIR" rev-parse --short HEAD)"
	log "git pull --ff-only ..."
	git -C "$APP_DIR" pull --ff-only || { warn "pull failed (local changes?) — staying on current tree."; return 0; }
	new_sha="$(git -C "$APP_DIR" rev-parse --short HEAD)"
	[[ "$old_sha" == "$new_sha" ]] && log "already at latest ($new_sha)" || ok "updated $old_sha → $new_sha"
}

restart_or_start_service() {
	if service_exists && service_active; then
		log "restarting $SERVICE..."
		as_root systemctl restart "$SERVICE"
	elif service_exists; then
		as_root systemctl start "$SERVICE"
	else
		as_root systemctl enable --now "$SERVICE"
	fi
}

update_release_binary() {
	local latest
	current_version="${INSTALLED_VERSION:-unknown}"
	detect_repo
	[[ -n "$REPO" ]] || die "release updates require a known repo — rerun with: install --repo OWNER/NAME"
	if ! latest="$(latest_release_version)"; then
		die "cannot reach GitHub API to check releases (network? rate limit?). Try again later."
	fi
	if [[ -z "$latest" ]]; then
		die "no releases published under $REPO yet."
	fi
	if [[ "$latest" == "$current_version" && "$FLAG_FORCE" != "1" ]]; then
		log "already at $latest — nothing to update (--force redeploys anyway)."
		return 0
	fi
	warn "binary update: ${current_version} → ${latest}"
	# mv-based atomic swap below works even while the service is running;
	# restart afterwards so the new build actually executes.
	install_release_binary "$latest"
	save_state
	restart_or_start_service
}

do_update() {
	load_state
	require_systemd
	[[ -z "$MODE" ]] && die "not installed yet — run '${0##*/}' install first."

	case "$MODE" in
		docker)
			[[ ! -f "$APP_DIR/go.mod" ]] && { log "standalone install — refreshing project snapshot..."; bootstrap_app_dir; }
			build_and_apply ;;
		native)
			if [[ "${BINARY_SOURCE:-}" == "release" ]]; then
				update_release_binary
			else
				git_pull_latest
				build_and_apply
			fi ;;
	esac
	wait_healthy
	ok "update complete."
}

build_and_apply() {
	log "rebuilding (${MODE} mode)..."
	if [[ "$MODE" == "docker" ]]; then
		ensure_docker_image
		service_exists && as_root systemctl try-restart "$SERVICE" 2>/dev/null || true
	else
		service_active && { log "stopping service for binary swap..."; as_root systemctl stop "$SERVICE"; }
		build_binary
		if service_exists; then
			as_root systemctl start "$SERVICE"
		else
			warn "unit not installed yet — installing..."
			install_unit "$(render_template "$(gen_native_unit)")"
			as_root systemctl enable --now "$SERVICE"
		fi
	fi
}

do_remove() {
	load_state
	require_systemd
	if ! service_exists && ! have docker; then
		die "${SERVICE} is not installed."
	fi
	ui_yesno "Remove ${SERVICE}?" \
"Do you want to STOP, DISABLE and REMOVE the ${SERVICE} systemd service?
(this leaves your data and source tree intact)" || { log "aborted."; return 0; }

	if service_exists; then
		as_root systemctl disable --now "$SERVICE" >/dev/null 2>&1 || true
		as_root rm -f "$UNIT"
		as_root systemctl daemon-reload
		ok "removed $UNIT"
	fi

	if [[ "${MODE:-}" == "docker" ]] && have docker && [[ -f "$APP_DIR/$COMPOSE_FILE_DEFAULT" ]]; then
		if ui_yesno "Docker resources" "Also tear down the docker compose stack (containers + network)? Your bind-mounted data stays."; then
			( cd "$APP_DIR" && docker compose --env-file "$STATE_ENV" -f "$COMPOSE_FILE_DEFAULT" down ) || true
			if ui_yesno "Docker image" "Remove the pulled/built image as well?"; then
				docker rmi "ghcr.io/karutoil/grok-build-webui:latest" >/dev/null 2>&1 || true
				docker rmi "${APP_NAME}:local" >/dev/null 2>&1 || true
			fi
		fi
	fi

	do_purge_offer
	ok "removal finished."
}

do_purge_offer() {
	if [[ "${FLAG_PURGE}" == "1" ]]; then
		purge_data
		return 0
	fi
	ui_yesno "Delete data?" \
"ALSO delete application data?

  ${DATA_DIR:-$APP_DIR/data}

This deletes users, projects, settings and session history.
NOT recoverable." || return 0
	purge_data
}

purge_data() {
	local dir="${DATA_DIR:-$APP_DIR/data}"
	if [[ -z "$dir" || "$dir" == "/" || ! -d "$dir" ]]; then
		warn "data dir '$dir' not found or unsafe — skipping deletion."
		return 0
	fi
	printf '  %stype %spurge%s %sto confirm deleting %s : ' \
		"$C_BOLD" "$C_RED" "$C_BOLD" "$C_RST" "$dir" >&2
	local ans; prompt_read -r ans || ans=""   # EOF → no confirmation, data kept
	if [[ "$ans" == "purge" ]]; then
		rm -rf -- "$dir"
		ok "deleted $dir"
		if ui_yesno ".env" "Remove saved settings file .env too?"; then rm -f "$STATE_ENV"; ok "removed .env"; fi
	else
		warn "confirmation mismatch — data left intact."
	fi
}

do_status() {
	load_state
	printf '\n%sinstallation:%s %s\n' "$C_BOLD" "$C_RST" "${MODE:-<not installed>}"
	if [[ -f "$STATE_ENV" ]]; then
		printf 'settings:     %s\n' "$STATE_ENV"
		printf 'port:         %s\n' "${PORT:-?}"
		printf 'data:         %s\n' "${DATA_DIR:-?}"
		printf 'binary:       %s (%s)\n' "$APP_DIR/$APP_NAME" "${BINARY_SOURCE:-unknown}${INSTALLED_VERSION:+, $INSTALLED_VERSION}"
		printf 'release repo: %s\n' "${REPO:-<unset>}"
	fi
	if [[ "${STANDALONE:-0}" == "1" ]]; then
		printf 'standalone:   yes (no git clone) — re-run or update anytime with:\n'
		printf '              curl -fsSL %s/scripts/install.sh | bash\n' "$RAW_BASE"
	fi
	if service_exists; then
		systemctl status "$SERVICE" --no-pager -l || true
	else
		err "unit $SERVICE.service not installed"
		if [[ -d "${DATA_DIR:-$APP_DIR/data}" ]]; then
			log "existing data dir detected — run 'install' to (re)create the service."
		fi
	fi
	printf '\n'
}

do_logs() {
	load_state
	journalctl -u "$SERVICE" -f -n 100 --no-pager
}

do_config() {
	load_state
	require_systemd
	[[ -z "$MODE" ]] && die "nothing configured yet — run install first."
	log "current settings shown as defaults — change what you need (Enter keeps them)."
	collect_common_settings
	save_state
	if [[ "$MODE" == "docker" || -n "${FROM_SOURCE:-}" ]]; then
		ensure_sources_present
	fi
	if [[ "$MODE" == "docker" ]]; then
		# compose up -d recreates the container with the new env values
		build_and_apply
	else
		ensure_runtime_binary                 # fetch binary if missing; no-op otherwise
		log "regenerating systemd unit with new settings..."
		install_unit "$(render_template "$(gen_native_unit)")"
		restart_or_start_service
	fi
	save_state                              # persist any new version/source info
	wait_healthy
	ok "configuration applied."
}

do_doctor() {
	load_state
	local rows=()
	local OK="${C_GRN}OK${C_RST}" FAIL="${C_RED}FAIL${C_RST}" WARN="${C_YLW}WARN${C_RST}"
	chk() { rows+=("$(printf '  %-26s' "$1") $2  ${C_DIM}$3${C_RST}"); }

	if [[ -d /run/systemd/system ]]; then chk "systemd" "$OK" ""; else chk "systemd" "$FAIL" "this installer requires systemd"; fi
	if have go; then
		chk "go toolchain" "$OK" "$(go version 2>/dev/null | awk '{print $3}')"
	else
		chk "go toolchain" "$WARN" "needed for native mode only"
	fi
	if have docker; then
		if docker compose version >/dev/null 2>&1; then
			chk "docker + compose v2" "$OK" "$(docker --version | sed 's/,.*//')"
		else
			chk "docker + compose v2" "$FAIL" "compose plugin missing"
		fi
	else
		chk "docker engine" "$WARN" "needed for docker mode only"
	fi
	local grok_loc
	grok_loc="$(command -v grok 2>/dev/null || echo "${HOME}/.grok/bin/grok (expected)")"
	if command -v grok >/dev/null 2>&1 || [[ -x "${HOME}/.grok/bin/grok" ]]; then
		chk "grok cli" "$OK" "$grok_loc"
	else
		chk "grok cli" "$FAIL" "not found — install the Grok Build CLI first"
	fi
	detect_repo
	if [[ -n "$REPO" ]]; then
		local rel
		if rel="$(latest_release_version)"; then
			[[ -n "$rel" ]] && chk "release repo" "$OK" "$REPO latest=$rel" || chk "release repo" "$WARN" "$REPO has no releases yet"
		else
			chk "release repo" "$WARN" "$REPO unreachable from here"
		fi
	else
		chk "release repo" "$WARN" "unset — set --repo OWNER/NAME for Go-less installs"
	fi
	if [[ -n "$WT" ]]; then chk "TUI backend" "$OK" "$WT"; else chk "TUI backend" "$WARN" "plain-text fallback (install whiptail for menus)"; fi
	if have curl || have wget; then chk "curl/wget" "$OK" "health probes"; else chk "curl/wget" "$WARN" "health checks disabled"; fi

	local p="${PORT:-8080}"
	if port_taken "$p"; then chk "port $p" "$WARN" "in use (fine if reinstalling $SERVICE)"; else chk "port $p" "$OK" "free"; fi

	printf '\n%senvironment doctor%s — app dir: %s\n' "$C_BOLD" "$C_RST" "$APP_DIR"
	printf '%s\n' "${rows[@]}"
	printf '\n'
}

interactive_main_menu() {
	banner
	while true; do
		load_state   # refresh after any action
		local installed="<fresh>"
		service_exists && installed="installed ($(systemctl is-active "$SERVICE" 2>/dev/null))"
		choice="$(ui_menu "Main menu" "service: ${SERVICE} — ${installed}\napp dir: ${APP_DIR}" \
			install "guided install as a systemd service (choose docker or native)" \
			update  "pull latest code, rebuild, restart the service" \
			config  "change port / URL / grok binary, apply changes" \
			status  "show detailed service status" \
			logs    "follow service logs (ctrl-c to exit)" \
			remove  "uninstall the service (data deletion asked separately)" \
			doctor  "environment sanity check" \
			quit    "exit")" || exit 0
		case "$choice" in
			install) do_install ;;
			update)  do_update ;;
			config)  do_config ;;
			status)  clear_screen; do_status; press_any ;;
			logs)    do_logs ;;
			remove)  do_remove; load_state ;;
			doctor)  clear_screen; do_doctor; press_any ;;
			quit)    exit 0 ;;
		esac
	done
}

clear_screen() { [[ -t 1 ]] && printf '\033[2J\033[H'; }
press_any()    { printf '%s' "${C_DIM}(enter to return to menu)${C_RST}" >&2; prompt_read -r _ || true; printf '\n'; }

usage() {
	# Self-contained on purpose: under `curl ... | bash` there is no script
	# file to parse headers from ($0 is just "bash").
	cat <<'USAGE'
grok-build-webui installer / manager

Usage:
  scripts/install.sh                                      # interactive TUI
  scripts/install.sh install|update|remove|status|logs|config|doctor [options]

Zero-setup install (no clone needed):
  curl -fsSL https://raw.githubusercontent.com/karutoil/grok-build-webui/main/scripts/install.sh | bash

Piped + non-interactive ("bash -s" passes the words after it as arguments):
  curl -fsSL https://raw.githubusercontent.com/karutoil/grok-build-webui/main/scripts/install.sh | bash -s install --mode docker --port 8080 -y

Options:
  --mode docker|native   installation mode (default: native prebuilt binary)
  --port N               TCP port for the web UI (default: 8080)
  --public-url URL       base URL when behind a reverse proxy/tunnel (http(s):// required)
  --data-dir PATH        host path for persistent data
  --repo OWNER/NAME      override GitHub repo for releases and raw files
  --from-source          build the binary locally with go instead of downloading
  -y, --yes              accept defaults; never prompt

Commands:
  install   guided install as a systemd service (docker compose or native)
  update    pull latest code, rebuild, restart the service
  remove    uninstall the service (asks before deleting anything; --purge deletes data)
  config    change port / URL / grok binary and apply
  status    detailed service status
  logs      follow service logs (ctrl-c to exit)
  doctor    environment sanity check

Every prompt accepts its default with plain Enter.
USAGE
	exit "${1:-0}"
}

# ============================================================================
# argument parsing & entrypoint
# ============================================================================

main() {
	CMD=""
	while [[ $# -gt 0 ]]; do
		case "$1" in
			install|update|remove|status|logs|config|doctor) CMD="$1" ;;
			--mode)       FORCE_MODE="$2"; MODE="$2"; shift ;;
			--port)       PORT="$2"; shift ;;
			--public-url) PUBLIC_URL="$2"; shift ;;
			--data-dir)   DATA_DIR="$2"; shift ;;
			--repo)       REPO="$2"; shift ;;
			-y|--yes)     FLAG_YES=1 ;;
			--purge)      FLAG_PURGE=1 ;;
			--force)      FLAG_FORCE=1 ;;
			--from-source) FROM_SOURCE=1 ;;
			-h|--help)    usage 0 ;;
			*)            err "unknown argument: $1"; usage 2 ;;
		esac
		shift
	done

	# Interactive input plumbing for piped runs was set up at top level
	# (PIPED_RUN / GROK_UI_FD); nothing to do here.

	case "${CMD:-}" in
		"")
			if [[ "$PIPED_RUN" == 1 && -z "$GROK_UI_FD" ]]; then
				warn "no interactive terminal available (stdin is not a TTY)."
				log  "for a hands-off install, pipe arguments after 'bash -s', e.g.:"
				log  "  curl -fsSL ${RAW_BASE}/scripts/install.sh | bash -s install --mode docker --port 8080 -y"
				usage 0
			fi
			interactive_main_menu ;;
		install)  banner; do_install ;;
		update)   banner; do_update ;;
		remove)   banner; do_remove ;;
		config)   banner; do_config ;;
		status)   do_status ;;
		logs)     do_logs ;;
		doctor)   do_doctor ;;
		*)        usage 2 ;;
	esac
}

# Only run when executed directly or piped to bash; sourcing (for testing) is a no-op.
if [[ "${BASH_SOURCE[0]:-$0}" == "$0" ]]; then
	main "$@"
fi
