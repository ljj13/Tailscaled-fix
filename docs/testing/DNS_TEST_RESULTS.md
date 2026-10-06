# Verification and device acceptance — 2026-10-06

This report records dnsfix.1/dnsfix.2 verification. See the
[WebUI 1 release notes](../releases/v1.102.5-dnsfix.2-webui.1.md) for later UI
and hostname checks; no new device tests are claimed by this documentation move.

Tested on Windows with Ubuntu 24.04 WSL and Go 1.26.6. dnsfix.2 also tested over
ADB on rooted Redmi Note 8 Pro `pnq47xf6899t4huc`.

Final dnsfix.2 device acceptance was confirmed by the device owner on
2026-10-06. The acceptance results below are owner-reported; the earlier ADB
measurements remain recorded separately. No functional code changed after the
accepted build at `23e9caa3ae98061ee1c70db9345065f5a6e1e680`.

## Final Redmi Note 8 Pro acceptance — PASS

| Scenario | Result |
| --- | --- |
| Mobile data + FlClash OFF | PASS |
| Mobile data + FlClash ON | PASS |
| VPN ON→OFF | PASS |
| Wi-Fi + FlClash ON | PASS — device owner confirmed |

Across these accepted scenarios, Android underlying network selection, DNS,
main marked route, table52, exemptions, tailscale ping, and kernel ping all
passed. Wi-Fi + FlClash ON is now device-validated, rather than fixture-only.

## dnsfix.2 verification

| Check | Result |
| --- | --- |
| Helper Go tests | 20 passed with race detector; go vet passed |
| Python build/script/upgrade checks | 8 passed (2 build + 6 shell) |
| Real-helper CLI regressions | 2 passed: first-boot network-change retry; failed refresh preserves resolver bytes and state/0600 |
| Complete relevant Tailscale packages | net/dns, net/dnscache, net/netns, osrouter passed with the existing fixture overlay |
| Bootstrap watcher race tests | 2 passed |
| Actual standard Go resolver integration | Passed: private DNS selection and atomic file replacement/reload without restart |
| Shell syntax / ShellCheck / WebUI JS | Passed |
| Independent review | CLAT routing, explicit empty underlying, public-cache revocation, first-boot race, multi-VPN precedence, interface-specific source fixed; final review found no blocking findings |
| Device network selection, FlClash ON | Active physical106/CELLULAR/ccmni1, VPN107 then108/CELLULAR\|VPN, underlying106; tun0 excluded |
| Device automatic OFF→ON recovery | At 19:59:16 OFF active-default; at 19:59:48 ON VPN108→106; both kept 120.80.80.80,221.5.88.88 and reachable=true |
| Device marked route | Before fix main/default and mark0x10020000 both went tun0; after fix main/default via ccmni1 and marked DNS route dev ccmni1 |
| Device manual vs watchdog | Same mark and physical route; both successfully resolve with network DNS; discovery about 54–69 ms, successful probes about 33–63 ms in captured samples |
| Device coexistence | Running, tailscale0, table52 four routes, exemptions pre/out/nat OK; tailscale ping and kernel ping both succeeded |
| Device identity/config | Tailnet IP remains100.118.66.106; settings/routes SHA256 unchanged; no login/logout performed |
| Device injected failure (isolated state directory) | All candidates failed for an .invalid test name; reachable=false/retained=true; verified source/list unchanged; bootstrap SHA256 identical before/after |
| Remaining coverage limits | CLAT-only access and other OEM dump formats remain fixture-only; root-manager cover-install/reboot was not independently rerun by the assistant |

The resolver integration initially failed because its two immediate replacements
received exactly the same kernel mtime (confirmed by logging identical nanosecond
timestamps). The fixture now waits across Go's five-second reload gate before
the second write, asserts different mtimes, and passes. Production watchdog
refreshes are separated by at least 15 seconds; retained failed refreshes do not
rewrite the resolver. First-boot and byte-preservation tests execute the real
compiled helper with isolated OEM-style dumpsys commands.

The earlier assistant-run ADB verification used updated helper/service plus
the existing dnsfix.1 daemon (the daemon networking patches are unchanged in
dnsfix.2). The subsequent owner-confirmed dnsfix.2 acceptance is recorded above.
The release reuses the accepted dnsfix.2 ZIP without rebuilding or changing its
binaries; embedded build provenance remains the functional commit `23e9caa`.

Release artifact: `tailscaled-v1.102.5-dnsfix.2-arm64.zip`.
SHA256: `c848a47a8ebbcc3f03594c30583651e17b2013ee02bee77954a83317281ddfae`.

## Historical dnsfix.1 verification

| Check | Result |
| --- | --- |
| Python module/script/upgrade suite | 7 tests passed |
| Android DNS helper | 8 tests passed with race detector; `go vet` passed |
| Tailscale `net/dns` complete package suite | Passed with fixture overlay |
| Tailscale `net/dnscache` complete package suite | Passed |
| Tailscale `net/netns` complete package suite | Passed |
| Tailscale `wgengine/router/osrouter` complete package suite | Passed with patched-mark fixture overlay |
| New bootstrap watcher tests | 2 tests passed with race detector, including missed-registration recovery |
| Actual Go resolver integration | Initial private DNS selection and atomic replacement/reload passed without process restart |
| Regression baseline | Unmodified Go failed as intended by selecting host DNS rather than the fixture |
| Missed-registration regression baseline | Old watcher failed as intended; periodic content check repaired it |
| Shell syntax and ShellCheck | Passed; exclusions are sourced/environment variables, intentionally split legacy argument strings, and unused shared settings |
| WebUI JavaScript | `node --check webroot/app.js` passed |
| Package | Static Linux arm64 daemon + helper; required osrouter/fwmark/private-DNS markers; ZIP paths and CRC checked; SHA256 generated |
| Review | Four material DNS edge cases fixed, then re-reviewed; final minor watcher race also fixed and regression-tested |

The upstream fixture overlay is described in [DNS_FIX.md](../dns/DNS_FIX.md). Original
unadapted upstream tests failed on intentionally changed resolver paths and
patched marks; this is documented rather than counted as an unmodified-suite
pass. A first watcher race-detector run also caught test hook cleanup racing
with the existing asynchronous watcher; the test now waits for registration
and shutdown before restoring the hook, and its final race run passes.

Upgrade fixtures execute the installer with root-manager APIs replaced and
external Android process discovery isolated. They verify first install and
upgrade, retained node state bytes, retained `0600` state permissions, retained
user settings/routes, new helper installation, and missing payload rejection
before touching the installed service. Mobile route fixtures cover both
gateway and direct-device defaults. The offline startup fixture verifies that
disabled public fallback leaves a retry worker which starts the daemon when
Android DNS becomes available.

These historical local-only dnsfix.1 checks did not establish live Android or
proxy behavior. Current dnsfix.2 Redmi acceptance is recorded above; it does not
extend coverage to other devices, all root managers, or Android Private DNS modes.
