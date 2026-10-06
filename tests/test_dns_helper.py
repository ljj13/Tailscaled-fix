"""Execute the real Linux helper with OEM-style ConnectivityService fixtures."""
import json
import os
import pathlib
import subprocess
import tempfile
import time
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class DNSHelperTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.build = tempfile.TemporaryDirectory()
        cls.binary = pathlib.Path(cls.build.name) / 'android-dns'
        subprocess.run(['go', 'build', '-o', str(cls.binary), '.'], cwd=ROOT / 'tools/android-dns', check=True)

    @classmethod
    def tearDownClass(cls):
        cls.build.cleanup()

    def fixture(self, tmp, changed):
        bins = tmp / 'bin'
        bins.mkdir()
        dump = '''Active default network: 106
Current Networks:
 NetworkAgentInfo{network{106} ni{MOBILE CONNECTED} lp{{InterfaceName: ccmni1 DnsAddresses: [/192.0.2.1]}} nc{[Transports: CELLULAR UnderlyingNetworks: Null]}}
 NetworkAgentInfo{network{107} ni{VPN CONNECTED} lp{{InterfaceName: tun0 DnsAddresses: [/172.19.0.2]}} nc{[Transports: CELLULAR|VPN UnderlyingNetworks: [106]]}}
'''
        (tmp / 'before').write_text(dump)
        (tmp / 'after').write_text(dump.replace('106', '109') if changed else dump)
        (bins / 'dumpsys').write_text('''#!/bin/sh
if [ -f "$FIXTURE/seen" ]; then cat "$FIXTURE/after"; else touch "$FIXTURE/seen"; cat "$FIXTURE/before"; fi
''')
        for cmd in ('getprop', 'ip'):
            (bins / cmd).write_text('#!/bin/sh\nexit 0\n')
        for cmd in bins.iterdir():
            cmd.chmod(0o755)
        return {**os.environ, 'FIXTURE': str(tmp), 'PATH': str(bins) + ':' + os.environ['PATH']}

    def test_first_boot_network_change_returns_retry_without_localhost_bootstrap(self):
        with tempfile.TemporaryDirectory() as directory:
            tmp = pathlib.Path(directory)
            env = self.fixture(tmp, changed=True)
            state = tmp / 'state'
            r = subprocess.run([str(self.binary), '--dir', str(state), '--iface', 'tun0', '--fallback', ''], env=env, capture_output=True, text=True, timeout=20)
            self.assertNotEqual(r.returncode, 0)
            self.assertIn('network changed before first bootstrap', r.stderr)
            self.assertFalse((state / 'bootstrap-resolv.conf').exists())

    def test_failed_probes_preserve_verified_file_and_identity(self):
        with tempfile.TemporaryDirectory() as directory:
            tmp = pathlib.Path(directory)
            env = self.fixture(tmp, changed=False)
            state = tmp / 'state'
            state.mkdir()
            contents = b'# retained resolver\nnameserver 192.0.2.2\noptions timeout:2 attempts:2\n'
            (state / 'bootstrap-resolv.conf').write_bytes(contents)
            (state / 'tailscaled.state').write_bytes(b'node identity')
            (state / 'tailscaled.state').chmod(0o600)
            (state / 'dns-verified.json').write_text(json.dumps({
                'Network': '106', 'Iface': 'ccmni1', 'Source': 'android-linkproperties',
                'Servers': ['192.0.2.2'], 'BootID': pathlib.Path('/proc/sys/kernel/random/boot_id').read_text().strip(),
                'Verified': time.strftime('%Y-%m-%dT%H:%M:%SZ', time.gmtime()),
            }))
            r = subprocess.run([str(self.binary), '--dir', str(state), '--iface', 'tun0', '--fallback', ''], env=env, capture_output=True, text=True, timeout=20)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertIn('dns_retained=true', r.stdout)
            self.assertIn('dns_reachable=false', r.stdout)
            self.assertEqual((state / 'bootstrap-resolv.conf').read_bytes(), contents)
            r = subprocess.run([str(self.binary), '--dir', str(state), '--iface', 'tun0', '--fallback', ''], env=env, capture_output=True, text=True, timeout=20)
            self.assertEqual(r.returncode, 0, r.stderr)
            self.assertEqual((state / 'bootstrap-resolv.conf').read_bytes(), contents)
            self.assertEqual((state / 'tailscaled.state').read_bytes(), b'node identity')
            self.assertEqual((state / 'tailscaled.state').stat().st_mode & 0o777, 0o600)


if __name__ == '__main__':
    unittest.main()
