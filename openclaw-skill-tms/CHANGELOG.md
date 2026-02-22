# Changelog

All notable changes to this skill are documented here.

## [1.0.2] - Version bump

- Version bump (1.0.1 was already released).

## [1.0.1] - Webhook auth for OpenClaw

- **Webhook:** TMS now supports optional `TMS_WEBHOOK_TOKEN`; when set, TMS sends `Authorization: Bearer <token>` when calling `TMS_WEBHOOK_URL`, so OpenClaw gateway hooks can authenticate the request. Skill README and SKILL.md updated with webhook setup (URL + token, hooks mapping).

## [1.0.0] - Initial release

- TMS REST API skill: add downloads (video URL, magnet, .torrent), list downloads, delete download, search torrents (Prowlarr).
- Configuration via `TMS_API_URL` and `TMS_API_KEY`.
- Optional webhook support for completion/failure/stopped events.
