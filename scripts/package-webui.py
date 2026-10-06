#!/usr/bin/env python3
"""Package a UI preview, preserving core payload except authorized author metadata."""
import argparse
import hashlib
import json
import pathlib
import re
import subprocess
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
ACCEPTED_SHA = 'c848a47a8ebbcc3f03594c30583651e17b2013ee02bee77954a83317281ddfae'
DEFAULT_BASE = ROOT / 'dist/tailscaled-v1.102.5-dnsfix.2-arm64.zip'
DEFAULT_OUTPUT = ROOT / 'dist/tailscaled-v1.102.5-dnsfix.2-webui-miuix-preview.3-arm64.zip'
MODULE_AUTHOR = 'FogPurification'


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
    manifest = {'edition': 'Miuix WebUI Preview 3', 'ui_revision': revision, 'module_author': MODULE_AUTHOR,
                'base_zip': base.name, 'base_sha256': digest,
                'ui_sha256': {key: hashlib.sha256(value).hexdigest() for key, value in ui.items()}}
    ui['webroot/ui-build.json'] = (json.dumps(manifest, indent=2) + '\n').encode('utf-8')
    output.parent.mkdir(parents=True, exist_ok=True)
    try:
        with zipfile.ZipFile(base) as old, zipfile.ZipFile(temporary, 'w', zipfile.ZIP_DEFLATED, compresslevel=9) as new:
            if old.testzip() is not None:
                raise ValueError('Base ZIP CRC validation failed')
            for entry in old.infolist():
                if not entry.filename.startswith('webroot/'):
                    payload = old.read(entry.filename)
                    new.writestr(entry, author_metadata(payload) if entry.filename == 'module.prop' else payload)
            for name, data in ui.items():
                entry = zipfile.ZipInfo(name, date_time=(2026, 10, 7, 0, 0, 0))
                entry.create_system = 3
                entry.external_attr = 0o100644 << 16
                entry.compress_type = zipfile.ZIP_DEFLATED
                new.writestr(entry, data)
        with zipfile.ZipFile(base) as old, zipfile.ZipFile(temporary) as new:
            if new.testzip() is not None:
                raise ValueError('Preview ZIP CRC validation failed')
            core = {name for name in old.namelist() if not name.startswith('webroot/')}
            if core != {name for name in new.namelist() if not name.startswith('webroot/')}:
                raise ValueError('Core file list changed')
            for name in core:
                expected = author_metadata(old.read(name)) if name == 'module.prop' else old.read(name)
                if expected != new.read(name) or old.getinfo(name).external_attr != new.getinfo(name).external_attr:
                    raise ValueError(f'Core bytes or modes changed: {name}')
            required = ('customize.sh', 'module.prop', 'service.sh', 'META-INF/com/google/android/update-binary',
                        'files/android-dns', 'files/tailscale.combined', 'webroot/app.js', 'webroot/demo.js', 'webroot/commands.js')
            for name in required:
                if name not in new.namelist():
                    raise ValueError(f'Missing installer payload: {name}')
        temporary.replace(output)
    finally:
        if temporary.exists():
            temporary.unlink()
    digest = hashlib.sha256(output.read_bytes()).hexdigest()
    checksum.write_bytes(f'{digest}  {output.name}\n'.encode('ascii'))
    print(f'{output}\nSHA256 {digest}\nCore payload: {len(core) - 1} entries identical; module.prop changes only author={MODULE_AUTHOR}; all modes preserved')
    return output


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--base', type=pathlib.Path, default=DEFAULT_BASE)
    parser.add_argument('--output', type=pathlib.Path, default=DEFAULT_OUTPUT)
    args = parser.parse_args()
    package(args.base, args.output)
