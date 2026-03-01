# Changelog

All notable changes to this skill are documented here.

## [1.0.6] - Inline API spec (no fetch required)

- **Full API contract in SKILL.md:** The complete OpenAPI spec (paths, schemas, examples) is now embedded in the skill document. The agent no longer needs to request `{base_url}/api/v1/openapi-llm.yaml` to use the TMS API — all information for building and invoking API calls is available immediately. Base URL and authentication rules are stated explicitly; full URL for each endpoint is `{BaseURL}/api/v1/...`.

## [1.0.4] - Optional TMS_API_URL (default for same-host)

- **TMS_API_URL is optional:** When agent and TMS run on the same host, default base URL is `http://127.0.0.1:8080` (TMS default API listen). No env vars are required for same-host; skill metadata `requires.env` is now `[]`.

## [1.0.3] - Optional API key, clearer spec URL

- **TMS_API_KEY is optional:** When TMS and OpenClaw run on the same host, requests from localhost are accepted without a key; only `TMS_API_URL` was required (now also optional).
- **OpenAPI spec for agents:** SKILL.md now states exactly where to get the API contract: `{base_url}/api/v1/openapi-llm.yaml` (with default for same-host).

## [1.0.2] - Version bump

- Version bump (1.0.1 was already released).

## [1.0.1] - Webhook auth for OpenClaw

- **Webhook:** TMS now supports optional `TMS_WEBHOOK_TOKEN`; when set, TMS sends `Authorization: Bearer <token>` when calling `TMS_WEBHOOK_URL`, so OpenClaw gateway hooks can authenticate the request. Skill README and SKILL.md updated with webhook setup (URL + token, hooks mapping).

## [1.0.0] - Initial release

- TMS REST API skill: add downloads (video URL, magnet, .torrent), list downloads, delete download, search torrents (Prowlarr).
- Configuration via `TMS_API_URL` and `TMS_API_KEY`.
- Optional webhook support for completion/failure/stopped events.
