---
kind: memory
status: active
date: 2026-08-29
last-verified: 2026-10-03
agent: codex
evidence: cmd/telegram-media-server/main.go, internal/app/app.go, internal/api/server.go, internal/config/config.go, ops/ansible/vpn.yml, scripts/setup-proxy, scripts/test_setup_proxy.py
---

# Repository map

## Composition and lifecycle

`cmd/telegram-media-server/main.go` is the composition root. It loads environment
configuration, opens the database, initializes localization, download and deletion
services, creates the Telegram bot and shared `app.App`, resumes incomplete work,
optionally starts the REST API, starts periodic updaters, routes Telegram updates,
and performs graceful shutdown on `SIGINT` or `SIGTERM`.

`internal/app/app.go` defines the dependency container passed to handlers. It owns
the bot, database, configuration, download manager, and deletion queue references.

## Package boundaries

- `internal/api`: optional HTTP server, versioned API routes, and embedded OpenAPI UI.
- `internal/bot` and `internal/handlers`: Telegram transport and update handlers.
- `internal/config`: environment-derived configuration and defaults.
- `internal/database`: persistent state access.
- `internal/downloader`: orchestration plus torrent, qBittorrent, and video backends.
- `internal/deletion` and `internal/filemanager`: deletion scheduling and file handling.
- `internal/models`: shared domain types.
- `internal/notifier`, `internal/prowlarr`, and `internal/tvcompat`: integrations and
  compatibility support.
- `internal/testutils`: shared test helpers.

## Optional subscription VPN

The Arch Linux Ansible installer optionally manages a separate loopback Xray
client and hourly systemd updater; Go owns neither the VPN process nor subscription
credentials. `scripts/setup-proxy` consumes the first subscription profile,
validates a candidate with Xray, atomically replaces changed configs, and rolls back
startup failures. Cached configs survive fetch failures and offline boots.
Ansible verifies proxy HTTPS before replacing TMS proxy settings. Telegram always
uses the configured proxy; yt-dlp retains the configured domain selection. The
shared Telegram proxy setting also feeds optional OpenClaw configuration.
`make deploy` remains binary-only. Runtime acceptance requires a target-host check;
local conversion and rollback tests do not prove VPN connectivity.

## REST API security invariant

`internal/api/server.go` distinguishes local or Docker-host callers from remote
callers. Local access can be allowed without an API key. Remote requests must not
gain access by silently bypassing the configured API-key policy; when no remote key
is configured, non-local requests are rejected.
