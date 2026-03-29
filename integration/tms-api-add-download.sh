#!/usr/bin/env bash
# Integration helper: POST TMS /api/v1/downloads (not part of unit test suite).
#
# Default: sends integration/fixtures/big-buck-bunny.torrent as torrent_base64 (works with TMS in Docker).
#
# Usage (from repo root):
#   ./integration/tms-api-add-download.sh
#   ./integration/tms-api-add-download.sh /path/to/file.torrent
#   ./integration/tms-api-add-download.sh /path/to/file.torrent 'Display title'
#   ./integration/tms-api-add-download.sh 'https://example.com/video.mp4'
#   TMS_BASE_URL=http://127.0.0.1:8080 TMS_API_KEY=secret ./integration/tms-api-add-download.sh
#
# Env:
#   TMS_BASE_URL      — API base (no trailing slash), default http://127.0.0.1:8080
#   TMS_API_KEY       — optional; sent as Authorization: Bearer
#   TMS_TEST_TORRENT  — override default .torrent path

set -euo pipefail

BASE="${TMS_BASE_URL:-http://127.0.0.1:8080}"
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
DEFAULT_TORRENT="${TMS_TEST_TORRENT:-$SCRIPT_DIR/fixtures/big-buck-bunny.torrent}"

if ! command -v curl >/dev/null 2>&1; then
	echo "error: curl is required" >&2
	exit 1
fi
if ! command -v python3 >/dev/null 2>&1; then
	echo "error: python3 is required (for JSON body)" >&2
	exit 1
fi

MODE="torrent"
TORRENT_FILE=""
VIDEO_URL=""
OPT_TITLE="${2:-}"

if [[ -n "${1:-}" ]]; then
	if [[ -f "$1" ]]; then
		TORRENT_FILE="$1"
		MODE="torrent"
	else
		VIDEO_URL="$1"
		MODE="url"
		OPT_TITLE="${2:-}"
	fi
else
	TORRENT_FILE="$DEFAULT_TORRENT"
fi

if [[ "$MODE" == "torrent" ]]; then
	if [[ ! -f "$TORRENT_FILE" ]]; then
		echo "error: torrent file not found: $TORRENT_FILE" >&2
		exit 1
	fi
	BODY=$(python3 -c '
import base64, json, pathlib, sys
path = pathlib.Path(sys.argv[1])
raw = path.read_bytes()
req = {"torrent_base64": base64.standard_b64encode(raw).decode("ascii")}
if len(sys.argv) > 2 and sys.argv[2]:
    req["title"] = sys.argv[2]
print(json.dumps(req))
' "$TORRENT_FILE" "$OPT_TITLE")
else
	if [[ -n "$OPT_TITLE" ]]; then
		BODY=$(python3 -c 'import json,sys; print(json.dumps({"url": sys.argv[1], "title": sys.argv[2]}))' "$VIDEO_URL" "$OPT_TITLE")
	else
		BODY=$(python3 -c 'import json,sys; print(json.dumps({"url": sys.argv[1]}))' "$VIDEO_URL")
	fi
fi

CURL_ARGS=(
	-sS
	-w "\n\nhttp_status:%{http_code}\n"
	-X POST
	"${BASE}/api/v1/downloads"
	-H "Content-Type: application/json"
	-d "$BODY"
)
if [[ -n "${TMS_API_KEY:-}" ]]; then
	CURL_ARGS+=(-H "Authorization: Bearer ${TMS_API_KEY}")
fi

echo "POST ${BASE}/api/v1/downloads" >&2
if [[ "$MODE" == "torrent" ]]; then
	echo "torrent_base64 from: $TORRENT_FILE" >&2
else
	echo "url: ${VIDEO_URL}" >&2
fi
curl "${CURL_ARGS[@]}"
