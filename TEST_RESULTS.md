# Local verification — 2026-10-06

Tested on Windows with Ubuntu 24.04 WSL and Go 1.26.6. No Android device is attached.

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

The upstream fixture overlay is described in [DNS_FIX.md](DNS_FIX.md). Original
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

These checks do not prove live Redmi/KernelSU/Magisk/SELinux, Android Private DNS,
network switching or Clash/Mihomo traffic behavior. Use the minimal device
checks in [DNS_FIX.md](DNS_FIX.md) to complete that acceptance testing.
