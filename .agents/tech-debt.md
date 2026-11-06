---
kind: ledger
status: active
date: 2026-08-29
last-verified: 2026-11-06
agent: codex
evidence: .agents/README.md, internal/downloader/manager/concurrent_test.go, internal/downloader/manager/first_episode_test.go, internal/logutils/logutils.go
---

# Agent-verified technical debt

Only evidence-backed debt belongs here. Each entry needs a stable identifier, impact,
repository evidence, lifecycle status, and an observable exit condition.

| ID | Status | Impact | Evidence | Exit condition |
| --- | --- | --- | --- | --- |
| TD-001 | open | A Telegram bot credential may remain valid after appearing in historical startup diagnostics; future bot errors are now redacted. | `internal/bot/bot.go`, `internal/bot/bot_test.go` | Rotate the Telegram bot token, update the managed secret, redeploy successfully, and verify authentication with the replacement token. |
| TD-002 | open | The whole manager race suite can fail because a monitor from an earlier test races with global logger reinitialization; this weakens concurrency verification. | Reproduced at immutable `3c7a44f` with `go test -race -short ./internal/downloader/manager`; `concurrent_test.go`, `first_episode_test.go`, `logutils.go` | Ensure test monitor goroutines terminate before logger reinitialization (or synchronize logger configuration), then pass the whole manager race suite. |
