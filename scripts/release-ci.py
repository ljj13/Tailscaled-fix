#!/usr/bin/env python3
"""Pinned local/CI Release preparation, verification and draft transaction."""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import re
import shutil
import subprocess
import tempfile
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]


def run(args, cwd=ROOT, **kwargs):
    return subprocess.check_output([str(a) for a in args], cwd=cwd, text=True, **kwargs).strip()


def config(root=ROOT):
    return json.loads((root / 'scripts/release-config.json').read_text(encoding='utf-8'))


def metadata(root, tag):
    fields = {}
    for line in (root / 'module.prop').read_text(encoding='utf-8').splitlines():
        if '=' in line:
            key, value = line.split('=', 1)
            if key in fields:
                raise ValueError('Duplicate module metadata')
            fields[key] = value
    if (not re.fullmatch(r'v1\.102\.5-[A-Za-z0-9._-]+', tag)
            or fields.get('version') != tag or fields.get('id') != 'tailscaled'
            or fields.get('author') != 'FogPurification'
            or not re.fullmatch(r'[1-9][0-9]{0,9}', fields.get('versionCode', ''))
            or int(fields['versionCode']) > 2147483647):
        raise ValueError('Tag/module metadata mismatch or unsupported binary base')
    if not (root / f'docs/releases/{tag}.md').is_file():
        raise ValueError('Missing versioned release notes')
    return {'version': tag, 'versionCode': int(fields['versionCode']),
            'asset': f'tailscaled-{tag}-arm64.zip'}


def prepare(root, tag, untagged=False):
    meta = metadata(root, tag)
    cfg = config(root)
    head = run(['git', 'rev-parse', 'HEAD'], root)
    if not untagged and run(['git', 'rev-parse', f'{tag}^{{}}'], root) != head:
        raise ValueError('Build must use the exact tag commit')
    if run(['go', 'env', 'GOVERSION'], root) != 'go' + cfg['go']:
        raise ValueError('Wrong Go toolchain')
    base = root / 'dist' / cfg['base_asset']
    base.parent.mkdir(exist_ok=True)
    if not base.exists():
        url = f"https://github.com/{cfg['repository']}/releases/download/{cfg['base_tag']}/{cfg['base_asset']}"
        temporary = base.with_suffix('.download')
        try:
            run(['curl', '--fail', '--location', '--proto', '=https', '--proto-redir', '=https',
                 '--tlsv1.2', '--retry', '3', '--output', temporary, url], root)
            if hashlib.sha256(temporary.read_bytes()).hexdigest() != cfg['base_sha256']:
                raise ValueError('Downloaded base SHA256 mismatch')
            temporary.replace(base)
        finally:
            temporary.unlink(missing_ok=True)
    if hashlib.sha256(base.read_bytes()).hexdigest() != cfg['base_sha256']:
        raise ValueError('Base SHA256 mismatch')
    (root / 'files').mkdir(exist_ok=True)
    with zipfile.ZipFile(base) as z:
        if z.testzip() is not None:
            raise ValueError('Invalid base ZIP')
        for name in ('tailscale.combined', 'android-dns', 'build-info.json'):
            (root / 'files' / name).write_bytes(z.read('files/' + name))
    for helper in ('hostname', 'netdiag'):
        run(['python3', f'scripts/build-{helper}.py'], root)
    # Preserve a clean pinned source cache; patch a separate disposable test copy.
    clean = root / 'build/release-source'
    if not clean.exists():
        clean.parent.mkdir(exist_ok=True)
        run(['git', 'clone', '--depth', '1', '--branch', 'v1.102.5', cfg['tailscale_source'], clean], root)
    if (run(['git', 'rev-parse', 'HEAD'], clean) != cfg['tailscale_commit']
            or run(['git', 'status', '--porcelain'], clean)):
        raise ValueError('Pinned source cache is wrong or dirty')
    work = root / 'build/release-tests'
    if work.exists():
        if work.is_symlink() or work.resolve().parent != (root / 'build').resolve():
            raise ValueError('Unsafe test workspace')
        shutil.rmtree(work)
    source = work / 'source'
    shutil.copytree(clean, source)
    for script, output in [('prepare-build.py', 'overlay'), ('prepare-tests.py', 'test-overlay')]:
        run(['python3', root / 'scripts' / script, source, work / output], root)
    print(f"Prepared {tag} at {head}; pinned base {cfg['base_sha256']}")
    return meta


