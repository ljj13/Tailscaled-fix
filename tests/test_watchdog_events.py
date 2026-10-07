"""Run the real watchdog body with isolated commands; never alter host routes."""
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class WatchdogEventTests(unittest.TestCase):
    def run_watchdog(self, mode):
        with tempfile.TemporaryDirectory() as name:
            directory = pathlib.Path(name)
            (directory / 'bin').mkdir()
            helper = directory / 'bin/android-netdiag'
            if mode != 'missing':
                helper.write_text('''#!/bin/sh
echo "$$" > "$TEST_DIR/observer.pid"
trap 'echo cleaned >> "$TEST_DIR/events"; exit 0' TERM
if [ "$MODE" = event ]; then
 sleep 0.15
 kill -USR1 "$2"
elif [ "$MODE" = burst ]; then
 sleep 0.15
 kill -USR1 "$2"
 sleep 0.03
 kill -USR1 "$2"
elif [ "$MODE" = dead ]; then
 exit 1
fi
while :; do sleep 0.1; done
''')
                helper.chmod(0o755)
            text = (ROOT / 'tailscale/scripts/tailscaled.service').read_text()
            body = text[text.index('watchdog_cleanup()'):text.index('start_watchdog()')]
            self.assertIn('+ 15', body)
            # Shorten only the isolated periodic test interval, not production.
            body = body.replace('+ 15', '+ 1')
            prelude = f'''tailscale_dir='{directory}'
tailscaled_run_dir='{directory}'
module_dir='{directory}'
sync_routes_auto=1
sync_every_ticks=4
diag() {{ echo "$*" >> "$TEST_DIR/events"; }}
daemon_pid() {{ echo 123; }}
routes_ok() {{ return 0; }}
add_routes() {{ echo forbidden-route >> "$TEST_DIR/events"; }}
exempt_tunnel() {{ :; }}
sync_outer_ipv6() {{ echo sync >> "$TEST_DIR/events"; }}
refresh_dns() {{ echo dns >> "$TEST_DIR/events"; }}
routes_sync() {{ echo subnet >> "$TEST_DIR/events"; }}
init_android_hostname() {{ echo hostname >> "$TEST_DIR/events"; }}
'''
            script = directory / 'watchdog'
            script.write_text(prelude + body + '\nroute_watchdog\n')
            env = {**os.environ, 'MODE': mode, 'TEST_DIR': name}
            process = subprocess.Popen(['sh', str(script)], env=env)
            try:
                import time
                time.sleep(0.5 if mode in ('event', 'burst') else 1.4)
                events = (directory / 'events').read_text()
                process.terminate()
                process.wait(timeout=3)
                if helper.exists():
                    pid = int((directory / 'observer.pid').read_text())
                    with self.assertRaises(ProcessLookupError):
                        os.kill(pid, 0)
                return events
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait()

    def test_verified_event_wakes_sleep_and_cleans_up_observer(self):
        text = self.run_watchdog('event')
        self.assertIn('physical network event', text)
        self.assertIn('sync', text)
        self.assertNotIn('subnet', text)

    def test_missing_or_failed_observer_keeps_periodic_self_healing(self):
        for mode in ('missing', 'dead'):
            with self.subTest(mode=mode):
                text = self.run_watchdog(mode)
                self.assertIn('sync', text)
                self.assertIn('hostname', text)

    def test_burst_does_not_run_reconciliation_concurrently(self):
        text = self.run_watchdog('burst')
        self.assertLessEqual(text.count('\nsync\n'), 2)
        self.assertNotIn('forbidden-route', text)


if __name__ == '__main__':
    unittest.main()
