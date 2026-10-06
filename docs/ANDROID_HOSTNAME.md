# Android default hostname initialization — Preview 4

The standalone GOOS=linux daemon uses the kernel hostname when `Prefs.Hostname`
is empty. On Android that can be `localhost`, producing a machine name such as
`localhost-0`. The owner requested an Android-derived default saved to prefs,
while preserving all explicit names, OS reporting, networking and node identity.

## Initialization and source priority

`tools/android-hostname` is an isolated static Linux arm64 helper built with
Go 1.26.6. It reads the installed CLI's `debug prefs` through the preserved
socket arguments and parses the complete JSON. Only an existing, empty string
`Hostname` permits initialization. Nonempty names, including deliberately set
`localhost` or `localhost-0`, are preserved; missing/null/malformed/unavailable
preferences leave initialization pending.

Candidate priority is:

1. `settings get global device_name` — Android's user-visible device name.
2. `getprop persist.sys.device_name` — an OEM device-name property.
3. `settings get secure bluetooth_name` — another user-name fallback.
4. `ro.product.marketname`, `ro.product.vendor.marketname`,
   `ro.product.odm.marketname`.
5. `ro.product.model`, then `ro.product.vendor.model`.
6. `ro.product.device`, then `ro.product.vendor.device`.

Each read is bounded to one second. Normalize to lowercase ASCII letters,
digits and hyphens, collapse separators, trim edge hyphens, truncate to one
63-byte DNS label, then trim trailing hyphens again. Dots are separators, not
FQDN components. Empty, multiline, `null`, `unknown` and localhost names are
skipped. An entirely non-ASCII user name falls through to the next candidate;
the helper does not fabricate a transliteration or expose serial/MAC/Android ID.
For example, `Redmi Note 8 Pro` becomes `redmi-note-8-pro`.

The helper re-reads prefs before its sole write:
`tailscale --socket=... set --hostname=<normalized-name>`. It never runs `up`,
`login`, `logout`, changes a network preference, or opens `tailscaled.state`.
Prefs reads and writes each have a three-second timeout. Failed discovery,
reads or writes do not create a completion marker, so future service starts and
watchdog ticks retry. Hooks run after existing DNS/routing repair; after success
they perform marker checks only.

Android documents `Settings.Global.DEVICE_NAME` in its
[API reference](https://developer.android.com/reference/android/provider/Settings.Global#DEVICE_NAME).
Tailscale documents default machine names and hostname overrides in
[Machine names](https://tailscale.com/docs/concepts/machine-names).

## Protecting user choice

`/data/adb/tailscale/hostname-initialized` records source and normalized initial
name once. Observing an existing explicit preference also completes initialization
with `source=preserved-explicit`. Subsequent Android device-name changes do not
trigger automatic renaming.

Both `tailscaled.service set-pref hostname` and the module's CLI wrapper record
`/data/adb/tailscale/hostname-user-set` before an explicit naming request, then
wait for an in-flight initializer to finish. The wrapper recognizes both
`--hostname`/`--hostname=...` and Go's `-hostname`/`-hostname=...` forms, retaining
argument order, the custom socket and updater guard. Explicit intent permanently
opts out, including a request to clear the hostname or one whose CLI call fails.
Initialization never removes this marker. Direct calls to the raw daemon/CLI
binary bypass wrapper intent tracking; existing nonempty prefs are nevertheless
checked twice and preserved. Use the module wrapper or WebUI for manual naming.

Initialization and manual protection use Linux `flock` on a persistent private
inode; it is never unlinked to recover a lock. Kernel release on process exit or
SIGKILL avoids stale PID/boot locks and cleanup races. Child commands have
`Pdeathsig=SIGKILL`, with their creating OS thread held through `Wait`, so killing
an initializer also stops a pending CLI child. A manual operation waits at most
35 seconds and fails visibly if the initializer remains busy, rather than racing
its write. Markers are private (file 0600/directory 0700).

## Installation and packaging

The installer validates the new `android-hostname` payload along with the two
existing binaries before stopping an installation, installs it under `bin/`,
and retains its existing state/settings/routes permission behavior. Existing
hostname markers survive upgrade as well. A fresh source build includes the
helper. The formal WebUI 1 source build writes
`tailscaled-v1.102.5-dnsfix.2-webui.1-arm64.zip`, preserving the accepted dnsfix.2
artifact filename. Formal publication uses `package-webui.py --release` and also
updates module version/versionCode; the Preview 4 comparison below is historical.

Preview 4 is packaged over the exact accepted dnsfix.2 ZIP
(`c848a47a8ebbcc3f03594c30583651e17b2013ee02bee77954a83317281ddfae`):

- The accepted tailscaled/CLI and DNS helper binaries and build-info are reused.
- Only `customize.sh`, `tailscale/scripts/tailscaled.service` and
  `system/bin/tailscale` are overlaid, plus the new `files/android-hostname`.
- `module.prop` retains its previous fields except the authorized
  `author=FogPurification`; module ID/version/versionCode stay the same.
- All other 13 non-WebUI entries and existing Unix modes are preserved.
- Helper ELF architecture/static linkage, pinned Go version, binary hash and
  all helper source hashes are checked before packaging; stale builds are refused.

Reproduce locally under Linux/WSL with the pinned Go on PATH:

```sh
python3 scripts/build-hostname.py
python3 -m unittest discover -s tests -p 'test_*.py' -v
node tests/webui-command.test.cjs
node tests/webui.test.cjs
python3 scripts/package-webui.py
```

The shell-command and browser tests can also run from Windows; their POSIX shell
fixture uses WSL. The browser suite produces 24 screenshots under
`build/webui-screenshots/` and covers all existing pages and actions.

## Verification and phone check

Tests cover source priority and normalization, preserved overrides, unrelated
prefs/identity/socket/config retention, failed reads/names/writes and recovery,
startup/watchdog retry, concurrent initialization and manual service/CLI naming,
single-dash arguments, and SIGKILL during both prefs reads and pending writes.
Installer tests cover fresh/upgrade paths and preserved private markers.
Packaging tests compare all unchanged payload bytes and modes with the accepted
ZIP and reject stale helper sources or corrupted binaries.

Local acceptance on 2026-10-07: all 26 Python tests passed (13 hostname tests,
10 existing DNS/shell tests, three packaging tests); 54 native shell-command
checks passed; the complete browser suite passed across seven pages, five
scenarios and both themes, producing 24 screenshots. Shellcheck, shell syntax
and helper `go vet` passed. Independent review found no remaining Critical or
Important issues. This is desktop/WSL acceptance, not device acceptance.

No new device acceptance is claimed. Install Preview 4 over the current module
and reboot. The smallest useful read-only check is:

```sh
su -c '/data/adb/modules/tailscaled/system/bin/tailscale debug prefs' | grep Hostname
su -c '/data/adb/tailscale/scripts/tailscaled.service diag' | grep hostname_
```

For an empty previous preference, expect an Android-derived name and
`hostname_initialization=done` with `hostname_source`. Existing explicit names
remain unchanged. After manually naming through Settings, expect
`hostname_initialization=manual-protected`; restart or reboot to confirm that
name remains. The diagnostic `hostname_initial` is historical, not a substitute
for the current `Prefs.Hostname`. No re-login or node deletion is required.
