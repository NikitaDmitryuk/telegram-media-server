#!/usr/bin/env bash
#
# install.sh — Interactive installer for Telegram Media Server (Arch Linux only)
#
# Installs binary, systemd service, and .env config. Menu options:
# — qBittorrent: install from pacman, systemd, port 8081, login/password in .env;
# — Prowlarr: install from AUR (yay/paru), systemd, port 9696, API key from config.xml into .env.
# Indexers in Prowlarr are added manually (web UI).
# With existing .env: by default only offers to update binary and service (config untouched).
# Answer "n" to be prompted only for missing parameters (existing values not overwritten).
# Run: sudo ./scripts/install.sh  or  sudo make install
#
set -euo pipefail

# --- constants (match Makefile) ---
BINARY_NAME=telegram-media-server
INSTALL_DIR=/usr/local/bin
CONFIG_DIR=/etc/telegram-media-server
SERVICE_DIR=/usr/lib/systemd/system
LOCALES_SRC=locales
LOCALES_DEST=/usr/local/share/telegram-media-server/locales
QBIT_SERVICE_NAME=qbittorrent-nox
QBIT_WEBUI_PORT=8081
PROWLARR_SERVICE_NAME=prowlarr
PROWLARR_PORT=9696
PROWLARR_DATA_PATHS="/var/lib/prowlarr /etc/prowlarr"

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

# --- check root ---
check_root() {
  if [[ $EUID -ne 0 ]]; then
    err "This script must be run as root. Use: sudo $0"
    exit 1
  fi
}

# --- Arch Linux only ---
check_arch() {
  if [[ ! -f /etc/arch-release ]] || ! command -v pacman &>/dev/null; then
    err "This installer supports Arch Linux only."
    err "Other OS detected. Use manual install: make install and configure .env from .env.example"
    exit 1
  fi
}

# --- Arch packages (pacman) ---
set_arch_packages() {
  PKG_UPDATE="true"
  PKG_INSTALL="pacman -S --noconfirm"
  PKG_QBIT=qbittorrent-nox
  PKG_GO=go
  PKG_ARIA2=aria2
  PKG_FFMPEG=ffmpeg
  PKG_YTDLP=yt-dlp
}

# --- check dependencies; optionally install missing ---
check_and_install_deps() {
  local need_go=0 need_aria2=0 need_ffmpeg=0 need_ytdlp=0 need_qbit=0
  command -v go      &>/dev/null || need_go=1
  command -v aria2c  &>/dev/null || need_aria2=1
  command -v ffmpeg  &>/dev/null || need_ffmpeg=1
  command -v yt-dlp  &>/dev/null || need_ytdlp=1

  if [[ $need_go -eq 1 ]] || [[ $need_aria2 -eq 1 ]] || [[ $need_ffmpeg -eq 1 ]] || [[ $need_ytdlp -eq 1 ]]; then
    warn "Some dependencies are missing:"
    [[ $need_go -eq 1 ]]    && echo "  - go (for build)"
    [[ $need_aria2 -eq 1 ]] && echo "  - aria2 (torrents, unless using qBittorrent)"
    [[ $need_ffmpeg -eq 1 ]] && echo "  - ffmpeg"
    [[ $need_ytdlp -eq 1 ]] && echo "  - yt-dlp"
    echo
    read -r -p "Install missing packages via pacman? [y/N] " ans
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      $PKG_UPDATE
      [[ $need_go -eq 1 ]]    && $PKG_INSTALL $PKG_GO
      [[ $need_aria2 -eq 1 ]] && $PKG_INSTALL $PKG_ARIA2
      [[ $need_ffmpeg -eq 1 ]] && $PKG_INSTALL $PKG_FFMPEG
      [[ $need_ytdlp -eq 1 ]] && $PKG_INSTALL $PKG_YTDLP
    else
      err "Install packages manually (go, aria2, ffmpeg, yt-dlp) and run the script again."
      exit 1
    fi
  fi

  if ! command -v yt-dlp &>/dev/null; then
    err "yt-dlp not found. Install: https://github.com/yt-dlp/yt-dlp#installation"
    exit 1
  fi
  ok "Dependencies: go, aria2, ffmpeg, yt-dlp — OK."
}

