// Presentation only; unknown, false and absent data remain distinct.
const object = (value) => value && typeof value === "object" && !Array.isArray(value) ? value : {};
const string = (value) => typeof value === "string" ? value : "";
const list = (value) => Array.isArray(value) ? value.filter((v) => v && typeof v === "object") : [];
const strings = (value) => Array.isArray(value) ? value.filter((v) => typeof v === "string").join(", ") : "";
const boolean = (value) => value === true ? "是" : value === false ? "否" : "未知";
const region = (value) => { const r = object(value); return [string(r.code), string(r.name)].filter(Boolean).join(" · ") || (r.id ? `region ${r.id}` : "未提供"); };
const paths = { direct: "Direct", derp: "DERP", "peer-relay": "Peer relay", offline: "离线", idle: "空闲 · 当前路径未确认", unknown: "当前路径未知" };
const result = (value) => { const r = object(value); return [r.timeout === true ? "超时" : "", string(r.error), string(r.stdout), string(r.stderr), r.truncated === true ? "输出已截断" : ""].filter(Boolean).join("\n") || "未提供"; };

export function diagnosticRows(input) {
  const report = object(input), self = object(report.self), n = object(report.netcheck), raw = object(report.raw), network = object(report.network);
  return {
    self: [
      ["设备名称", "Self.HostName", string(self.hostname)],
      ["Reported OS", "Self.OS", string(self.os)],
      ["Tailnet IPv4", "Self.TailscaleIPs", strings(self.ipv4)],
      ["Tailnet IPv6", "Self.TailscaleIPs", strings(self.ipv6) || "未提供"],
      ["Home DERP", "Self.Relay · 不代表当前通信路径", region(self.relay)],
    ],
    endpoints: list(report.endpoints).map((e) => [string(e.address) || "无效 endpoint", `${string(e.family)} · ${string(e.scope)}`, [string(e.interface) || "接口未知", string(e.interface_source)].filter(Boolean).join(" · ")]),
    peers: list(report.peers).map((p) => [string(p.hostname) || "未知节点", [...[strings(p.ipv4), strings(p.ipv6)].filter(Boolean), `home DERP: ${region(p.relay)}`].join(" · "), [p.online === true ? "控制平面在线" : p.online === false ? "控制平面离线" : "控制平面状态未知", paths[string(p.path)] || paths.unknown, string(p.current_endpoint), string(p.peer_relay)].filter(Boolean).join(" · ")]),
    netcheck: [
      ["UDP", "STUN 往返 · 不等于 daemon 端口可入站", boolean(n.udp)],
      ["IPv4", "netcheck.IPv4", boolean(n.ipv4)],
      ["IPv6", "netcheck.IPv6", boolean(n.ipv6)],
      ["MappingVariesByDestIP", "IPv4 NAT 映射是否随目的地址变化", boolean(n.mapping_varies_by_dest_ip)],
      ["PortMapping", "UPnP / NAT-PMP / PCP", ["UPnP", "PMP", "PCP"].map((key) => `${key}: ${boolean(object(n.port_mapping)[key])}`).join(" · ")],
      ["Nearest DERP", "netcheck.PreferredDERP", region(n.nearest_derp)],
      ["公网 IPv4 探测结果", "临时 netcheck socket", string(n.global_ipv4)],
      ["公网 IPv6 探测结果", "临时 netcheck socket", string(n.global_ipv6)],
      ["Captive portal", "辅助线索，不能单独确定根因", boolean(n.captive_portal)],
    ],
    udp: list(report.udp_listeners).map((e) => [`${string(e.family)} UDP ${e.port || "未知端口"}`, "daemon 本地 listener · 实际端口可能不是 41641", string(e.address)]),
    outer: [
      ["采集时物理接口", "Android network discovery", string(network.dns_iface)],
      ["选中的 Android 网络", "network / transport", [string(network.dns_network), string(network.dns_transport)].filter(Boolean).join(" · ")],
      ["VPN / underlying", "VPN → physical network", [string(network.dns_active_vpn), string(network.dns_underlying)].filter(Boolean).join(" → ")],
      ["选网依据", "新采集或缓存会明确标记", [string(network.source), string(network.dns_selection_reason)].filter(Boolean).join(" · ")],
      ["Outer fwmark", "仅用于只读 route get", string(report.outer_mark)],
      ["IPv4 outer route", "ip -4 route get · mark", result(raw.outer_ipv4)],
      ["IPv6 outer route", "ip -6 route get · mark", result(raw.outer_ipv6)],
    ].concat(Object.keys(raw).filter((key) => key.indexOf("peer_outer_") === 0).map((key) => [key.slice(11), "对端当前 endpoint 的 marked route", result(raw[key])])),
  };
}
