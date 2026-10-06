// Local, ephemeral data. Explicit demo mode never reaches the root bridge.
export const scenarios = {
  wifi: "Wi-Fi + FlClash",
  cellular: "移动数据",
  "needs-login": "等待登录",
  failure: "DNS / 路由异常",
  stopped: "服务已停止",
};
const base = {
  daemon: "1",
  pid: "23841",
  watchdog: "1",
  watchdog_pid: "23812",
  iface: "tailscale0",
  ip4: "100.101.23.8/32",
  routes: "100.64.0.0/10",
  routes_auto: "1",
  version: "v1.102.5-dnsfix.2-webui.1",
  binary_ok: "1",
  osrouter: "1",
  daemon_current: "1",
  main_default: "ok",
  exempt_prerouting: "OK",
  exempt_output: "OK",
  exempt_nat: "OK",
  backend: "Running",
  user: "demo@example.com",
  health: "",
  dns_start_pending: "0",
  dns_source: "android-linkproperties",
  dns_network: "120",
  dns_transport: "WIFI",
  dns_active_vpn: "121",
  dns_underlying: "120",
  dns_iface: "wlan0",
  dns_servers: "192.168.31.1",
  dns_reachable: "true",
  dns_probe_mark: "0x10020000",
  dns_excluded: "121:tun0:VPN; tailscale0:excluded-interface",
  dns_selection_reason: "active-vpn-underlying:120",
  dns_route_hint: "tun0",
  dns_physical_route: "default via 192.168.31.1 dev wlan0 table 1030",
  dns_checked: "2026-10-06T10:20:00+08:00",
  dns_retained: "false",
  dns_last_verified: "2026-10-06T10:20:00+08:00",
  dns_generation: "demo-wifi",
  dns_bootstrap_file: "/data/adb/tailscale/bootstrap-resolv.conf",
};
const kv = (obj) =>
  Object.keys(obj)
    .map((key) => `${key}=${obj[key]}`)
    .join("\n") + "\n";
