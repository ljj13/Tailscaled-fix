# WebUI 1 screenshot gallery

Captured 2026-10-07 from the local desktop browser suite with mock data.
These are not Android phone screenshots. All images use local resources.

## Pages and themes

| Page | Light | Dark |
|---|---|---|
| home | [Light](light-home.png) | [Dark](dark-home.png) |
| settings | [Light](light-settings.png) | [Dark](dark-settings.png) |
| network | [Light](light-network.png) | [Dark](dark-network.png) |
| dns | [Light](light-dns.png) | [Dark](dark-dns.png) |
| routing | [Light](light-routing.png) | [Dark](dark-routing.png) |
| logs | [Light](light-logs.png) | [Dark](dark-logs.png) |
| about | [Light](light-about.png) | [Dark](dark-about.png) |

## Mock states

- [wifi](scenario-wifi.png)
- [cellular](scenario-cellular.png)
- [needs-login](scenario-needs-login.png)
- [failure](scenario-failure.png)
- [stopped](scenario-stopped.png)

## Dialogs, sheet and responsive layouts

- [dialog-hostname](dialog-hostname.png)
- [dialog-logout](dialog-logout.png)
- [sheet-dns](sheet-dns.png)
- [width-320-dns](width-320-dns.png)
- [width-1280-dns](width-1280-dns.png)

Regenerate with `node tests/webui.test.cjs`, then copy the 24 current PNGs and
`index.html` from `build/webui-screenshots/` here. `index.html` is a local HTML
gallery; GitHub renders this Markdown gallery and its image links.
