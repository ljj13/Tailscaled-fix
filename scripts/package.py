#!/usr/bin/env python3
"""Create an installable ZIP with Unix modes, LF scripts, and build provenance."""
import hashlib
import json
import pathlib
import struct
import subprocess
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
files = [ROOT / 'files/tailscale.combined', ROOT / 'files/android-dns', ROOT / 'files/android-hostname', ROOT / 'files/android-netdiag']
for path in files:
    data = path.read_bytes()
    if data[:4] != b'\x7fELF' or data[4] != 2 or struct.unpack('<H', data[18:20])[0] != 183:
        raise SystemExit(f'{path.name}: expected Linux arm64 ELF')
    offset = struct.unpack('<Q', data[32:40])[0]
    size, count = struct.unpack('<HH', data[54:58])
    for i in range(count):
        kind = struct.unpack('<I', data[offset + i * size:offset + i * size + 4])[0]
        if kind in (2, 3):
            raise SystemExit(f'{path.name}: dynamically linked binary is not supported')
daemon = files[0].read_bytes()
for marker in (b'ts-postrouting', b'0x1e020000', b'/data/adb/tailscale/bootstrap-resolv.conf', b'/data/adb/tailscale/os-resolv.conf'):
    if marker not in daemon:
        raise SystemExit(f'missing binary marker {marker!r}')
manifest = {
    'tailscale_version': 'v1.102.5',
    'tailscale_commit': '5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115',
    'module_base': 'aec266c8c8a2115f83833c07f95bc5f71501a78d',
    'module_revision': subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip(),
    'go': 'go1.26.6', 'GOOS': 'linux', 'GOARCH': 'arm64', 'CGO_ENABLED': '0',
    'tags': ['ts_include_cli', 'ts_omit_ssh'],
    'sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in files},
}
(ROOT / 'files/build-info.json').write_text(json.dumps(manifest, indent=2) + '\n', encoding='utf-8')
output = ROOT / 'dist/tailscaled-v1.102.5-dnsfix.2-webui.2-arm64.zip'
output.parent.mkdir(exist_ok=True)
paths = []
for entry in ('META-INF', 'customize.sh', 'service.sh', 'system', 'tailscale', 'webroot', 'uninstall.sh', 'module.prop', 'files'):
    path = ROOT / entry
    paths.extend(sorted(path.rglob('*')) if path.is_dir() else [path])
with zipfile.ZipFile(output, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as z:
    for path in paths:
        if not path.is_file():
            continue
        name = path.relative_to(ROOT).as_posix()
        data = path.read_bytes()
        executable = name.endswith('.sh') or name.startswith(('system/bin/', 'tailscale/scripts/')) or name.endswith('update-binary') or name.startswith('files/') and path.name in ('tailscale.combined', 'android-dns', 'android-hostname', 'android-netdiag')
        if name.startswith(('system/', 'tailscale/', 'META-INF/')) or name.endswith('.sh'):
            data = data.replace(b'\r\n', b'\n')
        info = zipfile.ZipInfo(name, date_time=(2026, 10, 6, 0, 0, 0))
        info.create_system = 3
        info.external_attr = (0o100755 if executable else 0o100644) << 16
        info.compress_type = zipfile.ZIP_DEFLATED
        z.writestr(info, data)
with zipfile.ZipFile(output) as z:
    if z.testzip() is not None:
        raise SystemExit('ZIP CRC check failed')
    for required in ('META-INF/com/google/android/update-binary', 'customize.sh', 'files/android-dns', 'files/android-hostname', 'files/android-netdiag', 'files/tailscale.combined'):
        assert required in z.namelist(), required
digest = hashlib.sha256(output.read_bytes()).hexdigest()
output.with_suffix('.zip.sha256').write_text(f'{digest}  {output.name}\n', encoding='utf-8')
print(f'{output}\nSHA256 {digest}')
