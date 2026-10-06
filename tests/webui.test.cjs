// Browser interaction tests; tooling stays outside the shipped WebUI.
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
const http = require("node:http");
const ROOT = path.resolve(__dirname, "..");
const { chromium } = require(
  require.resolve("playwright", {
    paths: [path.join(ROOT, "build/browser-tools"), ROOT],
  }),
);
const acorn = require(
  require.resolve("acorn", {
    paths: [path.join(ROOT, "build/browser-tools"), ROOT],
  }),
);
for (const name of ["app.js", "ksu.js", "demo.js"])
  acorn.parse(fs.readFileSync(path.join(ROOT, "webroot", name), "utf8"), {
    ecmaVersion: 2019,
    sourceType: "module",
  });
const MIME = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".json": "application/json",
};
const server = http.createServer((req, res) => {
  const target = path.resolve(
    ROOT,
    "webroot",
    "." +
      new URL(req.url, "http://localhost").pathname.replace(
        /\/$/,
        "/index.html",
      ),
  );
  if (!target.startsWith(path.join(ROOT, "webroot") + path.sep)) {
    res.writeHead(403).end();
    return;
  }
  fs.readFile(target, (error, data) => {
    res.writeHead(error ? 404 : 200, {
      "Content-Type": MIME[path.extname(target)] || "text/plain",
    });
    res.end(error ? "Not found" : data);
  });
});
(async () => {
  await new Promise((resolve) => server.listen(0, "127.0.0.1", resolve));
  const base = `http://127.0.0.1:${server.address().port}`;
  const browser = await chromium.launch({
    headless: true,
    executablePath:
      process.env.WEBUI_BROWSER ||
      "C:/Program Files (x86)/Microsoft/Edge/Application/msedge.exe",
  });
  try {
    const shots = path.join(ROOT, "build/webui-screenshots");
    fs.mkdirSync(shots, { recursive: true });
    const counts = { pages: 0, scenarios: 0, screenshots: 0 };
    const page = await browser.newPage({
      viewport: { width: 390, height: 844 },
      isMobile: true,
      hasTouch: true,
      deviceScaleFactor: 1,
    });
    const errors = [];
    const requests = [];
    page.on("pageerror", (error) => errors.push(error.message));
    page.on("request", (request) => requests.push(request.url()));
    await page.goto(`${base}/?demo=wifi`);
    await page.locator("#status-main").filter({ hasText: "已连接" }).waitFor();
    await page.locator('[data-nav="settings"]').click();
    await page.locator("#page-settings").waitFor({ state: "visible" });
    assert.equal(await page.locator("#page-title").innerText(), "设置");
    assert.equal(await page.locator("#sw-accept-routes").isChecked(), true);
    await page.locator("#back").click();
    await page.locator("#page-home").waitFor({ state: "visible" });
    assert.deepEqual(errors, []);
    console.log("PASS WebUI navigation and initial preferences");
    async function idle(target = page) {
      await target.locator("#busy-label").waitFor({ state: "hidden" });
      await target.waitForTimeout(200);
    }
    async function shot(target, name) {
      await target.screenshot({
        path: path.join(shots, name + ".png"),
        fullPage: !/^(dialog|sheet)-/.test(name),
      });
      counts.screenshots++;
    }
    // Seven pages at phone width, both system themes; no horizontal overflow.
    for (const theme of ["light", "dark"]) {
      await page.emulateMedia({ colorScheme: theme });
      for (const name of [
        "home",
        "settings",
        "network",
        "dns",
        "routing",
        "logs",
        "about",
      ]) {
        await page.goto(`${base}/?demo=wifi#${name}`);
        await page
          .locator("#home-ip")
          .filter({ hasText: "100.101.23.8" })
          .waitFor({ state: "attached" });
        await page.waitForTimeout(300);
        assert.equal(await page.locator(`#page-${name}`).isVisible(), true);
        assert.equal(
          await page.evaluate(
            () => document.documentElement.scrollWidth > window.innerWidth,
          ),
          false,
          name + " overflow",
        );
        if (name === "dns") {
          for (const key of [
            "dns_network",
            "dns_transport",
            "dns_active_vpn",
            "dns_underlying",
            "dns_iface",
            "dns_excluded",
            "dns_selection_reason",
            "dns_route_hint",
            "dns_physical_route",
            "dns_reachable",
            "dns_probe_mark",
          ]) {
            assert.ok(
              (await page.locator("#page-dns").innerText()).includes(key),
              "missing " + key,
            );
          }
        }
        if (name === "routing")
          await page
            .locator("#routes-output")
            .filter({ hasText: "table 52" })
            .waitFor();
        if (name === "logs")
          await page
            .locator("#out")
            .filter({ hasText: "BackendState" })
            .waitFor();
        await shot(page, `${theme}-${name}`);
        counts.pages++;
      }
    }
    // Mock status fixtures and desktop/small-phone layout.
    await page.emulateMedia({ colorScheme: "light" });
    for (const [scenario, expected] of [
      ["wifi", "已连接"],
      ["cellular", "已连接"],
      ["needs-login", "等待登录"],
      ["failure", "正在连接"],
      ["stopped", "服务已停止"],
    ]) {
      await page.goto(`${base}/?demo=${scenario}`);
      await page
        .locator("#status-main")
        .filter({ hasText: expected })
        .waitFor();
      if (scenario === "failure")
        assert.equal(await page.locator("#health").isVisible(), true);
      if (scenario === "cellular")
        assert.match(await page.locator("#status-sub").innerText(), /移动数据/);
      await shot(page, `scenario-${scenario}`);
      counts.scenarios++;
    }
    for (const width of [320, 1280]) {
      await page.setViewportSize({ width, height: 900 });
      await page.goto(`${base}/?demo=wifi#dns`);
      await page.waitForTimeout(400);
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
        false,
      );
      await shot(page, `width-${width}-dns`);
    }
    await page.setViewportSize({ width: 320, height: 844 });
    await page.emulateMedia({ reducedMotion: "reduce" });
    for (const name of [
      "home",
      "settings",
      "network",
      "dns",
      "routing",
      "logs",
      "about",
    ]) {
      await page.goto(`${base}/?demo=wifi#${name}`);
      await page.waitForTimeout(250);
      assert.equal(
        await page.evaluate(
          () => document.documentElement.scrollWidth > innerWidth,
        ),
        false,
        "320px overflow: " + name,
      );
      assert.equal(
        await page
          .locator("#page-" + name)
          .evaluate((el) => getComputedStyle(el).animationName),
        "none",
      );
    }
    await page.emulateMedia({ reducedMotion: "no-preference" });
    await page.setViewportSize({ width: 390, height: 844 });
    await page.goto(`${base}/?demo=wifi#settings`);
    await page.locator("#sw-accept-routes").waitFor();
    await idle();
    // Every preference, hostname validation (shell injection), confirmations.
    for (const id of [
      "accept-routes",
      "accept-dns",
      "shields-up",
      "advertise-exit-node",
    ]) {
      const box = page.locator("#sw-" + id),
        before = await box.isChecked();
      await box.click();
      await idle();
      assert.equal(await box.isChecked(), !before);
    }
    await page.locator("#btn-hostname").click();
    await page.locator("#in-hostname").fill("bad; touch /tmp/injected");
    await page.locator("#dialog-confirm").click();
    assert.equal(await page.locator("#input-error").isVisible(), true);
    await page.locator("#in-hostname").fill("redmi-web-preview");
    await shot(page, "dialog-hostname");
    await page.locator("#dialog-confirm").click();
    await idle();
    await page
      .locator("#settings-hostname")
      .filter({ hasText: "redmi-web-preview" })
      .waitFor();
    await page.locator("#btn-logout").click();
    await page.keyboard.press("Escape");
    await page.locator("#overlay").waitFor({ state: "hidden" });
    assert.match(
      await page.locator("#settings-account").innerText(),
      /demo@example/,
    );
    await page.locator("#btn-logout").click();
    await shot(page, "dialog-logout");
    await page.locator("#dialog-confirm").click();
    await idle();
    await page.locator("#btn-login").click();
    await idle();
    await page.locator("#page-home").waitFor({ state: "visible" });
    assert.match(
      await page.locator("#login-url").getAttribute("href"),
      /^https:\/\/login\.tailscale\.com\//,
    );
    await page.locator("#stop-action").click();
    await page.locator("#dialog-confirm").click();
    await idle();
    await page
      .locator("#status-main")
      .filter({ hasText: "服务已停止" })
      .waitFor();
    await page.locator("#primary-action").click();
    await idle();
    await page
      .locator("#status-main")
      .filter({ hasText: "等待登录" })
      .waitFor();
    // Demo selector is usable without a browser reload.
    await page.locator("#demo-picker").click();
    await page
      .locator("#dialog-choices button")
      .filter({ hasText: "Wi-Fi" })
      .click();
    await idle();
    await page.locator("#status-main").filter({ hasText: "已连接" }).waitFor();
    await page.locator('[data-nav="network"]').click();
    await page.locator('[data-nav="dns"]').click();
    await page.locator('#page-dns [data-probe="dns"]').click();
    await idle();
    await page
      .locator("#dialog-output")
      .filter({ hasText: "Marked probe" })
      .waitFor();
    await shot(page, "sheet-dns");
    // Browser/system back closes the sheet first, preserving the current page.
    await page.goBack();
    await page.locator("#overlay").waitFor({ state: "hidden" });
    assert.equal(await page.locator("#page-dns").isVisible(), true);
    await page.locator('#page-dns [data-probe="dns"]').click();
    await idle();
    await page.evaluate(() => history.go(-2));
    await page.locator("#page-network").waitFor({ state: "visible" });
    assert.equal(new URL(page.url()).hash, "#network");
    await page.locator('[data-nav="dns"]').click();
    await page.locator("#btn-dns-refresh").click();
    await idle();
    await page
      .locator("#dialog-title")
      .filter({ hasText: "DNS 重新发现" })
      .waitFor();
    await page.locator("#dialog-cancel").click();
    await page.locator("#overlay").waitFor({ state: "hidden" });
    await page.locator("#back").click();
    await page.locator("#page-network").waitFor({ state: "visible" });
    await page.locator('[data-probe="selftest"]').click();
    await idle();
    await page
      .locator("#dialog-output")
      .filter({ hasText: "fwmark" })
      .waitFor();
    await page.locator("#dialog-cancel").click();
    await page.locator("#overlay").waitFor({ state: "hidden" });
    await page.locator("#diagnostic-log-link").click();
    await page
      .locator("#out")
      .filter({ hasText: "simulated diagnostics" })
      .waitFor();
    await page.locator("#btn-src-daemon").click();
    await page.locator("#out").filter({ hasText: "BackendState" }).waitFor();
    await page.locator("#btn-copy").click();
    await page
      .locator("#toast")
      .filter({ hasText: /已复制|长按/ })
      .waitFor();
    await page.locator("#btn-clear").click();
    await page.locator("#dialog-confirm").click();
    await idle();
    assert.equal(await page.locator("#out").innerText(), "(empty)");
    assert.ok(
      requests.every((url) => url.startsWith(base)),
      "external resource requested",
    );
    assert.deepEqual(errors, []);
    console.log(
      "PASS mock actions, five scenarios, seven pages, themes, diagnostics, dialogs/back, local resources",
    );

    // Independent fake native bridge: assert exact legacy commands and failed
    // writes/read failures. Demo fixtures are not used for this API contract.
    const native = await browser.newPage({
      viewport: { width: 390, height: 844 },
    });
    const nativeErrors = [];
    native.on("pageerror", (error) => nativeErrors.push(error.message));
    await native.addInitScript(() => {
      window.commands = [];
      window.polls = [];
      window.testVisible = true;
      Object.defineProperty(document, "visibilityState", {
        get: () => (window.testVisible ? "visible" : "hidden"),
      });
      const originalInterval = window.setInterval;
      window.setInterval = (callback, period) => {
        if (period === 5000 || period === 15000) {
          window.polls.push({ callback, period });
          return window.polls.length;
        }
        return originalInterval(callback, period);
      };
      window.failPref = false;
      window.failStatus = false;
      window.fakeStatus = {
        daemon: "1",
        backend: "Running",
        ip4: "100.64.1.2/32",
        user: "native@example.com",
        version: "v1.102.5-dnsfix.2",
        osrouter: "1",
        main_default: "ok",
        binary_ok: "1",
        daemon_current: "1",
        exempt_prerouting: "OK",
        exempt_output: "OK",
        exempt_nat: "OK",
        dns_network: "42",
        dns_transport: "WIFI",
        dns_active_vpn: "43",
        dns_underlying: "42",
        dns_iface: "wlan0",
        dns_servers: "192.168.1.1",
        dns_reachable: "true",
        dns_future_field: "<img src=x onerror=alert(1)>",
        dns_probe_mark: "0x10020000",
      };
      window.fakePrefs = {
        prefs_ok: "1",
        accept_routes: "1",
        accept_dns: "0",
        shields_up: "0",
        advertise_routes: "",
        hostname: "native-phone",
      };
      const kv = (obj) =>
        Object.keys(obj)
          .map((key) => key + "=" + obj[key])
          .join("\n");
      window.ksu = {
        toast() {},
        exec(command, options, callback) {
          window.commands.push(command);
          let stdout = "",
            errno = 0,
            stderr = "";
          if (command === "tailscaled.service webstatus") {
            stdout = kv(window.fakeStatus);
            if (window.failStatus) {
              errno = 1;
              stdout = "";
              stderr = "read failed";
            }
          } else if (command === "tailscaled.service prefs")
            stdout = kv(window.fakePrefs);
          else if (command.startsWith("tailscaled.service set-pref ")) {
            if (window.failPref) {
              errno = 1;
              stderr = "preference refused";
            } else {
              const parts = command.split(" ");
              if (parts[2] === "hostname") window.fakePrefs.hostname = parts[3];
              else if (parts[2] === "advertise-exit-node")
                window.fakePrefs.advertise_routes =
                  parts[3] === "on" ? "0.0.0.0/0" : "";
              else
                window.fakePrefs[parts[2].replace(/-/g, "_")] =
                  parts[3] === "on" ? "1" : "0";
            }
          } else if (command === "tailscaled.service logout") {
            window.fakeStatus.backend = "NeedsLogin";
            window.fakeStatus.user = "";
          } else if (command === "tailscale up --timeout=8s") {
            errno = 1;
            stderr = "https://login.tailscale.com/a/native-fixture";
          } else if (
            /^tailscaled.service (start|stop|restart)$/.test(command)
          ) {
            window.fakeStatus.daemon = command.endsWith(" stop") ? "0" : "1";
          } else if (command === "tailscaled.service routes")
            stdout = "=== table 52 ===\n100.64.0.0/10 dev tailscale0";
          else if (command === "tailscaled.service diag")
            stdout = "native diagnostics";
          else if (command === "tailscaled.service dns") {
            stdout = "native DNS marked probe\ncached dns_reachable=true";
            if (window.failDNS) {
              errno = 1;
              stderr = "android-dns: no reachable bootstrap DNS";
            }
          } else if (command === "tailscaled.service dns-refresh")
            stdout = "native discovery";
          else if (command === "tailscaled.service selftest")
            stdout = "native selftest";
          else if (command.startsWith("tail -n 250 "))
            stdout = "native daemon log";
          else if (command.startsWith(": > ")) stdout = "";
          else {
            errno = 99;
            stderr = "unexpected command";
          }
          setTimeout(
            () => window[callback](errno, stdout, stderr),
            command.startsWith("tail ") || command === "tailscaled.service diag"
              ? window.logDelay || 20
              : 20,
          );
        },
      };
    });
    await native.goto(base);
    await native
      .locator("#status-main")
      .filter({ hasText: "已连接" })
      .waitFor();
    await idle(native);
    assert.equal(await native.locator("#demo-bar").isVisible(), false);
    await native.locator('[data-nav="settings"]').click();
    await idle(native);
    await native.evaluate(() => {
      window.failPref = true;
    });
    await native.locator("#sw-accept-routes").click();
    await idle(native);
    assert.equal(
      await native.locator("#sw-accept-routes").isChecked(),
      true,
      "failed write must roll back",
    );
    await native.evaluate(() => {
      window.failPref = false;
    });
    for (const id of [
      "accept-routes",
      "accept-dns",
      "shields-up",
      "advertise-exit-node",
    ]) {
      await native.locator("#sw-" + id).click();
      await idle(native);
    }
    await native.locator("#btn-hostname").click();
    await native.locator("#in-hostname").fill("native-renamed");
    await native.locator("#dialog-confirm").click();
    await idle(native);
    await native.locator("#btn-logout").click();
    await native.locator("#dialog-confirm").click();
    await idle(native);
    await native.locator("#btn-login").click();
    await idle(native);
    assert.equal(
      await native.locator("#login-url").getAttribute("href"),
      "https://login.tailscale.com/a/native-fixture",
    );
    await native.locator("#stop-action").click();
    await native.locator("#dialog-confirm").click();
    await idle(native);
    await native.locator("#primary-action").click();
    await idle(native);
    await native.evaluate(() => {
      window.fakeStatus.backend = "Running";
    });
    await native.locator("#refresh").click();
    await idle(native);
    await native.locator("#primary-action").click();
    await idle(native);
    await native.locator('[data-nav="network"]').click();
    await native.locator('[data-nav="dns"]').click();
    await native.locator("#page-dns").waitFor({ state: "visible" });
    assert.match(
      await native.locator("#dns-detail-fields").innerText(),
      /dns_future_field/,
    );
    assert.equal(
      await native.locator("#dns-detail-fields img").count(),
      0,
      "service text must not become HTML",
    );
    await native.locator('#page-dns [data-probe="dns"]').click();
    await idle(native);
    await native.locator("#dialog-cancel").click();
    await native.locator("#overlay").waitFor({ state: "hidden" });
    await native.evaluate(() => {
      window.failDNS = true;
    });
    await native.locator('#page-dns [data-probe="dns"]').click();
    await idle(native);
    const failedProbe = await native.locator("#dialog-output").innerText();
    assert.match(failedProbe, /cached dns_reachable=true/);
    assert.match(failedProbe, /no reachable bootstrap DNS/);
    assert.match(failedProbe, /\[exit 1\]/);
    assert.match(
      await native.locator("#dialog-summary").innerText(),
      /本次检测命令失败/,
    );
    await native.locator("#dialog-cancel").click();
    await native.locator("#overlay").waitFor({ state: "hidden" });
    await native.locator("#btn-dns-refresh").click();
    await idle(native);
    await native.locator("#dialog-cancel").click();
    await native.locator("#overlay").waitFor({ state: "hidden" });
    await native.locator("#back").click();
    await native.locator('[data-probe="selftest"]').click();
    await idle(native);
    await native.locator("#dialog-cancel").click();
    await native.locator("#overlay").waitFor({ state: "hidden" });
    await native.locator('[data-nav="routing"]').click();
    await native
      .locator("#routes-output")
      .filter({ hasText: "table 52" })
      .waitFor();
    await native.locator("#back").click();
    await native.locator("#diagnostic-log-link").click();
    await native
      .locator("#out")
      .filter({ hasText: "native diagnostics" })
      .waitFor();
    await native.locator("#btn-src-daemon").click();
    await native.locator("#out").filter({ hasText: "native daemon" }).waitFor();
    // Slow reads + rapid source switches must coalesce instead of delaying
    // writes behind a long queue, and only the newest source may be rendered.
    await native.evaluate(() => {
      window.logDelay = 400;
      window.savedCommands = window.commands.slice();
      window.commands = [];
      document.getElementById("btn-log-refresh").click();
      for (let i = 0; i < 12; i++)
        document
          .getElementById(i % 2 ? "btn-src-diag" : "btn-src-daemon")
          .click();
    });
    await native
      .locator("#out")
      .filter({ hasText: "native diagnostics" })
      .waitFor();
    await native.waitForTimeout(200);
    const flooded = await native.evaluate(() =>
      window.commands.filter(
        (command) => command.startsWith("tail ") || command.endsWith(" diag"),
      ),
    );
    assert.equal(
      flooded.length,
      2,
      "coalesce to initial read plus newest source",
    );
    // Restore the command history used by the contract checks below.
    await native.evaluate(
      (previous) => {
        window.commands = previous.concat(window.commands);
        window.logDelay = 0;
      },
      await native.evaluate(() => window.savedCommands || []),
    );
    await native.locator("#btn-clear").click();
    await native.locator("#dialog-confirm").click();
    await idle(native);
    await native.locator("#back").click();
    await native.locator("#back").click();
    await native.locator("#page-home").waitFor({ state: "visible" });
    const beforePolling = await native.evaluate(() => window.commands.length);
    await native.evaluate(() => {
      document.getElementById("auto").checked = true;
      window.polls.find((poll) => poll.period === 5000).callback();
      window.testVisible = false;
      window.polls.forEach((poll) => poll.callback());
      window.testVisible = true;
    });
    await native.waitForTimeout(100);
    assert.equal(
      await native.evaluate(() => window.commands.length),
      beforePolling,
      "hidden/page-inactive polling must not execute root commands",
    );
    await native.evaluate(() => {
      window.failStatus = true;
    });
    await native.locator("#refresh").click();
    await idle(native);
    assert.equal(
      await native.locator("#status-main").innerText(),
      "已连接",
      "read failure must not pretend daemon stopped",
    );
    assert.match(await native.locator("#health").innerText(), /读取状态失败/);
    const commands = await native.evaluate(() => window.commands);
    for (const command of [
      "tailscaled.service start",
      "tailscaled.service stop",
      "tailscaled.service restart",
      "tailscaled.service logout",
      "tailscale up --timeout=8s",
      "tailscaled.service set-pref accept-routes off",
      "tailscaled.service set-pref accept-dns on",
      "tailscaled.service set-pref shields-up on",
      "tailscaled.service set-pref advertise-exit-node on",
      "tailscaled.service set-pref hostname native-renamed",
      "tailscaled.service webstatus",
      "tailscaled.service prefs",
      "tailscaled.service dns",
      "tailscaled.service dns-refresh",
      "tailscaled.service selftest",
      "tailscaled.service routes",
      "tailscaled.service diag",
      'tail -n 250 /data/adb/tailscale/run/tailscaled.log 2>/dev/null || echo "(no daemon log yet)"',
      ": > /data/adb/tailscale/run/tailscaled.log; : > /data/adb/tailscale/run/diag.log",
    ])
      assert.ok(
        commands.includes(command),
        "missing legacy command " + command,
      );
    assert.deepEqual(nativeErrors, []);
    // Forced demo must never call an available root bridge.
    await native.goto(`${base}/?demo=cellular`);
    await native
      .locator("#status-main")
      .filter({ hasText: "已连接" })
      .waitFor();
    await idle(native);
    await native.locator("#primary-action").click();
    await idle(native);
    assert.deepEqual(await native.evaluate(() => window.commands), []);
    console.log(
      "PASS native bridge command contract, failure rollback, cached state, unknown DNS fields, HTML safety, demo isolation",
    );
    console.log(JSON.stringify(counts));
    const images = fs
      .readdirSync(shots)
      .filter((name) => name.endsWith(".png") && !name.startsWith("overview-"));
    fs.writeFileSync(
      path.join(shots, "index.html"),
      '<meta charset="utf-8"><title>Miuix WebUI screenshots</title><style>body{font:16px system-ui;background:#eee;margin:24px}main{display:flex;gap:16px;flex-wrap:wrap}figure{margin:0;width:260px}img{width:260px}a{color:#2768dc}</style><h1>Miuix WebUI mock acceptance</h1><p>Local browser screenshots. Actual root-manager acceptance pending.</p><main>' +
        images
          .map(
            (name) =>
              '<figure><a href="' +
              name +
              '"><img src="' +
              name +
              '" loading="lazy"></a><p>' +
              name.replace(".png", "") +
              "</p></figure>",
          )
          .join("") +
        "</main>",
    );
  } finally {
    await browser.close();
    server.close();
  }
})().catch((error) => {
  console.error(error);
  server.close();
  process.exitCode = 1;
});
