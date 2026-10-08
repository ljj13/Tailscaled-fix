"""Offline Release transaction tests: no GitHub writes or tag creation."""
import base64
import hashlib
import importlib.util
import json
import pathlib
import tempfile
import unittest
import zipfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]


def load_tool():
    path = ROOT / 'scripts/release-ci.py'
    if not path.exists():
        raise AssertionError('Missing tag Release orchestration')
    spec = importlib.util.spec_from_file_location('release_ci', path)
    tool = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(tool)
    return tool


class FakeGitHub:
    def __init__(self):
        self.calls = []
        self.release = None
        self.fail = None
        self.pointer = {'version': 'old', 'versionCode': 1}

    def __call__(self, args, payload=None):
        self.calls.append(args)
        if self.fail and self.fail in args:
            raise RuntimeError('injected failure')
        if args[:2] == ['release', 'view']:
            return self.release
        if args[:2] == ['release', 'create']:
            self.release = {'isDraft': True, 'isPrerelease': False, 'assets': []}
        elif args[:2] == ['release', 'edit']:
            self.release['isDraft'] = False
        elif args[0] == 'api' and '--method' not in args:
            return {'sha': 'pointer-sha', 'content': base64.b64encode(json.dumps(self.pointer).encode()).decode()}
        elif args[0] == 'api':
            self.pointer = json.loads(base64.b64decode(payload['content']))
        return {}


