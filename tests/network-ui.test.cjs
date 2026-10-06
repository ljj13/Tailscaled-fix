const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");
(async () => {
  const file = path.join(__dirname, "../webroot/network.js");
  assert.ok(fs.existsSync(file), "missing defensive network diagnostic presentation");
  const { diagnosticRows } = await import("data:text/javascript;base64," + fs.readFileSync(file).toString("base64"));
  for (const input of [null, {}, { self: null, peers: null, endpoints: null, netcheck: null }, { peers: [null, {}], endpoints: [null, {}] }]) {
    const groups = diagnosticRows(input);
    assert.ok(Array.isArray(groups.self) && Array.isArray(groups.peers));
    assert.ok(groups.netcheck.some((row) => row[0] === "IPv6" && row[2] === "未知"));
  }
  const groups = diagnosticRows({
    self: { hostname: "phone", os: "linux", ipv4: ["100.1.2.3"], relay: { code: "hkg", name: "Hong Kong" } },
    peers: [{ hostname: "offline", online: false, path: "offline", relay: { code: "hkg" } }, { hostname: "idle", online: true, path: "idle" }],
    netcheck: { udp: false, ipv6: false, mapping_varies_by_dest_ip: null, port_mapping: { PMP: false } },
    raw: { outer_ipv6: { error: "deadline", timeout: true } },
  });
  assert.ok(groups.self.some((r) => r[0] === "Home DERP" && r[2].includes("Hong Kong")));
  assert.ok(groups.peers.some((r) => r[0] === "offline" && r[2].includes("离线")));
  assert.ok(groups.peers.some((r) => r[0] === "idle" && r[2].includes("空闲")));
  assert.ok(groups.netcheck.some((r) => r[0] === "UDP" && r[2] === "否"));
  assert.ok(groups.outer.some((r) => r[0].includes("IPv6") && r[2].includes("超时")));
  console.log("PASS network UI missing/null fields, offline/idle peers, no IPv6/DERP and timeout presentation");
})().catch((error) => { console.error(error); process.exitCode = 1; });