def test(root):
    subprocess.run(['python3', '-m', 'unittest', 'discover', '-s', 'tests', '-p', 'test_*.py', '-v'], cwd=root, check=True)
    for name in ('android-dns', 'android-hostname', 'android-netdiag'):
        for args in (['go', 'test', '-count=1', '-race', './...'], ['go', 'vet', './...']):
            subprocess.run(args, cwd=root / 'tools' / name, check=True)
    work = root / 'build/release-tests'
    subprocess.run(['go', 'test', '-count=1', '-overlay', str(work / 'test-overlay/overlay.json'),
                    './net/dns', './net/dnscache', './net/netns', './wgengine/router/osrouter'], cwd=work / 'source', check=True)
    subprocess.run(['go', 'test', '-count=1', '-race', '-run', 'TestAndroidBootstrap', './net/dns'], cwd=work / 'source', check=True)
    subprocess.run(['python3', 'tests/test_resolver.py'], cwd=root,
                   env={**os.environ, 'BUILD_DIR': str(work)}, check=True)
    scripts = [root / p for p in ('customize.sh', 'service.sh', 'uninstall.sh', 'scripts/build.sh')]
    scripts += sorted((root / 'tailscale/scripts').glob('*')) + sorted((root / 'system/bin').glob('*'))
    subprocess.run(['shellcheck', '-s', 'sh', '-S', 'warning', '-e', 'SC1090,SC1091,SC2034,SC2086,SC2154', *map(str, scripts)], check=True)
    browser = run(['node', '-e', "console.log(require('./build/browser-tools/node_modules/playwright').chromium.executablePath())"], root)
    for name in ('webui-command', 'network-ui', 'webui'):
        subprocess.run(['node', f'tests/{name}.test.cjs'], cwd=root,
                       env={**os.environ, 'WEBUI_BROWSER': os.environ.get('WEBUI_BROWSER', browser)}, check=True)
    run(['git', 'diff', '--check'], root)


def verify_bundle(root, bundle, tag):
    meta = metadata(root, tag)
    archive = bundle / meta['asset']
    digest = hashlib.sha256(archive.read_bytes()).hexdigest()
    if archive.with_suffix('.zip.sha256').read_text().strip() != f'{digest}  {archive.name}':
        raise ValueError('Bundle SHA256 mismatch')
    with zipfile.ZipFile(archive) as z:
        if z.testzip() is not None or len(z.namelist()) != len(set(z.namelist())):
            raise ValueError('Invalid ZIP')
        if z.read('module.prop') != (root / 'module.prop').read_bytes().replace(b'\r\n', b'\n'):
            raise ValueError('ZIP metadata mismatch')
        manifest = json.loads(z.read('webroot/ui-build.json'))
        if (manifest['ui_revision'] != run(['git', 'rev-parse', 'HEAD'], root)
                or manifest['module_version'] != tag or manifest['base_sha256'] != config(root)['base_sha256']):
            raise ValueError('ZIP provenance mismatch')
        for name, expected in {**manifest['module_script_sha256'], **manifest['ui_sha256']}.items():
            if hashlib.sha256(z.read(name)).hexdigest() != expected:
                raise ValueError('ZIP manifest hash mismatch: ' + name)
        for name, expected in config(root)['core_sha256'].items():
            if hashlib.sha256(z.read(name)).hexdigest() != expected:
                raise ValueError('Accepted binary changed: ' + name)
        script_names = ('customize.sh', 'system/bin/tailscale', 'tailscale/scripts/tailscaled.service')
        if set(manifest['module_script_sha256']) != set(script_names):
            raise ValueError('Unexpected script provenance')
        for name in script_names:
            if z.read(name) != (root / name).read_bytes().replace(b'\r\n', b'\n'):
                raise ValueError('Script differs from tagged source: ' + name)
        for helper, key in [('android-hostname', 'hostname_helper'), ('android-netdiag', 'network_diagnostics_helper')]:
            info = manifest[key]
            source = root / 'tools' / helper
            expected_sources = {p.relative_to(root).as_posix(): hashlib.sha256(p.read_bytes()).hexdigest()
                                for p in source.iterdir() if p.suffix == '.go' or p.name == 'go.mod'}
            if (info['sources_sha256'] != expected_sources
                    or info['sha256'] != hashlib.sha256(z.read('files/' + helper)).hexdigest()
                    or (info['go'], info['GOOS'], info['GOARCH'], info['CGO_ENABLED']) != ('go1.26.6', 'linux', 'arm64', '0')):
                raise ValueError('Helper provenance mismatch: ' + helper)
        expected_ui = {p.relative_to(root).as_posix(): p.read_bytes().replace(b'\r\n', b'\n')
                       for p in (root / 'webroot').rglob('*') if p.is_file()}
        if manifest['ui_sha256'] != {name: hashlib.sha256(data).hexdigest() for name, data in expected_ui.items()}:
            raise ValueError('WebUI differs from tagged source')
        base_entries = config(root)['base_entries']
        expected_names = set(base_entries) | set(script_names) | set(expected_ui) | {
            'module.prop', 'files/android-hostname', 'files/android-netdiag', 'webroot/ui-build.json'}
        if set(z.namelist()) != expected_names:
            raise ValueError('Noncanonical ZIP file list')
        for name, info in base_entries.items():
            if hashlib.sha256(z.read(name)).hexdigest() != info['sha256'] or z.getinfo(name).external_attr != info['external_attr']:
                raise ValueError('Accepted base entry changed: ' + name)
        for name in expected_names - set(base_entries):
            mode = 0o100755 if name in script_names or name.startswith('files/') else 0o100644
            if z.getinfo(name).external_attr >> 16 != mode:
                raise ValueError('ZIP permission mismatch: ' + name)
    return meta


