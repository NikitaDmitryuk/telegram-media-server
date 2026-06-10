#!/usr/bin/env bash
#
# install.sh — Interactive installer for Telegram Media Server (Arch Linux, Ubuntu/Debian)
#
# Installs binary, systemd service, and .env config. Menu options:
# — qBittorrent: install from package manager, systemd, port 8081, login/password in .env;
# — Prowlarr: Arch = AUR (yay/paru), Ubuntu = apt repo; systemd, port 9696, API key from config.xml into .env.
# Indexers in Prowlarr are added manually (web UI).
# With existing .env: by default only offers to update binary and service (config untouched).
# Answer "n" → then "Force reinstall?" [y/N]: "y" = re-enter all settings (backup .env to .env.bak.force), "n" = prompt only missing.
# Run: sudo ./scripts/install.sh  or  sudo make install
# Unattended: BOT_TOKEN=... sudo ./scripts/install.sh --yes
#
set -euo pipefail

# --- constants (match Makefile); SERVICE_DIR set in main() after detect_distro ---
BINARY_NAME=telegram-media-server
INSTALL_DIR=/usr/local/bin
CONFIG_DIR=/etc/telegram-media-server
LOCALES_SRC=locales
LOCALES_DEST=/usr/local/share/telegram-media-server/locales
QBIT_SERVICE_NAME=qbittorrent-nox
QBIT_WEBUI_PORT=8081
PROWLARR_SERVICE_NAME=prowlarr
PROWLARR_PORT=9696
PROWLARR_DATA_PATHS="/var/lib/prowlarr /etc/prowlarr"
MINIDLNA_SERVICE_NAME=minidlna
MINIDLNA_CONF=/etc/minidlna.conf
TMS_USER=tms
TMS_HOME=/var/lib/telegram-media-server

# --- installer mode flags (set by parse_args) ---
ASSUME_YES=0
FORCE_REINSTALL=0
UPDATE_ONLY=0
CREATE_TMS_USER=1
WITH_QBITTORRENT=""
WITH_PROWLARR=""
WITH_MINIDLNA=0

# --- colors and messages ---
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

info()  { echo -e "${CYAN}[INFO]${NC} $*"; }
ok()    { echo -e "${GREEN}[OK]${NC} $*"; }
warn()  { echo -e "${YELLOW}[WARN]${NC} $*"; }
err()   { echo -e "${RED}[ERROR]${NC} $*"; }

usage() {
  cat << USAGE
Usage: sudo scripts/install.sh [options]

Options:
  --yes, --non-interactive   Run unattended with safe defaults
  --with-qbittorrent         Install/configure qBittorrent (default with --yes)
  --no-qbittorrent           Do not install/configure qBittorrent
  --with-prowlarr            Install/configure Prowlarr (default with --yes)
  --no-prowlarr              Do not install/configure Prowlarr
  --with-minidlna            Install/configure minidlna
  --force-reinstall          Recreate .env from supplied env/defaults
  --update-only              Only update binary/service and merge .env.example
  --no-create-user           Run service as invoking user instead of dedicated tms user
  -h, --help                 Show this help

Unattended example:
  BOT_TOKEN=123:abc sudo scripts/install.sh --yes
  BOT_TOKEN=123:abc sudo scripts/install.sh --yes --with-minidlna
USAGE
}

parse_args() {
  while [[ $# -gt 0 ]]; do
    case "$1" in
      --yes|--non-interactive)
        ASSUME_YES=1
        ;;
      --with-qbittorrent)
        WITH_QBITTORRENT=1
        ;;
      --no-qbittorrent)
        WITH_QBITTORRENT=0
        ;;
      --with-prowlarr)
        WITH_PROWLARR=1
        ;;
      --no-prowlarr)
        WITH_PROWLARR=0
        ;;
      --with-minidlna)
        WITH_MINIDLNA=1
        ;;
      --force-reinstall)
        FORCE_REINSTALL=1
        ;;
      --update-only)
        UPDATE_ONLY=1
        ;;
      --no-create-user)
        CREATE_TMS_USER=0
        ;;
      -h|--help)
        usage
        exit 0
        ;;
      *)
        err "Unknown option: $1"
        usage
        exit 1
        ;;
    esac
    shift
  done
  if [[ "$ASSUME_YES" -eq 1 && -z "$WITH_QBITTORRENT" ]]; then
    WITH_QBITTORRENT=1
  fi
  if [[ "$ASSUME_YES" -eq 1 && -z "$WITH_PROWLARR" ]]; then
    WITH_PROWLARR=1
  fi
  if [[ -z "$WITH_QBITTORRENT" ]]; then
    WITH_QBITTORRENT=0
  fi
}

confirm_or_default() {
  local prompt="$1"
  local default="${2:-n}"
  local ans=""
  if [[ "$ASSUME_YES" -eq 1 ]]; then
    [[ "${default,,}" == "y" || "${default,,}" == "yes" ]]
    return $?
  fi
  read -r -p "$prompt" ans
  if [[ -z "$ans" ]]; then
    ans="$default"
  fi
  [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]
}

env_or_default() {
  local key="$1"
  local default="${2:-}"
  local value="${!key:-}"
  if [[ -n "$value" ]]; then
    echo "$value"
  else
    echo "$default"
  fi
}

generate_secret_if_missing() {
  local key="$1"
  local value="${!key:-}"
  if [[ -n "$value" ]] && ! is_placeholder "$value" "$key"; then
    echo "$value"
    return 0
  fi
  generate_password
}

require_or_prompt() {
  local key="$1"
  local prompt="$2"
  local value="${!key:-}"
  if ! is_placeholder "$value" "$key"; then
    echo "$value"
    return 0
  fi
  if [[ "$ASSUME_YES" -eq 1 ]]; then
    err "$key is required in non-interactive mode. Example: $key=... sudo scripts/install.sh --yes"
    exit 1
  fi
  value=$(read_value "$prompt")
  while [[ -z "$value" ]]; do
    err "$key cannot be empty."
    value=$(read_value "$prompt")
  done
  echo "$value"
}

upsert_env() {
  local env_file="$1"
  local key="$2"
  local value="$3"
  mkdir -p "$(dirname "$env_file")"
  touch "$env_file"
  if grep -q "^${key}=" "$env_file"; then
    grep -v "^${key}=" "$env_file" > "${env_file}.tmp"
    printf '%s=%s\n' "$key" "$value" >> "${env_file}.tmp"
    mv "${env_file}.tmp" "$env_file"
  else
    printf '%s=%s\n' "$key" "$value" >> "$env_file"
  fi
}

# --- ensure MOVIE_PATH exists and is writable by TMS and qBittorrent (same user) ---
ensure_movie_path_writable() {
  local movie_path="${1:?}"
  local run_user
  run_user=$(logname 2>/dev/null || echo "${SUDO_USER:-}")
  [[ -z "$run_user" || "$run_user" == root ]] && run_user="${SUDO_USER:-root}"
  mkdir -p "$movie_path"
  mkdir -p "${movie_path%/}/incomplete"
  if [[ -n "$run_user" ]] && getent passwd "$run_user" &>/dev/null; then
    chown -R "$run_user:$run_user" "$movie_path"
  else
    chown -R "${SUDO_UID:-0}:${SUDO_GID:-0}" "$movie_path" 2>/dev/null || true
  fi
  chmod 775 "$movie_path"
  chmod 775 "${movie_path%/}/incomplete"
}

# --- ensure dedicated user tms exists (system user, no login, owns MOVIE_PATH and runs services) ---
ensure_user_tms() {
  if getent passwd "$TMS_USER" &>/dev/null; then
    ok "User $TMS_USER already exists."
    return 0
  fi
  local nologin_path
  nologin_path="$(command -v nologin 2>/dev/null || echo /usr/sbin/nologin)"
  useradd --system --user-group \
    --home-dir "$TMS_HOME" \
    --create-home \
    --shell "$nologin_path" \
    "$TMS_USER"
  ok "Created user $TMS_USER (home $TMS_HOME)."
}

# --- check root ---
check_root() {
  if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root. Use: sudo $0"
    exit 1
  fi
}

# --- detect distro (Arch or Ubuntu/Debian) ---
detect_distro() {
  if [[ -f /etc/os-release ]]; then
    # shellcheck source=/dev/null
    . /etc/os-release
    case "${ID:-}" in
      arch|endeavouros|manjaro) DISTRO=arch ;;
      ubuntu|debian|linuxmint|pop) DISTRO=ubuntu ;;
      *) err "Unsupported distro: ${ID:-unknown}. Supported: Arch Linux, Ubuntu/Debian."; exit 1 ;;
    esac
  else
    err "Cannot detect distro (/etc/os-release missing)."
    exit 1
  fi
  ok "Detected distro: $DISTRO"
}

# --- Arch packages (pacman) ---
set_arch_packages() {
  PKG_UPDATE="true"
  PKG_INSTALL="pacman -S --noconfirm"
  PKG_INSTALL_LABEL="pacman"
  PKG_QBIT=qbittorrent-nox
  PKG_GO=go
  PKG_CURL=curl
  PKG_ARIA2=aria2
  PKG_FFMPEG=ffmpeg
  PKG_YTDLP=yt-dlp
  PKG_MINIDLNA=minidlna
  PKG_BASE_DEVEL=base-devel
  PKG_GIT=git
}

