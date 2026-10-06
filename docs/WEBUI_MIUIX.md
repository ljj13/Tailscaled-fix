# Miuix WebUI preview design and acceptance

## Scope

Stable dnsfix.2 was merged with a merge commit into main (`9ec00a2`), retaining
its complete history. `feature/webui-miuix` starts from that stable main. This
preview changes the WebUI only, with separate documentation, browser tests and
UI packaging support. Daemon, service, routes, DNS, root bridge and settings
remain unchanged. No release or feature merge is authorized before device
acceptance.

## Reference study

Reference: [GhostlockUI.kt at 289b5c6](https://github.com/YuKongA/ghostlock-app/blob/289b5c6fbc79ef5e8c326196d01b2d77aa5e69fd/app/src/main/kotlin/com/ghostlock/app/ui/GhostlockUI.kt),
plus the matching AdvancedUI and AboutUI. The reference uses Miuix components,
system light/dark schemes, a large TopAppBar, grouped Card / ArrowPreference /
SwitchPreference, a navigation back stack, OverlayDialog and OverlayBottomSheet.
Main content uses 12dp group spacing and responsive centered page padding.

The Web adaptation is original HTML/CSS/JS. It does not import reference source,
APK assets, Kotlin, Compose, or exploit functionality. System fonts and original
local SVG icons provide a consistent appearance without remote dependencies.

| Reference language | Web adaptation                                                                                                         |
| ------------------ | ---------------------------------------------------------------------------------------------------------------------- |
| TopAppBar          | Large 34px title, secondary-page back button and refresh action                                                        |
| Card / preferences | Shared 20px radius, neutral surfaces, 16px content padding, 12px groups; row title, summary, trailing value or chevron |
| SwitchPreference   | Accessible checkbox/switch, solid accent track and white thumb, immediate disabled/loading feedback                    |
| System theme       | Semantic CSS variables paired with prefers-color-scheme; neutral normal state text                                     |
| Navigation         | Home → settings/network/log/about; network → DNS/routing; hash/history back stack and scroll restoration               |
| OverlayDialog      | DOM modal for logout, stop, clear logs and hostname; focus management and visible cancel; no window.confirm dependency |
| OverlayBottomSheet | Read-only DNS/selftest output, copy and close actions; limited opacity/transform animation                             |
| Status expressions | 已连接 / 正常 in normal text; small restrained warning/error surfaces; no green OK dashboard                           |

## Information architecture

- **首页**: connection summary, physical connection type, hostname, Tailnet
  address/account, start/stop/restart/login and navigation rows.
- **设置**: accept routes, MagicDNS, shields up, advertise exit node, hostname,
  login/logout. Existing restrictions remain explained without changing behavior.
- **网络与诊断**: DNS and route/exemption summary, DNS details, route/table52
  output, full selftest. All dnsfix.2 fields remain available, including unknown
  future dns\_\* fields.
- **日志**: daemon / diagnostics, refresh, visible-only auto refresh, copy and
  confirmed clear. Output uses textContent and is selectable.
- **关于**: module version, WebUI preview edition, daemon/build integrity,
  capabilities and system theme.

## Behavior and compatibility

The original shell commands and KernelSU/APatch-compatible injected ksu bridge
are retained; Magisk usage continues to require its compatible WebUI host.
No new shell/service endpoint is introduced. DNS/routes/selftest buttons call
existing service commands only on user interaction.

Desktop without a bridge automatically uses a labelled local demo; `?demo=wifi`,
`cellular`, `needs-login`, `failure`, or `stopped` select a scenario. Explicit demo
mode never calls a native bridge, even when one exists. Mock actions and prefs
are local to the page and do not persist or change a device.

Compatibility target: Android WebView 76+ (system dark theme support). Use ES
modules/async-await already required by the previous UI, CSS variables, flex/grid
and env(safe-area-inset-\*). Avoid optional chaining, :has(), color-mix(), native
dialog APIs, external fonts, blur, heavy animation and framework dependencies.
Honor prefers-reduced-motion, 44–48px touch targets, keyboard focus, dialog traps,
safe areas and visibility-aware polling. Hardware back integration still needs
root-manager device acceptance; an explicit back button is always available.

## Local preview and acceptance

Serve `webroot` with a local HTTP server; file:// module loading varies by browser:

```sh
python -m http.server 8765 --bind 127.0.0.1 --directory webroot
```

Open `http://127.0.0.1:8765/?demo=wifi`. The bottom **本地演示** row switches
between Wi-Fi + FlClash, cellular, NeedsLogin, failure and stopped without a
reload. Deep links include `#settings`, `#network`, `#dns`, `#routing`, `#logs`
and `#about`. Change browser/OS color scheme to view the system dark theme.
This mode requires no ADB, root manager or phone.

Browser tooling is isolated in ignored `build/`; none is shipped in the module:

```sh
npm install --prefix build/browser-tools playwright@1.63.0 acorn@8.15.0 --no-audit --no-fund
node tests/webui.test.cjs
```

The test defaults to local Microsoft Edge on Windows. On other hosts set
`WEBUI_BROWSER` to the executable path of a local Chromium/Chrome/Edge browser.
It starts and stops its own localhost server. Artifacts are generated in
`build/webui-screenshots/`: 24 PNGs and a local `index.html` gallery.

### Verified local results (2026-10-06)

| Check                            | Result / evidence                                                                                                                                                          |
| -------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Seven pages, both system themes  | PASS; 14 full page screenshots at 390×844 with mobile/touch emulation                                                                                                      |
| Five status scenarios            | PASS; connected Wi-Fi + FlClash/cellular, NeedsLogin, DNS/routing failure, stopped                                                                                         |
| Narrow / desktop layout          | PASS; all pages at 320px without horizontal overflow; DNS detail also checked at 1280px                                                                                    |
| Reduced motion                   | PASS; page animation disabled on all seven pages                                                                                                                           |
| Actions / preferences            | PASS; start, stop, restart, login, logout, all four switches, hostname, logs, clear, copy, DNS, rediscovery, routes and selftest                                           |
| Legacy native command contract   | PASS; independent injected bridge fixture asserts the exact original command strings                                                                                       |
| Failure handling                 | PASS; failed preference rolls back; failed status read retains cached state; failed DNS probe preserves both streams, exit code and an explicit failure warning            |
| Slow log reads                   | PASS; 400ms response + 12 rapid source changes coalesce into two reads; obsolete output never replaces newest source                                                       |
| Navigation / modal Back          | PASS; normal Back and two-entry history jump keep URL, page and modal state consistent                                                                                     |
| DNS fields / escaping            | PASS; all required dnsfix.2 keys retained; unknown future key is displayed as text, not HTML                                                                               |
| Demo isolation / local resources | PASS; forced demo never calls injected native bridge; no external resource requests                                                                                        |
| Background work                  | PASS; hidden document and inactive log page execute no polling commands                                                                                                    |
| JS syntax / WebView review       | PASS ES2019 parse; flex-gap, inset, :focus-visible and safe-area fallbacks reviewed for Chromium 76                                                                        |
| Existing shell / DNS fixtures    | PASS: 10 tests, including upgrade/first install preserving state, settings, routes and state mode 0600                                                                     |
| Existing resolver integration    | PASS: bootstrap DNS uses and reloads replacement resolver without daemon restart                                                                                           |
| Existing Go / shell checks       | PASS: helper race tests, go vet, shellcheck and shell syntax                                                                                                               |
| Preview packaging                | PASS: accepted base SHA pinned; all 17 non-WebUI entries preserve exact bytes and Unix modes; CRC and UI SHA manifest verified; base/output/temp/checksum collision guards |

Screenshots were inspected page by page in both themes against the reference
component language: title hierarchy, neutral rounded cards, row summaries,
chevrons, accent switches, restrained status colors, dialog and bottom sheet.
This is a code-grounded design adaptation; no reference APK was run and no
pixel-for-pixel native screenshot comparison is claimed. Desktop Chromium
verification does not replace Android WebView/root-manager acceptance.

Independent code review found and verified fixes for slow-read coalescing,
failed probe output and multi-entry Back. No unresolved Critical or Important
review finding remains for this local preview.

### Installable preview ZIP

```sh
# Obtain the accepted base if dist/ is absent in a fresh checkout:
gh release download v1.102.5-dnsfix.2 --repo ljj13/Tailscaled-fix \
  --pattern tailscaled-v1.102.5-dnsfix.2-arm64.zip --dir dist
python scripts/package-webui.py
python tests/test_webui_package.py -v
```

Output: `dist/tailscaled-v1.102.5-dnsfix.2-webui-miuix-preview-arm64.zip`, plus
its `.sha256`. This remains an arm64 KernelSU/Magisk/APatch module with the
accepted dnsfix.2 module ID/version. Only `webroot/` entries are replaced;
the original binary build-info stays intact, with separate `webroot/ui-build.json`
recording UI revision, UI file hashes and accepted base artifact SHA.

No daemon/helper rebuild is needed for a UI-only preview. Reusing the accepted
payload avoids changing the already verified code or overwriting its release ZIP.

### Device acceptance still pending

After local review, install this preview ZIP through the usual manager and
check the same actions, login link, preference saves, log/copy/confirmation,
DNS/routing details, system themes, safe areas and manager hardware Back.
Preserve the existing dnsfix.2 Wi-Fi/cellular + FlClash coexistence checks.
This task does not use ADB or claim physical WebView/phone acceptance.
The feature branch remains separate from `main`; no release is published.
