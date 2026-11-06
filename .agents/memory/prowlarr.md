---
kind: memory
status: active
date: 2026-11-06
last-verified: 2026-11-06
agent: codex
evidence: internal/prowlarr/api.go, internal/prowlarr/api_test.go, internal/handlers/session/torrent_search_test.go, internal/api/server_test.go, scripts/setup_prowlarr.py, scripts/test_setup_prowlarr.py, ops/ansible/prowlarr-setup.yml, ops/ansible/prowlarr-setup-tasks.yml, ops/ansible/site.yml, ops/ansible/flaresolverr.yml
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


## Authenticated RuTracker and Cloudflare

Native RuTracker requires account credentials entered directly in Prowlarr, not
repository variables. For this VPN exit, .org/.net native login requests returned
Cloudflare HTTP 403 and direct mirrors timed out. Official FlareSolverr 3.5.2
opened the .net login form through the same proxy in 80 seconds; a 60-second probe
failed, while a 120-second probe and cookie/User-Agent replay passed. Anonymous
form access is not account authentication, and challenge success can change.

The optional flaresolverr.yml component is explicitly managed with
prowlarr_manage_flaresolverr. It pins the image digest, uses host networking to
reach the existing loopback VPN proxy, binds HTTP only to 127.0.0.1:8191 and
disables Prometheus (otherwise it can listen publicly on 8192). No host binds or
privileged mode are used. The image itself declares one anonymous /config volume;
rejecting all mounts would break repeat applies. The actual root health version
is 3.5.2 without a v prefix. First live creation and repeat apply passed with
changed=1 and changed=0, respectively. Docker is enabled and restart policy is
unless-stopped; a full host reboot is a separate check.

Configure a FlareSolverr indexer proxy with a 120-second timeout and the same
rutracker tag on the saved native source, .net base URL and Cyrillic stripping
disabled. Prowlarr forwards the global VPN proxy. Saved sources without a matching
tag skip the solver even if an unsaved test succeeded. IndexerProxy API resources
have no enable field: validate persisted tags/host/timeout and the proxy test.
Keep authenticated RuTracker manual; managed RuTor setup preserves it.

The initial 1 GiB/1 CPU/256-PID solver bounds caused cgroup PID denials during
native authentication. Raising this owned container to 2 GiB/1.5 CPU/512 PIDs
allowed Save to complete; the denial counter stopped increasing. These are tested
host settings, not universal minimum requirements. The saved authenticated source
returned eight results for Пацаны 2 сезон кубик в кубе; combined REST searches
returned eight for that query, 150 each for Матрица and Дюна and zero for a random
negative query. The actual bot handler with captured transport passed the same
8/150/150/0 controls, and the user confirmed the exact requested releases appeared
in Telegram. Managed RuTor repeat apply preserved the full private RuTracker
configuration hash with exactly two enabled sources. Solver repeat applies before
and after an explicit container restart reported changed=0; post-restart health,
loopback binding and resource/restart settings passed. Diagnostics only searched
and did not start downloads.

Queries are forwarded literally. A natural phrase with 2 сезон returned zero
RuTor results while Пацаны S02 кубик returned nine releases, including the requested
season and voice translation. This is independent of source readiness; automatic
season-query normalization is not implemented.