class GitHub:
    def __call__(self, args, payload=None):
        result = subprocess.run(['gh', *map(str, args)], text=True, capture_output=True,
                                input=json.dumps(payload) if payload is not None else None)
        if result.returncode:
            if args[:2] == ['release', 'view'] and 'release not found' in result.stderr.lower():
                return None
            raise RuntimeError(result.stderr.strip())
        try:
            return json.loads(result.stdout)
        except json.JSONDecodeError:
            return result.stdout.strip()


def verify_remote_assets(gh, tag, repo, assets):
    release = gh(['release', 'view', tag, '--repo', repo, '--json', 'assets,isDraft,isPrerelease'])
    remote = {a['name']: a for a in release['assets']}
    if set(remote) != {p.name for p in assets}:
        raise ValueError('Remote asset list is incomplete or unexpected')
    for path in assets:
        item = remote[path.name]
        if (item.get('state') != 'uploaded' or item.get('size') != path.stat().st_size
                or item.get('digest') != 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest()):
            raise ValueError('Remote asset verification failed: ' + path.name)


def update_pointer(gh, repo, meta):
    endpoint = f'repos/{repo}/contents/update.json'
    current = gh(['api', endpoint + '?ref=main'])
    old = json.loads(base64.b64decode(current['content']))
    if int(old.get('versionCode', 0)) > meta['versionCode']:
        print('Newer update pointer retained')
        return
    if int(old.get('versionCode', 0)) == meta['versionCode'] and old.get('version') != meta['version']:
        raise ValueError('versionCode collision; update pointer unchanged')
    tag = meta['version']
    desired = {'version': tag, 'versionCode': meta['versionCode'],
               'zipUrl': f"https://github.com/{repo}/releases/download/{tag}/{meta['asset']}",
               'changelog': f'https://raw.githubusercontent.com/{repo}/refs/tags/{tag}/docs/releases/{tag}.md'}
    if old == desired:
        return
    payload = {'message': f'chore: update module feed to {tag}', 'branch': 'main', 'sha': current['sha'],
               'content': base64.b64encode((json.dumps(desired, indent=2) + '\n').encode()).decode()}
    gh(['api', '--method', 'PUT', endpoint, '--input', '-'], payload)


def publish(root, bundle, tag, repo, gh=None):
    meta = verify_bundle(root, bundle, tag)  # No network mutation before local validation.
    gh = gh or GitHub()
    assets = [bundle / meta['asset'], bundle / (meta['asset'] + '.sha256')]
    feed = gh(['api', f'repos/{repo}/contents/update.json?ref=main'])
    pointer = json.loads(base64.b64decode(feed['content']))
    pointer_code = int(pointer.get('versionCode', 0))
    if pointer_code == meta['versionCode'] and pointer.get('version') != tag:
        raise ValueError('versionCode collision; release unchanged')
    latest = '--latest=false' if pointer_code > meta['versionCode'] else '--latest'
    release = gh(['release', 'view', tag, '--repo', repo, '--json', 'assets,isDraft,isPrerelease'])
    if release is None:
        gh(['release', 'create', tag, *map(str, assets), '--repo', repo, '--draft', '--verify-tag',
            '--title', tag, '--notes-file', str(root / f'docs/releases/{tag}.md')])
        draft = True
    else:
        if release.get('isPrerelease'):
            raise ValueError('Existing prerelease is not a formal release')
        draft = release['isDraft']
        if draft:
            gh(['release', 'upload', tag, *map(str, assets), '--repo', repo, '--clobber'])
    verify_remote_assets(gh, tag, repo, assets)
    if draft:
        gh(['release', 'edit', tag, '--repo', repo, '--draft=false', latest])
    update_pointer(gh, repo, meta)  # Only points at a complete published Release.


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('command', choices=['prepare', 'test', 'build', 'verify', 'publish'])
    parser.add_argument('--tag')
    parser.add_argument('--untagged', action='store_true', help='Read-only validation build, never publishing')
    parser.add_argument('--bundle', type=pathlib.Path, default=ROOT / 'dist')
    args = parser.parse_args()
    if args.command == 'test':
        test(ROOT)
        return
    if not args.tag:
        parser.error('--tag is required')
    if args.command == 'prepare':
        prepare(ROOT, args.tag, args.untagged)
    elif args.command == 'build':
        meta = metadata(ROOT, args.tag)
        run(['python3', 'scripts/package-webui.py', '--release', '--output', args.bundle / meta['asset']], ROOT)
        verify_bundle(ROOT, args.bundle, args.tag)
    elif args.command == 'verify':
        verify_bundle(ROOT, args.bundle, args.tag)
    else:
        if args.untagged or not os.environ.get('GITHUB_REF', '').startswith('refs/tags/'):
            parser.error('Publishing requires a tag-triggered workflow')
        if os.environ['GITHUB_REF'] != 'refs/tags/' + args.tag:
            parser.error('Workflow tag mismatch')
        publish(ROOT, args.bundle, args.tag, config()['repository'])


if __name__ == '__main__':
    main()
