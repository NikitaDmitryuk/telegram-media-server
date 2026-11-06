---
kind: plan
status: completed
date: 2026-11-06
last-verified: 2026-11-06
agent: codex
evidence: internal/tvcompat/tvcompat.go, internal/downloader/manager/manager.go, internal/downloader/manager/monitor.go, internal/downloader/video/ytdlp.go, internal/database/files.go
---

# Verified video preparation for older LG TVs

## Goal

Prepare downloaded MP4 video with working audio for an older Full HD LG over
DLNA. Preserve originals and report conversion failures instead of false success.

## Evidence and scope

- The former compatibility probe accepted H.264 without checking audio, frame
  rate, profile, pixel format or container. The worker skipped preparation for
  green entries, and conversion errors could not propagate to completion notices.
- The user confirmed playback of a separate sample encoded as H.264 Main 3.1,
  720p, constant 30000/1001 FPS, AAC-LC stereo 48 kHz, faststart MP4.
- The user requests Full HD and confirmed both a Main 4.1/AAC-LC 48 kHz 1080p
  sample and a stream-copy High 4.0/AAC-LC 44.1 kHz 1080p sample. The latter
  retains the unusual nominal FPS metadata and demonstrates that metadata alone
  does not establish an incompatibility or require video/audio encoding.
- The user explicitly requires processing only when necessary: no-op for a
  compatible file, copy/remux for container issues, and independent video/audio
  encoding for identified stream mismatches.
- File size/duration are not established causes. Avoid inferring device limits
  from an approximate TV age or splitting the requested full video unnecessarily.
- The complete stream-copy MP4 (7h38m, 5.1 GB, faststart, identical encoded
  stream hashes) failed on the TV; the complete stream-copy MPEG-TS worked with
  picture and sound. Full HD H.264 High 4.0 and AAC-LC 44.1 kHz can be retained.
- The failing MP4 uses `co64` offsets and a 45 MB index; the short working MP4
  uses `stco` and a 168 KB index. This supports a container workaround, not a
  proven claim that file size, duration or `co64` alone causes the failure.
- Extend the bounded profile with MPEG-TS for large MP4s using `co64`; preserve
  originals and atomically compare-and-swap one DB file path, never disguise TS
  content under an MP4 filename.

## Acceptance Criteria

Verified Full HD output with audible AAC-LC stereo; failed conversion preserves the
source, reports failure and never produces a ready notification.

## Tasks

1. Implement verified ffmpeg preparation and propagate failures through worker
   and completion paths; retain unique hidden original backups.
2. Test cancellation, ffmpeg/probe errors, output validation and original
   preservation; run full repository checks and one cohesive deep review.
3. Validate a Full HD sample and create the requested complete compatible copy.
   Live operation receipts remain in ignored `build/` artifacts, not this record.
4. Deploy the reviewed runtime fix, verify bot/API health and actual playback,
   document the final conversion contract, then move this plan to completed.

## Progress

Selective preparation, safe failure propagation, cancellation and durable one-path
CAS are implemented. Final deep review has no remaining findings. Complete-source
runtime acceptance passed in an isolated live DB: real TS path, Full HD High 4.0 /
AAC-LC 44.1 kHz retained, original preserved, second pass unchanged. Both passes
plus validation took 75 seconds. Device full TS playback is confirmed.
Runtime commit `8716613` was deployed with the official Ansible deploy playbook.
Post-deploy API health, Telegram `getMe` through the configured proxy, TMS and
DLNA service activity passed; the running binary hash matches the local artifact.

## Decisions

Preserve Full HD resolution, supported High/Main/Baseline profiles and compatible
AAC audio. Never encode merely because a nominal FPS value is unusual. Use bounded
timestamp checks and report inconclusive verification rather than guessing.
Keep original backups; metadata-only level changes cannot repair actual limits.

## Verification

`GOTOOLCHAIN=go1.26.0 make check`, `make test-integration`, affected-package tests,
focused cancellation/DB/TV race checks, lint and diff checks pass. A separate
immutable-baseline run reproduced the pre-existing manager logger race (TD-002).
Final review verified FULL-synchronous CAS and mandatory timing checks despite
missing SAR / unknown average FPS. No full power-loss injection was performed.
Live acceptance uses an isolated DB and separate files; receipts are ignored.

## Handoff

Completed. The working full TS remains in DLNA; generated test variants and the
source are preserved privately. Backups are retained for manual recovery/cleanup.
Live receipts stay in ignored `build/` artifacts. No torrent backend/settings
were changed: diagnostics established a 100 Mbps negotiated link despite both
endpoints advertising gigabit; throughput and cable/port cause remain unmeasured.