class ReleaseCITests(unittest.TestCase):
    def setUp(self):
        self.tool = load_tool()
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.tag = 'v1.102.5-dnsfix.2-webui.3'
        (self.root / 'module.prop').write_text(f'id=tailscaled\nauthor=FogPurification\nversion={self.tag}\nversionCode=110200505\n')
        (self.root / 'docs/releases').mkdir(parents=True)
        (self.root / f'docs/releases/{self.tag}.md').write_text('release notes')
        self.gh = FakeGitHub()
        self.meta = self.tool.metadata(self.root, self.tag)
        self.bundle = self.root / 'bundle'
        self.bundle.mkdir()

    def publish(self):
        def remote_check(*args):
            self.gh.calls.append(['remote-verify'])
            if self.gh.fail == 'remote-verify':
                raise RuntimeError('verify failed')
        with mock.patch.object(self.tool, 'verify_bundle', return_value=self.meta), \
             mock.patch.object(self.tool, 'verify_remote_assets', side_effect=remote_check):
            return self.tool.publish(self.root, self.bundle, self.tag, 'ljj13/Tailscaled-fix', self.gh)

    def test_success_stages_verifies_then_publishes_and_updates_pointer(self):
        self.publish()
        verbs = [a[:2] for a in self.gh.calls]
        self.assertIn(['release', 'create'], verbs)
        create = next(a for a in self.gh.calls if a[:2] == ['release', 'create'])
        self.assertIn('--draft', create)
        self.assertIn('--verify-tag', create)
        self.assertLess(verbs.index(['release', 'create']), verbs.index(['release', 'edit']))
        self.assertLess(verbs.index(['release', 'create']), verbs.index(['remote-verify']))
        self.assertLess(verbs.index(['remote-verify']), verbs.index(['release', 'edit']))
        self.assertEqual(self.gh.pointer['version'], self.tag)
        self.assertFalse(self.gh.release['isDraft'])

    def test_create_upload_failure_does_not_publish_or_update_pointer(self):
        self.gh.fail = 'create'
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertFalse(any(a[:2] == ['release', 'edit'] or '--method' in a for a in self.gh.calls))

    def test_remote_verification_failure_keeps_draft_and_old_pointer(self):
        self.gh.fail = 'remote-verify'
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertTrue(self.gh.release['isDraft'])
        self.assertEqual(self.gh.pointer['version'], 'old')

    def test_retry_published_release_only_checks_assets_and_updates_pointer(self):
        self.gh.release = {'isDraft': False, 'isPrerelease': False, 'assets': []}
        self.publish()
        self.assertFalse(any(a[:2] in (['release', 'create'], ['release', 'edit']) for a in self.gh.calls))
        self.assertEqual(self.gh.pointer['version'], self.tag)

    def test_newer_pointer_cannot_be_downgraded(self):
        self.gh.pointer = {'version': 'newer', 'versionCode': 110200999}
        self.publish()
        self.assertEqual(self.gh.pointer['version'], 'newer')
        self.assertFalse(any('--method' in a for a in self.gh.calls))
        edit = next(a for a in self.gh.calls if a[:2] == ['release', 'edit'])
        self.assertIn('--latest=false', edit)

    def test_metadata_rejects_wrong_tag_missing_notes_and_invalid_code(self):
        with self.assertRaises(ValueError):
            self.tool.metadata(self.root, 'v1.102.6-other')
        (self.root / f'docs/releases/{self.tag}.md').unlink()
        with self.assertRaises(ValueError):
            self.tool.metadata(self.root, self.tag)

    def test_invalid_version_code_and_duplicate_metadata_fail(self):
        path = self.root / 'module.prop'
        original = path.read_text()
        for code in ('0', '-1', '2147483648', 'abc'):
            path.write_text(original.replace('110200505', code))
            with self.assertRaises(ValueError):
                self.tool.metadata(self.root, self.tag)
        path.write_text(original + 'versionCode=123\n')
        with self.assertRaises(ValueError):
            self.tool.metadata(self.root, self.tag)

    def test_corrupt_bundle_fails_before_any_api(self):
        with mock.patch.object(self.tool, 'verify_bundle', side_effect=ValueError('bad SHA256')):
            with self.assertRaises(ValueError):
                self.tool.publish(self.root, self.bundle, self.tag, 'ljj13/Tailscaled-fix', self.gh)
        self.assertEqual(self.gh.calls, [])

    def test_remote_asset_mismatch_is_rejected(self):
        asset = self.bundle / 'test.zip'
        asset.write_bytes(b'good')
        self.gh.release = {'isDraft': True, 'assets': [
            {'name': 'test.zip', 'state': 'uploaded', 'size': 4, 'digest': 'sha256:' + hashlib.sha256(b'bad!').hexdigest()}]}
        with self.assertRaises(ValueError):
            self.tool.verify_remote_assets(self.gh, self.tag, 'ljj13/Tailscaled-fix', [asset])

    def test_feed_failure_after_publish_is_retryable_without_republishing(self):
        self.gh.fail = 'PUT'
        with self.assertRaises(RuntimeError):
            self.publish()
        self.assertFalse(self.gh.release['isDraft'])
        self.assertEqual(self.gh.pointer['version'], 'old')
        self.gh.fail = None
        self.gh.calls.clear()
        self.publish()
        self.assertEqual(self.gh.pointer['version'], self.tag)
        self.assertFalse(any(a[:2] == ['release', 'edit'] for a in self.gh.calls))

    def test_code_collision_fails_before_creating_release(self):
        self.gh.pointer = {'version': 'other-tag', 'versionCode': self.meta['versionCode']}
        with self.assertRaises(ValueError):
            self.publish()
        self.assertIsNone(self.gh.release)


class BundleValidationTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.tool = load_tool()
        spec = importlib.util.spec_from_file_location('packager', ROOT / 'scripts/package-webui.py')
        packager = importlib.util.module_from_spec(spec)
        spec.loader.exec_module(packager)
        cls.tmp = tempfile.TemporaryDirectory()
        cls.addClassCleanup(cls.tmp.cleanup)
        cls.bundle = pathlib.Path(cls.tmp.name)
        cls.tag = packager.RELEASE_TAG
        cls.archive = cls.bundle / f'tailscaled-{cls.tag}-arm64.zip'
        packager.package(packager.DEFAULT_BASE, cls.archive, release=True)
        cls.original = cls.archive.read_bytes()

    def setUp(self):
        self.archive.write_bytes(self.original)
        self.checksum()

    def checksum(self):
        self.archive.with_suffix('.zip.sha256').write_text(hashlib.sha256(self.archive.read_bytes()).hexdigest() + '  ' + self.archive.name + '\n')

    def rewrite(self, mutation):
        with zipfile.ZipFile(self.archive) as z:
            data = {i.filename: (i, z.read(i.filename)) for i in z.infolist()}
        mutation(data)
        with zipfile.ZipFile(self.archive, 'w') as z:
            for name, (info, payload) in data.items():
                z.writestr(info, payload)
        self.checksum()

    def test_canonical_bundle_passes_and_corrupt_checksum_fails(self):
        self.tool.verify_bundle(ROOT, self.bundle, self.tag)
        self.archive.with_suffix('.zip.sha256').write_text('bad checksum')
        with self.assertRaises(ValueError):
            self.tool.verify_bundle(ROOT, self.bundle, self.tag)

    def test_self_consistent_modified_webui_cannot_pass_tagged_source_verification(self):
        def mutation(data):
            name = 'webroot/app.js'
            info, payload = data[name]
            payload += b'\n// untagged code\n'
            data[name] = info, payload
            info, payload = data['webroot/ui-build.json']
            manifest = json.loads(payload)
            manifest['ui_sha256'][name] = hashlib.sha256(data[name][1]).hexdigest()
            data['webroot/ui-build.json'] = info, json.dumps(manifest).encode()
        self.rewrite(mutation)
        with self.assertRaisesRegex(ValueError, 'tagged source'):
            self.tool.verify_bundle(ROOT, self.bundle, self.tag)

    def test_extra_entry_and_changed_accepted_binary_are_rejected(self):
        def extra(data):
            data['surprise.txt'] = zipfile.ZipInfo('surprise.txt'), b'extra'
        self.rewrite(extra)
        with self.assertRaisesRegex(ValueError, 'file list'):
            self.tool.verify_bundle(ROOT, self.bundle, self.tag)
        self.archive.write_bytes(self.original)
        def binary(data):
            name = 'files/android-dns'
            info, payload = data[name]
            data[name] = info, payload + b'changed'
        self.rewrite(binary)
        with self.assertRaisesRegex(ValueError, 'Accepted binary'):
            self.tool.verify_bundle(ROOT, self.bundle, self.tag)

    def test_module_metadata_permissions_are_checked(self):
        def mutation(data):
            info, payload = data['module.prop']
            info.external_attr = 0o100777 << 16
            data['module.prop'] = info, payload
        self.rewrite(mutation)
        with self.assertRaisesRegex(ValueError, 'permission mismatch'):
            self.tool.verify_bundle(ROOT, self.bundle, self.tag)



class TestStableWebUISuites(unittest.TestCase):
    def test_release_runs_all_stable_webui_suites(self):
        tool = load_tool()
        with tempfile.TemporaryDirectory() as directory:
            root = pathlib.Path(directory)
            with mock.patch.object(tool.subprocess, 'run') as execute, mock.patch.object(tool, 'run', return_value='/browser'):
                tool.test(root)
            names = [call.args[0][1] for call in execute.call_args_list if call.args[0][0] == 'node']
            self.assertEqual(names, ['tests/' + name + '.test.cjs' for name in
                ('webui-command', 'network-ui', 'peers-ui', 'report-ui', 'theme-ui', 'webui')])
            self.assertTrue(all(call.kwargs['check'] for call in execute.call_args_list))

    def test_theme_failure_stops_release_test_entry(self):
        tool = load_tool()
        def execute(args, **kwargs):
            if args == ['node', 'tests/theme-ui.test.cjs']:
                raise tool.subprocess.CalledProcessError(1, args)
        with tempfile.TemporaryDirectory() as directory:
            with mock.patch.object(tool.subprocess, 'run', side_effect=execute) as calls, mock.patch.object(tool, 'run', return_value='/browser'):
                with self.assertRaises(tool.subprocess.CalledProcessError):
                    tool.test(pathlib.Path(directory))
                self.assertFalse(any(call.args[0] == ['node', 'tests/webui.test.cjs'] for call in calls.call_args_list))


if __name__ == '__main__':
    unittest.main()
