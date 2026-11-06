#!/usr/bin/env python3
"""Opt-in Prowlarr setup. Read one JSON object from stdin; never pass secrets in argv.

Output contains only changed/ready/managed/snapshot. Failures are sanitized JSON on
stderr, exit 1. Existing settings snapshots are never overwritten. The component
runs locally on the Prowlarr host and changes no service, VPN, or routing settings.
"""
import copy
import json
import os
import stat
import sys
import tempfile
import time
import urllib.error
import urllib.parse
import urllib.request


class SetupError(Exception):
    """A safe, operator-facing error, without remote bodies or credentials."""


class NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        raise SetupError("Prowlarr API redirect refused")


class API:
    def __init__(self, base_url, api_key):
        parsed = urllib.parse.urlsplit(base_url)
        if (parsed.scheme not in ("http", "https") or
                parsed.hostname not in ("127.0.0.1", "localhost", "::1") or
                parsed.username or parsed.password or parsed.query or parsed.fragment):
            raise SetupError("Prowlarr API URL must be a local HTTP(S) URL without credentials")
        if not api_key or not isinstance(api_key, str):
            raise SetupError("Prowlarr API key is required")
        self.base_url = base_url.rstrip("/") + "/api/v1/"
        self.api_key = api_key
        self.opener = urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect())

    def request(self, method, path, body=None):
        request = urllib.request.Request(
            self.base_url + path,
            data=None if body is None else json.dumps(body).encode(),
            headers={"X-Api-Key": self.api_key, "Content-Type": "application/json"},
            method=method)
        try:
            with self.opener.open(request, timeout=60) as response:
                data = response.read()
                return json.loads(data) if data else None
        except urllib.error.HTTPError as exc:
            if exc.code in (401, 403):
                raise SetupError("Prowlarr API authentication/authorization failed") from None
            if path == "indexer/test":
                raise SetupError("Managed RuTor test failed (HTTP %s); verify public site access through the configured proxy" % exc.code) from None
            raise SetupError("Prowlarr API %s %s failed (HTTP %s); inspect Prowlarr locally" %
                             (method, path.split('/')[0], exc.code)) from None
        except (OSError, ValueError):
            raise SetupError("Prowlarr API request failed; inspect connectivity and Prowlarr locally") from None


def definition(item):
    if item.get("implementation") != "Cardigann":
        return None
    return next((f.get("value") for f in item.get("fields", [])
                 if f.get("name") == "definitionFile"), None)


def proxy_fields(proxy_url):
    try:
        p = urllib.parse.urlsplit(proxy_url)
        port = p.port or 80
        if (p.scheme != "http" or not p.hostname or p.path not in ("", "/") or
                p.query or p.fragment or not 1 <= port <= 65535):
            raise ValueError()
    except (ValueError, TypeError):
        raise SetupError("Managed Prowlarr proxy requires an existing HTTP proxy URL") from None
    return {"proxyEnabled": True, "proxyType": "http", "proxyHostname": p.hostname,
            "proxyPort": port, "proxyUsername": urllib.parse.unquote(p.username or ""),
            "proxyPassword": urllib.parse.unquote(p.password or ""),
            "proxyBypassLocalAddresses": True}


def snapshot(directory, state):
    os.makedirs(directory, mode=0o700, exist_ok=True)
    info = os.lstat(directory)
    if not stat.S_ISDIR(info.st_mode) or info.st_uid != os.geteuid() or info.st_mode & 0o077:
        raise SetupError("Prowlarr snapshot directory must be private and owned by the setup user")
    fd, path = tempfile.mkstemp(prefix="settings-", suffix=".json", dir=directory)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as stream:
            json.dump(state, stream, ensure_ascii=False)
            stream.flush()
            os.fsync(stream.fileno())
        with open(path, encoding="utf-8") as stream:
            if json.load(stream) != state:
                raise SetupError("Prowlarr settings snapshot verification failed")
        if stat.S_IMODE(os.stat(path).st_mode) != 0o600:
            raise SetupError("Prowlarr settings snapshot permissions are unsafe")
        dir_fd = os.open(directory, os.O_RDONLY | os.O_DIRECTORY)
        try:
            os.fsync(dir_fd)
        finally:
            os.close(dir_fd)
        return path
    except Exception:
        # Preserve partial evidence; never reuse or overwrite an earlier snapshot.
        raise SetupError("Unable to create and verify private Prowlarr settings snapshot") from None


def refresh_catalog(api, sleep=time.sleep):
    command = api.request("POST", "command", {"name": "IndexerDefinitionUpdate"})
    if not isinstance(command, dict) or not command.get("id"):
        raise SetupError("Prowlarr catalog refresh did not return a command ID")
    for _ in range(90):
        status = api.request("GET", "command/%s" % command["id"])
        if status.get("status", "").lower() == "completed":
            return
        if status.get("status", "").lower() in ("failed", "aborted", "cancelled"):
            raise SetupError("Prowlarr catalog refresh failed; verify catalog access through the configured proxy")
        sleep(2)
    raise SetupError("Prowlarr catalog refresh timed out; inspect its command status before retrying")


