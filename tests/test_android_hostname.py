"""Run hostname initialization and manual naming against an isolated installation."""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile
import time
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class AndroidHostnameTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory()
        cls.helper_binary = pathlib.Path(cls.build.name) / 'android-hostname'
        tool = ROOT / 'tools/android-hostname'
        if tool.exists():
            subprocess.run(['go', 'build', '-o', str(cls.helper_binary), '.'], cwd=tool, check=True)
        else:
            cls.helper_binary.write_text('#!/bin/sh\nexit 0\n')
            cls.helper_binary.chmod(0o755)

    @classmethod
    def tearDownClass(cls):
        cls.build.cleanup()

    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = pathlib.Path(self.temporary.name)
        self.state = self.root / 'state'
        (self.state / 'scripts').mkdir(parents=True)
        (self.state / 'run').mkdir()
        self.bins = self.root / 'bin'
        self.bins.mkdir()
        self.prefs = {'ControlURL': 'https://controlplane.tailscale.com', 'RouteAll': True,
                      'CorpDNS': True, 'WantRunning': True, 'LoggedOut': False,
                      'Hostname': '', 'ShieldsUp': False, 'AdvertiseRoutes': [],
                      'Persist': {'PrivateNodeKey': 'fixture-identity', 'LoginName': 'fixture-user'}}
        self.write_prefs()
        self.sources = {'settings:global:device_name': "Fog's Redmi Note 8 Pro",
                        'prop:ro.product.marketname': 'Redmi Note 8 Pro',
                        'prop:ro.product.model': 'M1906G7G', 'prop:ro.product.device': 'begonia'}
        self.write_sources()
        self.identity = self.state / 'run/tailscaled.state'
        self.identity.write_bytes(b'untouched node identity')
        self.identity.chmod(0o600)
        (self.state / 'routes').write_text('192.168.5.0/24\n')
        for command in ('settings', 'getprop'):
            self.install(self.bins / command, '''#!/usr/bin/env python3
import json,os,pathlib,sys
root=pathlib.Path(os.environ['FIXTURE'])
source=json.loads((root/'sources.json').read_text())
if pathlib.Path(sys.argv[0]).name=='settings':
    assert sys.argv[1]=='get'
    key='settings:'+':'.join(sys.argv[2:])
    default='null'
else:
    key='prop:'+sys.argv[1]
    default=''
print(source.get(key,default))
''')
        self.cli = self.bins / 'tailscale'
        self.install(self.cli, '''#!/usr/bin/env python3
import json,os,pathlib,sys,time
root=pathlib.Path(os.environ['FIXTURE'])
assert sys.argv[1]=='--socket='+str(root/'custom.sock'),sys.argv
args=sys.argv[2:]
prefs=root/'prefs.json'
if args==['debug','prefs']:
    if (root/'read-fail').exists(): sys.exit(1)
    if (root/'bad-prefs').exists(): print((root/'bad-prefs').read_text()); sys.exit(0)
    if (root/'pause-read').exists():
        (root/'read-entered').touch()
        time.sleep(0.6)
    print(json.dumps(json.loads(prefs.read_text()),indent='\t'))
elif args and args[0] in ('set','up','login'):
    if (root/'set-fail').exists(): sys.exit(1)
    if (root/'pause-set').exists() and any('hostname=fog-s-' in a for a in args):
        (root/'set-entered').touch()
        time.sleep(1.5)
    current=json.loads(prefs.read_text())
    for i,arg in enumerate(args):
        if arg.startswith(('--hostname=','-hostname=')): current['Hostname']=arg.split('=',1)[1]
        if arg in ('--hostname','-hostname'): current['Hostname']=args[i+1]
    prefs.write_text(json.dumps(current))
else: sys.exit(9)
''')
        (self.state / 'bin').mkdir()
        self.helper = self.state / 'bin/android-hostname'
        shutil.copyfile(self.helper_binary, self.helper)
        self.helper.chmod(0o755)
        self.settings = self.state / 'settings.ini'
        self.settings.write_text(f'tailscale_dir="{self.state}"\ntailscale_bin="{self.cli}"\n'
                                 f'tailscale_bin_param="--socket={self.root}/custom.sock"\n')
        self.settings_before = self.settings.read_bytes()
        self.wrapper = self.root / 'wrapper'
        self.install(self.wrapper, (ROOT / 'system/bin/tailscale').read_text().replace(
            '/data/adb/tailscale/settings.ini', str(self.settings)))
        self.service = self.state / 'scripts/tailscaled.service'
        self.install(self.service, (ROOT / 'tailscale/scripts/tailscaled.service').read_text())
        self.env = {**os.environ, 'FIXTURE': str(self.root), 'PATH': f'{self.bins}:'+os.environ['PATH']}

    def install(self, path, text):
        path.write_text(text.replace('#!/system/bin/sh', '#!/bin/sh'))
        path.chmod(0o755)

    @staticmethod
    def stop_process(process):
        if process.poll() is None:
            process.kill()
        process.communicate(timeout=3)

    def write_prefs(self):
        (self.root / 'prefs.json').write_text(json.dumps(self.prefs))

    def write_sources(self):
        (self.root / 'sources.json').write_text(json.dumps(self.sources))

    def hostname(self):
        return json.loads((self.root / 'prefs.json').read_text())['Hostname']

    def initialize(self):
        return subprocess.run([str(self.helper), 'init', str(self.state), str(self.cli),
                               f'--socket={self.root}/custom.sock'], env=self.env,
                              capture_output=True, text=True, timeout=20)

    def test_user_name_is_initialized_once_and_only_hostname_changes(self):
        result = self.initialize()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.hostname(), 'fog-s-redmi-note-8-pro')
        after = json.loads((self.root / 'prefs.json').read_text())
        self.assertEqual({k:v for k,v in after.items() if k!='Hostname'},
                         {k:v for k,v in self.prefs.items() if k!='Hostname'})
        self.sources['settings:global:device_name'] = 'Different phone'
        self.write_sources()
        self.assertEqual(self.initialize().returncode, 0)
        self.assertEqual(self.hostname(), 'fog-s-redmi-note-8-pro')
        self.assertIn('source=settings.global.device_name', (self.state / 'hostname-initialized').read_text())
        self.assertEqual(self.identity.read_bytes(), b'untouched node identity')
        self.assertEqual(self.identity.stat().st_mode & 0o777, 0o600)
        self.assertEqual(self.settings.read_bytes(), self.settings_before)
        self.assertEqual((self.state / 'routes').read_text(), '192.168.5.0/24\n')

    def test_existing_explicit_name_including_localhost_is_never_replaced(self):
        for name in ('my-custom-name', 'localhost', 'localhost-0'):
            with self.subTest(name=name):
                (self.state / 'hostname-initialized').unlink(missing_ok=True)
                self.prefs['Hostname'] = name
                self.write_prefs()
                self.initialize()
                self.assertEqual(self.hostname(), name)
                # Even an explicit override subsequently reset through the raw CLI
                # must not re-enable automatic initialization after it was observed.
                self.prefs['Hostname'] = ''
                self.write_prefs()
                self.initialize()
                self.assertEqual(self.hostname(), '')

    def test_priority_and_invalid_user_name_fallback(self):
        cases = [({'settings:global:device_name': '我的手机', 'prop:ro.product.marketname': 'Redmi Note 8 Pro'}, 'redmi-note-8-pro'),
                 ({'settings:global:device_name': 'null', 'prop:persist.sys.device_name': 'My Phone'}, 'my-phone'),
                 ({'settings:secure:bluetooth_name': 'Personal phone', 'prop:ro.product.marketname': 'Retail phone'}, 'personal-phone'),
                 ({'prop:ro.product.vendor.marketname': 'POCO X3 NFC', 'prop:ro.product.model': 'code'}, 'poco-x3-nfc'),
                 ({'prop:ro.product.model': 'SM-G991B/DS', 'prop:ro.product.device': 'board'}, 'sm-g991b-ds'),
                 ({'prop:ro.product.device': 'begonia'}, 'begonia')]
        for sources, want in cases:
            with self.subTest(sources=sources):
                (self.state / 'hostname-initialized').unlink(missing_ok=True)
                self.prefs['Hostname'] = ''
                self.write_prefs()
                self.sources = sources
                self.write_sources()
                self.initialize()
                self.assertEqual(self.hostname(), want)

    def test_normalization_handles_punctuation_unicode_and_label_limit(self):
        cases = [(' __Mi..PHONE / 5G!! ', 'mi-phone-5g'), ('Fog的 Phone', 'fog-phone'),
                 ('A'*62+' '+'B'*10, 'a'*62), ('A'*100, 'a'*63)]
        for value, want in cases:
            with self.subTest(value=value):
                (self.state / 'hostname-initialized').unlink(missing_ok=True)
                self.prefs['Hostname'] = ''
                self.write_prefs()
                self.sources = {'settings:global:device_name': value}
                self.write_sources()
                self.initialize()
                self.assertEqual(self.hostname(), want)

    def test_unavailable_or_unreadable_preferences_and_names_retry_safely(self):
        (self.root / 'read-fail').touch()
        self.initialize()
        self.assertEqual(self.hostname(), '')
        self.assertFalse((self.state / 'hostname-initialized').exists())
        (self.root / 'read-fail').unlink()
        for bad in ('garbage', '{}', 'null', '{\n\t"Hostname": null\n}', '{\n\t"Hostname": "",\n'):
            (self.root / 'bad-prefs').write_text(bad)
            self.initialize()
            self.assertEqual(self.hostname(), '')
            self.assertFalse((self.state / 'hostname-initialized').exists())
        (self.root / 'bad-prefs').unlink()
        self.sources = {}
        self.write_sources()
        self.initialize()
        self.assertEqual(self.hostname(), '')
        self.assertFalse((self.state / 'hostname-initialized').exists())
        self.sources = {'prop:ro.product.model': 'Recovery Phone'}
        self.write_sources()
        (self.root / 'set-fail').touch()
        self.initialize()
        self.assertEqual(self.hostname(), '')
        self.assertFalse((self.state / 'hostname-initialized').exists())
        (self.root / 'set-fail').unlink()
        self.initialize()
        self.assertEqual(self.hostname(), 'recovery-phone')

    def test_wrapper_manual_clear_opts_out_before_first_initialization(self):
        r = subprocess.run([str(self.wrapper), 'set', '--hostname='], env=self.env, capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.initialize()
        self.assertEqual(self.hostname(), '')
        self.assertTrue((self.state / 'hostname-user-set').exists())

    def test_single_dash_hostname_flags_also_protect_manual_intent(self):
        for flags in (['-hostname='], ['-hostname', '']):
            with self.subTest(flags=flags):
                shutil.rmtree(self.state / 'hostname-user-set', ignore_errors=True)
                r = subprocess.run([str(self.wrapper), 'set', *flags], env=self.env, capture_output=True, text=True)
                self.assertEqual(r.returncode, 0, r.stderr)
                self.initialize()
                self.assertEqual(self.hostname(), '')
                self.assertTrue((self.state / 'hostname-user-set').exists())

    def test_manual_naming_wins_concurrent_initialization(self):
        (self.root / 'pause-read').touch()
        initializer = subprocess.Popen([str(self.helper), 'init', str(self.state), str(self.cli),
                                       f'--socket={self.root}/custom.sock'], env=self.env,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.stop_process, initializer)
        for _ in range(150):
            if (self.root / 'read-entered').exists(): break
            if initializer.poll() is not None: break
            time.sleep(0.01)
        self.assertTrue((self.root / 'read-entered').exists())
        # Run the real service API while its default-name read is in flight.
        manual = subprocess.run([str(self.service), 'set-pref', 'hostname', 'my-choice'],
                                env=self.env, capture_output=True, text=True, timeout=20)
        self.assertEqual(manual.returncode, 0, manual.stderr)
        initializer.communicate(timeout=20)
        self.assertEqual(self.hostname(), 'my-choice')
        self.prefs['Hostname'] = ''
        self.write_prefs()
        self.initialize()
        self.assertEqual(self.hostname(), '')

    def test_cli_manual_naming_wins_concurrent_initialization(self):
        (self.root / 'pause-read').touch()
        initializer = subprocess.Popen([str(self.helper), 'init', str(self.state), str(self.cli),
                                       f'--socket={self.root}/custom.sock'], env=self.env,
                                      stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
        self.addCleanup(self.stop_process, initializer)
        for _ in range(150):
            if (self.root / 'read-entered').exists(): break
            if initializer.poll() is not None: break
            time.sleep(0.01)
        self.assertTrue((self.root / 'read-entered').exists())
        manual = subprocess.run([str(self.wrapper), 'set', '--hostname', 'cli-choice'],
                                env=self.env, capture_output=True, text=True, timeout=20)
        self.assertEqual(manual.returncode, 0, manual.stderr)
        initializer.communicate(timeout=20)
        self.assertEqual(self.hostname(), 'cli-choice')
        self.assertTrue((self.state / 'hostname-user-set').exists())

    def test_killed_initializer_releases_lock_for_retry(self):
        (self.root / 'pause-read').touch()
        initializer = subprocess.Popen([str(self.helper), 'init', str(self.state), str(self.cli),
                                       f'--socket={self.root}/custom.sock'], env=self.env,
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(self.stop_process, initializer)
        for _ in range(150):
            if (self.root / 'read-entered').exists(): break
            if initializer.poll() is not None: break
            time.sleep(0.01)
        self.assertTrue((self.root / 'read-entered').exists())
        initializer.kill()
        initializer.wait(timeout=3)
        (self.root / 'pause-read').unlink()
        self.initialize()
        self.assertEqual(self.hostname(), 'fog-s-redmi-note-8-pro')

    def test_killed_initializer_cannot_leave_orphan_set_overwriting_manual_name(self):
        (self.root / 'pause-set').touch()
        initializer = subprocess.Popen([str(self.helper), 'init', str(self.state), str(self.cli),
                                       f'--socket={self.root}/custom.sock'], env=self.env,
                                      stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        self.addCleanup(self.stop_process, initializer)
        for _ in range(250):
            if (self.root / 'set-entered').exists(): break
            if initializer.poll() is not None: break
            time.sleep(0.01)
        self.assertTrue((self.root / 'set-entered').exists())
        initializer.kill()
        initializer.wait(timeout=3)
        manual = subprocess.run([str(self.wrapper), 'set', '--hostname=survives-crash'],
                                env=self.env, capture_output=True, text=True, timeout=20)
        self.assertEqual(manual.returncode, 0, manual.stderr)
        time.sleep(1.7)
        self.assertEqual(self.hostname(), 'survives-crash')

    def test_start_and_watchdog_retry_initialize_after_preferences_recover(self):
        text = (ROOT / 'tailscale/scripts/tailscaled.service').read_text()
        init = text[text.index('init_android_hostname()'):text.index('show_hostname_init()')]
        start = text[text.index('start_tailscaled()'):text.index('stop_tailscaled()')]
        watchdog = text[text.index('route_watchdog()'):text.index('start_watchdog()')]
        (self.root / 'read-fail').touch()
        fixture = f'. "{self.settings}"\ntailscaled_diag_log="{self.root}/diag.log"\n'
        fixture += '''
sync_routes_auto=0
daemon_pid() { echo 123; }
daemon_is_current() { return 0; }
log() { :; }
diag() { :; }
add_routes() { :; }
refresh_dns() { :; }
start_watchdog() { :; }
routes_ok() { return 0; }
exempt_tunnel() { :; }
ticks=0
sleep() {
  ticks=$((ticks+1))
  [ "$ticks" = 1 ] || exit 0
  rm -f "$FIXTURE/read-fail"
}
'''
        program = fixture + init + start + watchdog + '''
start_tailscaled || exit 10
[ ! -f "$tailscale_dir/hostname-initialized" ] || exit 11
route_watchdog
'''
        result = subprocess.run(['sh'], input=program, env=self.env, text=True, capture_output=True, timeout=20)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.hostname(), 'fog-s-redmi-note-8-pro')

    def test_simultaneous_initializers_do_not_reset_an_initialized_name(self):
        (self.root / 'pause-read').touch()
        commands = [str(self.helper), 'init', str(self.state), str(self.cli), f'--socket={self.root}/custom.sock']
        processes = [subprocess.Popen(commands, env=self.env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) for _ in range(4)]
        for process in processes:
            self.addCleanup(self.stop_process, process)
        for process in processes:
            stdout, stderr = process.communicate(timeout=20)
            self.assertEqual(process.returncode, 0, stderr)
        self.assertEqual(self.hostname(), 'fog-s-redmi-note-8-pro')
        self.sources = {'settings:global:device_name': 'Later phone'}
        self.write_sources()
        self.initialize()
        self.assertEqual(self.hostname(), 'fog-s-redmi-note-8-pro')


if __name__ == '__main__':
    unittest.main()
