---
kind: memory
status: active
date: 2026-11-06
last-verified: 2026-11-06
agent: codex
evidence: internal/prowlarr/api.go, internal/prowlarr/api_test.go, internal/handlers/session/torrent_search_test.go, internal/api/server_test.go, scripts/setup_prowlarr.py, scripts/test_setup_prowlarr.py, ops/ansible/prowlarr-setup.yml, ops/ansible/prowlarr-setup-tasks.yml, ops/ansible/site.yml
---

# Prowlarr search readiness

A healthy Prowlarr process and HTTP 200 search do not prove configured search:
an installation with zero indexers returns an empty array. The shared client
checks enabled searchable torrent indexers only after an empty search, respects
selected indexer IDs and propagates network/authentication failures separately.
ErrNoUsableIndexers becomes a localized bot setup message and REST HTTP 503 with
the existing error object; genuine empty results stay HTTP 200 with []. Positive
searches do not need an additional indexer request.

Proxy/indexer management is explicitly opt-in: prowlarr_manage_proxy and
prowlarr_managed_indexers (currently only rutor). The existing-installation
component reads the local Prowlarr key and existing TMS HTTP proxy without
upgrading packages or restarting services. The full installer shares the same
setup/readiness tasks. Unmanaged sources and unrelated settings are preserved;
multiple existing RuTor definitions cause a safe failure rather than deletion.

Before writes, setup verifies a unique private owner-only settings snapshot
(host, sources, app profiles); snapshots may contain credentials and must never
be committed or printed. The helper updates proxy fields with local-address
bypass, refreshes the installed version's official catalog, tests RuTor before
persistence and checks it again afterwards. Cyrillic stripping is disabled.
Repeated applies retain existing user priorities and do not create duplicates.
An apply may still create a backup and issue a catalog refresh when no managed
settings change. Removing the opt-in does not delete configured sources.

Prowlarr 2.5.2.5491 uses definition version 11. Its catalog updater catches
upstream download failures internally: completed command status alone does not
prove freshness. Require a usable official schema and real source tests, and
check catalog access separately when diagnosing failures. On this host the
catalog was unavailable directly but accessible through the existing VPN proxy;
RuTor worked through that proxy, while NoNaMe Club returned HTTP 403 and was not
selected. These external-access observations must be rechecked if routing,
provider availability or installed Prowlarr version changes.

Live first creation passed; the second component apply reported changed=0 with
one enabled searchable RuTor and no duplicates. Offline provisioning tests
verify that unrelated manually configured sources and host fields survive.

Live acceptance through Prowlarr and the deployed REST API returned 100 results
for each of two Russian movie queries and HTTP 200 [] for a random negative
query. The real bot handler, using an isolated captured transport, produced
result cards and the localized negative-result message without downloads or
Telegram sends. Read-only Telegram getMe through the configured proxy passed;
the user subsequently confirmed that search results appeared in Telegram. Binary fingerprint matched the
local build; server env, routes and qBittorrent config fingerprints and installed
Prowlarr version were unchanged. TMS/DLNA remained active and the accepted
Full HD TS test file remained present.