# --- Ubuntu/Debian packages (apt) ---
set_ubuntu_packages() {
  PKG_UPDATE="apt-get update -qq"
  PKG_INSTALL="apt-get install -y -qq"
  PKG_INSTALL_LABEL="apt"
  PKG_QBIT=qbittorrent-nox
  PKG_GO=golang-go
  PKG_CURL=curl
  PKG_ARIA2=aria2
  PKG_FFMPEG=ffmpeg
  PKG_YTDLP=yt-dlp
  PKG_MINIDLNA=minidlna
}

# --- check dependencies; optionally install missing ---
check_and_install_deps() {
  local need_go=0 need_curl=0 need_aria2=0 need_ffmpeg=0 need_ytdlp=0
  command -v go      &>/dev/null || need_go=1
  command -v curl    &>/dev/null || need_curl=1
  command -v aria2c  &>/dev/null || need_aria2=1
  command -v ffmpeg  &>/dev/null || need_ffmpeg=1
  command -v yt-dlp  &>/dev/null || need_ytdlp=1

  if [[ $need_go -eq 1 ]] || [[ $need_curl -eq 1 ]] || [[ $need_aria2 -eq 1 ]] || [[ $need_ffmpeg -eq 1 ]] || [[ $need_ytdlp -eq 1 ]]; then
    warn "Some dependencies are missing:"
    [[ $need_go -eq 1 ]]    && echo "  - go (for build)"
    [[ $need_curl -eq 1 ]]  && echo "  - curl (service readiness checks)"
    [[ $need_aria2 -eq 1 ]] && echo "  - aria2 (torrents, unless using qBittorrent)"
    [[ $need_ffmpeg -eq 1 ]] && echo "  - ffmpeg"
    [[ $need_ytdlp -eq 1 ]] && echo "  - yt-dlp"
    echo
    if [[ "$ASSUME_YES" -eq 1 ]]; then
      ans="y"
    else
      read -r -p "Install missing packages via ${PKG_INSTALL_LABEL:-package manager}? [y/N] " ans
    fi
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      $PKG_UPDATE
      [[ $need_go -eq 1 ]]    && $PKG_INSTALL $PKG_GO
      [[ $need_curl -eq 1 ]]  && $PKG_INSTALL $PKG_CURL
      [[ $need_aria2 -eq 1 ]] && $PKG_INSTALL $PKG_ARIA2
      [[ $need_ffmpeg -eq 1 ]] && $PKG_INSTALL $PKG_FFMPEG
      [[ $need_ytdlp -eq 1 ]] && $PKG_INSTALL $PKG_YTDLP
    else
      err "Install packages manually (go, curl, aria2, ffmpeg, yt-dlp) and run the script again."
      exit 1
    fi
  fi

  if ! command -v yt-dlp &>/dev/null; then
    err "yt-dlp not found. Install: https://github.com/yt-dlp/yt-dlp#installation"
    exit 1
  fi
  ok "Dependencies: go, curl, aria2, ffmpeg, yt-dlp — OK."
}

# --- check Go version meets go.mod requirement (warn if too old) ---
check_go_version() {
  local repo_root="${1:?}"
  [[ ! -f "$repo_root/go.mod" ]] && return 0
  if ! command -v go &>/dev/null; then
    return 0
  fi
  local required current first
  required=$(grep '^go ' "$repo_root/go.mod" 2>/dev/null | awk '{print $2}')
  current=$(go version 2>/dev/null | grep -oE 'go[0-9]+\.[0-9]+' | sed 's/go//')
  [[ -z "$required" || -z "$current" ]] && return 0
  first=$(printf '%s\n' "$required" "$current" | sort -V | head -1)
  if [[ "$first" == "$current" && "$current" != "$required" ]]; then
    warn "Go $current installed, but go.mod requires $required."
    warn "Install latest Go: https://go.dev/dl/"
  fi
}

# --- read key value from .env (single line, no export) ---
# Use "grep ... || true" so missing keys do not trigger set -e / pipefail
get_env_value() {
  local f="$1" k="$2"
  [[ ! -f "$f" ]] && return 0
  (grep -E "^${k}=" "$f" 2>/dev/null || true) | sed -n "s/^${k}=//p" | head -1
  return 0
}

