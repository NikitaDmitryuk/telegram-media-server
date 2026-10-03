"""Subscription conversion and transactional updates; no network/systemd mutations."""
import base64
from io import BytesIO
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import tempfile
import threading
import time
import unittest
from unittest.mock import Mock, patch
from urllib.error import HTTPError, URLError
from urllib.parse import urlencode


SCRIPT = Path(__file__).with_name("setup-proxy")
MODULE = runpy.run_path(str(SCRIPT), run_name="setup_proxy_test")
GLOBALS = MODULE["update_subscription"].__globals__


def uri(transport="tcp", host="example.invalid", **query):
    values = dict(type=transport, security="reality", pbk="A" * 43,
                  sni="example.invalid", sid="abcd", fp="chrome")
    if transport == "tcp":
        values["flow"] = "xtls-rprx-vision"
    values.update(query)
    return "vless://00000000-0000-0000-0000-000000000001@" + host + ":443?" + urlencode(values)


class SubscriptionTest(unittest.TestCase):
    def test_text_and_base64(self):
        text = "#profile-title: ultra\r\n\r\n" + uri() + "\n" + uri(host="other.invalid")
        for raw in (text.encode(), base64.b64encode(text.encode()), base64.b64encode(text.encode()).rstrip(b"=")):
            self.assertEqual(MODULE["first_vless_from_text"](MODULE["decode_subscription"](raw)), uri())

    def test_first_invalid_profile_is_not_skipped(self):
        for first in ("trojan://secret@example.invalid", "invalid", "vless://secret@invalid:bad"):
            with self.assertRaises((ValueError, SystemExit)):
                link = MODULE["first_vless_from_text"](first + "\n" + uri())
                MODULE["parse_vless_link"](link)

    def test_empty_and_invalid_responses(self):
        for raw in (b"", b"<html>error</html>", b"\xff", b"not base64!"):
            with self.assertRaises(ValueError):
                MODULE["first_vless_from_text"](MODULE["decode_subscription"](raw))

    def test_tcp_and_ipv6(self):
        outbound = MODULE["parse_vless_link"](uri(host="[2001:db8::1]"))
        server = outbound["settings"]["vnext"][0]
        self.assertEqual(server["address"], "2001:db8::1")
        self.assertEqual(server["users"][0]["flow"], "xtls-rprx-vision")

    def test_xhttp_preserves_extra_and_single_decoding(self):
        extra = {"xPaddingBytes": "100-1000"}
        outbound = MODULE["parse_vless_link"](uri("xhttp", path="/encoded%2Fpath", mode="auto", extra=json.dumps(extra)))
        self.assertEqual(outbound["streamSettings"]["xhttpSettings"],
                         {"path": "/encoded%2Fpath", "mode": "auto", "extra": extra})
        self.assertNotIn("flow", outbound["settings"]["vnext"][0]["users"][0])

    def test_invalid_fields_do_not_leak_credentials(self):
        for link in ("vless://private-uuid@host:99999", uri("unknown"), uri(pbk=""),
                     uri("xhttp", extra="[1]"), uri("xhttp", extra="private-token"),
                     uri("xhttp", flow="xtls-rprx-vision")):
            with self.assertRaises(ValueError) as error:
                MODULE["parse_vless_link"](link)
            self.assertNotIn("private-", str(error.exception))

    def test_download_direct_and_errors_sanitized(self):
        opener = Mock()
        with patch("urllib.request.build_opener", return_value=opener) as build:
            opener.open.return_value = BytesIO(uri().encode())
            self.assertEqual(MODULE["fetch_subscription"]("https://example.invalid/sub/private-token"), uri())
            self.assertEqual(build.call_args.args[0].proxies, {})
            self.assertEqual(opener.open.call_args.kwargs["timeout"], 15)
            for error in (URLError("private-token"), HTTPError("https://example.invalid/sub/private-token", 403, "private-token", {}, BytesIO())):
                opener.open.side_effect = error
                with self.assertRaises(ValueError) as caught:
                    MODULE["fetch_subscription"]("https://example.invalid/sub/private-token")
                self.assertNotIn("private-token", str(caught.exception))
            error.close()

    def test_limit_and_https(self):
        opener = Mock()
        opener.open.return_value = BytesIO(b"x" * (1024 * 1024 + 1))
        with patch("urllib.request.build_opener", return_value=opener):
            with self.assertRaises(ValueError):
                MODULE["fetch_subscription"]("https://example.invalid/sub/test")
        with self.assertRaises(ValueError):
            MODULE["fetch_subscription"]("http://example.invalid/sub/test")
        with self.assertRaises(ValueError):
            MODULE["HTTPSRedirectHandler"]().redirect_request(None, None, 302, "", {}, "http://example.invalid")

    def test_service_requires_both_proxy_listeners(self):
        with patch.dict(GLOBALS, run=Mock(return_value=subprocess.CompletedProcess([], 0))), \
                patch('time.sleep'), patch('socket.create_connection') as connect:
            connect.side_effect = OSError('not listening')
            self.assertFalse(MODULE['service_ready']('test-vpn.service'))
            connect.side_effect = None
            self.assertTrue(MODULE['service_ready']('test-vpn.service'))
            self.assertEqual(connect.call_args.args[0], ('127.0.0.1', 10809))

    @unittest.skipUnless(shutil.which("xray"), "Xray not installed")
    def test_real_xray_accepts_tcp_and_xhttp(self):
        for transport in ("tcp", "xhttp"):
            link = uri(transport, **({"path": "/xhttp", "mode": "auto", "extra": '{"xPaddingBytes":"100-1000"}'} if transport == "xhttp" else {}))
            config = MODULE["build_full_config"](MODULE["parse_vless_link"](link))
            with tempfile.TemporaryDirectory() as directory:
                path = Path(directory) / "config.json"
                path.write_text(json.dumps(config))
                result = subprocess.run([shutil.which("xray"), "run", "-test", "-config", str(path)], capture_output=True)
                self.assertEqual(result.returncode, 0, "Xray rejected synthetic " + transport + " config")


