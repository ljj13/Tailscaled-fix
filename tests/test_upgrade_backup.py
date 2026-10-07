"""Real installer snapshots in isolated directories, never touching Android."""
import os
import fcntl
import pathlib
import shutil
import subprocess
import tempfile
import unittest

ROOT = pathlib.Path(__file__).resolve().parents[1]


class UpgradeBackupTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = pathlib.Path(self.tmp.name)
        self.install = self.root / 'state'
        self.boot = self.root / 'service.d'
        (self.install / 'run').mkdir(parents=True)
        state = self.install / 'run/tailscaled.state'
        state.write_bytes(b'PRIVATE IDENTITY')
        state.chmod(0o600)

    def old_install(self):
        (self.install / 'scripts').mkdir()
        (self.install / 'scripts/tailscaled.service').write_text('#!/bin/sh\necho stop >> "$STOP_LOG"\n')
        (self.install / 'scripts/tailscaled.service').chmod(0o755)
        (self.install / 'settings.ini').write_text('user settings')
        (self.install / 'settings.ini').chmod(0o640)
        (self.install / 'routes').write_text('192.168.50.0/24\n')
        (self.install / 'hostname-initialized').write_text('my-phone')
        (self.install / 'hostname-initialized').chmod(0o600)
        (self.install / 'hostname-user-set').mkdir(mode=0o700)
        (self.install / 'installed-module.prop').write_text('version=v-old\n')

    def upgrade(self, version='v-next', fail_copy=False):
        module = self.root / 'module'
        if module.exists():
            shutil.rmtree(module)
        shutil.copytree(ROOT / 'tailscale', module / 'tailscale')
        # Repeated upgrades must never execute the real stop/routing code on host.
        (module / 'tailscale/scripts/tailscaled.service').write_text('#!/bin/sh\necho stop >> "$STOP_LOG"\n')
        shutil.copytree(ROOT / 'system', module / 'system')
        (module / 'files').mkdir()
        for name in ('tailscale.combined', 'android-dns', 'android-hostname', 'android-netdiag'):
            (module / 'files' / name).write_text('binary')
        (module / 'service.sh').write_text('boot')
        (module / 'module.prop').write_text(f'id=tailscaled\nversion={version}\n')
        prelude = '''ui_print() { :; }
abort() { echo "$*" >&2; exit 1; }
pgrep() { return 1; }
sleep() { :; }
chown() { :; }
set_perm() { chmod "$4" "$1"; }
set_perm_recursive() { find "$1" -type d -exec chmod "$4" {} \\; ; find "$1" -type f -exec chmod "$5" {} \\; ; }
'''
        script = (ROOT / 'customize.sh').read_text().replace('/data/adb/tailscale', str(self.install)).replace('/data/adb/service.d', str(self.boot))
        if fail_copy:
            prelude += 'cp() { return 1; }\n'
        return subprocess.run(['sh'], input=prelude + script, text=True, capture_output=True,
                              env={**os.environ, 'BOOTMODE': 'true', 'ARCH': 'arm64', 'MODPATH': str(module),
                                   'STOP_LOG': str(self.root / 'stopped')})

    def snapshots(self):
        return sorted((self.install / 'backups').glob('*/*/.complete'))

    def test_snapshot_preserves_old_scripts_config_markers_and_live_identity(self):
        self.old_install()
        result = self.upgrade()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.snapshots()), 1)
        snap = self.snapshots()[0].parent
        self.assertEqual(snap.parent.name, 'v-old')
        self.assertEqual((snap / 'settings.ini').read_text(), 'user settings')
        self.assertIn('STOP_LOG', (snap / 'scripts/tailscaled.service').read_text())
        self.assertTrue((snap / 'hostname-user-set').is_dir())
        self.assertEqual((self.install / 'run/tailscaled.state').read_bytes(), b'PRIVATE IDENTITY')
        self.assertEqual((self.install / 'run/tailscaled.state').stat().st_mode & 0o777, 0o600)
        self.assertEqual((self.install / 'settings.ini').stat().st_mode & 0o777, 0o640)
        self.assertEqual((self.install / 'settings.ini').read_text(), 'user settings')
        self.assertNotIn(b'PRIVATE IDENTITY', b''.join(p.read_bytes() for p in snap.rglob('*') if p.is_file()))
        for p in (self.install / 'backups').rglob('*'):
            self.assertEqual(p.stat().st_mode & 0o777, 0o700 if p.is_dir() else 0o600, str(p))
        self.assertEqual((self.install / 'backups').stat().st_mode & 0o777, 0o700)

    def test_first_install_does_not_snapshot_or_delete_existing_state(self):
        result = self.upgrade()
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.snapshots(), [])
        self.assertEqual((self.install / 'run/tailscaled.state').read_bytes(), b'PRIVATE IDENTITY')

    def test_repeated_versions_are_distinct_and_only_five_completed_snapshots_remain(self):
        self.old_install()
        for i in range(8):
            result = self.upgrade('v-next')
            self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(len(self.snapshots()), 5)
        self.assertEqual(len({p.parent.name for p in self.snapshots()}), 5)
        self.assertTrue(all(p.parent.parent.name == 'v-next' for p in self.snapshots()))

    def test_symlink_source_aborts_before_service_stop_and_does_not_read_target(self):
        self.old_install()
        target = self.root / 'outside'
        target.write_text('private outside')
        (self.install / 'settings.ini').unlink()
        (self.install / 'settings.ini').symlink_to(target)
        result = self.upgrade()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / 'stopped').exists())
        self.assertEqual(self.snapshots(), [])
        self.assertEqual(target.read_text(), 'private outside')

    def test_symlink_backup_root_and_invalid_version_fail_closed(self):
        self.old_install()
        outside = self.root / 'outside'
        outside.mkdir()
        (self.install / 'backups').symlink_to(outside, target_is_directory=True)
        self.assertNotEqual(self.upgrade().returncode, 0)
        self.assertFalse((self.root / 'stopped').exists())
        self.assertEqual(list(outside.iterdir()), [])
        (self.install / 'backups').unlink()
        self.assertNotEqual(self.upgrade('../../escape').returncode, 0)
        self.assertFalse((self.root / 'stopped').exists())

    def test_nested_symlink_and_special_file_are_not_copied(self):
        self.old_install()
        outside = self.root / 'secret'
        outside.write_text('do not copy')
        link = self.install / 'scripts/link'
        link.symlink_to(outside)
        self.assertNotEqual(self.upgrade().returncode, 0)
        self.assertFalse((self.root / 'stopped').exists())
        link.unlink()
        os.mkfifo(self.install / 'scripts/pipe')
        self.assertNotEqual(self.upgrade().returncode, 0)
        self.assertEqual(self.snapshots(), [])

    def test_copy_failure_and_lock_contention_abort_before_stop(self):
        self.old_install()
        self.assertNotEqual(self.upgrade(fail_copy=True).returncode, 0)
        self.assertFalse((self.root / 'stopped').exists())
        self.assertEqual(self.snapshots(), [])
        with (self.install / 'backups/.lock').open('w') as lock:
            fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
            self.assertNotEqual(self.upgrade().returncode, 0)
        self.assertFalse((self.root / 'stopped').exists())
        self.assertEqual((self.install / 'settings.ini').read_text(), 'user settings')
        self.assertEqual((self.install / 'run/tailscaled.state').read_bytes(), b'PRIVATE IDENTITY')

    def test_unowned_directories_survive_retention_and_stale_staging_is_removed(self):
        self.old_install()
        foreign = self.install / 'backups/foreign/00000001'
        foreign.mkdir(parents=True)
        (foreign / 'keep').write_text('not owned')
        stale = self.install / 'backups/v-old/.pending-99999'
        stale.mkdir(parents=True)
        (stale / 'partial').write_text('partial copy')
        for i in range(6):
            self.assertEqual(self.upgrade().returncode, 0)
        self.assertTrue((foreign / 'keep').exists())
        self.assertFalse(stale.exists())
        self.assertEqual(len(self.snapshots()), 5)


if __name__ == '__main__':
    unittest.main()
