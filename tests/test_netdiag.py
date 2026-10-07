"""Run the actual selftest and diagnostic collector with isolated command fixtures."""
import json
import os
import pathlib
import shutil
import socket
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class NetworkDiagnosticServiceTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        if not shutil.which('go'):
            raise unittest.SkipTest('Go is required for diagnostic helper integration')
        cls.work = tempfile.TemporaryDirectory()
        cls.binary = pathlib.Path(cls.work.name) / 'android-netdiag'
        subprocess.run(['go', 'build', '-o', str(cls.binary), '.'], cwd=ROOT / 'tools/android-netdiag', check=True)

    @classmethod
    def tearDownClass(cls):
        cls.work.cleanup()

    def fixture(self, directory, mode, helper=True):
        directory = pathlib.Path(directory)
        (directory / 'bin').mkdir()
        (directory / 'run').mkdir()
        (directory / 'scripts').mkdir()
        identity = directory / 'run/tailscaled.state'
        identity.write_bytes(b'unchanged node identity')
        identity.chmod(0o600)
        cli = directory / 'bin/tailscale'
        cli.write_text('#!/bin/sh\n' + ('exec sleep 10\n' if mode == 'timeout' else "printf '{invalid JSON'\n"))
        cli.chmod(0o755)
        daemon = directory / 'bin/tailscaled'
        daemon.write_text('#!/bin/sh\necho version-fixture\n')
        daemon.chmod(0o755)
        if helper:
            wrapper = directory / 'bin/android-netdiag'
            wrapper.write_text(f'#!/bin/sh\nexec "{self.binary}" --timeout=150ms "$@"\n')
            wrapper.chmod(0o755)
        settings = directory / 'settings.ini'
        settings.write_text(f'tailscale_dir="{directory}"\ntailscale_bin_param="--socket={directory}/run/tailscaled.sock"\n')
        script = directory / 'scripts/tailscaled.service'
        content = (ROOT / 'tailscale/scripts/tailscaled.service').read_text()
        content = content[:content.index('# ------------------------------------------------------------------ dispatch --')]
        content += '''
daemon_pid() { echo 123; }
hash_file() { echo fixture-hash; }
has_osrouter() { return 0; }
daemon_is_current() { return 0; }
main_route_matches() { return 0; }
detect_default_route() { :; }
_exempt_state() { echo OK; }
check_dns() { echo dns_reachable=true; }
ip() { :; }
iptables() { :; }
case "$1" in selftest) selftest ;; netdiag) network_diagnostics json ;; report) diagnostic_report ;; esac
'''
        script.write_text(content)
        listener = socket.socket(socket.AF_UNIX)
        listener.bind(str(directory / 'run/tailscaled.sock'))
        self.addCleanup(listener.close)
        return script, identity

    def test_selftest_completes_on_invalid_json_and_command_timeout(self):
        for mode in ('malformed', 'timeout'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                script, identity = self.fixture(directory, mode)
                result = subprocess.run(['sh', str(script), 'selftest'], capture_output=True, text=True, timeout=3)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('===== end selftest =====', result.stdout)
                self.assertIn('netcheck', result.stdout)
                if mode == 'timeout':
                    self.assertIn('timeout=true', result.stdout)
                self.assertEqual(identity.read_bytes(), b'unchanged node identity')
                self.assertEqual(identity.stat().st_mode & 0o777, 0o600)

    def test_missing_helper_returns_structured_failure_without_breaking_selftest(self):
        with tempfile.TemporaryDirectory() as directory:
            script, _ = self.fixture(directory, 'malformed', helper=False)
            result = subprocess.run(['sh', str(script), 'netdiag'], capture_output=True, text=True, timeout=3)
            self.assertEqual(result.returncode, 0)
            self.assertTrue(json.loads(result.stdout)['errors'])
            result = subprocess.run(['sh', str(script), 'selftest'], capture_output=True, text=True, timeout=3)
            self.assertEqual(result.returncode, 0)
            self.assertIn('helper unavailable', result.stdout)
            self.assertIn('===== end selftest =====', result.stdout)

    def test_report_partial_failure_is_safe_and_keeps_identity(self):
        for mode in ('malformed', 'timeout'):
            with self.subTest(mode=mode), tempfile.TemporaryDirectory() as directory:
                script, identity = self.fixture(directory, mode)
                result = subprocess.run(['sh', str(script), 'report'], capture_output=True, text=True, timeout=3)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertIn('Tailscaled-fix Diagnostic Report', result.stdout)
                self.assertIn('redaction: enabled', result.stdout)
                self.assertIn('<unavailable:', result.stdout)
                self.assertNotIn('unchanged node identity', result.stdout)
                self.assertEqual(identity.read_bytes(), b'unchanged node identity')

    def test_missing_report_helper_returns_safe_unavailable(self):
        with tempfile.TemporaryDirectory() as directory:
            script, identity = self.fixture(directory, 'malformed', helper=False)
            result = subprocess.run(['sh', str(script), 'report'], capture_output=True, text=True, timeout=3)
            self.assertEqual(result.returncode, 0)
            self.assertIn('redaction: enabled', result.stdout)
            self.assertIn('<unavailable: diagnostic helper missing>', result.stdout)
            self.assertNotIn('unchanged node identity', result.stdout)
            self.assertEqual(identity.read_bytes(), b'unchanged node identity')


if __name__ == '__main__':
    unittest.main()