const result = (stdout, errno = 0, stderr = "") => ({ errno, stdout, stderr });
export function createDemo(initial) {
  let name, status, prefs, cleared;
  function select(next) {
    name = Object.prototype.hasOwnProperty.call(scenarios, next)
      ? next
      : "wifi";
    status = Object.assign({}, base);
    prefs = {
      prefs_ok: "1",
      accept_routes: "1",
      accept_dns: "0",
      shields_up: "0",
      advertise_routes: "",
      hostname: "redmi-note-8-pro",
    };
    cleared = false;
    if (name === "cellular")
      Object.assign(status, {
        dns_network: "140",
        dns_transport: "CELLULAR",
        dns_active_vpn: "",
        dns_underlying: "",
        dns_iface: "ccmni1",
        dns_servers: "10.64.64.64,10.64.64.65",
        dns_excluded: "tailscale0:excluded-interface",
        dns_selection_reason: "active-physical-network:140",
        dns_route_hint: "ccmni1",
        dns_physical_route: "default dev ccmni1 table 1010",
        dns_generation: "demo-cellular",
      });
    if (name === "needs-login")
      Object.assign(status, {
        backend: "NeedsLogin",
        ip4: "",
        user: "",
        routes: "",
      });
    if (name === "failure")
      Object.assign(status, {
        backend: "Starting",
        main_default: "NONE",
        exempt_output: "BAD",
        dns_reachable: "false",
        dns_source: "public-fallback-unverified",
        dns_servers: "1.1.1.1,8.8.8.8,9.9.9.9",
        dns_selection_reason: "no-verified-candidates",
        dns_physical_route: "",
        health:
          "Control plane connection pending; bootstrap DNS probe timed out.",
      });
    if (name === "stopped")
      Object.assign(status, {
        daemon: "0",
        watchdog: "0",
        pid: "",
        watchdog_pid: "",
        backend: "",
        ip4: "",
        routes: "",
        iface: "",
        main_default: "NONE",
        exempt_prerouting: "",
        exempt_output: "",
        exempt_nat: "",
      });
  }
  select(initial);
  return {
    select,
    get name() {
      return name;
    },
    async exec(command) {
      await new Promise((resolve) => setTimeout(resolve, 80));
      if (command === "tailscaled.service webstatus") return result(kv(status));
      if (command === "tailscaled.service prefs") return result(kv(prefs));
      if (command === "tailscale status --json")
        return result(
          JSON.stringify({
            Self: {
              HostName: prefs.hostname || "localhost",
              DNSName:
                (prefs.hostname || "localhost-0") + ".demo-tailnet.ts.net.",
            },
          }),
        );
      if (/^tailscaled\.service (start|stop|restart)$/.test(command)) {
        const stop = command.endsWith(" stop");
        Object.assign(status, {
          daemon: stop ? "0" : "1",
          watchdog: stop ? "0" : "1",
          pid: stop ? "" : "23841",
          iface: stop ? "" : "tailscale0",
          backend: stop ? "" : status.user ? "Running" : "NeedsLogin",
          ip4: stop || !status.user ? "" : "100.101.23.8/32",
        });
        return result("demo: service action completed\n");
      }
      if (command === "tailscaled.service logout") {
        Object.assign(status, {
          backend: "NeedsLogin",
          user: "",
          ip4: "",
          routes: "",
        });
        return result("Logged out\n");
      }
      if (command === "tailscale up --timeout=8s")
        return result(
          status.user
            ? "Already logged in\n"
            : "To authenticate, visit:\nhttps://login.tailscale.com/a/demo-preview\n",
          status.user ? 0 : 1,
        );
      const pref = command.match(
        /^tailscaled\.service set-pref (accept-routes|accept-dns|shields-up|advertise-exit-node|hostname) ([A-Za-z0-9.-]+)$/,
      );
      if (pref) {
        if (pref[1] === "hostname") prefs.hostname = pref[2];
        else if (pref[1] === "advertise-exit-node")
          prefs.advertise_routes = pref[2] === "on" ? "0.0.0.0/0,::/0" : "";
        else prefs[pref[1].replace(/-/g, "_")] = pref[2] === "on" ? "1" : "0";
        return result("Preference updated\n");
      }
      if (command === "tailscaled.service dns-refresh")
        return result("demo: discovery refreshed\n" + kv(status));
      if (command === "tailscaled.service dns")
        return result(
          kv(
            Object.fromEntries(
              Object.entries(status).filter(
                ([key]) => key.indexOf("dns_") === 0,
              ),
            ),
          ) +
            "\nMarked probe: " +
            (status.dns_reachable === "true"
              ? `${status.dns_servers}:ok`
              : `${status.dns_servers}:timeout`),
        );
      if (command === "tailscaled.service routes")
        return result(
          `=== main marked route ===\n${status.main_default === "ok" ? status.dns_physical_route : "no default route"}\n\n=== table 52 ===\n${status.daemon === "1" ? "100.101.23.8 dev tailscale0\n100.64.0.0/10 dev tailscale0" : "(empty)"}\n\n=== proxy exemptions ===\npre=${status.exempt_prerouting || "absent"} out=${status.exempt_output || "absent"} nat=${status.exempt_nat || "absent"}\n`,
        );
      if (command === "tailscaled.service selftest")
        return result(
          `demo selftest (simulated)\nlinux + osrouter: present\nfwmark: 0x10020000\nDNS: ${status.dns_reachable}\nmain: ${status.main_default}\nexemption: ${status.exempt_output}\nTailnet ping: ${status.backend === "Running" ? "pong" : "unavailable"}\n`,
        );
      if (command === "tailscaled.service diag")
        return result(
          cleared ? "(empty)" : "=== simulated diagnostics ===\n" + kv(status),
        );
      if (
        command ===
        'tail -n 250 /data/adb/tailscale/run/tailscaled.log 2>/dev/null || echo "(no daemon log yet)"'
      )
        return result(
          cleared
            ? "(empty)"
            : `2026/10/06 10:20:00 linux + osrouter\n2026/10/06 10:20:01 DNS source=${status.dns_source} interface=${status.dns_iface}\n2026/10/06 10:20:02 BackendState=${status.backend || "Stopped"}\n`,
        );
      if (
        command ===
        ": > /data/adb/tailscale/run/tailscaled.log; : > /data/adb/tailscale/run/diag.log"
      ) {
        cleared = true;
        return result("");
      }
      return result("", 1, "Unknown demo command");
    },
  };
}
