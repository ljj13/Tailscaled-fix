#!/usr/bin/env python3
"""Package previews over the accepted binaries with explicit script overlays."""
import argparse
import hashlib
import json
import pathlib
import re
import struct
import subprocess
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
ACCEPTED_SHA = 'c848a47a8ebbcc3f03594c30583651e17b2013ee02bee77954a83317281ddfae'
DEFAULT_BASE = ROOT / 'dist/tailscaled-v1.102.5-dnsfix.2-arm64.zip'
DEFAULT_OUTPUT = ROOT / 'dist/tailscaled-v1.102.5-dnsfix.2-webui-miuix-preview.4-arm64.zip'
MODULE_AUTHOR = 'FogPurification'
SCRIPT_OVERLAYS = ('tailscale/scripts/tailscaled.service', 'system/bin/tailscale', 'customize.sh')


def hostname_payload():
    binary = ROOT / 'files/android-hostname'
    data = binary.read_bytes()
    info = json.loads(binary.with_suffix('.build.json').read_bytes())
    if info['sha256'] != hashlib.sha256(data).hexdigest():
        raise ValueError('Hostname helper hash mismatch; rebuild it')
    sources = {p.relative_to(ROOT).as_posix() for p in (ROOT / 'tools/android-hostname').iterdir()
               if p.suffix == '.go' or p.name == 'go.mod'}
    if sources != set(info['sources_sha256']):
        raise ValueError('Hostname helper source list changed; rebuild it')
    for name, digest in info['sources_sha256'].items():
        if hashlib.sha256((ROOT / name).read_bytes()).hexdigest() != digest:
            raise ValueError('Hostname helper source changed; rebuild it')
    if (info['go'], info['GOOS'], info['GOARCH'], info['CGO_ENABLED']) != ('go1.26.6', 'linux', 'arm64', '0'):
        raise ValueError('Hostname helper toolchain mismatch')
    if data[:5] != b'\x7fELF\x02' or struct.unpack('<H', data[18:20])[0] != 183:
        raise ValueError('Hostname helper must be Linux arm64 ELF')
    offset = struct.unpack('<Q', data[32:40])[0]
    size, count = struct.unpack('<HH', data[54:58])
    for i in range(count):
        if struct.unpack('<I', data[offset+i*size:offset+i*size+4])[0] in (2, 3):
            raise ValueError('Hostname helper must be static')
    return data, info


def author_metadata(data):
    if len(re.findall(rb'(?m)^author=[^\r\n]*', data)) != 1:
        raise ValueError('Expected exactly one module author field')
    return re.sub(rb'(?m)^author=[^\r\n]*', f'author={MODULE_AUTHOR}'.encode('utf-8'), data)


def package(base, output):
    temporary = output.with_suffix('.zip.tmp')
    checksum = output.with_suffix('.zip.sha256')
    if base.resolve() in {path.resolve() for path in (output, temporary, checksum)}:
        raise ValueError('Preview output must not overwrite the accepted ZIP')
    digest = hashlib.sha256(base.read_bytes()).hexdigest()
    if digest != ACCEPTED_SHA:
        raise ValueError('Base ZIP does not match the accepted dnsfix.2 artifact')
    revision = subprocess.check_output(['git', 'rev-parse', 'HEAD'], cwd=ROOT, text=True).strip()
    ui = {p.relative_to(ROOT).as_posix(): p.read_bytes().replace(b'\r\n', b'\n')
          for p in sorted((ROOT / 'webroot').rglob('*')) if p.is_file()}
    scripts = {name: (ROOT / name).read_bytes().replace(b'\r\n', b'\n') for name in SCRIPT_OVERLAYS}
    helper, helper_info = hostname_payload()
    payloads = {**scripts, 'files/android-hostname': helper}
    manifest = {'edition': 'Miuix WebUI Preview 4', 'ui_revision': revision, 'module_author': MODULE_AUTHOR,
                'base_zip': base.name, 'base_sha256': digest,
                'module_script_sha256': {key: hashlib.sha256(value).hexdigest() for key, value in scripts.items()},
                'hostname_helper': helper_info,
                'ui_sha256': {key: hashlib.sha256(value).hexdigest() for key, value in ui.items()}}
    ui['webroot/ui-build.json'] = (json.dumps(manifest, indent=2) + '\n').encode('utf-8')
    output.parent.mkdir(parents=True, exist_ok=True)
    try:
        with zipfile.ZipFile(base) as old, zipfile.ZipFile(temporary, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as new:
            if old.testzip() is not None:
                raise ValueError('Base ZIP CRC validation failed')
            for entry in old.infolist():
                if not entry.filename.startswith('webroot/'):
                    if entry.filename in payloads:
                        continue
                    payload = old.read(entry.filename)
                    new.writestr(entry, author_metadata(payload) if entry.filename == 'module.prop' else payload)
            for name, data in {**payloads, **ui}.items():
                entry = zipfile.ZipInfo(name, date_time=(2026, 10, 7, 0, 0, 0))
                entry.create_system = 3
                entry.external_attr = (0o100755 if name in payloads else 0o100644) << 16
                entry.compress_type = zipfile.ZIP_DEFLATED
                new.writestr(entry, data)
        with zipfile.ZipFile(base) as old, zipfile.ZipFile(temporary) as new:
            if new.testzip() is not None:
                raise ValueError('Preview ZIP CRC validation failed')
            core = {name for name in old.namelist() if not name.startswith('webroot/')}
            if core | set(payloads) != {name for name in new.namelist() if not name.startswith('webroot/')}:
                raise ValueError('Core file list changed')
            for name in core:
                expected = scripts.get(name, author_metadata(old.read(name)) if name == 'module.prop' else old.read(name))
                if expected != new.read(name) or old.getinfo(name).external_attr != new.getinfo(name).external_attr:
                    raise ValueError(f'Core bytes or modes changed: {name}')
            required = ('customize.sh', 'module.prop', 'service.sh', 'META-INF/com/google/android/update-binary',
                        'files/android-dns', 'files/tailscale.combined', 'webroot/app.js', 'webroot/demo.js', 'webroot/commands.js')
            for name in (*required, *payloads):
                if name not in new.namelist():
                    raise ValueError(f'Missing installer payload: {name}')
            for name, data in payloads.items():
                if new.read(name) != data or new.getinfo(name).external_attr >> 16 != 0o100755:
                    raise ValueError(f'Hostname payload bytes or mode changed: {name}')
        temporary.replace(output)
    finally:
        if temporary.exists():
            temporary.unlink()
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    checksum.write_bytes(f'{digest}  {output.name}\n'.encode('ascii'))
    preserved = len(core - set(scripts) - {'module.prop'})
    print(f'{output}\nSHA256 {digest}\nCore payload: {preserved} entries identical; 3 hostname script overlays and 1 isolated helper; module.prop author={MODULE_AUTHOR}; all modes preserved')
    return output


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', type=pathlib.Path, default=DEFAULT_BASE)
    parser.add_argument('--output', type=pathlib.Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    package(args.base, args.output)