def searchable(item, profiles):
    profile = next((p for p in profiles if p.get("id") == item.get("appProfileId")), {})
    return (item.get("enable") is True and item.get("supportsSearch") is True and
            item.get("protocol") in ("torrent", 1) and
            profile.get("enableInteractiveSearch") is True)


def setup(options, api=None, sleep=time.sleep):
    managed = options.get("managed_indexers", [])
    manage_proxy = options.get("manage_proxy", False)
    if not isinstance(managed, list) or any(x != "rutor" for x in managed):
        raise SetupError("Only the public RuTor definition (rutor) may be managed")
    if not isinstance(manage_proxy, bool) or len(set(managed)) != len(managed):
        raise SetupError("Invalid Prowlarr management options")
    if not managed and not manage_proxy:
        return {"changed": False, "ready": True, "managed": [], "snapshot": None}
    desired_proxy = proxy_fields(options.get("proxy_url", "")) if manage_proxy else None
    api = api or API(options.get("url", "http://127.0.0.1:9696"), options.get("api_key"))
    host = api.request("GET", "config/host")
    existing = api.request("GET", "indexer")
    profiles = api.request("GET", "appprofile")
    backup = None

    def before_write():
        nonlocal backup
        if backup is None:
            backup = snapshot(options.get("snapshot_dir", "/var/backups/tms-prowlarr"),
                              {"host": host, "indexers": existing, "app_profiles": profiles})

    changed = False
    if desired_proxy and any(host.get(k) != v for k, v in desired_proxy.items()):
        before_write()
        updated = copy.deepcopy(host)
        updated.update(desired_proxy)
        api.request("PUT", "config/host", updated)
        actual = api.request("GET", "config/host")
        if any(actual.get(k) != v for k, v in desired_proxy.items()):
            raise SetupError("Prowlarr proxy settings did not persist")
        changed = True
    if managed:
        # The official command updates local definitions; snapshot precedes it too.
        before_write()
        refresh_catalog(api, sleep)
        schemas = api.request("GET", "indexer/schema")
        standard = next((p for p in profiles if str(p.get("name", "")).lower() == "standard"
                         and p.get("enableInteractiveSearch") is True), None)
        if standard is None:
            raise SetupError("A Standard Prowlarr app profile with interactive search enabled is required")
        for name in managed:
            matches = [i for i in existing if definition(i) == name]
            if len(matches) > 1:
                raise SetupError("Multiple RuTor definitions exist; reconcile them locally before managed setup")
            schema = next((s for s in schemas if definition(s) == name), None)
            if schema is None:
                raise SetupError("RuTor is absent from the refreshed official catalog; verify version-11 catalog access")
            candidate = copy.deepcopy(matches[0] if matches else schema)
            candidate.update(enable=True, appProfileId=standard["id"])
            base_url = next((f for f in candidate.get("fields", []) if f.get("name") == "baseUrl"), None)
            if base_url is not None and not base_url.get("value"):
                urls = schema.get("indexerUrls", [])
                if not urls:
                    raise SetupError("RuTor catalog has no official base URL; inspect catalog compatibility")
                base_url["value"] = urls[0]
            strip = next((f for f in candidate.get("fields", []) if f.get("name") == "stripcyrillic"), None)
            if strip is None:
                raise SetupError("RuTor schema lacks stripcyrillic; inspect catalog compatibility")
            strip["value"] = False
            if not searchable(candidate, profiles):
                raise SetupError("RuTor does not advertise enabled torrent search capability")
            # Test the complete candidate before any indexer persistence, even on no-op applies.
            api.request("POST", "indexer/test", candidate)
            if not matches or candidate != matches[0]:
                if matches:
                    api.request("PUT", "indexer/%s" % candidate["id"], candidate)
                else:
                    candidate.pop("id", None)
                    api.request("POST", "indexer", candidate)
                changed = True
        persisted = api.request("GET", "indexer")
        for name in managed:
            matches = [i for i in persisted if definition(i) == name]
            if (len(matches) != 1 or not searchable(matches[0], profiles) or
                    not any(f.get("name") == "stripcyrillic" and f.get("value") is False
                            for f in matches[0].get("fields", []))):
                raise SetupError("Managed RuTor readiness failed: enabled searchable torrent source required")
            api.request("POST", "indexer/test", matches[0])
    return {"changed": changed, "ready": True, "managed": managed, "snapshot": backup}


def main():
    try:
        result = setup(json.load(sys.stdin))
        print(json.dumps(result))
        return 0
    except SetupError as exc:
        print(json.dumps({"error": str(exc)}), file=sys.stderr)
    except Exception:
        print(json.dumps({"error": "Prowlarr setup failed; inspect private local configuration"}), file=sys.stderr)
    return 1


if __name__ == "__main__":
    sys.exit(main())
