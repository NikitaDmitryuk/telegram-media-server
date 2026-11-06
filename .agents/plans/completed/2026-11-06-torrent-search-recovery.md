---
kind: plan
status: completed
date: 2026-11-06
last-verified: 2026-11-06
agent: codex
evidence: internal/prowlarr/api.go, internal/handlers/session/torrent_search.go, internal/api/handlers.go, ops/ansible/site.yml, ops/ansible/test.yml
---

# Restore configured torrent search

## Goal

Restore Russian public torrent search through the existing managed VPN proxy,
and distinguish missing search sources from a genuine empty result.

## Acceptance Criteria

RuTor searches for two common Russian movie titles return results through
Prowlarr and TMS API/bot handling; a nonexistent title returns a genuine empty
result. No diagnostic download is started. Reapplying configuration creates no
duplicates or unrelated changes. Existing LG playback fixes stay deployed.

## Evidence

Prowlarr 2.5.2.5491 is running with valid API access but zero indexers; all four
examined scheduled backups are empty too. Control search returns HTTP 200 and
an empty array. The definition service returns HTTP 403 directly and HTTP 200
through the current HTTP proxy. RuTor times out directly but through the proxy
returns 100 matching release/magnet links for each control movie and zero for a
random negative query. NoNaMe Club returns HTTP 403 through the proxy and is not
selected. No credentials or raw production data belong in this record.

## Tasks

1. Back up settings, configure Prowlarr proxy and provision RuTor idempotently
   through its supported API and official definitions.
2. Return ErrNoUsableIndexers after an empty search with no enabled/searchable
   torrent sources; map it to a localized bot message and HTTP 503.
3. Cover client/handler/API and provisioning failure/idempotence cases, run checks
   and a cohesive review; update context and user/operator documentation.
4. Deploy together with current compatibility fixes, verify real positive and
   negative searches without downloads, and achieve green CI.

## Progress

All implementation, live deployment and acceptance work is complete. Built-in Prowlarr backup
was verified before enabling the existing HTTP proxy. The catalog became
available; first component apply created and tested RuTor successfully.
Independent cohesive review found no actionable blocker.

## Decisions

Use public RuTor without credentials, preserve Cyrillic and current query/ranking
behavior. Proxy management and managed-indexer list are explicit opt-ins; leave
manually configured sources and unrelated settings intact. Source readiness is
required only where managed sources are requested. Preserve existing public JSON
error shape and real empty-result semantics. No Prowlarr upgrade or qBittorrent /
network-route change is part of this work.

## Verification

Go 1.26 make check, integration tests, focused race tests for Prowlarr/session/API,
16 Python provisioning tests, whitespace and both Ansible syntax checks pass.
Live repeated component apply changed=0 with one searchable RuTor. Deployed
binary and translations match local artifacts; Prowlarr/API return 100 results
for Матрица and Дюна and zero for a random negative query. Captured real bot
handler returns cards or localized not-found without downloads/Telegram sends.
Telegram getMe passed; the user confirmed that results appeared in the bot.
Environment/routes/qBittorrent configuration and Prowlarr version are unchanged;
TMS and DLNA are active and the working Full HD TS remains present. All nine
GitHub checks passed on runtime revision 1b1bc46 in PR #177, including Docker,
CodeQL and coverage. Completion documentation receives its own final CI run.
Catalog command completion alone is not proof of a fresh download because
upstream catches download errors; verify live results and catalog access separately.

## Handoff

Runtime revision 1b1bc46 is deployed together with LG compatibility fixes.
Use the Prowlarr component playbook for future source setup; full installation
is unnecessary for an existing host. Backups remain private for recovery.
Keep credentials and raw live receipts outside version-controlled context.
