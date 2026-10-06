#!/usr/bin/env python3
"""Build the isolated static Linux arm64 hostname helper and source provenance."""
import hashlib
import json
import os
import pathlib
import subprocess

ROOT = pathlib.Path(__file__).resolve().parents[1]
SOURCE = ROOT / 'tools/android-hostname'
OUTPUT = ROOT / 'files/android-hostname'


def build():
    version = subprocess.check_output(['go', 'env', 'GOVERSION'], text=True).strip()
    if version != 'go1.26.6':
        raise ValueError('Hostname helper requires Go 1.26.6')
    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    subprocess.run(['go', 'build', '-trimpath', '-buildvcs=false', '-ldflags=-s -w', '-o', str(OUTPUT), '.'],
                   cwd=SOURCE, env={**os.environ, 'CGO_ENABLED': '0', 'GOOS': 'linux', 'GOARCH': 'arm64'}, check=True)
    manifest = {'go': version, 'GOOS': 'linux', 'GOARCH': 'arm64', 'CGO_ENABLED': '0',
                'sha256': hashlib.sha256(OUTPUT.read_bytes()).hexdigest(),
                'sources_sha256': {p.relative_to(ROOT).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                                   for p in sorted(SOURCE.iterdir()) if p.suffix == '.go' or p.name == 'go.mod'}}
    OUTPUT.with_suffix('.build.json').write_bytes((json.dumps(manifest, indent=2) + '\n').encode())
    print(f'{OUTPUT}\nSHA256 {manifest["sha256"]}')


if __name__ == '__main__':
    build()
