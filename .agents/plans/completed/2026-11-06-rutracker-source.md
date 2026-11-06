---
kind: plan
status: completed
date: 2026-11-06
last-verified: 2026-11-06
agent: codex
evidence: internal/prowlarr/api.go, ops/ansible/prowlarr-setup.yml, ops/ansible/flaresolverr.yml, .agents/memory/prowlarr.md
---

# Add authenticated RuTracker search

## Goal

Connect the native RuTracker source requested after RuTor recovery, using account
entry directly in Prowlarr and preserving the existing VPN, routes and downloads.

## Acceptance Criteria

A saved enabled native RuTracker passes authentication/source testing and real
search through Prowlarr/TMS. Its solver uses the existing VPN proxy, listens only
on loopback and survives restart. Reapplying managed RuTor setup preserves the
manual private source. No diagnostic downloads or credential publication.

## Evidence

RuTor literal search for Пацаны 2 сезон кубик в кубе returns zero; Пацаны S02 кубик
returns nine second-season releases. This is a query/title notation difference,
not absent source configuration. Native RuTracker login through the same VPN
returns HTTP 403/Cloudflare; all three direct mirrors timed out.

Official FlareSolverr 3.5.2 in a bounded loopback-only container opened the .net
login form in 80 seconds with no remaining challenge. The same VPN proxy plus
solver cookies/User-Agent replayed that form with HTTP 200. A 60-second first
probe failed; the successful probe allowed 120 seconds. This proves anonymous
access, not account authentication. No alternate VPN or host route was selected.

## Tasks

1. Preserve private Prowlarr state before source/proxy/tag writes.
2. Configure tagged FlareSolverr and prepare native RuTracker .net with Cyrillic
   stripping disabled; the user enters account credentials locally.
3. Make the pinned loopback solver container persistent and reproducible.
4. Verify saved-source authentication, actual queries, restart/idempotence and CI.

## Progress

The source is saved and enabled after account entry directly in Prowlarr. Initial
solver bounds caused cgroup PID denials; increasing its limits to 2 GiB, 1.5 CPU
and 512 PIDs allowed validation to finish without further denials. Native RuTracker
and deployed REST both return eight results for Пацаны 2 сезон кубик в кубе.
Combined REST returns 150 each for Матрица and Дюна, and HTTP 200 [] for a random
negative query. No downloads were started.

The pinned loopback component is deployed with restart policy unless-stopped.
A CI coverage run exposed a separate fast-exit stderr race in yt-dlp monitoring;
the reviewed fix drains diagnostics before Wait and makes cancellation close the
reader before reaping, including inherited stderr. Regression, race, coverage,
make check, integration and Ansible checks pass locally.

## Decisions

Keep RuTor as the explicitly managed public source. Treat authenticated RuTracker
as a manual source so its credentials never enter repository configuration.
Use only the existing VPN proxy; bind the helper exclusively to 127.0.0.1.
Saved sources need the shared solver tag: an unsaved test can otherwise work
while persisted search bypasses the solver. Do not infer bad credentials from
Cloudflare rejection before login.

## Verification

Saved native RuTracker authentication and exact-query search passed. Captured real
bot handlers returned 8 requested releases, 150 each for Матрица and Дюна, and
zero plus the localized not-found message for a random query. The user confirmed
the requested releases also appeared in the real Telegram bot.

Managed RuTor repeat apply reported changed=0 and preserved the full private native
RuTracker configuration hash, with exactly two enabled sources. Solver repeat
applies before and after its explicit restart reported changed=0. After restart,
health 3.5.2, loopback binding, resource limits and restart policy passed. A full
host reboot was not performed.

Go 1.26 make check, integration, exact CI coverage, build, focused race tests and
Ansible/context checks passed. CI Test, Build and security checks passed for the
runtime commit. The deployed binary fingerprint matches the local build; Telegram
getMe, TMS, DLNA and the accepted Full HD TS remain healthy. Server environment,
routes, qBittorrent configuration and Prowlarr version match the pre-change receipt.
No diagnostic downloads were started.

## Handoff

PR #177 contains the implementation and final CI checks. Keep the connected
Prowlarr tab available. Never copy credentials, cookies, raw
responses or private server addresses into context.
