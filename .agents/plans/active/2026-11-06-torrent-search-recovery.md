---
kind: plan
status: active
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

Application and provisioning changes are implemented. Built-in Prowlarr backup
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
Repeated live apply, deployment, live search acceptance and CI remain pending.
Catalog command completion alone is not proof of a fresh download because
upstream catches download errors; verify live results and catalog access separately.

## Handoff

Continue from current deployed compatibility branch; do not roll back the LG fix.
Keep backup credentials and live receipts outside version-controlled context.
