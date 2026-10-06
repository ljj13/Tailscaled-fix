# Android DNS fix for v1.102.5

本文为 dnsfix.1 阶段的历史审计；当前 DNS 行为以 [dnsfix.2](DNS_FIX_2.md) 为准，
真机与本地验证记录见 [DNS 测试报告](../testing/DNS_TEST_RESULTS.md)。

**dnsfix.2 VPN/underlying 更新与真机证据：见 [DNS_FIX_2.md](DNS_FIX_2.md)。**

下文保留 dnsfix.1 的初始审计；dnsfix.2 的网络选择与缓存规则以上述文档为准。

## Scope and pinned versions

- Module base: keweiya/tailscaled tag `v1.102.5`, commit `aec266c8c8a2115f83833c07f95bc5f71501a78d`.
- Fork: ljj13/Tailscaled-fix, branch `fix/android-dns-v1.102.5`.
- Tailscale source: tag `v1.102.5`, commit `5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115`.
- Go 1.26.6, `GOOS=linux GOARCH=arm64 CGO_ENABLED=0`, tags `ts_include_cli,ts_omit_ssh`.

## Root cause and audit

The reported device has no `/etc/resolv.conf` and no loopback DNS listener. Go's
pure Go resolver reads `/etc/resolv.conf`; without it, default nameservers are
`127.0.0.1:53` and `[::1]:53`. A refused lookup prevents normal control-plane
connection setup and explains the prolonged `Starting` state despite usable
fwmarks and routes. This is a source-level explanation of the reported failure,
not a claim that this workstation reproduced Android's backend state.

The existing workflow's two sed substitutions changed only Tailscale's
`net/dns/resolvconfpath_default.go`. That path is its OS DNS configurator's file;
it does **not** change Go's standard-library resolver. No startup or watchdog code
generated that private resolver file either. Adding a module `system/etc` file
therefore cannot be the complete solution, even apart from the reported failed
systemless mount.

The audit covered all module scripts and wrappers, installer, uninstaller,
settings, build workflow, and WebUI commands:

| Area | Existing behavior / finding | Result |
| --- | --- | --- |
| `service.sh`, `start.sh` | Boot completion wait, bounded route wait, module disable watch | Preserved |
| `customize.sh` | Stops stale daemon, replaces program files, retains settings/routes, uses persistent run directory | Preserved; required payload checked before stopping; state permissions no longer recursively changed |
| CLI / daemon wrappers | Persistent socket; updater blocked; service manages ordinary starts | Preserved |
| Binary guard | Pristine patched daemon restored if replaced | Preserved |
| DNS build | Only Tailscale OS config path was patched | Actual Go resolver read **and reload-stat** paths now patched through build-local overlay |
| DNS startup | No Android DNS discovery or resolver provisioning | Added before first daemon lookup |
| Network detection | IPv4 `ip route get`; empty gateway lost positional fields on direct-device mobile routes | Gateway sentinel fixes field parsing; gateway and direct-device paths tested |
| Routing | Patched bypass rules, main default/source route, osrouter table 52, manual table 1099 fallback | Existing algorithms and mark values retained |
| Proxy coexistence | Rule-1 RETURN on tunnel mangle PREROUTING/OUTPUT and nat OUTPUT | Preserved; DNS sockets use the existing bypass mark |
| Watchdog | Repairs routes/exemptions every tick, periodically syncs subnet routes | DNS refresh added after route repair; offline DNS startup gets retry worker |
| Diagnostics | Routing/build checks, no DNS source or actual query check | `dns`, `dns-refresh`, `diag`, `selftest`, WebUI show DNS information |
| Upgrade | Recursive permissions could make state `0755` | Permissions now scoped to binaries/scripts; existing `0600` identity is preserved |
| Uninstall | Explicit module uninstall deletes `/data/adb/tailscale` | Existing destructive uninstall behavior is unchanged; upgrade by installing ZIP directly, without uninstalling first |