class UpdateTest(unittest.TestCase):
    def setUp(self):
        self.directory = tempfile.TemporaryDirectory()
        self.addCleanup(self.directory.cleanup)
        self.source = Path(self.directory.name) / "subscription.url"
        self.target = Path(self.directory.name) / "config.json"
        self.source.write_text("https://example.invalid/sub/private-token")
        self.run = Mock(return_value=subprocess.CompletedProcess([], 0))
        self.ready = Mock(return_value=True)
        self.fetch = Mock(return_value=uri("xhttp", path="/xhttp"))
        p = patch.dict(GLOBALS, run=self.run, service_ready=self.ready,
                       fetch_subscription=self.fetch, find_xray_binary=lambda: "/usr/bin/xray")
        p.start()
        self.addCleanup(p.stop)

    def update(self):
        return MODULE["update_subscription"](self.source, self.target, "test-vpn.service", os.getgid())

    def test_install_atomic_permissions_and_idempotence(self):
        replace = os.replace
        with patch("os.replace", wraps=replace) as atomic:
            self.assertTrue(self.update())
            atomic.assert_called_once()
        self.assertEqual(self.target.stat().st_mode & 0o777, 0o640)
        self.run.reset_mock()
        self.assertFalse(self.update())
        self.run.assert_not_called()
        self.assertEqual(list(self.target.parent.glob(".candidate-*")), [])

    def test_failed_fetch_preserves_cached_config(self):
        self.target.write_bytes(b"old-config")
        self.fetch.side_effect = ValueError("Cannot download subscription")
        with self.assertRaises(ValueError):
            self.update()
        self.assertEqual(self.target.read_bytes(), b"old-config")
        self.run.assert_not_called()

    def test_failed_validation_preserves_cached_config(self):
        self.target.write_bytes(b"old-config")
        self.run.return_value = subprocess.CompletedProcess([], 1, stderr="private-token")
        with self.assertRaisesRegex(ValueError, "Xray rejected"):
            self.update()
        self.assertEqual(self.target.read_bytes(), b"old-config")
        self.assertEqual(self.run.call_count, 1)

    def test_failed_start_rolls_back(self):
        self.target.write_bytes(b"old-config")
        self.run.side_effect = [subprocess.CompletedProcess([], code) for code in (0, 1, 0)]
        with self.assertRaisesRegex(ValueError, "previous config restored"):
            self.update()
        self.assertEqual(self.target.read_bytes(), b"old-config")
        self.assertEqual(self.run.call_count, 3)

    def test_missing_listener_rolls_back_and_failed_recovery_reported(self):
        self.target.write_bytes(b"old-config")
        self.ready.return_value = False
        with self.assertRaisesRegex(ValueError, "service recovery failed"):
            self.update()
        self.assertEqual(self.target.read_bytes(), b"old-config")

    def test_first_install_failure_removes_config_and_stops_client(self):
        self.ready.return_value = False
        with self.assertRaises(ValueError):
            self.update()
        self.assertFalse(self.target.exists())
        self.assertEqual(self.run.call_args.args[0], ["systemctl", "stop", "test-vpn.service"])

    def test_lock_covers_update(self):
        entered, release = threading.Event(), threading.Event()
        errors = []

        def fetch(_):
            entered.set()
            release.wait(2)
            return uri()

        def update():
            try:
                self.update()
            except Exception as error:
                errors.append(error)

        self.fetch.side_effect = fetch
        first, second = threading.Thread(target=update), threading.Thread(target=update)
        first.start()
        self.assertTrue(entered.wait(1))
        second.start()
        try:
            time.sleep(0.1)
            self.assertEqual(self.fetch.call_count, 1)
        finally:
            release.set()
            first.join(3)
            second.join(3)
        self.assertFalse(first.is_alive() or second.is_alive())
        self.assertEqual(errors, [])
        self.assertEqual(self.run.call_count, 2)  # validate + restart, once


if __name__ == "__main__":
    unittest.main()
