# OpenClaw skill: TMS (Telegram Media Server)

This skill teaches OpenClaw to use the **Telegram Media Server (TMS)** REST API: add downloads by URL (video, magnet, .torrent), list downloads, delete a download, and search torrents. You need a running TMS instance; this skill only sends HTTP requests to it.

## What is Telegram Media Server?

**Telegram Media Server** is a Telegram bot that accepts links to streaming video or torrent files, downloads them (via yt-dlp and aria2), and can distribute content on your local network via a DLNA server (e.g. minidlna). It supports yt-dlp–compatible video URLs, magnet links, and .torrent files, with optional Prowlarr integration for torrent search.

This **OpenClaw skill** talks to the same TMS backend over its REST API. Instead of using the Telegram bot, you can ask your OpenClaw agent to add downloads, list status, remove a download, or search torrents — all via natural language in chat. The skill only needs the TMS base URL and API key; it does not install any code or run binaries.

**Install Telegram Media Server** (required before using this skill):

- **Repository:** [github.com/NikitaDmitryuk/telegram-media-server](https://github.com/NikitaDmitryuk/telegram-media-server)
- Clone, build, and configure as described in the project [README](https://github.com/NikitaDmitryuk/telegram-media-server#readme). The TMS REST API is enabled by default; for access from OpenClaw (another host) set `TMS_API_KEY` in TMS `.env` and ensure the API listen address (`TMS_API_LISTEN`) is reachable from where OpenClaw runs.

## What it does

The skill lets your OpenClaw agent talk to a running TMS instance over its REST API. The agent can:

- **Add downloads** — by video URL (yt-dlp), magnet link, or .torrent URL.
- **List downloads** — see status (queued, downloading, converting, completed, failed, stopped), progress, and errors.
- **Delete a download** — stop and remove by ID.
- **Search torrents** — when TMS has Prowlarr configured, search indexers and add results as downloads.

All requests use the TMS API base URL and API key you configure. The skill does not emulate the Telegram bot; it uses the same backend via HTTP.

## Requirements

- A running **Telegram Media Server** with the API enabled (`TMS_API_ENABLED=true`), a valid `TMS_API_KEY`, and network access from the machine where OpenClaw runs.
- **Environment variables** for the agent: `TMS_API_URL` (e.g. `http://tms-host:8080`) and `TMS_API_KEY` (same value as in TMS config). These are required so the skill can authenticate; the key is never logged or exposed in prompts.
- **Optional:** Prowlarr configured on TMS for torrent search. **Optional:** `TMS_WEBHOOK_URL` in TMS config so OpenClaw can be notified when a download completes, fails, or is stopped.

## Installation

1. **Copy the skill** into your OpenClaw skills directory:
   - Workspace (per-agent): copy `openclaw-skill-tms` to your agent's `/skills` (e.g. `./skills/tms` or the folder that maps to `/skills`).
   - Managed (all agents): copy to `~/.openclaw/skills/tms`.

2. **Configure environment** for the agent that will use TMS:
   - `TMS_API_URL` — base URL of TMS API (e.g. `http://tms-host:8080`).
   - `TMS_API_KEY` — API key (same as `TMS_API_KEY` in TMS config).

   In `~/.openclaw/openclaw.json` you can set per-skill env:
   ```json5
   {
     skills: {
       entries: {
         tms: {
           enabled: true,
           env: {
             TMS_API_URL: "http://localhost:8080",
             TMS_API_KEY: "your-api-key"
           }
         }
       }
     }
   }
   ```

3. **Optional — Webhook:** To have OpenClaw notified when a download completes (and e.g. post to Telegram), enable gateway hooks in OpenClaw and add a `tms` mapping, then in TMS set:
   - `TMS_WEBHOOK_URL` — e.g. `http://127.0.0.1:18789/hooks/tms` (gateway port and path from your OpenClaw config).
   - `TMS_WEBHOOK_TOKEN` — same value as `hooks.token` in OpenClaw (TMS sends it as `Authorization: Bearer <token>`). Generate with `openssl rand -hex 32` if you create a new token.
   TMS will POST JSON `{ id, title, status, error?, event_id }` on completion/failure/stopped.

## How to use

After installation and config, ask the agent in natural language. Examples:

- *"Add a download for this video: https://..."*
- *"Show my current downloads"* / *"List downloads"*
- *"Delete download 3"*
- *"Search torrents for Inception 1080p"* (requires Prowlarr on TMS)

The agent will use the TMS API (health, list, add, delete, search) as described in SKILL.md and in the OpenAPI spec at `{TMS_API_URL}/api/v1/openapi-llm.yaml`.

## Examples

| User says | Agent action |
|-----------|--------------|
| *"Add this link: https://youtube.com/watch?v=..."* | `POST /api/v1/downloads` with `{"url": "..."}`; reports back id and title. |
| *"What's downloading?"* | `GET /api/v1/downloads`; summarizes list with status and progress. |
| *"Remove download 2"* | `DELETE /api/v1/downloads/2`; confirms removal. |
| *"Find torrents for Matrix 1080p"* | `GET /api/v1/search?q=Matrix%201080p`; can then add one via `POST /downloads` with magnet or torrent URL. |

## ClawHub

If this skill is published to [ClawHub](https://clawhub.ai/), install the CLI (`npm i -g clawhub` — not available via Homebrew), then:

```bash
clawhub install tms
```

After publication, the skill page on ClawHub may list the exact slug if it differs.

## API docs

- Human-readable Swagger UI: `{TMS_API_URL}/api/v1/docs`
- OpenAPI YAML (human): `{TMS_API_URL}/api/v1/openapi.yaml`
- OpenAPI YAML (LLM-oriented): `{TMS_API_URL}/api/v1/openapi-llm.yaml`

## Troubleshooting

- **"API unreachable" / connection errors** — Check `TMS_API_URL` (no trailing slash), firewall, and that TMS is running with API enabled. Test with: `curl -H "Authorization: Bearer $TMS_API_KEY" "$TMS_API_URL/api/v1/health"`.
- **401 Unauthorized** — `TMS_API_KEY` must match the key in TMS config. Ensure the same value is set in `skills.entries.tms.env.TMS_API_KEY` or in the agent’s environment.
- **Search returns nothing or times out** — TMS must have Prowlarr configured (`PROWLARR_URL`, `PROWLARR_API_KEY`). Search has a timeout (e.g. 15s); slow indexers may not respond in time.
- **Swagger / OpenAPI not loading** — When no API key is set, doc routes accept only localhost; if they fail, check base URL and that the API process is bound to the expected host/port.

## Security and trust

This skill is **instruction-only**: it contains no install scripts, no code to execute, and no extra binaries. OpenClaw uses it to decide when and how to call your TMS API. The only requirements are two environment variables: `TMS_API_URL` (base URL of your TMS) and `TMS_API_KEY` (for authentication). Both are used solely for HTTP requests to the TMS endpoints described in SKILL.md (health, list, add, delete, search, OpenAPI spec). The skill does not read unrelated files, harvest other env vars, or send data to third-party endpoints. Optional webhook support is configured on the TMS side (`TMS_WEBHOOK_URL`, and `TMS_WEBHOOK_TOKEN` for OpenClaw hooks auth), not by the skill.

**Before installing:**

1. **Trusted TMS host** — Ensure `TMS_API_URL` points to a TMS instance you control or trust, and that the network path (e.g. LAN or VPN) is what you expect.
2. **API key** — Use a TMS API key with the minimal permissions you need; avoid reusing a key that has broader or admin access elsewhere.
3. **Webhooks** — If you set `TMS_WEBHOOK_URL` on TMS, you are exposing an endpoint that will receive completion/failure/stopped events. Secure and authenticate that endpoint.
4. **Autonomous invocation** — By default the agent can invoke this skill on its own (e.g. start or stop downloads). If you want to allow only explicit user requests, disable model invocation for this skill or restrict when it is enabled.
5. **Secrets** — Store `TMS_API_KEY` in per-skill or agent config (e.g. `openclaw.json`), not in public repos. Rotate the key if it may have been compromised.

The skill may be subject to security scanning (e.g. VirusTotal, OpenClaw checks) as part of the ClawHub listing; its behavior is limited to the described API client role.

## License

This skill is part of the [telegram-media-server](https://github.com/NikitaDmitryuk/telegram-media-server) repository and is under the same [MIT License](https://github.com/NikitaDmitryuk/telegram-media-server/blob/main/LICENSE) as the project.

## Publishing (for maintainers)

To publish or update this skill on ClawHub from the repo root:

1. Install CLI via npm (ClawHub is not in Homebrew): `npm i -g clawhub` or `pnpm add -g clawhub`.
2. Log in: `clawhub login` (or `clawhub login --token <token>`).
3. Publish:  
   `clawhub publish ./openclaw-skill-tms --slug tms --name "TMS (Telegram Media Server)" --version 1.0.0 --changelog "Initial release" --tags latest`
4. For updates: bump version and changelog, then run `clawhub publish` again with new `--version` and `--changelog`.

See [CHANGELOG.md](CHANGELOG.md) for version history.
