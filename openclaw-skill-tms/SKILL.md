---
name: tms
version: "1.0.2"
description: Manage downloads via Telegram Media Server (TMS) REST API — add by URL (video/magnet/torrent), list, delete, search torrents.
metadata:
  {"openclaw":{"requires":{"env":["TMS_API_URL","TMS_API_KEY"]},"primaryEnv":"TMS_API_KEY"}}
---

# TMS (Telegram Media Server) API skill

Use this skill when the user wants to add downloads, check download status, stop a download, or search for torrents via the TMS backend. All requests go to the TMS REST API.

## Configuration

- **Base URL:** from environment variable `TMS_API_URL` (e.g. `http://tms-host:8080`).
- **Authentication:** send every API request with either `Authorization: Bearer <TMS_API_KEY>` or header `X-API-Key: <TMS_API_KEY>` (value from env `TMS_API_KEY`).

## Operations

1. **Health check** — `GET {TMS_API_URL}/api/v1/health` — returns `{"status":"ok"}` if the API is up.

2. **List downloads** — `GET {TMS_API_URL}/api/v1/downloads` — returns a JSON array of downloads with `id`, `title`, `status` (queued, downloading, converting, completed, failed, stopped), `progress`, `conversion_progress`, `error` (if failed), `position_in_queue` (if queued). Snapshot is best-effort.

3. **Add download** — `POST {TMS_API_URL}/api/v1/downloads` with JSON body `{"url": "<url>"}`. URL can be: video URL (yt-dlp), magnet link (`magnet:...`), or .torrent file URL. Response: `201` with `{"id": <number>, "title": "<string>"}`. Use `id` for delete or status.

4. **Delete download** — `DELETE {TMS_API_URL}/api/v1/downloads/{id}` — stops and removes the download. Response: `204` no body. `id` is the numeric id from the add response or list.

5. **Search torrents** — `GET {TMS_API_URL}/api/v1/search?q=<query>&limit=20&quality=1080` — requires Prowlarr configured on TMS. `q` is required; `limit` (1–100, default 20) and `quality` (optional filter) may be used. Returns array of `{title, size, magnet, torrent_url, indexer_name, peers}`. Use `magnet` or `torrent_url` in POST /downloads to add a download.

## Full API spec for tools

Machine-readable OpenAPI spec for LLM/tool use: **GET {TMS_API_URL}/api/v1/openapi-llm.yaml** — fetch this URL to get the full contract (parameters, schemas, examples) when building or invoking API calls.

## Webhook (optional)

If TMS is configured with `TMS_WEBHOOK_URL` pointing to an endpoint OpenClaw can receive, TMS will POST to that URL when a download completes, fails, or is stopped. Body: `id`, `title`, `status` (completed|failed|stopped), `error` (if failed), `event_id` (UUID). When `TMS_WEBHOOK_TOKEN` is set in TMS config, TMS sends `Authorization: Bearer <TMS_WEBHOOK_TOKEN>` (required for OpenClaw gateway hooks). Delivery is best-effort (no guaranteed delivery). Use this to notify the user in chat when a download finishes.