# --- read key value from .env (single line, no export) ---
get_env_value() {
  local f="$1" k="$2"
  [[ ! -f "$f" ]] && return 0
  grep -E "^${k}=" "$f" 2>/dev/null | sed -n "s/^${k}=//p" | head -1
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

# --- interactive input with password masking ---
read_secret() {
  local name="$1"
  local min_len="${2:-1}"
  local val=""
  while true; do
    read -r -s -p "$name: " val
    echo
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

  info "Enter required parameters (passwords are hidden)."
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
      mkdir -p "$movie_path"
      chown "$SUDO_UID":"$SUDO_GID" "$movie_path" 2>/dev/null || true
      ok "Created directory $movie_path"
    fi
  fi

  echo
  local admin_pass
  admin_pass=$(read_secret "ADMIN_PASSWORD (min 8 characters)" 8)

  local regular_pass
  regular_pass=$(read_value "REGULAR_PASSWORD (leave empty to use same as admin)")

  local lang
  lang=$(read_value "LANG (en/ru)" "en")

  # Build .env
  cat > "$env_file" << ENVEOF
# Generated by install.sh — Telegram Media Server
# REQUIRED
BOT_TOKEN=$bot_token
MOVIE_PATH=$movie_path
ADMIN_PASSWORD=$admin_pass
ENVEOF
  [[ -n "$regular_pass" ]] && echo "REGULAR_PASSWORD=$regular_pass" >> "$env_file"
  echo "LANG=$lang" >> "$env_file"
  echo "" >> "$env_file"

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

  # Optional: Prowlarr (when use_prowlarr=1 and key present; else ask manually)
  echo ""
  if [[ "${6:-0}" -eq 1 && -n "${8:-}" ]]; then
    echo "# Prowlarr (installed by installer)" >> "$env_file"
    echo "PROWLARR_URL=${7:-http://127.0.0.1:9696}" >> "$env_file"
    echo "PROWLARR_API_KEY=${8}" >> "$env_file"
    echo "" >> "$env_file"
  elif [[ "${6:-0}" -eq 1 ]]; then
    echo "# Prowlarr (installed; add API Key manually: Settings → General → Security)" >> "$env_file"
    echo "PROWLARR_URL=${7:-http://127.0.0.1:9696}" >> "$env_file"
    echo "PROWLARR_API_KEY=" >> "$env_file"
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

  read -r -p "Set TMS_API_KEY for REST API access from other machines? [y/N] " ans
  if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
    local api_key
    api_key=$(openssl rand -hex 32 2>/dev/null || head -c 32 /dev/urandom | xxd -p -c 32)
    echo "TMS_API_KEY=$api_key" >> "$env_file"
    ok "TMS_API_KEY generated and written to .env"
    echo "" >> "$env_file"
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
      [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]] && mkdir -p "$movie_path" && chown "${SUDO_UID:-0}:${SUDO_GID:-0}" "$movie_path" 2>/dev/null || true
    fi
  else
    ok "MOVIE_PATH already set: $movie_path"
  fi

  if is_placeholder "$admin_pass" "ADMIN_PASSWORD"; then
    admin_pass=$(read_secret "ADMIN_PASSWORD (min 8 characters)" 8)
  else
    ok "ADMIN_PASSWORD already set."
  fi

  if is_placeholder "$regular_pass" "REGULAR_PASSWORD"; then
    regular_pass=$(read_value "REGULAR_PASSWORD (empty = same as admin)")
  fi
  if [[ -z "$lang" ]]; then
    lang=$(read_value "LANG (en/ru)" "en")
  else
    ok "LANG already set: $lang"
  fi

  # Overlay only these keys onto existing .env
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
      echo "${key}=${val}" >> "${env_file}.tmp"
      mv "${env_file}.tmp" "$env_file"
    else
      echo "${key}=${val}" >> "$env_file"
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
    warn "File $unit_path already exists. Skipping creation."
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

# --- install Prowlarr from AUR, start service, extract API key from config.xml ---
# Returns API key via echo (empty string on error).
install_prowlarr_arch() {
  local run_user
  run_user=$(logname 2>/dev/null || echo "${SUDO_USER:-}")
  if [[ -z "$run_user" || "$run_user" == root ]]; then
    err "AUR install requires a non-root user (run: sudo -u your_user or set SUDO_USER)."
    return 1
  fi

  local aur_helper=""
  for h in yay paru; do
    if sudo -u "$run_user" command -v "$h" &>/dev/null; then
      aur_helper="$h"
      break
    fi
  done
  if [[ -z "$aur_helper" ]]; then
    warn "AUR helper (yay or paru) not found. Install one and run the installer again."
    return 1
  fi

  info "Installing Prowlarr from AUR (${aur_helper})..."
  if ! sudo -u "$run_user" "$aur_helper" -S prowlarr-bin --noconfirm --needed 2>/dev/null; then
    warn "Failed to install prowlarr-bin. Try manually: $aur_helper -S prowlarr-bin"
    return 1
  fi
  ok "Prowlarr installed."

  systemctl daemon-reload
  systemctl enable "$PROWLARR_SERVICE_NAME"
  systemctl start "$PROWLARR_SERVICE_NAME" 2>/dev/null || true
  info "Waiting for Prowlarr first start (up to 60s)..."
  local config_path=""
  for i in {1..30}; do
    sleep 2
    for base in $PROWLARR_DATA_PATHS; do
      if [[ -f "$base/config.xml" ]]; then
        config_path="$base/config.xml"
        break 2
      fi
    done
    # trigger config generation by first request
    curl -s -o /dev/null "http://127.0.0.1:${PROWLARR_PORT}" 2>/dev/null || true
  done
  if [[ -z "$config_path" || ! -f "$config_path" ]]; then
    warn "Prowlarr config.xml not found. Open http://127.0.0.1:${PROWLARR_PORT} once, then add API Key to .env manually (Settings → General → Security)."
    echo ""
    return 0
  fi
  local api_key
  api_key=$(sed -n 's/.*<ApiKey>\([^<]*\)<\/ApiKey>.*/\1/p' "$config_path" 2>/dev/null | head -1)
  if [[ -n "$api_key" ]]; then
    echo "$api_key"
    ok "Prowlarr API Key read from $config_path"
  else
    warn "API Key not in config.xml yet. Open http://127.0.0.1:${PROWLARR_PORT}, then add key to .env (Settings → General → Security)."
    echo ""
  fi
  return 0
}

# --- main TMS installer (binary + unit + locales) ---
install_tms_binary_and_service() {
  local repo_root="${1:?}"
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

  systemctl daemon-reload
  systemctl enable telegram-media-server
  systemctl restart telegram-media-server
  ok "telegram-media-server service installed and started."
}

# --- main ---
main() {
  echo "=============================================="
  echo "  Telegram Media Server — installer"
  echo "=============================================="
  echo

  check_root
  check_arch
  set_arch_packages

  echo
  echo -e "${YELLOW}Warning: default passwords are used (e.g. qBittorrent Web UI: admin/adminadmin).${NC}"
  echo "Do not use this installer on an internet-exposed server without changing passwords."
  echo
  read -r -p "Continue with installation? [y/N] " ans
  if [[ "${ans,,}" != "y" && "${ans,,}" != "yes" ]]; then
    echo "Exiting."
    exit 0
  fi
  echo

  check_and_install_deps

  REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
  if [[ ! -f "$REPO_ROOT/cmd/telegram-media-server/main.go" ]]; then
    err "Repository root not found (expected cmd/telegram-media-server/main.go). Run from project directory."
    exit 1
  fi

  mkdir -p "$CONFIG_DIR"
  ENV_FILE="${CONFIG_DIR}/.env"

  USE_QBITTORRENT=0
  QBIT_USER="admin"
  QBIT_PASS="adminadmin"
  USE_PROWLARR=0
  PROWLARR_URL_AUTO="http://127.0.0.1:9696"
  PROWLARR_API_KEY_AUTO=""

  # Upgrade mode: .env exists — offer to only update binary
  if [[ -f "$ENV_FILE" ]]; then
    echo
    info "Existing installation detected ($ENV_FILE present)."
    read -r -p "Update only binary and service (leave config unchanged)? [Y/n] " ans
    if [[ "${ans,,}" != "n" && "${ans,,}" != "no" ]]; then
      bash "$REPO_ROOT/scripts/merge-env.sh" "$ENV_FILE" "$REPO_ROOT/.env.example"
      install_tms_binary_and_service "$REPO_ROOT"
      echo
      ok "Done. Config was not changed."
      systemctl status telegram-media-server --no-pager 2>/dev/null || true
      exit 0
    fi
    # User wants to change something — prompt only for what is missing
    info "Only missing values will be prompted (existing values are kept)."
    echo
    EXISTING_QBIT=$(get_env_value "$ENV_FILE" "QBITTORRENT_URL")
    EXISTING_PROWLARR_URL=$(get_env_value "$ENV_FILE" "PROWLARR_URL")
    EXISTING_PROWLARR_KEY=$(get_env_value "$ENV_FILE" "PROWLARR_API_KEY")
    if [[ -z "$EXISTING_QBIT" ]]; then
      read -r -p "Configure qBittorrent? (port $QBIT_WEBUI_PORT, systemd) [y/N] " ans
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
          echo "qBittorrent Web UI login and password (default admin/adminadmin)."
          QBIT_USER=$(read_value "qBittorrent login" "admin")
          read -r -p "Generate random password? [Y/n] " gen_ans
          if [[ "${gen_ans,,}" != "n" && "${gen_ans,,}" != "no" ]]; then
            QBIT_PASS=$(openssl rand -base64 16 2>/dev/null | tr -dc 'A-Za-z0-9' | head -c 16)
            [[ -z "$QBIT_PASS" ]] && QBIT_PASS=$(head -c 16 /dev/urandom | xxd -p | tr -d '\n' | head -c 16)
            echo "Password generated (save it): $QBIT_PASS"
          else
            QBIT_PASS=$(read_secret "qBittorrent password" 1)
            [[ -z "$QBIT_PASS" ]] && QBIT_PASS="adminadmin"
          fi
        fi
      fi
    else
      ok "qBittorrent already configured (QBITTORRENT_URL set)."
    fi

    if [[ -z "$EXISTING_PROWLARR_URL" && -z "$EXISTING_PROWLARR_KEY" ]]; then
      read -r -p "Install and configure Prowlarr? (AUR, port $PROWLARR_PORT) [y/N] " ans
      if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
        USE_PROWLARR=1
        PROWLARR_API_KEY_AUTO=$(install_prowlarr_arch)
      fi
    else
      ok "Prowlarr already configured (PROWLARR_* set)."
    fi

    bash "$REPO_ROOT/scripts/merge-env.sh" "$ENV_FILE" "$REPO_ROOT/.env.example"
    fill_missing_env "$ENV_FILE" "$USE_QBITTORRENT" "http://127.0.0.1:$QBIT_WEBUI_PORT" "$QBIT_USER" "$QBIT_PASS" "$USE_PROWLARR" "$PROWLARR_URL_AUTO" "$PROWLARR_API_KEY_AUTO"
  else
    # New install: full dialog
    echo
    read -r -p "Configure qBittorrent for torrents? (Web UI port $QBIT_WEBUI_PORT, systemd) [y/N] " ans
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
        echo "qBittorrent Web UI login and password (default admin/adminadmin — change on exposed servers)."
        QBIT_USER=$(read_value "qBittorrent login" "admin")
        read -r -p "Generate random password for qBittorrent? [Y/n] " gen_ans
        if [[ "${gen_ans,,}" != "n" && "${gen_ans,,}" != "no" ]]; then
          QBIT_PASS=$(openssl rand -base64 16 2>/dev/null | tr -dc 'A-Za-z0-9' | head -c 16)
          [[ -z "$QBIT_PASS" ]] && QBIT_PASS=$(head -c 16 /dev/urandom | xxd -p | tr -d '\n' | head -c 16)
          echo "Password generated (save it): $QBIT_PASS"
        else
          QBIT_PASS=$(read_secret "qBittorrent password" 1)
          [[ -z "$QBIT_PASS" ]] && QBIT_PASS="adminadmin"
        fi
      fi
    fi

    echo
    read -r -p "Install and configure Prowlarr (torrent search, AUR, port $PROWLARR_PORT)? [y/N] " ans
    if [[ "${ans,,}" == "y" || "${ans,,}" == "yes" ]]; then
      USE_PROWLARR=1
      PROWLARR_API_KEY_AUTO=$(install_prowlarr_arch)
    fi

    collect_env "$ENV_FILE" "$USE_QBITTORRENT" "http://127.0.0.1:$QBIT_WEBUI_PORT" "$QBIT_USER" "$QBIT_PASS" "$USE_PROWLARR" "$PROWLARR_URL_AUTO" "$PROWLARR_API_KEY_AUTO"
  fi

  install_tms_binary_and_service "$REPO_ROOT"

  if [[ $USE_QBITTORRENT -eq 1 ]]; then
    install_qbittorrent_systemd ""
  fi

  echo
  echo "=============================================="
  ok "Installation complete."
  echo "=============================================="
  echo "Config:     $ENV_FILE"
  echo "Service:    systemctl status telegram-media-server"
  echo "Logs:       journalctl -u telegram-media-server -f"
  if [[ $USE_QBITTORRENT -eq 1 ]]; then
    echo "qBittorrent: systemctl status $QBIT_SERVICE_NAME  |  Web UI: http://127.0.0.1:$QBIT_WEBUI_PORT"
  fi
  if [[ ${USE_PROWLARR:-0} -eq 1 ]]; then
    echo "Prowlarr:    systemctl status $PROWLARR_SERVICE_NAME  |  Web UI: http://127.0.0.1:$PROWLARR_PORT (add indexers manually)"
  fi
  echo
}

main "$@"
