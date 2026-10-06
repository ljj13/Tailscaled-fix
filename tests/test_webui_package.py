"""Verify preview ZIP isolation against the accepted stable installer payload."""
import hashlib
import importlib.util
import json
import pathlib
import tempfile
import unittest
import zipfile

ROOT = pathlib.Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('package_webui', ROOT / 'scripts/package-webui.py')
packager = importlib.util.module_from_spec(spec)
spec.loader.exec_module(packager)


class WebUIPackageTests(unittest.TestCase):
    def test_core_payload_and_modes_are_identical(self):
        if not packager.DEFAULT_BASE.exists():
            self.skipTest('Download the accepted dnsfix.2 ZIP to dist first')
        with tempfile.TemporaryDirectory() as directory:
            output = pathlib.Path(directory) / 'preview.zip'
            packager.package(packager.DEFAULT_BASE, output)
            with zipfile.ZipFile(packager.DEFAULT_BASE) as old, zipfile.ZipFile(output) as new:
                for name in old.namelist():
                    if not name.startswith('webroot/'):
                        self.assertEqual(new.read(name), old.read(name), name)
                        self.assertEqual(new.getinfo(name).external_attr, old.getinfo(name).external_attr, name)
                self.assertEqual(new.read('webroot/ksu.js'), (ROOT / 'webroot/ksu.js').read_bytes().replace(b'\r\n', b'\n'))
                self.assertEqual(new.testzip(), None)
                info = json.loads(new.read('webroot/ui-build.json'))
                self.assertEqual(info['base_sha256'], packager.ACCEPTED_SHA)
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


if __name__ == '__main__':
    unittest.main()
