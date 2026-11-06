---
kind: memory
status: active
date: 2026-11-06
last-verified: 2026-11-06
agent: codex
evidence: internal/tvcompat/prepare.go, internal/tvcompat/transport.go, internal/tvcompat/mp4_atoms.go, internal/tvcompat/prepare_test.go, internal/tvcompat/transport_test.go, internal/downloader/manager/conversion_control.go, internal/app/completion.go, internal/database/files.go
---

# Legacy LG video preparation

- Compatibility mode uses separate container/video/audio decisions. A compatible
  faststart MP4 stays byte-identical; remux copies supported streams; audio-only
  repair copies video. Never encode just because nominal `r_frame_rate` is high.
- The bounded profile accepts progressive 8-bit H.264 Baseline/Main/High up to
  Full HD/30 FPS and level 4.0/4.1, with AAC-LC stereo 44.1/48 kHz. Encoding uses
  Main/yuv420p with bounded rate/reference settings; lower configured levels are
  explicitly rejected rather than falsely declaring 1080p as level 3.1.
- Packet timestamps are sampled at the beginning, middle and end. This cannot
  prove every packet or every television works. Missing SAR is not an explicit
  mismatch; unknown average FPS (including TS `0/0`) requires measured PTS, not
  nominal `r_frame_rate`. Inconclusive checks preserve the
  original and report preparation failure.
- Device evidence: a short 1080p High 4.0/AAC-LC 44.1 kHz stream-copy MP4 played,
  while the complete 7h38m/5.1 GB MP4 failed despite identical encoded stream
  hashes. The complete MPEG-TS stream copy played with picture and sound.
  Both retain Full HD. The exact TV model remains unknown.
- The failing full MP4 had a 45 MB `moov` and `co64` tables; the short working
  sample used a 168 KB `moov` and `stco`. This does not isolate `co64`, index size,
  duration or file size as the cause. For this legacy profile, >4 GiB MP4s with
  `co64` conservatively use a real `.ts` destination and the same selective
  per-stream decisions. A compatible `.ts` is unchanged on repeat processing.
- Replacement retains a unique hidden original under `.tvcompat-originals`.
  TS publication never overwrites an existing file and updates exactly one main
  DB path through an optional compare-and-swap capability. Its SQLite transaction
  uses `synchronous=FULL` on one pinned connection before the source is unlinked,
  then restores the prior setting; normal WAL commits alone are insufficient
  for the file-path durability ordering. Ambiguous DB errors
  retain both files. A crash can leave an extra visible file; retain it until the
  current DB path is checked. Backups persist for manual recovery/cleanup.
- TV-preparation failure is distinct from download failure: the completion loop
  keeps source and DB records, sends failure, and never reports readiness. The
  qBittorrent resume loop does not re-download after this terminal stage failure.
- Stop/delete cancels and joins queued/running conversions, including recovered
  jobs, before deletion can proceed. This prevents a late publication recreating
  a deleted movie. Only MP4/MPEG-TS are prepared by this bounded implementation.

An isolated live test of the complete source passed in 75 seconds including
validation and a second pass: real TS DB path, 1920x1080 High 4.0/AAC-LC 44.1 kHz,
unchanged second pass and preserved source. The device accepted the equivalent
complete stream-copy TS; no power-loss fault injection was performed.
