"""Offline HTTP contract tests for explicit, non-destructive Prowlarr setup."""
import contextlib
import copy
import io
import json
import os
from pathlib import Path
import stat
import tempfile
import threading
import unittest
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from unittest import mock

from scripts import setup_prowlarr as setup


SCHEMA = {"implementation": "Cardigann", "name": "RuTor", "enable": False,
          "protocol": "torrent", "supportsSearch": True, "appProfileId": 1,
          "indexerUrls": ["https://rutor.info/"],
          "fields": [{"name": "definitionFile", "value": "rutor"},
                     {"name": "stripcyrillic", "value": True},
                     {"name": "baseUrl", "value": "https://rutor.info/"}]}
MANUAL = {"id": 9, "implementation": "Other", "name": "Manual", "enable": False}


class FakeServer:
    def __init__(self):
        self.host = {"id": 1, "proxyEnabled": False, "bindAddress": "*", "apiKey": "sensitive",
                     "proxyBypassFilter": "keep.example", "proxyPassword": "previous"}
        self.indexers = [copy.deepcopy(MANUAL)]
        self.profiles = [{"id": 1, "name": "Standard", "enableInteractiveSearch": True}]
        self.schemas = [copy.deepcopy(SCHEMA)]
        self.calls = []
        self.fail = None
        self.command_status = "completed"
        self.disable_on_save = False
        self.snapshot_dir = None
        self.assert_backup = True
        owner = self

        class Handler(BaseHTTPRequestHandler):
            def log_message(self, *args):
                pass

            def handle_api(self):
                path = self.path.removeprefix('/api/v1/')
                body = self.rfile.read(int(self.headers.get('Content-Length', '0')))
                body = json.loads(body) if body else None
                owner.calls.append((self.command, path, body, self.headers.get('X-Api-Key')))
                if self.command in ('POST', 'PUT') and owner.assert_backup:
                    snapshots = list(Path(owner.snapshot_dir).glob('settings-*.json'))
                    if not snapshots or stat.S_IMODE(snapshots[0].stat().st_mode) != 0o600:
                        self.send_error(500)
                        return
                status, result = 200, None
                if owner.fail == path:
                    status, result = 403, {"error": "secret-token unsafe-proxy-password"}
                elif path == 'config/host':
                    if self.command == 'PUT':
                        owner.host = body
                    result = owner.host
                elif path == 'indexer' and self.command == 'GET':
                    result = owner.indexers
                elif path == 'indexer' and self.command == 'POST':
                    body['id'] = 10
                    if owner.disable_on_save:
                        body['enable'] = False
                    owner.indexers.append(body)
                    result = body
                elif path.startswith('indexer/') and self.command == 'PUT':
                    owner.indexers = [body if i.get('id') == body['id'] else i for i in owner.indexers]
                    result = body
                elif path == 'appprofile':
                    result = owner.profiles
                elif path == 'indexer/schema':
                    result = owner.schemas
                elif path == 'command':
                    result = {'id': 2}
                elif path == 'command/2':
                    result = {'id': 2, 'status': owner.command_status}
                elif path == 'indexer/test':
                    result = None
                else:
                    status = 404
                self.send_response(status)
                self.send_header('Content-Type', 'application/json')
                self.end_headers()
                self.wfile.write(json.dumps(result).encode())

            do_GET = handle_api
            do_PUT = handle_api
            do_POST = handle_api

        self.server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        self.thread = threading.Thread(target=self.server.serve_forever, daemon=True)
        self.thread.start()

    def close(self):
        self.server.shutdown()
        self.server.server_close()
        self.thread.join()


class SetupTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.server = FakeServer()
        self.server.snapshot_dir = self.tmp.name
        self.options = {'url': 'http://127.0.0.1:%s' % self.server.server.server_port,
                        'api_key': 'private-api-key', 'manage_proxy': True,
                        'proxy_url': 'http://user:pass%40word@127.0.0.1:10809',
                        'managed_indexers': ['rutor'], 'snapshot_dir': self.tmp.name}

    def tearDown(self):
        self.server.close()
        self.tmp.cleanup()

    def run_setup(self):
        return setup.setup(self.options, sleep=lambda _: None)

    def test_backup_precedes_mutations_and_preserves_original_settings(self):
        result = self.run_setup()
        with open(result['snapshot']) as stream:
            saved = json.load(stream)
        self.assertFalse(saved['host']['proxyEnabled'])
        self.assertEqual(saved['indexers'], [MANUAL])
        self.assertTrue(result['ready'])
        self.assertTrue(result['changed'])
        self.assertEqual(stat.S_IMODE(os.stat(result['snapshot']).st_mode), 0o600)
        self.assertEqual(self.server.host['bindAddress'], '*')
        self.assertEqual(self.server.host['proxyBypassFilter'], 'keep.example')
        self.assertEqual(self.server.host['proxyPassword'], 'pass@word')
        self.assertTrue(self.server.host['proxyBypassLocalAddresses'])
        self.assertNotIn('private-api-key', json.dumps(result))
        self.assertNotIn('pass@word', json.dumps(result))
        self.assertTrue(all(call[3] == 'private-api-key' for call in self.server.calls))
        self.assertTrue(all('apikey' not in call[1].lower() for call in self.server.calls))

    def test_idempotence_and_manual_source_preservation(self):
        first = self.run_setup()
        self.server.indexers[1]['priority'] = 42
        second = self.run_setup()
        self.assertTrue(first['changed'])
        self.assertFalse(second['changed'])
        self.assertNotEqual(first['snapshot'], second['snapshot'])
        self.assertEqual(self.server.indexers[0], MANUAL)
        self.assertEqual(self.server.indexers[1]['priority'], 42)
        self.assertEqual(len(self.server.indexers), 2)
        self.assertFalse(next(f['value'] for f in self.server.indexers[1]['fields']
                              if f['name'] == 'stripcyrillic'))
        saves = [c for c in self.server.calls if c[0] in ('POST', 'PUT') and
                 (c[1] == 'indexer' or c[1].startswith('indexer/') and c[0] == 'PUT')]
        self.assertEqual(len(saves), 1)

    def test_failed_indexer_test_never_persists_indexer(self):
        self.server.fail = 'indexer/test'
        with self.assertRaisesRegex(setup.SetupError, 'authorization'):
            self.run_setup()
        self.assertEqual(self.server.indexers, [MANUAL])

    def test_catalog_error_is_sanitized_and_never_persists(self):
        self.server.command_status = 'failed'
        with self.assertRaisesRegex(setup.SetupError, 'catalog refresh failed'):
            self.run_setup()
        self.assertEqual(self.server.indexers, [MANUAL])

    def test_catalog_timeout(self):
        self.server.command_status = 'queued'
        with self.assertRaisesRegex(setup.SetupError, 'timed out'):
            self.run_setup()

    def test_missing_catalog_and_search_profile_fail_readiness(self):
        self.server.schemas = []
        with self.assertRaisesRegex(setup.SetupError, 'absent'):
            self.run_setup()
        self.server.schemas = [SCHEMA]
        self.server.profiles[0]['enableInteractiveSearch'] = False
        with self.assertRaisesRegex(setup.SetupError, 'Standard'):
            self.run_setup()

    def test_new_schema_base_url_comes_from_official_catalog(self):
        self.server.schemas[0]['fields'][2]['value'] = None
        self.run_setup()
        self.assertEqual(self.server.indexers[1]['fields'][2]['value'], 'https://rutor.info/')

    def test_persisted_disabled_source_fails_readiness(self):
        self.server.disable_on_save = True
        with self.assertRaisesRegex(setup.SetupError, 'readiness failed'):
            self.run_setup()

    def test_existing_disabled_source_is_enabled_without_replacement(self):
        source = copy.deepcopy(SCHEMA)
        source.update(id=17, enable=False, custom='preserved')
        self.server.indexers.append(source)
        self.run_setup()
        self.assertEqual(self.server.indexers[1]['id'], 17)
        self.assertEqual(self.server.indexers[1]['custom'], 'preserved')
        self.assertTrue(self.server.indexers[1]['enable'])

    def test_disabled_unmanaged_install_is_noop(self):
        self.assertEqual(setup.setup({})['changed'], False)
        self.assertEqual(self.server.calls, [])

    def test_proxy_only_does_not_gate_unmanaged_indexers(self):
        self.options['managed_indexers'] = []
        self.assertTrue(self.run_setup()['ready'])
        self.assertFalse(any(c[1] == 'indexer/test' for c in self.server.calls))

    def test_unsupported_source_and_bad_proxy_fail_before_requests(self):
        self.options['managed_indexers'] = ['noname']
        with self.assertRaises(setup.SetupError):
            self.run_setup()
        self.options['managed_indexers'] = ['rutor']
        self.options['proxy_url'] = 'socks5://user:secret@host:1080'
        with self.assertRaises(setup.SetupError):
            self.run_setup()
        self.assertEqual(self.server.calls, [])

    def test_backup_failure_prevents_writes(self):
        os.chmod(self.tmp.name, 0o755)
        with self.assertRaisesRegex(setup.SetupError, 'private'):
            self.run_setup()
        self.assertTrue(all(c[0] == 'GET' for c in self.server.calls))

    def test_auth_failure_never_echoes_remote_body_or_credentials(self):
        self.server.fail = 'config/host'
        stderr = io.StringIO()
        with mock.patch('sys.stdin', io.StringIO(json.dumps(self.options))), contextlib.redirect_stderr(stderr):
            self.assertEqual(setup.main(), 1)
        self.assertIn('authorization', stderr.getvalue())
        self.assertNotIn('secret-token', stderr.getvalue())
        self.assertNotIn('private-api-key', stderr.getvalue())
        self.assertNotIn('pass%40word', stderr.getvalue())

    def test_ambiguous_existing_definitions_fail(self):
        self.server.indexers.extend([copy.deepcopy(SCHEMA), copy.deepcopy(SCHEMA)])
        with self.assertRaisesRegex(setup.SetupError, 'Multiple'):
            self.run_setup()

    def test_api_refuses_external_url_and_inline_credentials(self):
        for url in ('https://remote.invalid', 'http://u:p@127.0.0.1:9696'):
            with self.assertRaises(setup.SetupError):
                setup.API(url, 'key')


if __name__ == '__main__':
    unittest.main()
