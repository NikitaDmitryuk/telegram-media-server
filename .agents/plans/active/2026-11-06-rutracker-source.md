---
kind: plan
status: active
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

Private snapshot of host/sources/proxies/tags was verified. Dedicated rutracker
tag and solver proxy are persisted with a 120-second timeout. Native source form
is prepared with .net and the same tag. The user entered credentials and clicked
Save; validation remains pending with a spinner, so inspect backend phases before
resubmitting or restarting anything. The persistent opt-in component is implemented and deployed. First creation
changed=1 and repeated apply changed=0; version/loopback/resource/restart checks
pass. Focused review findings on the declared volume, health version and metrics
were corrected and verified.

## Decisions

Keep RuTor as the explicitly managed public source. Treat authenticated RuTracker
as a manual source so its credentials never enter repository configuration.
Use only the existing VPN proxy; bind the helper exclusively to 127.0.0.1.
Saved sources need the shared solver tag: an unsaved test can otherwise work
while persisted search bypasses the solver. Do not infer bad credentials from
Cloudflare rejection before login.

## Verification

Anonymous control and .net login probes passed, including proxy/browser control,
no OOM and loopback binding. Persistent component apply/idempotence passed. Account/source acceptance remains pending while the submitted Save request
validates against Cloudflare; credentials were entered only in the local UI.
The existing recovery PR head 8a3be0c has all nine checks green; new component
changes require final verification and CI.

## Handoff

Keep the prepared browser form available for local account entry. Never copy
credentials, cookies, raw responses or private server addresses into context.