# --- value is empty or typical placeholder from .env.example ---
is_placeholder() {
  local v="$1" k="${2:-}"
  [[ -z "$v" ]] && return 0
  [[ "$v" == *your_telegram_bot_token* ]] && return 0
  [[ "$v" == *your_admin_password* ]] && return 0
  [[ "$v" == *your_regular_password* ]] && return 0
  [[ "$v" == *path/to/your* ]] && return 0
  [[ "$v" == /path/to/your/* ]] && return 0
  return 1
}

# --- generate random password (safe for .env, min 12 chars) ---
generate_password() {
  if command -v openssl &>/dev/null; then
    openssl rand -base64 12 2>/dev/null | tr -d '\n/+=' | head -c 16
  else
    head -c 12 /dev/urandom 2>/dev/null | base64 | tr -d '\n/+=' | head -c 16
  fi
  echo
}

# --- interactive input for password (visible); offer [G]enerate random ---
read_secret() {
  local name="$1"
  local min_len="${2:-1}"
  local val=""
  while true; do
    read -r -p "$name ([G]enerate random or type your own): " val
    if [[ "${val,,}" == "g" || "${val,,}" == "generate" ]]; then
      val=$(generate_password)
      [[ ${#val} -ge "$min_len" ]] && echo "$val" && return 0
    fi
    if [[ ${#val} -ge "$min_len" ]]; then
      echo "$val"
      return 0
    fi
    [[ $min_len -gt 1 ]] && err "Minimum $min_len characters. Try again."
  done
}

# --- interactive input for plain string ---
read_value() {
  local prompt="$1"
  local default="${2:-}"
  local val=""
  if [[ -n "$default" ]]; then
    read -r -p "$prompt [$default]: " val
    echo "${val:-$default}"
  else
    read -r -p "$prompt: " val
    echo "$val"
  fi
}

# --- collect .env from dialog ---
collect_env() {
  local env_file="$1"
  local use_qbittorrent="${2:-0}"
  local qbit_url="${3:-}"
  local qbit_user="${4:-admin}"
  local qbit_pass="${5:-}"

  info "Enter required parameters."
  echo

  local bot_token
  bot_token=$(read_value "BOT_TOKEN (from @BotFather)")
  while [[ -z "$bot_token" ]]; do
    err "BOT_TOKEN cannot be empty."
    bot_token=$(read_value "BOT_TOKEN (from @BotFather)")
  done

  local movie_path
  movie_path=$(read_value "MOVIE_PATH (download directory)" "/var/lib/telegram-media-server/media")
  while [[ -z "$movie_path" ]]; do
    err "MOVIE_PATH cannot be empty."
    movie_path=$(read_value "MOVIE_PATH" "/var/lib/telegram-media-server/media")
  done
  if [[ ! -d "$movie_path" ]]; then
    read -r -p "Directory $movie_path does not exist. Create? [Y/n] " ans
    if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
      ensure_movie_path_writable "$movie_path"
      ok "Created directory $movie_path (writable for TMS and qBittorrent)"
    fi
  fi

  echo
  local admin_pass
  admin_pass=$(read_secret "ADMIN_PASSWORD (min 8 chars)" 8)
  echo
  local regular_pass
  read -r -p "REGULAR_PASSWORD (leave empty = same as admin, or [G]enerate random): " regular_input
  if [[ "${regular_input,,}" == "g" || "${regular_input,,}" == "generate" ]]; then
    regular_pass=$(generate_password)
  else
    regular_pass="$regular_input"
  fi
  echo

  local lang
  lang=$(read_value "LANG (en/ru)" "en")

  echo
  read -r -p "Enable compatibility mode for old Smart TVs (H.264 level cap, remux after download)? [y/N] " ans_tv
  local video_compat="false"
  [[ "${ans_tv,,}" == "y" || "${ans_tv,,}" == "yes" ]] && video_compat="true"

  read -r -p "Configure proxy for Telegram and/or content (bypass blocking, e.g. Russia)? [y/N] " ans_proxy
  local telegram_proxy="" content_proxy="" content_domains=""
  if [[ "${ans_proxy,,}" == "y" || "${ans_proxy,,}" == "yes" ]]; then
    info "Proxy for Telegram Bot API (HTTP/HTTPS/SOCKS5). Example: socks5://127.0.0.1:1080"
    telegram_proxy=$(read_value "TELEGRAM_PROXY" "socks5://127.0.0.1:1080")
    read -r -p "Use same proxy for video content (YouTube, Rutube, etc.)? [y/N] " ans_content
    if [[ "${ans_content,,}" == "y" || "${ans_content,,}" == "yes" ]]; then
      content_proxy="${telegram_proxy}"
      content_domains=$(read_value "CONTENT_PROXY_DOMAINS (comma-separated: youtube.com,youtu.be,rutube.ru,vk.com; empty = proxy for all)" "")
    fi
  fi

  # Build .env (use printf for passwords so $ and other chars are not expanded by shell)
  cat > "$env_file" << ENVEOF
# Generated by install.sh — Telegram Media Server
# REQUIRED
BOT_TOKEN=$bot_token
MOVIE_PATH=$movie_path
ENVEOF
  printf 'ADMIN_PASSWORD=%s\n' "$admin_pass" >> "$env_file"
  [[ -n "$regular_pass" ]] && printf 'REGULAR_PASSWORD=%s\n' "$regular_pass" >> "$env_file"
  echo "LANG=$lang" >> "$env_file"
  echo "" >> "$env_file"

  if [[ "$video_compat" == "true" ]]; then
    echo "# Compatibility mode for old Smart TVs (H.264 level cap)" >> "$env_file"
    echo "VIDEO_COMPATIBILITY_MODE=true" >> "$env_file"
    echo "" >> "$env_file"
  fi

  if [[ -n "$telegram_proxy" ]]; then
    echo "# Proxy for Telegram (bypass blocking)" >> "$env_file"
    printf 'TELEGRAM_PROXY=%s\n' "$telegram_proxy" >> "$env_file"
    if [[ -n "$content_proxy" ]]; then
      echo "# Proxy for video content (yt-dlp); domains = use proxy only for these (empty = all)" >> "$env_file"
      printf 'CONTENT_PROXY=%s\n' "$content_proxy" >> "$env_file"
      printf 'CONTENT_PROXY_DOMAINS=%s\n' "$content_domains" >> "$env_file"
    fi
    echo "" >> "$env_file"
  fi

  # qBittorrent
  if [[ "$use_qbittorrent" -eq 1 ]]; then
    local url="${qbit_url:-http://127.0.0.1:$QBIT_WEBUI_PORT}"
    local user="${qbit_user:-admin}"
    local pass="${qbit_pass:-adminadmin}"
    echo "# qBittorrent (configured by installer)" >> "$env_file"
    echo "QBITTORRENT_URL=$url" >> "$env_file"
    echo "QBITTORRENT_USERNAME=$user" >> "$env_file"
    echo "QBITTORRENT_PASSWORD=$pass" >> "$env_file"
    echo "" >> "$env_file"
  fi

  # Optional: Prowlarr — only write PROWLARR_* when we have the key (else app would fail validation: PROWLARR_API_KEY required when PROWLARR_URL set)
  echo ""
  if [[ "${6:-0}" -eq 1 && -n "${8:-}" ]]; then
    echo "# Prowlarr (installed by installer)" >> "$env_file"
    echo "PROWLARR_URL=${7:-http://127.0.0.1:9696}" >> "$env_file"
    printf 'PROWLARR_API_KEY=%s\n' "${8}" >> "$env_file"
    echo "" >> "$env_file"
  elif [[ "${6:-0}" -eq 1 ]]; then
    echo "# Prowlarr installed; add PROWLARR_URL and PROWLARR_API_KEY when ready (Settings → General → Security)" >> "$env_file"
    echo "" >> "$env_file"
  else
    read -r -p "Add Prowlarr to .env manually (URL and API Key)? [y/N] " ans
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      local prowlarr_url prowlarr_key
      prowlarr_url=$(read_value "PROWLARR_URL" "http://127.0.0.1:9696")
      prowlarr_key=$(read_value "PROWLARR_API_KEY (Settings → General → Security)")
      echo "PROWLARR_URL=$prowlarr_url" >> "$env_file"
      echo "PROWLARR_API_KEY=$prowlarr_key" >> "$env_file"
      echo "" >> "$env_file"
    fi
  fi

  # OpenClaw: needs both REST API (skill calls) and webhook (completion/failure notifications). See .env.example.
  read -r -p "Configure OpenClaw integration (enable REST API + webhook)? [y/N] " ans
  if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
    local api_key webhook_url webhook_token
    api_key=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 32)
    webhook_url=$(read_value "TMS_WEBHOOK_URL" "http://127.0.0.1:18789/hooks/tms")
    webhook_token=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 32)
    echo "# TMS REST API + webhook for OpenClaw (skill uses API; webhook for completion/failure notifications)" >> "$env_file"
    echo "TMS_API_ENABLED=true" >> "$env_file"
    echo "TMS_API_KEY=$api_key" >> "$env_file"
    echo "TMS_WEBHOOK_URL=$webhook_url" >> "$env_file"
    echo "TMS_WEBHOOK_TOKEN=$webhook_token" >> "$env_file"
    echo "" >> "$env_file"
    ok "OpenClaw: API enabled, TMS_API_KEY and webhook written. In OpenClaw set hooks.token = $webhook_token and skill env TMS_API_KEY = (same key)."
  fi

  chmod 600 "$env_file"
  ok "Config written: $env_file (mode 600)"
}

# --- prompt only for missing values and overlay onto existing .env ---
fill_missing_env() {
  local env_file="$1"
  local use_qbittorrent="${2:-0}"
  local qbit_url="${3:-}"
  local qbit_user="${4:-admin}"
  local qbit_pass="${5:-}"
  local use_prowlarr="${6:-0}"
  local prowlarr_url="${7:-http://127.0.0.1:9696}"
  local prowlarr_key="${8:-}"

  local bot_token movie_path admin_pass regular_pass lang
  bot_token=$(get_env_value "$env_file" "BOT_TOKEN")
  movie_path=$(get_env_value "$env_file" "MOVIE_PATH")
  admin_pass=$(get_env_value "$env_file" "ADMIN_PASSWORD")
  regular_pass=$(get_env_value "$env_file" "REGULAR_PASSWORD")
  lang=$(get_env_value "$env_file" "LANG")

  if is_placeholder "$bot_token" "BOT_TOKEN"; then
    info "BOT_TOKEN not set or is placeholder."
    bot_token=$(read_value "BOT_TOKEN (from @BotFather)")
    while [[ -z "$bot_token" ]]; do
      err "BOT_TOKEN cannot be empty."
      bot_token=$(read_value "BOT_TOKEN (from @BotFather)")
    done
  else
    ok "BOT_TOKEN already set."
  fi

  if is_placeholder "$movie_path" "MOVIE_PATH"; then
    movie_path=$(read_value "MOVIE_PATH (download directory)" "/var/lib/telegram-media-server/media")
    while [[ -z "$movie_path" ]]; do
      movie_path=$(read_value "MOVIE_PATH" "/var/lib/telegram-media-server/media")
    done
    if [[ ! -d "$movie_path" ]]; then
      read -r -p "Directory $movie_path does not exist. Create? [Y/n] " ans
      if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
        ensure_movie_path_writable "$movie_path"
        ok "Created directory $movie_path (writable for TMS and qBittorrent)"
      fi
    fi
  else
    ok "MOVIE_PATH already set: $movie_path"
  fi

  if is_placeholder "$admin_pass" "ADMIN_PASSWORD"; then
    admin_pass=$(read_secret "ADMIN_PASSWORD (min 8 chars)" 8)
    echo
  else
    ok "ADMIN_PASSWORD already set."
  fi

  if is_placeholder "$regular_pass" "REGULAR_PASSWORD"; then
    read -r -p "REGULAR_PASSWORD (empty = same as admin, or [G]enerate random): " regular_input
    if [[ "${regular_input,,}" == "g" || "${regular_input,,}" == "generate" ]]; then
      regular_pass=$(generate_password)
    else
      regular_pass="$regular_input"
    fi
  fi
  if [[ -z "$lang" ]]; then
    lang=$(read_value "LANG (en/ru)" "en")
  else
    ok "LANG already set: $lang"
  fi

  # Overlay only these keys onto existing .env (use printf for passwords so $ and other chars are not expanded)
  local key val
  for key in BOT_TOKEN MOVIE_PATH ADMIN_PASSWORD REGULAR_PASSWORD LANG; do
    case "$key" in
      BOT_TOKEN) val="$bot_token" ;;
      MOVIE_PATH) val="$movie_path" ;;
      ADMIN_PASSWORD) val="$admin_pass" ;;
      REGULAR_PASSWORD) val="$regular_pass" ;;
      LANG) val="$lang" ;;
    esac
    [[ "$key" == "REGULAR_PASSWORD" && -z "$val" ]] && continue
    if grep -q "^${key}=" "$env_file"; then
      grep -v "^${key}=" "$env_file" > "${env_file}.tmp"
      printf '%s=%s\n' "$key" "$val" >> "${env_file}.tmp"
      mv "${env_file}.tmp" "$env_file"
    else
      printf '%s=%s\n' "$key" "$val" >> "$env_file"
    fi
  done

  if [[ "$use_qbittorrent" -eq 1 ]]; then
    for key in QBITTORRENT_URL QBITTORRENT_USERNAME QBITTORRENT_PASSWORD; do
      case "$key" in
        QBITTORRENT_URL) val="${qbit_url:-http://127.0.0.1:$QBIT_WEBUI_PORT}" ;;
        QBITTORRENT_USERNAME) val="${qbit_user:-admin}" ;;
        QBITTORRENT_PASSWORD) val="${qbit_pass:-adminadmin}" ;;
      esac
      if grep -q "^${key}=" "$env_file"; then
        grep -v "^${key}=" "$env_file" > "${env_file}.tmp"
        echo "${key}=${val}" >> "${env_file}.tmp"
        mv "${env_file}.tmp" "$env_file"
      else
        echo "${key}=${val}" >> "$env_file"
      fi
    done
  fi

  if [[ "$use_prowlarr" -eq 1 ]]; then
    for key in PROWLARR_URL PROWLARR_API_KEY; do
      val=$([[ "$key" == "PROWLARR_URL" ]] && echo "$prowlarr_url" || echo "$prowlarr_key")
      if grep -q "^${key}=" "$env_file"; then
        grep -v "^${key}=" "$env_file" > "${env_file}.tmp"
        echo "${key}=${val}" >> "${env_file}.tmp"
        mv "${env_file}.tmp" "$env_file"
      else
        echo "${key}=${val}" >> "$env_file"
      fi
    done
  fi

  chmod 600 "$env_file"
  ok "Only missing parameters were added to .env."
}

existing_or_env_or_default() {
  local env_file="$1"
  local key="$2"
  local default="${3:-}"
  local env_value="${!key:-}"
  local existing_value=""
  if [[ -n "$env_value" ]]; then
    echo "$env_value"
    return 0
  fi
  existing_value=$(get_env_value "$env_file" "$key")
  if ! is_placeholder "$existing_value" "$key"; then
    echo "$existing_value"
    return 0
  fi
  echo "$default"
}

required_existing_or_env() {
  local env_file="$1"
  local key="$2"
  local env_value="${!key:-}"
  local existing_value=""
  if [[ -n "$env_value" ]]; then
    echo "$env_value"
    return 0
  fi
  existing_value=$(get_env_value "$env_file" "$key")
  if ! is_placeholder "$existing_value" "$key"; then
    echo "$existing_value"
    return 0
  fi
  err "$key is required in non-interactive mode. Example: $key=... sudo scripts/install.sh --yes"
  exit 1
}

write_unattended_env() {
  local env_file="$1"
  local reset="${2:-0}"
  local bot_token movie_path admin_pass regular_pass lang

  bot_token=$(required_existing_or_env "$env_file" "BOT_TOKEN")
  movie_path=$(existing_or_env_or_default "$env_file" "MOVIE_PATH" "/var/lib/telegram-media-server/media")
  admin_pass=$(existing_or_env_or_default "$env_file" "ADMIN_PASSWORD" "")
  regular_pass=$(existing_or_env_or_default "$env_file" "REGULAR_PASSWORD" "")
  lang=$(existing_or_env_or_default "$env_file" "LANG" "en")

  if is_placeholder "$admin_pass" "ADMIN_PASSWORD"; then
    admin_pass=$(generate_password)
  fi
  if is_placeholder "$regular_pass" "REGULAR_PASSWORD"; then
    regular_pass=$(generate_password)
  fi

  mkdir -p "$(dirname "$env_file")"
  if [[ "$reset" -eq 1 || ! -f "$env_file" ]]; then
    cat > "$env_file" << ENVEOF
# Generated by install.sh — Telegram Media Server
ENVEOF
  fi

  upsert_env "$env_file" "BOT_TOKEN" "$bot_token"
  upsert_env "$env_file" "MOVIE_PATH" "$movie_path"
  upsert_env "$env_file" "ADMIN_PASSWORD" "$admin_pass"
  upsert_env "$env_file" "REGULAR_PASSWORD" "$regular_pass"
  upsert_env "$env_file" "LANG" "$lang"

  if [[ -n "${VIDEO_COMPATIBILITY_MODE:-}" ]]; then
    upsert_env "$env_file" "VIDEO_COMPATIBILITY_MODE" "$VIDEO_COMPATIBILITY_MODE"
  fi
  if [[ -n "${TELEGRAM_PROXY:-}" ]]; then
    upsert_env "$env_file" "TELEGRAM_PROXY" "$TELEGRAM_PROXY"
  fi
  if [[ -n "${CONTENT_PROXY:-}" ]]; then
    upsert_env "$env_file" "CONTENT_PROXY" "$CONTENT_PROXY"
  fi
  if [[ -n "${CONTENT_PROXY_DOMAINS:-}" ]]; then
    upsert_env "$env_file" "CONTENT_PROXY_DOMAINS" "$CONTENT_PROXY_DOMAINS"
  fi

  upsert_env "$env_file" "TMS_API_ENABLED" "${TMS_API_ENABLED:-true}"
  upsert_env "$env_file" "TMS_API_KEY" "$(existing_or_env_or_default "$env_file" "TMS_API_KEY" "$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 32)")"
  if [[ -n "${TMS_WEBHOOK_URL:-}" ]]; then
    upsert_env "$env_file" "TMS_WEBHOOK_URL" "$TMS_WEBHOOK_URL"
    upsert_env "$env_file" "TMS_WEBHOOK_TOKEN" "$(existing_or_env_or_default "$env_file" "TMS_WEBHOOK_TOKEN" "$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 32)")"
  fi

  if [[ "$WITH_QBITTORRENT" -eq 1 ]]; then
    upsert_env "$env_file" "QBITTORRENT_URL" "$(existing_or_env_or_default "$env_file" "QBITTORRENT_URL" "http://127.0.0.1:$QBIT_WEBUI_PORT")"
    upsert_env "$env_file" "QBITTORRENT_USERNAME" "$(existing_or_env_or_default "$env_file" "QBITTORRENT_USERNAME" "admin")"
    upsert_env "$env_file" "QBITTORRENT_PASSWORD" "$(existing_or_env_or_default "$env_file" "QBITTORRENT_PASSWORD" "adminadmin")"
  fi

  if [[ "$WITH_PROWLARR" -eq 1 ]]; then
    if [[ -n "${PROWLARR_API_KEY:-}" ]]; then
      upsert_env "$env_file" "PROWLARR_URL" "$(existing_or_env_or_default "$env_file" "PROWLARR_URL" "http://127.0.0.1:$PROWLARR_PORT")"
      upsert_env "$env_file" "PROWLARR_API_KEY" "$PROWLARR_API_KEY"
    fi
  fi

  chmod 600 "$env_file"
  ok "Unattended config written: $env_file"
}

ensure_qbittorrent_package() {
  if command -v qbittorrent-nox &>/dev/null; then
    return 0
  fi
  info "Installing qbittorrent-nox..."
  $PKG_UPDATE
  $PKG_INSTALL $PKG_QBIT
  ok "Installed qbittorrent-nox"
}

run_unattended_install() {
  local repo_root="$1"
  local env_file="$2"

  if [[ "$CREATE_TMS_USER" -eq 1 ]]; then
    ensure_user_tms
    TMS_RUN_USER="$TMS_USER"
  else
    TMS_RUN_USER=$(logname 2>/dev/null || echo "${SUDO_USER:-root}")
    info "Services will run as: $TMS_RUN_USER"
  fi

  if [[ -f "$env_file" && "$FORCE_REINSTALL" -eq 0 ]]; then
    UPDATE_ONLY=1
  fi
  if [[ "$FORCE_REINSTALL" -eq 1 && -f "$env_file" ]]; then
    cp "$env_file" "${env_file}.bak.force"
    ok "Backed up config to ${env_file}.bak.force"
  fi

  write_unattended_env "$env_file" "$FORCE_REINSTALL"
  bash "$repo_root/scripts/merge-env.sh" "$env_file" "$repo_root/.env.example"

  local movie_path_owner
  movie_path_owner=$(get_env_value "$env_file" "MOVIE_PATH")
  if [[ -n "$movie_path_owner" ]]; then
    mkdir -p "$movie_path_owner"
    mkdir -p "${movie_path_owner%/}/incomplete"
    chown -R "$TMS_RUN_USER:$TMS_RUN_USER" "$movie_path_owner"
    chmod 775 "$movie_path_owner"
    chmod 775 "${movie_path_owner%/}/incomplete"
    ok "MOVIE_PATH $movie_path_owner owned by $TMS_RUN_USER"
  fi

  if [[ "$WITH_QBITTORRENT" -eq 1 ]]; then
    repair_qbittorrent_connection "$TMS_RUN_USER" "$env_file" "$movie_path_owner"
  fi

  if [[ "$WITH_PROWLARR" -eq 1 ]]; then
    PROWLARR_API_KEY_AUTO=$(install_prowlarr) || PROWLARR_API_KEY_AUTO=""
    if [[ -n "$PROWLARR_API_KEY_AUTO" ]]; then
      upsert_env "$env_file" "PROWLARR_URL" "$(existing_or_env_or_default "$env_file" "PROWLARR_URL" "http://127.0.0.1:$PROWLARR_PORT")"
      upsert_env "$env_file" "PROWLARR_API_KEY" "$PROWLARR_API_KEY_AUTO"
    fi
  fi

  if [[ "$WITH_MINIDLNA" -eq 1 ]]; then
    install_minidlna "$movie_path_owner" "$TMS_RUN_USER"
  fi

  install_tms_binary_and_service "$repo_root" "$TMS_RUN_USER"
}

# --- create and enable qBittorrent systemd unit ---
install_qbittorrent_systemd() {
  local run_user="${1:-}"
  if [[ -z "$run_user" ]]; then
    run_user=$(logname 2>/dev/null || echo "${SUDO_USER:-root}")
  fi
  local qbit_bin
  qbit_bin=$(command -v qbittorrent-nox 2>/dev/null || echo "/usr/bin/qbittorrent-nox")
  local unit_path="/etc/systemd/system/${QBIT_SERVICE_NAME}.service"
  if [[ -f "$unit_path" ]]; then
    warn "File $unit_path already exists. Ensuring service user, port, enable and start state."
    cp "$unit_path" "${unit_path}.bak.installer" 2>/dev/null || true
    if grep -q '^User=' "$unit_path" 2>/dev/null; then
      sed -i "s|^User=.*|User=$run_user|" "$unit_path"
    else
      sed -i "/^\[Service\]/a User=$run_user" "$unit_path"
    fi
    if grep -q '^Environment=QBT_WEBUI_PORT=' "$unit_path" 2>/dev/null; then
      sed -i "s|^Environment=QBT_WEBUI_PORT=.*|Environment=QBT_WEBUI_PORT=$QBIT_WEBUI_PORT|" "$unit_path"
    else
      sed -i "/^\[Service\]/a Environment=QBT_WEBUI_PORT=$QBIT_WEBUI_PORT" "$unit_path"
    fi
    systemctl daemon-reload
    systemctl enable "$QBIT_SERVICE_NAME"
    systemctl restart "$QBIT_SERVICE_NAME" 2>/dev/null || systemctl start "$QBIT_SERVICE_NAME" 2>/dev/null || true
    return 0
  fi
  cat > "$unit_path" << UNITEOF
[Unit]
Description=qBittorrent (headless, Web UI port $QBIT_WEBUI_PORT)
After=network-online.target
Wants=network-online.target

[Service]
Type=simple
User=$run_user
ExecStart=$qbit_bin
Environment=QBT_WEBUI_PORT=$QBIT_WEBUI_PORT
Restart=always

[Install]
WantedBy=multi-user.target
UNITEOF
  systemctl daemon-reload
  systemctl enable "$QBIT_SERVICE_NAME"
  systemctl start "$QBIT_SERVICE_NAME" 2>/dev/null || true
  ok "qBittorrent service: $unit_path (User=$run_user, port $QBIT_WEBUI_PORT), enabled at boot."
}

# PBKDF2 hash for password "adminadmin" (qBittorrent Web UI). Do not use on internet-exposed servers.
QBIT_ADMIN_HASH='@ByteArray(ARQ77eY1NUZaQsuDHbIMCA==:0WMRkYTUWVT9wVvdDtHAjU9b3b7uB8NR1Gur2hmQCvCDpm39Q+PsJRJPaCU51dEiz+dTzh8qbPsL8WkFljQYFQ==)'

generate_qbittorrent_password_hash() {
  local password="${1:?}"
  if [[ "$password" == "adminadmin" ]]; then
    echo "$QBIT_ADMIN_HASH"
    return 0
  fi
  if ! command -v python3 &>/dev/null; then
    warn "python3 not found; cannot generate qBittorrent PBKDF2 hash for a custom password."
    return 1
  fi
  python3 -c 'import base64, hashlib, os, sys; password=sys.argv[1].encode(); salt=os.urandom(16); digest=hashlib.pbkdf2_hmac("sha512", password, salt, 100000, 64); print("@ByteArray(%s:%s)" % (base64.b64encode(salt).decode(), base64.b64encode(digest).decode()))' "$password"
}

upsert_qbittorrent_pref() {
  local config_file="${1:?}"
  local key="${2:?}"
  local value="${3:?}"
  if grep -F -q "${key}=" "$config_file" 2>/dev/null; then
    KEY="$key" VALUE="$value" awk 'BEGIN { key = ENVIRON["KEY"]; value = ENVIRON["VALUE"] } index($0, key "=") == 1 { $0 = key "=" value } { print }' "$config_file" > "${config_file}.tmp"
    mv "${config_file}.tmp" "$config_file"
  elif grep -q '^\[Preferences\]' "$config_file" 2>/dev/null; then
    KEY="$key" VALUE="$value" awk 'BEGIN { key = ENVIRON["KEY"]; value = ENVIRON["VALUE"] } { print } /^\[Preferences\]$/ && !done { print key "=" value; done = 1 }' "$config_file" > "${config_file}.tmp"
    mv "${config_file}.tmp" "$config_file"
  else
    printf '[Preferences]\n%s=%s\n' "$key" "$value" >> "$config_file"
  fi
}

cleanup_qbittorrent_pref_typos() {
  local config_file="${1:?}"
  awk '
    !/^(DownloadsSavePath|DownloadsTempPath|WebUIAddress|WebUIPort|WebUIUsername|WebUIPassword_PBKDF2|WebUILocalHostAuth)=/
  ' "$config_file" > "${config_file}.tmp"
  mv "${config_file}.tmp" "$config_file"
}

# --- set qBittorrent save path and Web UI credentials so Web UI and TMS work after install ---
set_qbittorrent_save_path() {
  local run_user="${1:?}"
  local movie_path="${2:?}"
  local qbit_user="${3:-admin}"
  local qbit_pass="${4:-adminadmin}"
  local force_credentials="${5:-0}"
  local home_dir
  home_dir=$(getent passwd "$run_user" 2>/dev/null | cut -d: -f6)
  [[ -z "$home_dir" ]] && home_dir="/home/$run_user"
  local config_dir="$home_dir/.config/qBittorrent"
  local config_file="$config_dir/qBittorrent.conf"
  info "Waiting for qBittorrent config (up to 20s) to set save path and Web UI credentials (if not already set)..."
  local i
  for i in {1..20}; do
    sleep 1
    [[ -f "$config_file" ]] && break
    curl -s -o /dev/null "http://127.0.0.1:${QBIT_WEBUI_PORT}" 2>/dev/null || true
  done
  if [[ ! -f "$config_file" ]]; then
    warn "qBittorrent config not found at $config_file. Set save path and login manually in Web UI."
    return 0
  fi
  cleanup_qbittorrent_pref_typos "$config_file"
  local save_path="${movie_path%/}/"
  local temp_path="${movie_path%/}/incomplete/"
  upsert_qbittorrent_pref "$config_file" 'Downloads\SavePath' "$save_path"
  upsert_qbittorrent_pref "$config_file" 'Downloads\TempPath' "$temp_path"
  upsert_qbittorrent_pref "$config_file" 'WebUI\Address' "127.0.0.1"
  upsert_qbittorrent_pref "$config_file" 'WebUI\Port' "$QBIT_WEBUI_PORT"

  if [[ "$force_credentials" -eq 1 ]] || ! grep -q '^WebUI\\Username=' "$config_file" 2>/dev/null; then
    upsert_qbittorrent_pref "$config_file" 'WebUI\Username' "$qbit_user"
  fi
  if [[ "$force_credentials" -eq 1 ]] || ! grep -q '^WebUI\\Password_PBKDF2=' "$config_file" 2>/dev/null; then
    local qbit_hash
    qbit_hash=$(generate_qbittorrent_password_hash "$qbit_pass") || return 1
    upsert_qbittorrent_pref "$config_file" 'WebUI\Password_PBKDF2' "$qbit_hash"
  fi
  # Keep localhost API calls authenticated so installer and TMS validate the real Web UI credentials.
  upsert_qbittorrent_pref "$config_file" 'WebUI\LocalHostAuth' "true"
  chown "$run_user" "$config_file" 2>/dev/null || true
  systemctl restart "$QBIT_SERVICE_NAME" 2>/dev/null || true
  ok "qBittorrent Web UI configured on 127.0.0.1:$QBIT_WEBUI_PORT; credentials synchronized with $CONFIG_DIR/.env when repair is requested."
}

validate_qbittorrent_api() {
  local qbit_url="${1:?}"
  local qbit_user="${2:-admin}"
  local qbit_pass="${3:-adminadmin}"
  local cookie_file login_body version_body status
  cookie_file=$(mktemp)
  login_body=$(curl -sS --connect-timeout 3 --max-time 8 -c "$cookie_file" \
    -H "Referer: ${qbit_url%/}/" \
    --data-urlencode "username=$qbit_user" \
    --data-urlencode "password=$qbit_pass" \
    -w '\n%{http_code}' \
    "${qbit_url%/}/api/v2/auth/login" 2>&1) || {
      rm -f "$cookie_file"
      warn "qBittorrent login request failed for $qbit_url as $qbit_user: $login_body"
      return 1
    }
  status=$(printf '%s' "$login_body" | tail -n 1)
  login_body=$(printf '%s' "$login_body" | sed '$d')
  if [[ "$status" == "403" ]]; then
    rm -f "$cookie_file"
    warn "qBittorrent login forbidden for $qbit_url as $qbit_user (403; IP may be banned or credentials are rejected)."
    return 1
  fi
  if [[ "$status" != "200" && "$status" != "204" ]]; then
    rm -f "$cookie_file"
    warn "qBittorrent login failed for $qbit_url as $qbit_user (status=$status, body=$login_body)."
    return 1
  fi
  if [[ "$(printf '%s' "$login_body" | tr -d '\r\n')" == "Fails." ]]; then
    rm -f "$cookie_file"
    warn "qBittorrent login failed for $qbit_url as $qbit_user (invalid credentials)."
    return 1
  fi
  version_body=$(curl -sS --connect-timeout 3 --max-time 8 -b "$cookie_file" \
    -H "Referer: ${qbit_url%/}/" \
    -w '\n%{http_code}' \
    "${qbit_url%/}/api/v2/app/version" 2>&1) || {
      rm -f "$cookie_file"
      warn "qBittorrent app/version request failed for $qbit_url: $version_body"
      return 1
    }
  rm -f "$cookie_file"
  status=$(printf '%s' "$version_body" | tail -n 1)
  version_body=$(printf '%s' "$version_body" | sed '$d')
  if [[ "$status" != "200" ]]; then
    warn "qBittorrent app/version failed for $qbit_url (status=$status, body=$version_body)."
    return 1
  fi
  ok "qBittorrent Web API check OK: $qbit_url, user=$qbit_user, version=$version_body"
  return 0
}

is_local_qbittorrent_url() {
  local qbit_url="${1:?}"
  [[ "$qbit_url" == "http://127.0.0.1:${QBIT_WEBUI_PORT}"* || "$qbit_url" == "http://localhost:${QBIT_WEBUI_PORT}"* || "$qbit_url" == "http://[::1]:${QBIT_WEBUI_PORT}"* ]]
}

repair_qbittorrent_connection() {
  local run_user="${1:?}"
  local env_file="${2:?}"
  local movie_path="${3:?}"
  local qbit_url qbit_user qbit_pass
  qbit_url=$(get_env_value "$env_file" "QBITTORRENT_URL")
  [[ -z "$qbit_url" ]] && return 0
  qbit_user=$(existing_or_env_or_default "$env_file" "QBITTORRENT_USERNAME" "admin")
  qbit_pass=$(existing_or_env_or_default "$env_file" "QBITTORRENT_PASSWORD" "adminadmin")

  info "Checking qBittorrent Web API connectivity: url=$qbit_url user=$qbit_user"
  if validate_qbittorrent_api "$qbit_url" "$qbit_user" "$qbit_pass"; then
    return 0
  fi
  if ! is_local_qbittorrent_url "$qbit_url"; then
    err "qBittorrent check failed and $qbit_url is not the local installer-managed URL; fix QBITTORRENT_* in $env_file or the remote qBittorrent Web UI."
    return 1
  fi

  warn "Repairing local qBittorrent configuration and retrying..."
  ensure_qbittorrent_package
  install_qbittorrent_systemd "$run_user"
  set_qbittorrent_save_path "$run_user" "$movie_path" "$qbit_user" "$qbit_pass" 1
  sleep 2
  validate_qbittorrent_api "$qbit_url" "$qbit_user" "$qbit_pass"
}

# --- install and configure minidlna for DLNA (media_dir = MOVIE_PATH, runs as user minidlna) ---
# $1 = movie_path (must be readable by minidlna), $2 = run_user (if tms, we add minidlna to group tms and chmod 750)
install_minidlna() {
  local movie_path="${1:?}"
  local run_user="${2:-}"
  if ! command -v minidlnad &>/dev/null && [[ -n "${PKG_INSTALL:-}" ]]; then
    info "Installing minidlna..."
    $PKG_UPDATE
    $PKG_INSTALL $PKG_MINIDLNA
    ok "minidlna installed"
  fi
  if ! command -v minidlnad &>/dev/null; then
    warn "minidlnad not found. Install minidlna manually and configure /etc/minidlna.conf (media_dir=V,$movie_path)."
    return 1
  fi
  if [[ -f "$MINIDLNA_CONF" ]]; then
    cp "$MINIDLNA_CONF" "${MINIDLNA_CONF}.bak.installer"
    ok "Backed up $MINIDLNA_CONF to ${MINIDLNA_CONF}.bak.installer"
  fi
  local media_dir_line="media_dir=V,${movie_path%/}"
  local friendly_name="friendly_name=Telegram Media Server"
  if grep -q '^media_dir=' "$MINIDLNA_CONF" 2>/dev/null; then
    sed -i "s|^media_dir=.*|$media_dir_line|" "$MINIDLNA_CONF"
  else
    echo "$media_dir_line" >> "$MINIDLNA_CONF"
  fi
  if grep -q '^friendly_name=' "$MINIDLNA_CONF" 2>/dev/null; then
    sed -i "s|^friendly_name=.*|$friendly_name|" "$MINIDLNA_CONF"
  else
    echo "$friendly_name" >> "$MINIDLNA_CONF"
  fi
  (grep -q '^inotify=' "$MINIDLNA_CONF" 2>/dev/null && sed -i 's/^inotify=.*/inotify=yes/' "$MINIDLNA_CONF") || echo "inotify=yes" >> "$MINIDLNA_CONF"
  if [[ -n "$run_user" && "$run_user" == "$TMS_USER" ]]; then
    if getent group "$TMS_USER" &>/dev/null && getent passwd minidlna &>/dev/null; then
      usermod -aG "$TMS_USER" minidlna 2>/dev/null || true
      chmod 750 "$movie_path"
      ok "minidlna added to group $TMS_USER; MOVIE_PATH is readable by minidlna"
    fi
  else
    warn "Ensure user minidlna can read $movie_path (e.g. chmod o+rx or add minidlna to the group that owns MOVIE_PATH)."
  fi
  systemctl daemon-reload
  systemctl enable "$MINIDLNA_SERVICE_NAME"
  systemctl restart "$MINIDLNA_SERVICE_NAME" 2>/dev/null || true
  ok "minidlna configured (media_dir=$movie_path) and service started; DLNA clients can discover on port 8200."
}

# --- wait for Prowlarr to start and return API key (from config.xml or manual input) ---
# Call after enabling/starting prowlarr service. Returns API key via echo (empty on skip).
wait_prowlarr_and_get_key() {
  systemctl daemon-reload
  systemctl enable "$PROWLARR_SERVICE_NAME"
  systemctl start "$PROWLARR_SERVICE_NAME" 2>/dev/null || true
  info "Waiting for Prowlarr first start (up to 60s)..." >&2
  local config_path=""
  local i
  for i in {1..30}; do
    sleep 2
    for base in $PROWLARR_DATA_PATHS; do
      if [[ -f "$base/config.xml" ]]; then
        config_path="$base/config.xml"
        break 2
      fi
    done
    curl -s -o /dev/null "http://127.0.0.1:${PROWLARR_PORT}" 2>/dev/null || true
  done
  if [[ -z "$config_path" || ! -f "$config_path" ]]; then
    warn "Prowlarr config.xml not found. Prowlarr is running at http://127.0.0.1:${PROWLARR_PORT}" >&2
    if [[ "$ASSUME_YES" -eq 1 ]]; then
      warn "Skipping Prowlarr API key prompt in non-interactive mode. Set PROWLARR_API_KEY in $CONFIG_DIR/.env when available." >&2
      echo ""
      return 0
    fi
    read -r -p "Get API Key from Settings → General → Security and enter it here (or Enter to skip): " api_key_manual
    if [[ -n "$api_key_manual" ]]; then
      echo "$api_key_manual"
      ok "Using entered Prowlarr API Key" >&2
    else
      echo ""
    fi
    return 0
  fi
  local api_key
  api_key=$(sed -n 's/.*<ApiKey>\([^<]*\)<\/ApiKey>.*/\1/p' "$config_path" 2>/dev/null | head -1)
  if [[ -n "$api_key" ]]; then
    echo "$api_key"
    ok "Prowlarr API Key read from $config_path" >&2
  else
    info "Prowlarr is running at http://127.0.0.1:${PROWLARR_PORT}" >&2
    if [[ "$ASSUME_YES" -eq 1 ]]; then
      warn "Prowlarr API key was not found automatically. Set PROWLARR_API_KEY in $CONFIG_DIR/.env when available." >&2
      echo ""
      return 0
    fi
    read -r -p "Get API Key from Settings → General → Security and enter it here (or Enter to skip): " api_key_manual
    if [[ -n "$api_key_manual" ]]; then
      echo "$api_key_manual"
      ok "Using entered Prowlarr API Key" >&2
    else
      echo ""
    fi
  fi
  return 0
}

# --- install Prowlarr from AUR (Arch), start service, extract API key ---
# Returns API key via echo (empty string on error).
ensure_arch_aur_helper() {
  local run_user="${1:?}"
  local helper=""
  for h in yay paru; do
    if sudo -u "$run_user" command -v "$h" &>/dev/null; then
      echo "$h"
      return 0
    fi
  done

  info "AUR helper not found. Installing yay automatically for Prowlarr..." >&2
  $PKG_INSTALL --needed "$PKG_BASE_DEVEL" "$PKG_GIT" >&2
  local build_dir="/tmp/tms-yay-build"
  rm -rf "$build_dir"
  sudo -u "$run_user" git clone https://aur.archlinux.org/yay-bin.git "$build_dir" >&2
  sudo -u "$run_user" bash -lc "cd '$build_dir' && makepkg --noconfirm" >&2
  local yay_pkg
  yay_pkg=$(find "$build_dir" -maxdepth 1 -name 'yay-bin-*.pkg.tar.*' | head -1)
  if [[ -z "$yay_pkg" ]]; then
    warn "yay package was not produced by makepkg." >&2
    rm -rf "$build_dir"
    return 1
  fi
  pacman -U --noconfirm "$yay_pkg" >&2
  rm -rf "$build_dir"
  if sudo -u "$run_user" command -v yay &>/dev/null; then
    echo "yay"
    return 0
  fi
  warn "Failed to install yay automatically." >&2
  return 1
}

install_prowlarr_arch() {
  local run_user
  run_user=$(logname 2>/dev/null || echo "${SUDO_USER:-}")
  if [[ -z "$run_user" || "$run_user" == root ]]; then
    err "AUR install requires a non-root user (run: sudo -u your_user or set SUDO_USER)." >&2
    return 1
  fi

  local aur_helper=""
  aur_helper=$(ensure_arch_aur_helper "$run_user") || aur_helper=""
  if [[ -z "$aur_helper" ]]; then
    warn "AUR helper (yay or paru) not available. Install prowlarr-bin manually or rerun after installing yay." >&2
    return 1
  fi

  info "Installing Prowlarr from AUR (${aur_helper})..." >&2
  if ! sudo -u "$run_user" "$aur_helper" -S prowlarr-bin --noconfirm --needed >&2; then
    warn "Failed to install prowlarr-bin. Try manually: $aur_helper -S prowlarr-bin" >&2
    return 1
  fi
  ok "Prowlarr installed." >&2
  wait_prowlarr_and_get_key
  return 0
}

# --- install Prowlarr from Servarr apt repo (Ubuntu/Debian), start service, extract API key ---
# Returns API key via echo (empty string on error).
install_prowlarr_ubuntu() {
  info "Installing Prowlarr from Servarr apt repo..." >&2
  mkdir -p /usr/share/keyrings
  local key_file="/usr/share/keyrings/prowlarr-repo.key"
  if [[ ! -s "$key_file" ]]; then
    if command -v wget &>/dev/null; then
      wget -qO "$key_file" "https://repo.prowlarr.com/prowlarr-repo.key" 2>/dev/null || true
    fi
  fi
  if [[ -s "$key_file" ]]; then
    echo "deb [signed-by=$key_file] https://apt.prowlarr.com/debian prowlarr-main main" > /etc/apt/sources.list.d/prowlarr.list
  else
    apt-key adv --keyserver hkp://keyserver.ubuntu.com:80 --recv-keys 2009837CBFFD68F45BC180471F4F90DE2A9B4BF8 2>/dev/null || true
    echo "deb https://apt.prowlarr.com/debian prowlarr-main main" > /etc/apt/sources.list.d/prowlarr.list
  fi
  apt-get update -qq >&2
  if ! apt-get install -y -qq prowlarr >&2; then
    warn "Failed to install prowlarr. Try: apt-get install -y prowlarr" >&2
    return 1
  fi
  ok "Prowlarr installed." >&2
  wait_prowlarr_and_get_key
  return 0
}

# --- install Prowlarr (distro-specific), returns API key via echo ---
install_prowlarr() {
  if [[ "$DISTRO" == "arch" ]]; then
    install_prowlarr_arch
  else
    install_prowlarr_ubuntu
  fi
}

# --- main TMS installer (binary + unit + locales) ---
# $1 = repo_root, $2 = run_user (optional; if "tms", service runs as User=tms and .env is readable by tms)
install_tms_binary_and_service() {
  local repo_root="${1:?}"
  local run_user="${2:-}"
  local build_dir="${repo_root}/build"
  local version
  version=$(cd "$repo_root" && git describe --tags --always --dirty 2>/dev/null || echo "unknown")
  local build_time
  build_time=$(date -u '+%Y-%m-%d_%H:%M:%S')
  local ldflags="-X main.Version=${version} -X main.BuildTime=${build_time}"

  info "Building $BINARY_NAME..."
  (cd "$repo_root" && go build -ldflags "$ldflags" -trimpath -o "$build_dir/$BINARY_NAME" ./cmd/telegram-media-server)
  ok "Build complete."

  install -Dm755 "${build_dir}/${BINARY_NAME}" "${INSTALL_DIR}/${BINARY_NAME}"
  install -Dm644 "${repo_root}/.env.example" "${CONFIG_DIR}/.env.example"
  install -Dm644 "${repo_root}/telegram-media-server.service" "${SERVICE_DIR}/telegram-media-server.service"
  install -d "$LOCALES_DEST"
  install -Dm644 "${repo_root}/${LOCALES_SRC}"/* "$LOCALES_DEST/" 2>/dev/null || true

  if [[ -f "${CONFIG_DIR}/.env" ]]; then
    info "Merging new parameters into existing .env..."
    bash "${repo_root}/scripts/merge-env.sh" "${CONFIG_DIR}/.env" "${CONFIG_DIR}/.env.example"
  fi

  if [[ -n "$run_user" && "$run_user" == "$TMS_USER" ]]; then
    chown root:"$TMS_USER" "${CONFIG_DIR}/.env"
    chmod 640 "${CONFIG_DIR}/.env"
    ok ".env readable by $TMS_USER"
    ok "telegram-media-server will run as $TMS_USER (see unit file)"
  else
    mkdir -p /etc/systemd/system/telegram-media-server.service.d
    local run_group
    run_group=$(id -gn "$run_user" 2>/dev/null || echo "$run_user")
    printf '%s\n' '[Service]' "User=$run_user" "Group=$run_group" > /etc/systemd/system/telegram-media-server.service.d/override.conf
    ok "telegram-media-server will run as $run_user (override; unit defaults to User=$TMS_USER)"
  fi

  # Allow TMS user to run journalctl for the /logs command (read systemd journal)
  if [[ -n "$run_user" ]] && getent group systemd-journal &>/dev/null; then
    if usermod -aG systemd-journal "$run_user" 2>/dev/null; then
      ok "Added $run_user to group systemd-journal (admin /logs command will work)"
    fi
  fi

  systemctl daemon-reload
  systemctl enable telegram-media-server
  systemctl restart telegram-media-server
  ok "telegram-media-server service installed and started."
}

# --- main ---
main() {
  parse_args "$@"

  echo "=============================================="
  echo "  Telegram Media Server — installer"
  echo "=============================================="
  echo

  cleanup() {
    local exit_code=$?
    if [[ $exit_code -ne 0 ]]; then
      err "Installation failed (exit code $exit_code). Check output above."
    fi
  }
  trap cleanup EXIT

  check_root
  detect_distro
  if [[ "$DISTRO" == "arch" ]]; then
    set_arch_packages
    SERVICE_DIR=/usr/lib/systemd/system
  else
    set_ubuntu_packages
    SERVICE_DIR=/etc/systemd/system
  fi

  if [[ "$ASSUME_YES" -eq 0 ]]; then
    echo
    echo -e "${YELLOW}Warning: default passwords are used (e.g. qBittorrent Web UI: admin/adminadmin).${NC}"
    echo "Do not use this installer on an internet-exposed server without changing passwords."
    echo
    if ! confirm_or_default "Continue with installation? [y/N] " "n"; then
      echo "Exiting."
      exit 0
    fi
    echo
  fi

  check_and_install_deps

  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  if [[ ! -f "$REPO_ROOT/cmd/telegram-media-server/main.go" ]]; then
    err "Repository root not found (expected cmd/telegram-media-server/main.go). Run from project directory."
    exit 1
  fi
  check_go_version "$REPO_ROOT"

  mkdir -p "$CONFIG_DIR"
  ENV_FILE="${CONFIG_DIR}/.env"

  if [[ "$ASSUME_YES" -eq 1 ]]; then
    run_unattended_install "$REPO_ROOT" "$ENV_FILE"
    echo
    echo "=============================================="
    ok "Unattended installation complete."
    echo "=============================================="
    echo "Config:     $ENV_FILE"
    echo "Service:    systemctl status telegram-media-server"
    echo "Logs:       journalctl -u telegram-media-server -f"
    if [[ "$WITH_QBITTORRENT" -eq 1 ]]; then
      echo "qBittorrent: systemctl status $QBIT_SERVICE_NAME  |  Web UI: http://127.0.0.1:$QBIT_WEBUI_PORT"
    fi
    if [[ "$WITH_PROWLARR" -eq 1 ]]; then
      echo "Prowlarr:    systemctl status $PROWLARR_SERVICE_NAME  |  Web UI: http://127.0.0.1:$PROWLARR_PORT"
    fi
    if [[ "$WITH_MINIDLNA" -eq 1 ]]; then
      echo "minidlna:    systemctl status $MINIDLNA_SERVICE_NAME"
    fi
    echo
    return 0
  fi

  echo
  if [[ "$CREATE_TMS_USER" -eq 1 ]] && confirm_or_default "Create dedicated user '$TMS_USER' for TMS and qBittorrent (recommended; grants write access to download dir)? [Y/n] " "y"; then
    ensure_user_tms
    TMS_RUN_USER="$TMS_USER"
  else
    TMS_RUN_USER=$(logname 2>/dev/null || echo "${SUDO_USER:-root}")
    info "Services will run as: $TMS_RUN_USER (ensure this user has write access to MOVIE_PATH)."
  fi

  USE_QBITTORRENT=0
  QBIT_USER="admin"
  QBIT_PASS="adminadmin"
  USE_PROWLARR=0
  PROWLARR_URL_AUTO="http://127.0.0.1:9696"
  PROWLARR_API_KEY_AUTO=""
  USE_MINIDLNA=0

  # Upgrade mode: .env exists — offer to only update binary
  if [[ -f "$ENV_FILE" ]]; then
    echo
    info "Existing installation detected ($ENV_FILE present)."
    if [[ "$UPDATE_ONLY" -eq 1 ]]; then
      ans="y"
    elif [[ "$FORCE_REINSTALL" -eq 1 ]]; then
      ans="n"
    else
      read -r -p "Update only binary and service (leave config unchanged)? [Y/n] " ans
    fi
    if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
      bash "$REPO_ROOT/scripts/merge-env.sh" "$ENV_FILE" "$REPO_ROOT/.env.example"
      if [[ "$TMS_RUN_USER" == "$TMS_USER" ]]; then
        movie_path_owner=$(get_env_value "$ENV_FILE" "MOVIE_PATH")
        if [[ -n "$movie_path_owner" ]]; then
          mkdir -p "$movie_path_owner"
          mkdir -p "${movie_path_owner%/}/incomplete"
          chown -R "$TMS_RUN_USER:$TMS_RUN_USER" "$movie_path_owner"
          ok "MOVIE_PATH $movie_path_owner owned by $TMS_RUN_USER"
        fi
      fi
      if [[ -n "$(get_env_value "$ENV_FILE" "QBITTORRENT_URL")" ]]; then
        movie_path_owner=$(get_env_value "$ENV_FILE" "MOVIE_PATH")
        if [[ -n "$movie_path_owner" ]]; then
          repair_qbittorrent_connection "$TMS_RUN_USER" "$ENV_FILE" "$movie_path_owner"
        else
          warn "QBITTORRENT_URL is set but MOVIE_PATH is empty; qBittorrent repair skipped."
        fi
      fi
      install_tms_binary_and_service "$REPO_ROOT" "$TMS_RUN_USER"
      echo
      ok "Done. Config was not changed."
      systemctl status telegram-media-server --no-pager 2>/dev/null || true
      exit 0
    fi
    if [[ "$FORCE_REINSTALL" -eq 1 ]]; then
      ans="y"
    else
      read -r -p "Force reinstall (re-enter all settings from scratch)? [y/N] " ans
    fi
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      cp "$ENV_FILE" "${ENV_FILE}.bak.force"
      ok "Backed up config to ${ENV_FILE}.bak.force"
      echo
      if [[ "$WITH_QBITTORRENT" -eq 1 ]]; then
        ans="y"
      else
        read -r -p "Configure qBittorrent for torrents? (Web UI port $QBIT_WEBUI_PORT, systemd) [y/N] " ans
      fi
      if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
        USE_QBITTORRENT=1
        if ! command -v qbittorrent-nox &>/dev/null; then
          read -r -p "qbittorrent-nox not installed. Install? [Y/n] " ians
          if [[ "${ians,,}" != "n" && "${ians,,}" != "no" ]]; then
            $PKG_UPDATE
            $PKG_INSTALL $PKG_QBIT
            ok "Installed qbittorrent-nox"
          fi
        fi
        if command -v qbittorrent-nox &>/dev/null; then
          info "qBittorrent: using admin/adminadmin for Web UI and .env (installer will write this into qBittorrent config). Change after first login in Settings → Web UI and update .env if needed."
        fi
      fi
      echo
      if [[ "$WITH_PROWLARR" -eq 1 ]]; then
        ans="y"
      elif [[ "$WITH_PROWLARR" == "0" ]]; then
        ans="n"
      else
        read -r -p "Install and configure Prowlarr? (port $PROWLARR_PORT) [Y/n] " ans
      fi
      if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
        USE_PROWLARR=1
        PROWLARR_API_KEY_AUTO=$(install_prowlarr) || PROWLARR_API_KEY_AUTO=""
      fi
      echo
      if [[ "$WITH_MINIDLNA" -eq 1 ]]; then
        ans="y"
      else
        read -r -p "Install and configure minidlna for DLNA distribution? (port 8200) [y/N] " ans
      fi
      if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
        USE_MINIDLNA=1
      fi
      collect_env "$ENV_FILE" "$USE_QBITTORRENT" "http://127.0.0.1:$QBIT_WEBUI_PORT" "$QBIT_USER" "$QBIT_PASS" "$USE_PROWLARR" "$PROWLARR_URL_AUTO" "$PROWLARR_API_KEY_AUTO"
    else
      # Prompt only for what is missing
      info "Only missing values will be prompted (existing values are kept)."
      echo
      EXISTING_QBIT=$(get_env_value "$ENV_FILE" "QBITTORRENT_URL")
      EXISTING_PROWLARR_URL=$(get_env_value "$ENV_FILE" "PROWLARR_URL")
      EXISTING_PROWLARR_KEY=$(get_env_value "$ENV_FILE" "PROWLARR_API_KEY")
      if [[ -z "$EXISTING_QBIT" ]]; then
        if [[ "$WITH_QBITTORRENT" -eq 1 ]]; then
          ans="y"
        else
          read -r -p "Configure qBittorrent? (port $QBIT_WEBUI_PORT, systemd) [y/N] " ans
        fi
        if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
          USE_QBITTORRENT=1
          if ! command -v qbittorrent-nox &>/dev/null; then
            read -r -p "qbittorrent-nox not installed. Install? [Y/n] " ians
            if [[ "${ians,,}" != "n" && "${ians,,}" != "no" ]]; then
              $PKG_UPDATE
              $PKG_INSTALL $PKG_QBIT
              ok "Installed qbittorrent-nox"
            fi
          fi
        fi
        if command -v qbittorrent-nox &>/dev/null; then
          info "qBittorrent: using admin/adminadmin for Web UI and .env (installer will write this into qBittorrent config). Change after first login in Settings → Web UI and update .env if needed."
        fi
      else
        ok "qBittorrent already configured (QBITTORRENT_URL set)."
      fi

    if [[ -z "$EXISTING_PROWLARR_URL" && -z "$EXISTING_PROWLARR_KEY" ]]; then
        if [[ "$WITH_PROWLARR" -eq 1 ]]; then
          ans="y"
        elif [[ "$WITH_PROWLARR" == "0" ]]; then
          ans="n"
        else
          read -r -p "Install and configure Prowlarr? (port $PROWLARR_PORT) [Y/n] " ans
        fi
        if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
          USE_PROWLARR=1
          PROWLARR_API_KEY_AUTO=$(install_prowlarr) || PROWLARR_API_KEY_AUTO=""
        fi
      else
        ok "Prowlarr already configured (PROWLARR_* set)."
      fi
      echo
      if [[ "$WITH_MINIDLNA" -eq 1 ]]; then
        ans="y"
      else
        read -r -p "Install and configure minidlna for DLNA distribution? (port 8200) [y/N] " ans
      fi
      if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
        USE_MINIDLNA=1
      fi

      bash "$REPO_ROOT/scripts/merge-env.sh" "$ENV_FILE" "$REPO_ROOT/.env.example"
      fill_missing_env "$ENV_FILE" "$USE_QBITTORRENT" "http://127.0.0.1:$QBIT_WEBUI_PORT" "$QBIT_USER" "$QBIT_PASS" "$USE_PROWLARR" "$PROWLARR_URL_AUTO" "$PROWLARR_API_KEY_AUTO"
    fi
  else
    # New install: full dialog
    echo
    if [[ "$WITH_QBITTORRENT" -eq 1 ]]; then
      ans="y"
    else
      read -r -p "Configure qBittorrent for torrents? (Web UI port $QBIT_WEBUI_PORT, systemd) [y/N] " ans
    fi
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      USE_QBITTORRENT=1
      if ! command -v qbittorrent-nox &>/dev/null && [[ -n "${PKG_INSTALL:-}" ]]; then
        read -r -p "qbittorrent-nox not installed. Install? [Y/n] " ians
        if [[ "${ians,,}" != "n" && "${ians,,}" != "no" ]]; then
          $PKG_UPDATE
          $PKG_INSTALL $PKG_QBIT
          ok "Installed qbittorrent-nox"
        fi
      fi
      if command -v qbittorrent-nox &>/dev/null; then
        info "qBittorrent: using admin/adminadmin for Web UI and .env (installer will write this into qBittorrent config). Change after first login in Settings → Web UI and update .env if needed."
      fi
    fi

    echo
    if [[ "$WITH_PROWLARR" -eq 1 ]]; then
      ans="y"
    elif [[ "$WITH_PROWLARR" == "0" ]]; then
      ans="n"
    else
      read -r -p "Install and configure Prowlarr (torrent search, AUR, port $PROWLARR_PORT)? [Y/n] " ans
    fi
    if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
      USE_PROWLARR=1
      PROWLARR_API_KEY_AUTO=$(install_prowlarr) || PROWLARR_API_KEY_AUTO=""
    fi
    echo
    if [[ "$WITH_MINIDLNA" -eq 1 ]]; then
      ans="y"
    else
      read -r -p "Install and configure minidlna for DLNA distribution? (port 8200) [y/N] " ans
    fi
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      USE_MINIDLNA=1
    fi

    collect_env "$ENV_FILE" "$USE_QBITTORRENT" "http://127.0.0.1:$QBIT_WEBUI_PORT" "$QBIT_USER" "$QBIT_PASS" "$USE_PROWLARR" "$PROWLARR_URL_AUTO" "$PROWLARR_API_KEY_AUTO"
  fi

  movie_path_owner=$(get_env_value "$ENV_FILE" "MOVIE_PATH")
  if [[ -n "$movie_path_owner" ]]; then
    mkdir -p "$movie_path_owner"
    mkdir -p "${movie_path_owner%/}/incomplete"
    chown -R "$TMS_RUN_USER:$TMS_RUN_USER" "$movie_path_owner"
    ok "MOVIE_PATH $movie_path_owner owned by $TMS_RUN_USER"
  fi

  if [[ $USE_QBITTORRENT -eq 1 || -n "$(get_env_value "$ENV_FILE" "QBITTORRENT_URL")" ]]; then
    QBIT_MOVIE_PATH=$(get_env_value "$ENV_FILE" "MOVIE_PATH")
    if [[ -n "$QBIT_MOVIE_PATH" ]]; then
      repair_qbittorrent_connection "$TMS_RUN_USER" "$ENV_FILE" "$QBIT_MOVIE_PATH"
    else
      warn "MOVIE_PATH not set in .env; set qBittorrent default save path manually in Web UI (Settings → Downloads)."
    fi
  fi

  install_tms_binary_and_service "$REPO_ROOT" "$TMS_RUN_USER"

  if [[ ${USE_MINIDLNA:-0} -eq 1 ]]; then
    if [[ -n "$movie_path_owner" ]]; then
      install_minidlna "$movie_path_owner" "$TMS_RUN_USER"
    else
      warn "MOVIE_PATH not set; skipping minidlna. Configure /etc/minidlna.conf manually (media_dir=V,<path>) and start minidlna."
    fi
  fi

  echo
  echo "=============================================="
  ok "Installation complete."
  echo "=============================================="
  echo "Config:     $ENV_FILE"
  echo "Service:    systemctl status telegram-media-server"
  echo "Logs:       journalctl -u telegram-media-server -f"
  if [[ $USE_QBITTORRENT -eq 1 ]]; then
    echo "qBittorrent: systemctl status $QBIT_SERVICE_NAME  |  Web UI: http://127.0.0.1:$QBIT_WEBUI_PORT (login: admin / adminadmin)"
  fi
  if [[ ${USE_PROWLARR:-0} -eq 1 ]]; then
    echo "Prowlarr:    systemctl status $PROWLARR_SERVICE_NAME  |  Web UI: http://127.0.0.1:$PROWLARR_PORT (add indexers manually)"
  fi
  if [[ ${USE_MINIDLNA:-0} -eq 1 ]]; then
    echo "minidlna:    systemctl status $MINIDLNA_SERVICE_NAME  |  DLNA on port 8200 (friendly_name=Telegram Media Server)"
  fi
  echo
}

main "$@"
