---
kind: memory
status: active
date: 2026-08-29
last-verified: 2026-11-06
agent: codex
evidence: Makefile, .pre-commit-config.yaml, .github/workflows/ci.yml, scripts/test_setup_proxy.py, ops/ansible/site.yml, ops/ansible/vpn.yml, ops/ansible/templates/telegram-media-server-vpn-update.service.j2, internal/bot/bot_test.go
---

# Verification commands

The Makefile and CI define the supported verification surface.

| Command | Purpose |
| --- | --- |
| `make agent-context-check` | Unit-test and run the repository-context validator. |
| `make lint` | Run golangci-lint. |
| `make vet` | Run `go vet` across all packages. |
| `make test-unit` | Run unit tests with race detection and coverage. |
| `make test-proxy` | Verify VPN subscription conversion, update serialization, rollback, and real Xray config validation when installed. |
| `make ansible-check` | Syntax-check install, deploy, and remote-test playbooks; does not deploy. |
| `make test-integration` | Run integration-tagged Go tests. |
| `make test-docker` | Run Docker-tagged tests. |
| `make test-coverage` | Generate the HTML coverage report. |
| `make build-simple` | Build the server binary locally. |
| `make check` | Run the normal local verification suite. |
| `make pre-commit-run` | Execute every configured pre-commit hook. |

For documentation-only changes, the context validator and relevant formatting or
link checks are sufficient. For Go behavior changes, run `make check` at minimum;
add integration, Docker, build, or targeted tests according to the affected path.

For VPN installer changes, run `make test-proxy` and `make ansible-check`. On an
existing host, `site.yml --tags vpn,deploy` configures the VPN and dependent TMS /
OpenClaw settings without reinstalling unrelated components. Before changing an
existing client, preserve its configuration and resolve local port conflicts.
Live acceptance checks first-profile matching, Telegram through the proxy, an
actual yt-dlp transfer, failed-refresh cache preservation, unchanged-refresh
restart avoidance, cached client restart, and boot enablement. Cached restart
and valid systemd units do not establish that a full host reboot was tested.

The VPN refresh service retries failures every 30 seconds without a start-rate
limit, so boot-time network/DNS failures do not defer recovery until the hourly
timer. Disabling managed VPN must stop the update service as well as its timer
and client: stopping only the timer does not cancel service restart attempts.

VPN HTTPS, Telegram `getMe`, and REST health checks do not establish that bot
commands can send replies. Acceptance must include `/start` and the movie-list
response: an invalid UTF-8 movie name can make Telegram reject the whole list
even while polling and API health remain healthy. `internal/bot/bot_test.go`
checks outgoing message encoding at the HTTP form boundary.

On 2026-10-03, home-server acceptance passed: first-profile matching, Telegram
`getMe`, a real YouTube video/audio transfer, preserved config after a failed
refresh, unchanged-refresh restart avoidance, and an idempotent repeated deploy
(`changed=0`). A full host reboot restored the VPN, timer, TMS and the existing
media services; the boot refresh succeeded, and post-reboot Telegram and TMS
health checks passed.
