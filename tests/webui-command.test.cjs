// Execute the WebUI's actual native command adapter in a shell with no module
// names on PATH, instead of accepting every command in a fake native bridge.
const fs = require("node:fs");
const path = require("node:path");
const { spawnSync } = require("node:child_process");
const ROOT = path.resolve(__dirname, "..");
(async () => {
  const adapter = path.join(ROOT, "webroot/commands.js");
  // Before the fix app.js passed these commands unchanged to exec(). This
  // fallback lets the regression fail on the original lookup error itself.
  let nativeCommand = (command) => command;
  if (fs.existsSync(adapter)) {
    const module = await import(
      "data:text/javascript;base64," +
        fs.readFileSync(adapter).toString("base64")
    );
    nativeCommand = module.nativeCommand;
  }
  const commands = [
    "webstatus",
    "prefs",
    "start",
    "stop",
    "restart",
    "logout",
    "set-pref accept-routes on",
    "set-pref accept-dns off",
    "set-pref shields-up on",
    "set-pref advertise-exit-node off",
    "set-pref hostname regression-phone",
    "dns",
    "dns-refresh",
    "routes",
    "diag",
    "selftest",
    "netdiag",
  ].map((action) => "tailscaled.service " + action);
  commands.push("tailscale up --timeout=8s");
  commands.push("tailscale status --json");
  commands.push("tailscale ping --timeout=3s --c=3 --until-direct=false 100.72.239.86");
  commands.push("tailscale ping --timeout=3s --c=3 --until-direct=false fd7a:115c:a1e0::1");
  const input = JSON.stringify({
    root: ROOT.replace(/\\/g, "/"),
    commands: commands.map((logical) => ({
      logical,
      native: nativeCommand(logical),
    })),
  });
  const windows = process.platform === "win32";
  const fixturePath = path.join(ROOT, "tests/webui_shell_fixture.py");
  const wslFixturePath = fixturePath
    .replace(/\\/g, "/")
    .replace(
      /^([A-Za-z]):\//,
      (_, drive) => "/mnt/" + drive.toLowerCase() + "/",
    );
  const runner = windows ? "wsl" : "python3";
  const args = windows
    ? [
        "-d",
        process.env.WEBUI_WSL_DISTRO || "Ubuntu-24.04",
        "--",
        "python3",
        wslFixturePath,
      ]
    : [fixturePath];
  const result = spawnSync(runner, args, {
    input,
    encoding: "utf8",
    timeout: 60000,
  });
  process.stdout.write(result.stdout || "");
  process.stderr.write(result.stderr || "");
  if (result.error) throw result.error;
  if (result.status !== 0) process.exitCode = result.status || 1;
})().catch((error) => {
  console.error(error);
  process.exitCode = 1;
});
