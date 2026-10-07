"""Exercise physical policy selection and cleanup without touching host networking."""
import json
import os
import pathlib
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class OuterIPv6Tests(unittest.TestCase):
    def setUp(self):
        self.work = tempfile.TemporaryDirectory()
        self.addCleanup(self.work.cleanup)
        self.directory = pathlib.Path(self.work.name)
        self.state = self.directory / 'network.json'
        self.provider = self.directory / 'android-dns'
        self.provider.write_text('#!/bin/sh\ncat "$NETWORK_FILE"\n')
        self.provider.chmod(0o755)
        self.network_file = self.directory / 'network'
        self.cli = self.directory / 'tailscale'
        self.cli.write_text('#!/bin/sh\necho "$*" >> "$RESTUN_LOG"\n')
        self.cli.chmod(0o755)
        ip = self.directory / 'ip'
        ip.write_text('''#!/usr/bin/env python3
import json,os,sys
from pathlib import Path
p=Path(os.environ['MOCK_STATE']);s=json.loads(p.read_text());a=sys.argv[1:]
assert a[0]=='-6',a
if a[1:]==['rule','show']:print('\\n'.join(s['rules']))
elif a[1:3]==['rule','add']:
 if s.get('fail_add'):sys.exit(1)
 pref=a[a.index('pref')+1];mark=a[a.index('fwmark')+1];table=a[a.index('lookup')+1]
 s['rules'].append(f'{pref}: from all fwmark {mark} iif lo lookup {table}')
elif a[1:3]==['rule','del']:
 pref=a[a.index('pref')+1];table=a[a.index('lookup')+1]
 for i,r in enumerate(s['rules']):
  if r.startswith(pref+':') and r.endswith('lookup '+table) and '0x10020000/0x1e020000' in r:
   del s['rules'][i];break
elif a[1:4]==['route','show','table']:print(s['tables'].get(a[4],''))
elif a[1:4]==['addr','show','dev']:print(s['addresses'].get(a[4],''))
else:sys.exit(1)
s['calls'].append(a);p.write_text(json.dumps(s))
''')
        ip.chmod(0o755)
        self.env = {**os.environ, 'PATH': str(self.directory) + ':' + os.environ['PATH'],
                    'MOCK_STATE': str(self.state), 'NETWORK_FILE': str(self.network_file),
                    'RESTUN_LOG': str(self.directory / 'restun')}
        self.data = {'rules': [], 'tables': {}, 'addresses': {}, 'calls': []}
        self.identity = self.directory / 'tailscaled.state'
        self.identity.write_bytes(b'existing identity')
        self.identity.chmod(0o600)

    def physical(self, netid=114, iface='ccmni1', table=None, ipv6=True):
        table = table or iface
        self.network_file.write_text(f'dns_network={netid}\ndns_iface={iface}\ndns_transport=CELLULAR\ndns_underlying={netid}\n')
        self.data['rules'] = [r for r in self.data['rules'] if r.startswith('5209:')]
        self.data['rules'] += [f'16000: from all fwmark {hex(0x10000 | netid)}/0x1ffff iif lo lookup {table}',
                               '16000: from all fwmark 0x10067/0x1ffff iif lo lookup tun0']
        self.data['tables'][table] = f'default via fe80::5 dev {iface} proto ra metric 1024\n' if ipv6 else ''
        self.data['addresses'][iface] = 'inet6 2408:1234::1/64 scope global' if ipv6 else ''
        self.state.write_text(json.dumps(self.data))

    def run_sync(self, remove=False):
        service = (ROOT / 'tailscale/scripts/tailscaled.service').read_text(encoding='utf-8')
        self.assertTrue('# Android outer IPv6 physical policy' in service,
                        'physical IPv6 policy repair is missing')
        functions = service[service.index('# Android outer IPv6 physical policy'):service.index('fix_controlplane_routing()')]
        prelude = f'''tailscale_dir='{self.directory}'
tailscaled_run_dir='{self.directory}'
android_dns_bin='{self.provider}'
tailscale_bin='{self.cli}'
tailscale_bin_param='--socket=custom.sock'
diag() {{ :; }}
daemon_pid() {{ echo 123; }}
'''
        result = subprocess.run(['sh'], input=prelude + functions + ('\nclear_outer_ipv6\n' if remove else '\nsync_outer_ipv6\n'),
                                env=self.env, text=True, capture_output=True, timeout=8)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.data = json.loads(self.state.read_text())
        self.assertEqual(self.identity.read_bytes(), b'existing identity')
        self.assertEqual(self.identity.stat().st_mode & 0o777, 0o600)
        return (self.directory / 'outer-ipv6-status').read_text()

    def owned(self):
        return [r for r in self.data['rules'] if r.startswith('5209:')]

    def test_cellular_and_changed_netid_use_actual_table_and_restun_once(self):
        self.physical()
        status = self.run_sync()
        self.assertIn('outer_ipv6_network=114', status)
        self.assertIn('outer_ipv6_table=ccmni1', status)
        self.assertEqual(len(self.owned()), 1)
        self.assertIn('iif lo', self.owned()[0])
        self.run_sync()
        self.assertEqual(len((self.directory / 'restun').read_text().splitlines()), 1)
        self.physical(120, 'ccmni3', '1032')
        status = self.run_sync()
        self.assertIn('outer_ipv6_network=120', status)
        self.assertEqual(self.owned(), ['5209: from all fwmark 0x10020000/0x1e020000 iif lo lookup 1032'])
        self.assertEqual(len((self.directory / 'restun').read_text().splitlines()), 2)
        self.assertTrue(all(c[0] == '-6' and not (c[1] == 'route' and c[2] != 'show') for c in self.data['calls']))

    def test_ipv4_only_wifi_removes_previous_cellular_policy(self):
        self.physical(); self.run_sync()
        self.physical(121, 'wlan0', ipv6=False)
        self.assertIn('outer_ipv6_state=unavailable', self.run_sync())
        self.assertEqual(self.owned(), [])

    def test_vpn_or_unverified_netid_table_never_becomes_outer_route(self):
        for iface, table in [('tun0', 'tun0'), ('tailscale0', 'tailscale0'), ('ccmni1', '52'), ('ccmni1', '1099')]:
            with self.subTest(iface=iface, table=table):
                self.physical(114, iface, table)
                self.run_sync()
                self.assertEqual(self.owned(), [])
        self.physical()
        self.data['rules'][0] = '16000: from all fwmark 0x10073/0x1ffff iif lo lookup ccmni1'
        self.state.write_text(json.dumps(self.data))
        self.run_sync()
        self.assertEqual(self.owned(), [])

    def test_missing_discovery_cleans_stale_policy_and_keeps_daemon_safe(self):
        self.physical(); self.run_sync()
        self.network_file.write_text('')
        self.assertIn('outer_ipv6_state=unavailable', self.run_sync())
        self.assertEqual(self.owned(), [])

    def test_foreign_priority_is_not_deleted_or_overridden(self):
        self.physical()
        self.data['rules'].append('5209: from all fwmark 0x1234 lookup 200')
        self.state.write_text(json.dumps(self.data))
        self.assertIn('outer_ipv6_state=conflict', self.run_sync())
        self.assertEqual(self.owned(), ['5209: from all fwmark 0x1234 lookup 200'])

    def test_add_failure_and_stop_do_not_claim_success_or_leave_policy(self):
        self.physical()
        self.data['fail_add'] = True
        self.state.write_text(json.dumps(self.data))
        self.assertIn('outer_ipv6_state=error', self.run_sync())
        self.assertEqual(self.owned(), [])
        self.data['fail_add'] = False
        self.state.write_text(json.dumps(self.data))
        self.run_sync()
        self.assertIn('outer_ipv6_state=stopped', self.run_sync(remove=True))
        self.assertEqual(self.owned(), [])

    def test_restun_timeout_does_not_block_service_or_remove_valid_policy(self):
        self.physical()
        self.cli.write_text('#!/bin/sh\nexec sleep 20\n')
        self.assertIn('outer_ipv6_state=active', self.run_sync())
        self.assertEqual(len(self.owned()), 1)

    def test_from_specific_or_extra_selector_foreign_rules_are_preserved(self):
        for foreign in ['5209: from 2001:db8::/64 fwmark 0x10020000/0x1e020000 iif lo lookup 200',
                        '5209: from all fwmark 0x10020000/0x1e020000 iif lo uidrange 0-0 lookup 200']:
            with self.subTest(rule=foreign):
                self.data['rules'] = []
                self.physical()
                self.data['rules'].append(foreign)
                self.state.write_text(json.dumps(self.data))
                self.assertIn('outer_ipv6_state=conflict', self.run_sync())
                self.run_sync(remove=True)
                self.assertEqual(self.owned(), [foreign])

    def test_toybox_flock_arguments_and_explicit_descriptor_passage(self):
        flock = self.directory / 'flock'
        flock.write_text('''#!/bin/sh
case "$*" in '-n 9') exec /usr/bin/flock "$@" ;; *) echo toybox-unsupported-arguments >&2; exit 64 ;; esac
''')
        flock.chmod(0o755)
        self.physical(); self.run_sync()
        self.run_sync(remove=True)
        self.assertEqual(self.owned(), [])

    def test_same_table_foreign_and_owned_rules_do_not_allow_wildcard_deletion(self):
        self.physical()
        foreign = '5209: from 2001:db8::/64 fwmark 0x10020000/0x1e020000 iif lo lookup ccmni1'
        owned = '5209: from all fwmark 0x10020000/0x1e020000 iif lo lookup ccmni1'
        self.data['rules'] += [foreign, owned]
        self.state.write_text(json.dumps(self.data))
        self.assertIn('outer_ipv6_state=conflict', self.run_sync())
        self.assertEqual(self.owned(), [foreign, owned])
        self.assertIn('outer_ipv6_state=conflict', self.run_sync(remove=True))
        self.assertEqual(self.owned(), [foreign, owned])