Relevant primary sources:
[Go resolver config reader](https://go.dev/src/net/dnsconfig_unix.go),
[Go loopback defaults](https://go.dev/src/net/dnsconfig.go),
[AOSP LinkProperties DNS format](https://android.googlesource.com/platform/frameworks/base/+/android10-release/core/java/android/net/LinkProperties.java),
[AOSP NetworkAgentInfo dump format](https://android.googlesource.com/platform/frameworks/base/+/e83420591827d2e6559b6b036f80599e83b24999/services/core/java/com/android/server/connectivity/NetworkAgentInfo.java).

## Design

1. `android-dns` reads `dumpsys connectivity` for the current physical interface's
   LinkProperties DNS. It matches CLAT stacked interfaces to their parent network,
   supports older and newer NetworkAgentInfo formats, ignores historical request
   sections and VPN records, and handles IPv6 scope IDs. Old interface properties
   and global `net.dnsN` properties are secondary compatibility sources.
2. DNS candidates are validated and deduplicated. Loopback, unspecified,
   multicast, invalid addresses and Tailscale's own DNS addresses are rejected.
3. Candidates must resolve `controlplane.tailscale.com` through real DNS, with
   the same mark-only routing as the daemon. A socket connecting successfully is
   insufficient. Checks are bounded and concurrent; Go supplies UDP/TCP retry.
4. If Android DNS is unavailable or all its candidates fail, try configurable
   public DNS from several providers (`1.1.1.1,8.8.8.8,9.9.9.9,223.5.5.5,119.29.29.29`).
   Only successful candidates are published while online. The source explicitly
   reports `public-fallback`; this path queries public plaintext DNS independently
   of Android Private DNS. Users requiring only network DNS can set the fallback
   string empty. Legacy properties may be stale; they are reported as such and
   are validated before being used online.
5. When every probe fails, publish the new network's non-loopback candidates
   (or configured public candidates) as **unverified**, so reconnect remains
   possible. If fallback is disabled and no Android DNS exists yet, postpone
   daemon startup and keep an automatic retry watchdog alive.
6. Atomically replace `/data/adb/tailscale/bootstrap-resolv.conf` only when
   contents differ. Go reads and stats that path and reloads on subsequent
   lookups, at its five-second check interval. No mounts, symlinks, or listeners
   on loopback port 53 are required.
7. Tailscale's OS DNS manager uses independent `os-resolv.conf` and backup files.
   Its base config reads bootstrap, and a separate file watch notifies the
   existing DNS recompilation path when bootstrap changes. A 15-second content
   check also recovers missed watch registration/rename events. This prevents stale
   base forwarders after a network switch, including when `--accept-dns=true`.
   Android's OS-wide DNS and netd configuration are not modified.
8. Startup prepares the existing main route and bypass rules, then DNS, then
   starts the daemon. Watchdog rediscovers DNS on every loop after route repair.
   With normal command timings, allow 30–45 seconds after a switch; long vendor
   command delays, offline links, or absent TUN interfaces can take longer.

The DNS helper never reads or writes `tailscaled.state`, calls login/logout,
changes preferences, flushes table 52, or edits proxy rules. Existing `settings.ini`
survives updates; service-side defaults cover installations that lack new keys.
`--accept-dns=false` remains the recommendation for Android application DNS;
this fix repairs standalone daemon bootstrapping and internal forwarding,
and does not install an Android system-wide MagicDNS integration.

## Configuration and diagnostics

Optional additions to `/data/adb/tailscale/settings.ini`:

```sh
dns_fallback_servers="1.1.1.1,8.8.8.8,9.9.9.9,223.5.5.5,119.29.29.29"
dns_probe_domain="controlplane.tailscale.com"
```

Set `dns_fallback_servers=""` to disable public fallback. The helper is locked
against simultaneous refresh writers. `dns-status` reports source, selected
servers, interface, query name, reachability, timestamp, and per-candidate errors.
`tailscaled.service dns` checks the published resolvers immediately;
`dns-refresh` requests rediscovery without restarting tailscaled.

## Test and build procedure

```sh
python3 -m unittest discover -s tests -p test_dns_build.py -v
python3 tests/test_shell.py
sh scripts/build.sh
python3 tests/test_resolver.py
```

`build.sh` runs the complete relevant `net/dns`, `net/dnscache`, `net/netns`, and
`wgengine/router/osrouter` package suites and the DNS helper's race tests/vet.
The upstream suites have literal `/etc` paths and stock fwmark golden values:
their unadapted run fails (`TestDNSTrampleRecovery`, `TestDirectManager`,
`TestDirectBrokenRename`, `TestDirectBrokenRemove`, `TestLinuxDNSMode`,
`TestRouterStates`). `prepare-tests.py` overlays only those fixture literals
and fixture directories with the intentional module paths and patched marks.
It does not relax, skip, or delete assertions.

`tests/test_resolver.py --baseline` is an intentional failing regression run:
unmodified Go uses the host DNS instead of the fixture. The fixed run verifies
both initial selection and atomic replacement/reload without restarting a
process. Tests use local synthetic DNS replies, so they do not depend on a
particular public network.

Packaging checks arm64 ELF headers, static-build provenance, osrouter/fwmark and
private DNS markers, required installer paths, LF shell content and ZIP CRC.
`files/build-info.json` records source commits, toolchain, flags and binary SHA256;
the ZIP has a separate SHA256 file. There is no remote publish or main merge in
the build workflow.

## Minimal device verification

Install `tailscaled-v1.102.5-dnsfix.1-arm64.zip` directly through KernelSU/Magisk,
then reboot. Keep existing state and login; no new login or preference reset is
needed. Run:

```sh
su -c 'tailscaled.service selftest'
su -c 'tailscale status'
```

Expected: patched linux/osrouter, current daemon, main default present,
table 52 intact, all three exemptions OK, reachable DNS with an explicit source,
and `BackendState=Running` for an already authorized node.

Switch Wi-Fi to mobile data, wait about 30–45 seconds, then run:

```sh
su -c 'tailscaled.service dns; tailscale status'
```

Switch back and repeat. Confirm resolver/interface follow the current link,
status returns to Running, and the existing node/account is retained. If the
source is public fallback, the details should explain Android DNS extraction or
query failure. Test one existing tailnet/subnet TCP connection while Clash/Mihomo
is enabled; `selftest` also checks routing/exemptions and pings an available peer.

No Android device is connected to this workstation. Real-device KernelSU/Magisk
mount/SELinux behavior, OEM dumpsys output, switching, and live proxy coexistence
remain the device acceptance checks. IPv6 DNS is supported when the existing
marked main route can reach it; this change does not add an IPv6 default-routing
or DNS64 subsystem to the module.
