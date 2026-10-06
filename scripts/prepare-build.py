#!/usr/bin/env python3
"""Patch pinned Tailscale source and generate a build-local Go overlay.

The overlay changes net's actual resolver reader, including its reload path.
It does not mutate a shared GOROOT or depend on systemless /etc mounts.
"""
import json
import pathlib
import subprocess
import sys

repo = pathlib.Path(__file__).resolve().parents[1]
source = pathlib.Path(sys.argv[1]).resolve()
output = pathlib.Path(sys.argv[2]).resolve()
output.mkdir(parents=True, exist_ok=True)
goroot = pathlib.Path(subprocess.check_output(['go', 'env', 'GOROOT'], text=True).strip())
original = goroot / 'src/net/dnsconfig_unix.go'
text = original.read_text()
needle = 'func dnsReadConfig(filename string) *dnsConfig {'
if text.count(needle) != 1:
    raise SystemExit('unsupported Go resolver source; refusing an unpatched build')
replacement = needle + '\n\tif filename == "/etc/resolv.conf" { filename = "/data/adb/tailscale/bootstrap-resolv.conf" }'
patched = output / 'dnsconfig_unix.go'
patched.write_text(text.replace(needle, replacement))
replacements = {str(original): str(patched)}
# Stat and read the same file during resolver reload.
original = goroot / 'src/net/dnsclient_unix.go'
text = original.read_text()
if text.count('"/etc/resolv.conf"') < 2:
    raise SystemExit('unsupported Go resolver reload source')
patched = output / 'dnsclient_unix.go'
patched.write_text(text.replace('"/etc/resolv.conf"', '"/data/adb/tailscale/bootstrap-resolv.conf"'))
replacements[str(original)] = str(patched)
(output / 'overlay.json').write_text(json.dumps({'Replace': replacements}))

subprocess.run(['git', 'apply', '-'], input=(repo / 'patches/linuxfw-mark.patch').read_text(), text=True, cwd=source, check=True)
path = source / 'net/dns/resolvconfpath_default.go'
text = path.read_text()
path.write_text(text.replace('/etc/resolv.conf', '/data/adb/tailscale/os-resolv.conf')
                   .replace('/etc/resolv.pre-tailscale-backup.conf', '/data/adb/tailscale/os-resolv.backup.conf'))
# Tailscale's DNS manager can manage its own private file; it never owns bootstrap.
# Read current Android DNS as base config even when accept-dns has a saved backup.
path = source / 'net/dns/direct.go'
text = path.read_text()
needle = 'func (m *directManager) GetBaseConfig() (OSConfig, error) {'
if text.count(needle) != 1:
    raise SystemExit('unsupported Tailscale directManager source')
text = text.replace(needle, needle + '\n\tif b, err := m.fs.ReadFile(androidBootstrapConf); err == nil { return readResolv(bytes.NewReader(b)) }')
text = text.replace('go m.runFileWatcher()', 'go m.runFileWatcher()\n\tgo m.runAndroidBootstrapWatcher()')
# Close should only clean up this build's private legacy file.
text = text.replace('m.fs.Remove("/etc/resolv.tailscale.conf")', 'm.fs.Remove("/data/adb/tailscale/os-resolv.legacy.conf")')
path.write_text(text)

(source / 'net/dns/android_bootstrap.go').write_text((repo / 'patches/android_bootstrap.go').read_text())
(source / 'net/dns/android_bootstrap_test.go').write_text((repo / 'patches/android_bootstrap_test.go').read_text())

target = source / 'cmd/tailscaled/android_dns_linux.go'
target.write_text((repo / 'patches/android_dns_linux.go').read_text())
print(output / 'overlay.json')
