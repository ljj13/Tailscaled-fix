import pathlib
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class DNSBuildTests(unittest.TestCase):
    def test_go_resolver_is_patched_not_only_tailscale_manager(self):
        build = (ROOT / 'scripts/prepare-build.py')
        self.assertTrue(build.exists(), 'Go standard-library resolver still reads missing /etc/resolv.conf')
        text = build.read_text(encoding='utf-8')
        self.assertIn('dnsconfig_unix.go', text)
        self.assertIn('bootstrap-resolv.conf', text)

    def test_dns_refresh_precedes_daemon_launch_and_runs_in_watchdog(self):
        text = (ROOT / 'tailscale/scripts/tailscaled.service').read_text(encoding='utf-8')
        start = text[text.index('start_tailscaled()'):text.index('stop_tailscaled()')]
        self.assertIn('refresh_dns', start)
        self.assertLess(start.index('refresh_dns'), start.index('nohup'))
        watchdog = text[text.index('route_watchdog()'):text.index('start_watchdog()')]
        self.assertIn('refresh_dns', watchdog)

if __name__ == '__main__':
    unittest.main()
