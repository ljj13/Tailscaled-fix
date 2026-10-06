#!/usr/bin/env python3
"""Align upstream literal fixtures with intentional path/mark build patches.

Only test files are overlaid. Assertions remain unchanged; ordinary Linux DNS
paths and stock fwmark golden strings cannot describe an Android module build.
"""
import json
import pathlib
import sys

source = pathlib.Path(sys.argv[1]).resolve()
output = pathlib.Path(sys.argv[2]).resolve()
output.mkdir(parents=True, exist_ok=True)
replacements = {}
for directory in ('net/dns', 'wgengine/router/osrouter'):
    for original in (source / directory).glob('*test.go'):
        text = original.read_text()
        changed = text
        if directory == 'net/dns':
            changed = changed.replace('/etc/resolv.conf', '/data/adb/tailscale/os-resolv.conf')
            changed = changed.replace('/etc/resolv.pre-tailscale-backup.conf', '/data/adb/tailscale/os-resolv.backup.conf')
            changed = changed.replace('/etc/resolv.tailscale.conf', '/data/adb/tailscale/os-resolv.legacy.conf')
            changed = changed.replace('filepath.Join(tmp, "etc")', 'filepath.Join(tmp, "data/adb/tailscale")')
        else:
            changed = changed.replace('0x80000/0xff0000', '0x10020000/0x1e020000')
            changed = changed.replace('0x40000/0xff0000', '0x8000000/0x1e020000')
        if changed != text:
            target = output / original.name
            target.write_text(changed)
            replacements[str(original)] = str(target)
(output / 'overlay.json').write_text(json.dumps({'Replace': replacements}))
