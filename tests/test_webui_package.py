"""Verify core isolation, with the explicit module-author metadata exception."""
import hashlib
import importlib.util
import json
import pathlib
import shutil
import tempfile
import unittest
import zipfile
from unittest import mock

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('package_webui', ROOT / 'scripts/package-webui.py')
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class WebUIPackageTests(unittest.TestCase):
    def test_release_metadata_updates_only_author_version_and_version_code(self):
        data = b'id=tailscaled\nname=Tailscale\nversion=v1.102.5-dnsfix.2\nversionCode=110200502\nauthor=keweiya\ndescription=keep unchanged\n'
        metadata = getattr(packager, 'release_metadata', packager.author_metadata)
        result = metadata(data)
        self.assertIn(b'version=v1.102.5-dnsfix.2-webui.1\n', result)
        self.assertIn(b'versionCode=110200503\n', result)
        self.assertIn(b'author=FogPurification\n', result)
        self.assertIn(b'id=tailscaled\nname=Tailscale\n', result)
        self.assertIn(b'description=keep unchanged\n', result)

    def test_release_zip_matches_module_metadata_and_preserves_accepted_binaries(self):
        with tempfile.TemporaryDirectory() as directory:
            output = pathlib.Path(directory) / 'release.zip'
            packager.package(packager.DEFAULT_BASE, output, release=True)
            with zipfile.ZipFile(packager.DEFAULT_BASE) as old, zipfile.ZipFile(output) as new:
                info = json.loads(new.read('webroot/ui-build.json'))
                self.assertEqual(info['edition'], 'Miuix WebUI 1')
                self.assertEqual(info['module_version'], 'v1.102.5-dnsfix.2-webui.1')
                self.assertEqual(new.read('module.prop'), (ROOT / 'module.prop').read_bytes().replace(b'\r\n', b'\n'))
                for name in ('files/tailscale.combined', 'files/android-dns', 'files/build-info.json'):
                    self.assertEqual(new.read(name), old.read(name))
                self.assertEqual(new.testzip(), None)

    def test_core_payload_preserves_network_binaries_and_limits_script_changes(self):
        if not packager.DEFAULT_BASE.exists():
            self.skipTest('Download the accepted dnsfix.2 ZIP to dist first')
        with tempfile.TemporaryDirectory() as directory:
            output = pathlib.Path(directory) / 'preview.zip'
            packager.package(packager.DEFAULT_BASE, output)
            with zipfile.ZipFile(packager.DEFAULT_BASE) as old, zipfile.ZipFile(output) as new:
                for name in ('customize.sh', 'tailscale/scripts/tailscaled.service', 'system/bin/tailscale', 'files/android-hostname'):
                    self.assertIn(name, new.namelist())
                    source = (ROOT / name).read_bytes()
                    self.assertEqual(new.read(name), source if name.startswith('files/') else source.replace(b'\r\n', b'\n'))
                    self.assertEqual(new.getinfo(name).external_attr >> 16, 0o100755)
                for name in old.namelist():
                    if not name.startswith('webroot/'):
                        if name == 'module.prop':
                            original = old.read(name).decode('utf-8').splitlines()
                            packaged = new.read(name).decode('utf-8').splitlines()
                            self.assertEqual([line for line in original if not line.startswith('author=')], [line for line in packaged if not line.startswith('author=')])
                            self.assertEqual([line for line in packaged if line.startswith('author=')], ['author=FogPurification'])
                        elif name not in ('customize.sh', 'tailscale/scripts/tailscaled.service', 'system/bin/tailscale'):
                            self.assertEqual(new.read(name), old.read(name), name)
                        self.assertEqual(new.getinfo(name).external_attr, old.getinfo(name).external_attr, name)
                self.assertEqual(new.read('webroot/ksu.js'), (ROOT / 'webroot/ksu.js').read_bytes().replace(b'\r\n', b'\n'))
                self.assertEqual(new.testzip(), None)
                info = json.loads(new.read('webroot/ui-build.json'))
                self.assertEqual(info['base_sha256'], packager.ACCEPTED_SHA)
                self.assertEqual(info['module_author'], 'FogPurification')
                self.assertEqual(hashlib.sha256(new.read('files/android-hostname')).hexdigest(), info['hostname_helper']['sha256'])
                for name, digest in info['module_script_sha256'].items():
                    self.assertEqual(hashlib.sha256(new.read(name)).hexdigest(), digest, name)
                for name, digest in info['ui_sha256'].items():
                    self.assertEqual(hashlib.sha256(new.read(name)).hexdigest(), digest, name)
            self.assertTrue(output.with_suffix('.zip.sha256').exists())

    def test_rejects_unverified_base_and_in_place_output(self):
        with tempfile.TemporaryDirectory() as directory:
            base = pathlib.Path(directory) / 'base.zip'
            base.write_bytes(b'unverified fixture')
            with self.assertRaisesRegex(ValueError, 'overwrite'):
                packager.package(base, base)
            with self.assertRaisesRegex(ValueError, 'accepted'):
                packager.package(base, pathlib.Path(directory) / 'preview.zip')
            for suffix in ('.zip.tmp', '.zip.sha256'):
                with self.assertRaisesRegex(ValueError, 'overwrite'):
                    packager.package(base.with_suffix(suffix), base)

    def test_rejects_stale_helper_source_and_corrupted_binary(self):
        with tempfile.TemporaryDirectory() as directory:
            fixture = pathlib.Path(directory)
            shutil.copytree(ROOT / 'tools/android-hostname', fixture / 'tools/android-hostname')
            (fixture / 'files').mkdir()
            for name in ('android-hostname', 'android-hostname.build.json'):
                shutil.copyfile(ROOT / 'files' / name, fixture / 'files' / name)
            with mock.patch.object(packager, 'ROOT', fixture):
                packager.hostname_payload()
                source = fixture / 'tools/android-hostname/main.go'
                original = source.read_bytes()
                source.write_bytes(original + b'\n// unbuilt source change\n')
                with self.assertRaisesRegex(ValueError, 'source changed'):
                    packager.hostname_payload()
                source.write_bytes(original)
                binary = fixture / 'files/android-hostname'
                binary.write_bytes(binary.read_bytes() + b'corrupted')
                with self.assertRaisesRegex(ValueError, 'hash mismatch'):
                    packager.hostname_payload()


if __name__ == '__main__':
    unittest.main()
