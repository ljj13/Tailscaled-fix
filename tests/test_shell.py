#!/usr/bin/env python3
"""Execute Android scripts against isolated fixtures; never run host iptables."""
import os
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]

class AndroidShellTests(unittest.TestCase):
    def test_shell_syntax(self):
        paths = [ROOT / p for p in ('customize.sh', 'service.sh', 'uninstall.sh', 'scripts/build.sh')]
        paths += list((ROOT / 'tailscale/scripts').glob('*')) + list((ROOT / 'system/bin').glob('*'))
        for p in paths:
            r = subprocess.run(['sh', '-n'], input=p.read_text(encoding='utf-8'), text=True, capture_output=True)
            self.assertEqual(r.returncode, 0, f'{p}: {r.stderr}')

    def test_upgrade_and_first_install_preserve_identity_and_config(self):
        for existing in (False, True):
            with self.subTest(existing=existing), tempfile.TemporaryDirectory() as tmp:
                tmp = pathlib.Path(tmp)
                install, module, boot = tmp / 'state', tmp / 'module', tmp / 'service.d'
                shutil.copytree(ROOT / 'tailscale', module / 'tailscale')
                shutil.copytree(ROOT / 'system', module / 'system')
                (module / 'files').mkdir()
                for f in ('tailscale.combined', 'android-dns', 'android-hostname', 'android-netdiag'):
                    (module / 'files' / f).write_text('test executable', encoding='utf-8')
                (module / 'service.sh').write_text('boot script', encoding='utf-8')
                (install / 'run').mkdir(parents=True)
                key = install / 'run/tailscaled.state'
                key.write_bytes(b'node identity and persisted preferences')
                key.chmod(0o600)
                initialized = install / 'hostname-initialized'
                initialized.write_bytes(b'source=settings.global.device_name\nhostname=my-phone\n')
                initialized.chmod(0o600)
                (install / 'hostname-user-set').mkdir(mode=0o700)
                if existing:
                    (install / 'settings.ini').write_text('user settings', encoding='utf-8')
                    (install / 'routes').write_text('192.168.100.0/24\n', encoding='utf-8')
                script = (ROOT / 'customize.sh').read_text(encoding='utf-8').replace('/data/adb/tailscale', str(install)).replace('/data/adb/service.d', str(boot))
                prelude = '''ui_print() { :; }
abort() { echo "$*" >&2; exit 1; }
pgrep() { return 1; }
sleep() { :; }
chown() { :; }  # Android installs run as root; this isolated fixture does not.
set_perm() { chmod "$4" "$1"; }
set_perm_recursive() { find "$1" -type d -exec chmod "$4" {} \\; ; find "$1" -type f -exec chmod "$5" {} \\; ; }
'''
                env = {**os.environ, 'BOOTMODE': 'true', 'ARCH': 'arm64', 'MODPATH': str(module)}
                r = subprocess.run(['sh'], input=prelude + script, text=True, env=env, capture_output=True)
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertEqual(key.read_bytes(), b'node identity and persisted preferences')
                self.assertEqual(key.stat().st_mode & 0o777, 0o600)
                self.assertEqual(initialized.read_bytes(), b'source=settings.global.device_name\nhostname=my-phone\n')
                self.assertEqual(initialized.stat().st_mode & 0o777, 0o600)
                self.assertTrue((install / 'hostname-user-set').is_dir())
                self.assertEqual((install / 'hostname-user-set').stat().st_mode & 0o777, 0o700)
                self.assertTrue((install / 'bin/android-dns').exists())
                self.assertTrue((boot / 'tailscaled_service.sh').exists())
                self.assertTrue((install / 'scripts/tailscaled.service').exists())
                self.assertTrue((install / 'bin/android-hostname').exists())
                self.assertTrue((install / 'bin/android-netdiag').exists())
                if existing:
                    self.assertEqual((install / 'settings.ini').read_text(), 'user settings')
                    self.assertEqual((install / 'routes').read_text(), '192.168.100.0/24\n')
                else:
                    self.assertIn('dns_fallback_servers=', (install / 'settings.ini').read_text())

    def test_missing_payload_aborts_before_stopping_or_replacing(self):
        with tempfile.TemporaryDirectory() as tmp:
            marker = pathlib.Path(tmp) / 'stopped'
            script = (ROOT / 'customize.sh').read_text(encoding='utf-8')
            prelude = f'ui_print() {{ :; }}\nabort() {{ exit 1; }}\npgrep() {{ touch "{marker}"; }}\n'
            env = {**os.environ, 'BOOTMODE': 'true', 'ARCH': 'arm64', 'MODPATH': tmp}
            r = subprocess.run(['sh'], input=prelude + script, text=True, env=env, capture_output=True)
            self.assertNotEqual(r.returncode, 0)
            self.assertFalse(marker.exists())

    def test_no_gateway_route_and_wifi_mobile_transition(self):
        service = (ROOT / 'tailscale/scripts/tailscaled.service').read_text(encoding='utf-8')
        functions = service[service.index('ordinary_default_route()'):service.index('# Does this binary link osrouter')]
        with tempfile.TemporaryDirectory() as tmp:
            tmp = pathlib.Path(tmp)
            mock = tmp / 'ip'
            mock.write_text('''#!/bin/sh
if [ "$1 $2 $3" = "route get 8.8.8.8" ]; then cat "$MOCK_PROBE"; exit 0; fi
if [ "$1 $2 $3" = "route show table" ]; then cat "$MOCK_MAIN"; exit 0; fi
if [ "$1 $2" = "route replace" ]; then echo "$*" >> "$MOCK_CHANGES"; exit 0; fi
exit 1
''')
            mock.chmod(0o755)
            env = {**os.environ, 'PATH': str(tmp) + ':' + os.environ['PATH'], 'MOCK_PROBE': str(tmp / 'probe'), 'MOCK_MAIN': str(tmp / 'main'), 'MOCK_CHANGES': str(tmp / 'changes')}
            for probe, main in [('8.8.8.8 via 192.168.1.1 dev wlan0 src 192.168.1.2', 'default via 192.168.1.1 dev wlan0'), ('8.8.8.8 dev rmnet_data0 src 10.1.2.3', 'default dev rmnet_data0')]:
                (tmp / 'probe').write_text(probe + '\n')
                (tmp / 'main').write_text(main + '\n')
                r = subprocess.run(['sh'], input=functions + '\ninfo=$(detect_default_route)\nmain_route_matches "$info" || exit 10\napply_main_default "$info"\n', text=True, env=env, capture_output=True)
                self.assertEqual(r.returncode, 0, r.stderr)
                self.assertIn(main.removeprefix('default '), r.stdout)
            changes = (tmp / 'changes').read_text()
            self.assertIn('default dev rmnet_data0 table main', changes)
            self.assertNotIn('table 52', changes)

    def test_vpn_default_is_never_copied_to_main(self):
        service = (ROOT / 'tailscale/scripts/tailscaled.service').read_text(encoding='utf-8')
        functions = service[service.index('ordinary_default_route()'):service.index('# Does this binary link osrouter')]
        with tempfile.TemporaryDirectory() as tmp:
            tmp = pathlib.Path(tmp)
            (tmp / 'ip').write_text('''#!/bin/sh
if [ "$1 $2" = "route get" ]; then echo '8.8.8.8 dev tun0 src 172.19.0.1'; exit 0; fi
if [ "$1 $2" = "route replace" ]; then echo "$*" >> "$CHANGES"; exit 0; fi
exit 1
''')
            (tmp / 'ip').chmod(0o755)
            helper = tmp / 'android-dns'
            helper.write_text("#!/bin/sh\necho dns_network=106\necho 'dns_physical_route=10.1.2.3 ccmni1 10.1.2.3'\n")
            helper.chmod(0o755)
            env = {**os.environ, 'PATH': str(tmp) + ':' + os.environ['PATH'], 'android_dns_bin': str(helper), 'CHANGES': str(tmp / 'changes')}
            r = subprocess.run(['sh'], input=functions + '\ninfo=$(detect_default_route)\napply_main_default "$info"\napply_main_default "- tun0 172.19.0.1" && exit 10\nexit 0\n', text=True, env=env, capture_output=True)
            self.assertEqual(r.returncode, 0, r.stderr)
            changes = (tmp / 'changes').read_text()
            self.assertIn('default via 10.1.2.3 dev ccmni1 table main', changes)
            self.assertNotIn('tun0', changes)
            helper.unlink()
            r = subprocess.run(['sh'], input=functions + '\ninfo=$(detect_default_route)\n[ -z "$info" ]\n', text=True, env=env, capture_output=True)
            self.assertEqual(r.returncode, 0, r.stderr)

    def test_offline_start_without_fallback_recovers_automatically(self):
        service = (ROOT / 'tailscale/scripts/tailscaled.service').read_text(encoding='utf-8')
        start = service[service.index('start_tailscaled()'):service.index('stop_tailscaled()')]
        watchdog = service[service.index('route_watchdog()'):service.index('start_watchdog()')]
        with tempfile.TemporaryDirectory() as tmp:
            tmp = pathlib.Path(tmp)
            env = {**os.environ, 'FIXTURE': str(tmp)}
            # Replace external Android operations; exercise production startup
            # and retry branches without changing the host network or state.
            fixture = r'''
tailscale_dir="$FIXTURE"
tailscaled_run_dir="$FIXTURE/run"
tailscaled_pid="$FIXTURE/run/daemon.pid"
tailscaled_log="$FIXTURE/daemon.log"
tailscaled_bin=/bin/true
tailscale_bin=/bin/true
module_dir="$FIXTURE/module"
sync_routes_auto=0
mkdir -p "$tailscaled_run_dir"
daemon_pid() { [ -f "$tailscaled_pid" ] && cat "$tailscaled_pid"; }
ensure_binary() { return 0; }
setup_home() { :; }
fix_controlplane_routing() { :; }
refresh_dns() { [ -f "$FIXTURE/online" ]; }
start_watchdog() { touch "$FIXTURE/worker"; }
diag() { :; }
log() { :; }
set_module_status() { :; }
add_routes() { :; }
exempt_tunnel() { :; }
routes_ok() { return 0; }
init_android_hostname() { :; }
nohup() { return 0; }
ticks=0
sleep() {
  [ "$1" = 15 ] || return 0
  ticks=$((ticks+1))
  [ "$ticks" -le 1 ] || exit 0
  touch "$FIXTURE/online"
}
'''
            program = fixture + start + watchdog + r'''
start_tailscaled
[ -f "$tailscaled_run_dir/dns-start-pending" ] || exit 11
[ -f "$FIXTURE/worker" ] || exit 12
[ ! -f "$tailscaled_pid" ] || exit 13
route_watchdog
'''
            r = subprocess.run(['sh'], input=program, text=True, env=env, capture_output=True)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertTrue((tmp / 'run/daemon.pid').exists())
            self.assertFalse((tmp / 'run/dns-start-pending').exists())

if __name__ == '__main__':
    unittest.main()
