import { exec, toast as nativeToast, bridgeAvailable } from "./ksu.js";
import { createDemo, scenarios } from "./demo.js";

const SVC = "tailscaled.service";
const DAEMON_LOG = "/data/adb/tailscale/run/tailscaled.log";
const DIAG_LOG = "/data/adb/tailscale/run/diag.log";
const $ = (id) => document.getElementById(id);
const requestedDemo = new URLSearchParams(location.search).get("demo");
const demo =
  requestedDemo !== null || !bridgeAvailable()
    ? createDemo(requestedDemo || "wifi")
    : null;
const state = {
  status: {},
  prefs: {},
  prefsReady: false,
  busy: false,
  page: "home",
  source: "daemon",
  statusRequest: null,
  prefsRequest: null,
  logRequest: null,
  routeRequest: null,
  depth: 0,
  scroll: {},
  modal: null,
  epoch: 0,
  readError: "",
};
const PAGES = {
  home: ["Tailscale", "让设备之间的连接更简单"],
  settings: ["设置", "让连接适合你的使用方式"],
  network: ["网络与诊断", "查看真实底层网络与连接详情"],
  dns: ["DNS", "解析来源、网络选择与实际可达性"],
  routing: ["路由与代理共存", "main、table 52 与 Clash / Mihomo exemptions"],
  logs: ["日志", "保留完整输出，方便定位问题"],
  about: ["关于", "版本、构建与支持能力"],
};
const SWITCHES = {
  "sw-accept-routes": "accept-routes",
  "sw-accept-dns": "accept-dns",
  "sw-shields-up": "shields-up",
  "sw-advertise-exit-node": "advertise-exit-node",
};
// Names are presentation only. Every dns_* key from the service is retained.
const DNS_NAMES = {
  dns_source: "DNS 来源",
  dns_servers: "当前 resolver",
  dns_reachable: "实际可达性",
  dns_network: "选中的 Android 网络",
  dns_transport: "网络类型",
  dns_active_vpn: "当前 Android VPN",
  dns_underlying: "VPN 底层网络",
  dns_iface: "物理接口",
  dns_excluded: "排除的网络 / 接口",
  dns_selection_reason: "网络选择原因",
  dns_route_hint: "普通默认路由提示",
  dns_physical_route: "物理网络路由",
  dns_probe_mark: "DNS probe fwmark",
  dns_checked: "最近检测",
  dns_retained: "保留已验证 DNS",
  dns_last_verified: "最近验证成功",
  dns_bootstrap_file: "Bootstrap resolver 文件",
  dns_generation: "网络配置 generation",
  dns_start_pending: "等待 DNS 启动",
};
const sleep = (ms) => new Promise((resolve) => setTimeout(resolve, ms));
let commandQueue = Promise.resolve();
function toast(message) {
  if (!demo && nativeToast(message)) return;
  $("toast").textContent = message;
  $("toast").classList.add("show");
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => $("toast").classList.remove("show"), 3500);
}
async function run(command, quiet = false) {
  // Serialize bridge requests, including probes and background reads. No probe
  // overlaps a write or runs with different routing flags from manual actions.
  const task = commandQueue.then(() =>
    demo ? demo.exec(command) : exec(command),
  );
  commandQueue = task.catch(() => {});
  let result;
  try {
    result = await task;
  } catch (error) {
    result = { errno: -1, stdout: "", stderr: String(error) };
  }
  if (result.errno !== 0 && !quiet) toast(errorText(result));
  return result;
}
function errorText(result) {
  return (
    result.stderr ||
    result.stdout ||
    (result.errno === 0 ? "未取得有效输出" : `命令失败 (${result.errno})`)
  )
    .trim()
    .split("\n")
    .pop()
    .slice(0, 180);
}
function rawOutput(result) {
  const output = [result.stdout, result.stderr].filter(Boolean).join("\n");
  return result.errno !== 0
    ? `${output}\n[exit ${result.errno}]`.trim()
    : output || "(empty)";
}
function parseKv(text) {
  const output = Object.create(null);
  String(text)
    .split("\n")
    .forEach((line) => {
      const at = line.indexOf("=");
      if (at > 0) output[line.slice(0, at).trim()] = line.slice(at + 1).trim();
    });
  return output;
}
function text(id, value) {
  $(id).textContent = value || "—";
}
function updateDisabled() {
  document.querySelectorAll(".root-action").forEach((el) => {
    el.disabled = state.busy;
  });
  Object.keys(SWITCHES).forEach((id) => {
    $(id).disabled =
      state.busy || !state.prefsReady || state.status.daemon !== "1";
  });
  $("demo-picker").disabled = state.busy;
  $("busy-label").hidden = !state.busy;
  $("content").setAttribute("aria-busy", String(state.busy));
}
async function operation(fn) {
  if (state.busy) return;
  state.busy = true;
  updateDisabled();
  try {
    await fn();
  } catch (error) {
    toast(String(error).slice(0, 180));
  } finally {
    state.busy = false;
    updateDisabled();
  }
}
function rows(id, entries) {
  const fragment = document.createDocumentFragment();
  entries.forEach(([title, key, value, kind]) => {
    const row = document.createElement("div");
    row.className = "preference data-row";
    const copy = document.createElement("div");
    copy.className = "preference-copy";
    const label = document.createElement("span");
    label.className = "preference-title";
    label.textContent = title;
    const summary = document.createElement("span");
    summary.className = "summary";
    summary.textContent = key;
    copy.append(label, summary);
    const trailing = document.createElement("span");
    trailing.className = `trailing ${kind || ""}`;
    trailing.textContent = value || "—";
    row.append(copy, trailing);
    fragment.appendChild(row);
  });
  $(id).textContent = "";
  $(id).appendChild(fragment);
}
function connectionType(s) {
  if (/WIFI/i.test(s.dns_transport || "")) return "Wi-Fi";
  if (/CELLULAR/i.test(s.dns_transport || "")) return "移动数据";
  if (/ETHERNET/i.test(s.dns_transport || "")) return "以太网";
  return s.dns_iface || "底层网络待确认";
}
function renderStatus() {
  const s = state.status;
  const known = Object.prototype.hasOwnProperty.call(s, "daemon");
  const running = s.daemon === "1";
  const connected = running && s.backend === "Running";
  const needsLogin = running && s.backend === "NeedsLogin";
  const mainBad = running && s.osrouter === "1" && s.main_default !== "ok";
  const exemptions = ["exempt_prerouting", "exempt_output", "exempt_nat"];
  const exOK = exemptions.every((key) => s[key] === "OK");
  const dnsOK = s.dns_reachable === "true";
  text(
    "status-main",
    !known
      ? "暂时无法读取"
      : connected
        ? "已连接"
        : needsLogin
          ? "等待登录"
          : running
            ? "正在连接"
            : s.dns_start_pending === "1"
              ? "等待网络"
              : "服务已停止",
  );
  text(
    "status-sub",
    !known
      ? "请刷新状态，检查模块与 WebUI bridge"
      : connected
        ? `${connectionType(s)}${s.dns_active_vpn ? " · 与 Android VPN 共存" : ""}`
        : needsLogin
          ? "授权后即可加入你的 Tailnet"
          : running
            ? `BackendState: ${s.backend || "读取中"}`
            : "启动服务，恢复设备之间的连接",
  );
  $("primary-action").dataset.action = !running
    ? "start"
    : needsLogin
      ? "login"
      : "restart";
  text(
    "primary-action",
    !running ? "启动服务" : needsLogin ? "登录 Tailnet" : "重启服务",
  );
  $("stop-action").hidden = !running;
  text("home-hostname", state.prefs.hostname);
  text("home-ip", s.ip4);
  text("home-user", s.user || (needsLogin ? "尚未登录" : "—"));
  text("home-version", s.version || "版本与构建信息");
  text(
    "home-network-summary",
    running
      ? `${connectionType(s)} · ${dnsOK && !mainBad && exOK ? "连接正常" : "查看连接详情"}`
      : "查看 DNS、路由与代理共存状态",
  );
  text("settings-account", s.user || "加入你的 Tailnet");
  const problems = [];
  if (state.readError) problems.push(state.readError);
  if (running && !dnsOK) problems.push("DNS 尚未验证成功，请查看网络与诊断。");
  if (mainBad) problems.push("main 默认路由缺失，控制连接可能受影响。");
  if (running && !exOK) problems.push("代理 exemption 需要检查。");
  if (running && s.daemon_current === "0")
    problems.push("服务仍在运行旧版本，请重启服务。");
  if (s.binary_ok === "0") problems.push("二进制与模块构建不一致。");
  if (running && s.health) problems.push(s.health);
  text("health", problems.join(" "));
  $("health").hidden = problems.length === 0;
  text(
    "network-dns",
    `${s.dns_source || "来源待确认"} · ${s.dns_servers || "尚未发现 resolver"}`,
  );
  text("network-dns-state", !running ? "未运行" : dnsOK ? "正常" : "待检查");
  $("network-dns-state").classList.toggle("warning", running && !dnsOK);
  text(
    "network-route-state",
    !running ? "未运行" : !mainBad && exOK ? "正常" : "待检查",
  );
  $("network-route-state").classList.toggle(
    "warning",
    running && (mainBad || !exOK),
  );
  rows(
    "network-fields",
    [
      "dns_network",
      "dns_transport",
      "dns_iface",
      "dns_active_vpn",
      "dns_underlying",
      "dns_selection_reason",
    ].map((key) => [DNS_NAMES[key], key, s[key]]),
  );
  const summaryKeys = ["dns_source", "dns_servers", "dns_reachable"];
  rows(
    "dns-summary-fields",
    summaryKeys.map((key) => [
      DNS_NAMES[key],
      key,
      key === "dns_reachable"
        ? s[key] === "true"
          ? "正常 · true"
          : s[key] === "false"
            ? "未验证成功 · false"
            : ""
        : s[key],
      key === "dns_reachable" && !dnsOK ? "warning" : "",
    ]),
  );
  const detailKeys = Object.keys(DNS_NAMES).filter(
    (key) => summaryKeys.indexOf(key) < 0,
  );
  Object.keys(s)
    .filter(
      (key) =>
        key.indexOf("dns_") === 0 &&
        detailKeys.indexOf(key) < 0 &&
        summaryKeys.indexOf(key) < 0,
    )
    .forEach((key) => detailKeys.push(key));
  rows(
    "dns-detail-fields",
    detailKeys.map((key) => [DNS_NAMES[key] || key, key, s[key]]),
  );
  rows("routing-fields", [
    [
      "main 默认路由",
      "main_default",
      s.main_default === "ok" ? "正常 · ok" : s.main_default,
      mainBad ? "warning" : "",
    ],
    ["Tailscale 接口", "iface", s.iface],
    ["Tailnet 地址", "ip4", s.ip4],
    ["已发布 / 检测到的路由", "routes", s.routes],
    ["自动路由条目", "routes_auto", s.routes_auto],
    [
      "Prerouting exemption",
      "exempt_prerouting",
      s.exempt_prerouting === "OK" ? "正常 · OK" : s.exempt_prerouting,
      running && s.exempt_prerouting !== "OK" ? "warning" : "",
    ],
    [
      "Output exemption",
      "exempt_output",
      s.exempt_output === "OK" ? "正常 · OK" : s.exempt_output,
      running && s.exempt_output !== "OK" ? "warning" : "",
    ],
    [
      "NAT exemption",
      "exempt_nat",
      s.exempt_nat === "OK" ? "正常 · OK" : s.exempt_nat,
      running && s.exempt_nat !== "OK" ? "warning" : "",
    ],
    [
      "Watchdog",
      "watchdog / watchdog_pid",
      `${s.watchdog === "1" ? "运行中" : "未运行"}${s.watchdog_pid ? " · PID " + s.watchdog_pid : ""}`,
    ],
  ]);
  text("about-version", s.version);
  rows("about-fields", [
    [
      "运行模式",
      "osrouter",
      s.osrouter === "1"
        ? "Linux · osrouter"
        : s.osrouter === "0"
          ? "Standalone"
          : "未取得",
    ],
    ["BackendState", "backend", s.backend],
    [
      "Daemon",
      "daemon / pid",
      running ? `运行中 · PID ${s.pid || "?"}` : "未运行",
    ],
    [
      "服务版本一致性",
      "daemon_current",
      !running ? "未运行" : s.daemon_current === "1" ? "当前构建" : "需重启",
    ],
    [
      "二进制完整性",
      "binary_ok",
      s.binary_ok === "1" ? "与模块构建一致" : "待检查",
      s.binary_ok === "0" ? "warning" : "",
    ],
    [
      "主题",
      "prefers-color-scheme",
      matchMedia("(prefers-color-scheme: dark)").matches
        ? "深色 · 跟随系统"
        : "浅色 · 跟随系统",
    ],
  ]);
  updateDisabled();
}
async function refreshStatus(announce = false) {
  if (state.statusRequest) return state.statusRequest;
  const epoch = state.epoch;
  state.statusRequest = (async () => {
    const result = await run(`${SVC} webstatus`, true);
    if (epoch !== state.epoch) return;
    const parsed = parseKv(result.stdout);
    if (
      result.errno === 0 &&
      (parsed.daemon === "1" || parsed.daemon === "0")
    ) {
      state.status = parsed;
      state.readError = "";
    } else
      state.readError =
        "读取状态失败，当前显示上次取得的数据。" + errorText(result);
    renderStatus();
    if (announce) toast(state.readError || "状态已刷新");
  })();
  try {
    await state.statusRequest;
  } finally {
    state.statusRequest = null;
  }
}
async function loadPrefs() {
  if (state.prefsRequest) return state.prefsRequest;
  const epoch = state.epoch;
  state.prefsRequest = (async () => {
    const result = await run(`${SVC} prefs`, true);
    if (epoch !== state.epoch) return;
    const prefs = parseKv(result.stdout);
    if (result.errno !== 0 || prefs.prefs_ok !== "1") {
      state.prefsReady = false;
      text("prefs-note", "暂时无法读取偏好设置。请启动并登录服务后刷新。");
    } else {
      state.prefs = prefs;
      state.prefsReady = true;
      Object.keys(SWITCHES).forEach((id) => {
        const key = SWITCHES[id];
        $(id).checked =
          key === "advertise-exit-node"
            ? (prefs.advertise_routes || "").indexOf("0.0.0.0/0") >= 0
            : prefs[key.replace(/-/g, "_")] === "1";
      });
      text("settings-hostname", prefs.hostname);
      text("prefs-note", "设置即时保存；不会重置 Tailscale state 或节点身份。");
    }
    renderStatus();
  })();
  try {
    await state.prefsRequest;
  } finally {
    state.prefsRequest = null;
  }
}
async function refreshPage(announce = false) {
  await refreshStatus(announce);
  if (state.page === "settings" || state.page === "home") await loadPrefs();
  if (state.page === "logs") await loadOutput();
  if (state.page === "routing") await loadRoutes();
}

// Hash/history navigation with a modal entry, so Back dismisses overlays first.
function showPage(name, focus = true) {
  if (!Object.prototype.hasOwnProperty.call(PAGES, name)) name = "home";
  state.scroll[state.page] = window.scrollY;
  state.page = name;
  document.querySelectorAll("[data-page]").forEach((el) => {
    el.hidden = el.dataset.page !== name;
  });
  text("page-title", PAGES[name][0]);
  text("page-subtitle", PAGES[name][1]);
  text("app-caption", name === "home" ? "你的私人网络" : "Tailscale");
  $("back").hidden = name === "home";
  document.title = `${PAGES[name][0]} · Tailscale`;
  if (focus) $("page-title").focus({ preventScroll: true });
  window.scrollTo(0, state.scroll[name] || 0);
  if (name === "settings") loadPrefs();
  if (name === "logs") loadOutput();
  if (name === "routing") loadRoutes();
}
function navigate(name) {
  if (state.modal || name === state.page) return;
  state.depth += 1;
  history.pushState({ page: name, depth: state.depth }, "", "#" + name);
  showPage(name);
}
function back() {
  if (state.modal) {
    closeModal();
    return;
  }
  if (state.depth > 0) history.back();
  else {
    const parent =
      state.page === "dns" || state.page === "routing" ? "network" : "home";
    history.replaceState({ page: parent, depth: 0 }, "", "#" + parent);
    showPage(parent);
  }
}
function finishModal(value) {
  const modal = state.modal;
  if (!modal) return;
  state.modal = null;
  $("overlay").hidden = true;
  document.body.classList.remove("modal-open");
  $("app-caption").closest(".app-shell").removeAttribute("aria-hidden");
  if (modal.focus && document.contains(modal.focus))
    modal.focus.focus({ preventScroll: true });
  modal.resolve(value);
}
function closeModal(value = null) {
  if (!state.modal || state.modal.closing) return;
  // Resolve only once history has removed the modal entry. A following action
  // can then safely navigate without racing the asynchronous popstate event.
  state.modal.closing = true;
  state.modal.value = value;
  history.back();
}
function openModal(options) {
  if (state.modal) return Promise.resolve(null);
  const focus = document.activeElement;
  $("toast").classList.remove("show");
  $("dialog-title").textContent = options.title;
  $("dialog-summary").textContent = options.summary || "";
  $("dialog-summary").classList.toggle("warning", Boolean(options.failure));
  $("dialog-input-group").hidden = !options.input;
  $("dialog-output").hidden = !options.output;
  $("dialog-output").textContent = options.output || "";
  $("dialog-choices").hidden = !options.choices;
  $("dialog-choices").textContent = "";
  $("input-error").hidden = true;
  $("in-hostname").value = options.input || "";
  $("dialog-cancel").textContent = options.output ? "关闭" : "取消";
  $("dialog-confirm").textContent = options.output
    ? "复制"
    : options.confirm || "确认";
  $("dialog-confirm").hidden = Boolean(options.choices);
  $("dialog-confirm").classList.toggle("danger", Boolean(options.danger));
  $("overlay").classList.toggle("sheet", Boolean(options.output));
  $("sheet-handle").hidden = !options.output;
  if (options.choices)
    options.choices.forEach(([key, label]) => {
      const button = document.createElement("button");
      button.className = "preference";
      button.textContent = label;
      button.onclick = () => closeModal(key);
      $("dialog-choices").appendChild(button);
    });
  $("overlay").hidden = false;
  document.body.classList.add("modal-open");
  document.querySelector(".app-shell").setAttribute("aria-hidden", "true");
  history.pushState(
    { page: state.page, depth: state.depth, modal: true },
    "",
    location.hash,
  );
  const promise = new Promise((resolve) => {
    state.modal = { resolve, focus, options, closing: false, value: null };
  });
  (options.input ? $("in-hostname") : $("dialog")).focus({
    preventScroll: true,
  });
  return promise;
}
async function confirm(title, summary) {
  return (
    (await openModal({ title, summary, confirm: "确认", danger: true })) ===
    true
  );
}
async function copyOutput(value) {
  if (!value) {
    toast("暂无可复制内容");
    return;
  }
  try {
    if (navigator.clipboard && window.isSecureContext)
      await navigator.clipboard.writeText(value);
    else {
      const area = document.createElement("textarea");
      area.value = value;
      area.style.position = "fixed";
      area.style.opacity = "0";
      (state.modal ? $("dialog") : document.body).appendChild(area);
      area.select();
      const ok = document.execCommand("copy");
      area.remove();
      if (!ok) throw new Error("copy unavailable");
    }
    toast("已复制");
  } catch (_) {
    toast("复制不可用，可长按选择输出文本");
  }
}

async function serviceAction(name) {
  if (name === "login") {
    await login();
    return;
  }
  if (
    name === "stop" &&
    !(await confirm(
      "停止服务？",
      "连接将暂时断开。配置、登录身份与节点 state 会保留。",
    ))
  )
    return;
  if (!["start", "stop", "restart"].includes(name)) return;
  await operation(async () => {
    const result = await run(`${SVC} ${name}`);
    if (result.errno === 0) {
      await sleep(demo ? 80 : 1500);
      toast("服务操作已完成");
    }
    await refreshStatus();
    await loadPrefs();
  });
}
async function login() {
  await operation(async () => {
    const result = await run("tailscale up --timeout=8s", true);
    const match = `${result.stdout}\n${result.stderr}`.match(
      /https:\/\/login\.tailscale\.com\/[A-Za-z0-9/?=&_.-]+/,
    );
    if (match) {
      $("login-url").href = match[0];
      $("login-url").textContent = demo
        ? "演示授权链接（无真实节点）"
        : match[0];
      $("login-box").hidden = false;
      if (state.page !== "home") navigate("home");
      toast("授权链接已准备好，请点击打开");
    } else if (/already logged in|Logged in/i.test(result.stdout || ""))
      toast("此设备已登录");
    else toast(errorText(result) || "未返回授权链接");
    await refreshStatus();
    await loadPrefs();
  });
}
async function logout() {
  if (
    !(await confirm(
      "退出 Tailnet？",
      "此设备将离开当前 Tailnet，下次连接需要重新授权。",
    ))
  )
    return;
  await operation(async () => {
    const result = await run(`${SVC} logout`);
    if (result.errno === 0) {
      $("login-box").hidden = true;
      toast("已退出登录");
    }
    await refreshStatus();
    await loadPrefs();
  });
}
async function togglePref(id) {
  const want = $(id).checked;
  await operation(async () => {
    const result = await run(
      `${SVC} set-pref ${SWITCHES[id]} ${want ? "on" : "off"}`,
    );
    if (result.errno !== 0) $(id).checked = !want;
    else {
      toast("设置已保存");
      await loadPrefs();
      await refreshStatus();
    }
  });
}
async function setHostname() {
  const value = await openModal({
    title: "设备名称",
    summary: "使用字母、数字、点或短横线。仅更新名称，不改变节点身份。",
    input: state.prefs.hostname || "tailscale",
    confirm: "保存",
  });
  if (value === null) return;
  await operation(async () => {
    const result = await run(`${SVC} set-pref hostname ${value}`);
    if (result.errno === 0) toast("设备名称已保存");
    await loadPrefs();
  });
}
async function probe(name) {
  if (!["dns", "selftest", "dns-refresh"].includes(name)) return;
  let output, failure;
  await operation(async () => {
    const result = await run(`${SVC} ${name}`, true);
    output = rawOutput(result);
    failure = result.errno !== 0;
    await refreshStatus();
  });
  if (output)
    await openModal({
      title:
        name === "selftest"
          ? "完整自检"
          : name === "dns-refresh"
            ? "DNS 重新发现"
            : "DNS 检测",
      summary: failure
        ? "本次检测命令失败。输出中的缓存状态不代表本次探测成功；请查看错误与退出码。"
        : "以下为服务的完整原始输出。",
      output,
      failure,
    });
}
async function loadRoutes() {
  if (state.routeRequest) return state.routeRequest;
  const epoch = state.epoch;
  state.routeRequest = (async () => {
    const result = await run(`${SVC} routes`, true);
    if (epoch === state.epoch)
      text(
        "routes-output",
        result.stdout || result.stderr ? rawOutput(result) : "(empty)",
      );
  })();
  try {
    await state.routeRequest;
  } finally {
    state.routeRequest = null;
  }
}
async function loadOutput() {
  // Coalesce repeated refresh/source clicks into one worker. A slow diagnostics
  // command must not fill the root queue with obsolete reads ahead of actions.
  if (state.logRequest) return state.logRequest;
  const task = (async () => {
    while (true) {
      const source = state.source,
        epoch = state.epoch;
      const result = await run(
        source === "daemon"
          ? `tail -n 250 ${DAEMON_LOG} 2>/dev/null || echo "(no daemon log yet)"`
          : `${SVC} diag`,
        true,
      );
      if (source === state.source && epoch === state.epoch) {
        const out = $("out"),
          stick = out.scrollTop + out.clientHeight >= out.scrollHeight - 40;
        text(
          "out",
          result.stdout || result.stderr ? rawOutput(result) : "(empty)",
        );
        if (stick) out.scrollTop = out.scrollHeight;
        break;
      }
      if (state.page !== "logs") break;
    }
  })();
  state.logRequest = task;
  try {
    await task;
  } finally {
    if (state.logRequest === task) state.logRequest = null;
  }
}
function setSource(source, read = true) {
  state.source = source;
  $("btn-src-daemon").setAttribute("aria-pressed", String(source === "daemon"));
  $("btn-src-diag").setAttribute("aria-pressed", String(source === "diag"));
  text("src-label", source === "daemon" ? "Daemon 日志" : "Diagnostics");
  if (read && state.page === "logs") loadOutput();
}
async function clearLogs() {
  if (
    !(await confirm(
      "清空全部日志？",
      "Daemon 与 Diagnostics 的日志内容会被清空。此操作不会清除配置或节点身份。",
    ))
  )
    return;
  await operation(async () => {
    const result = await run(`: > ${DAEMON_LOG}; : > ${DIAG_LOG}`);
    if (result.errno === 0) {
      await loadOutput();
      toast("日志已清空");
    }
  });
}
async function selectDemo() {
  const selected = await openModal({
    title: "演示场景",
    summary: "数据与操作仅存在于当前页面，不会调用 root bridge。",
    choices: Object.entries(scenarios),
  });
  if (!selected) return;
  // Drain old reads before changing fixtures; a late result must not overwrite
  // the newly selected network or preference values.
  await operation(async () => {
    await commandQueue;
    state.epoch += 1;
    demo.select(selected);
    state.status = {};
    state.prefs = {};
    state.prefsReady = false;
    $("login-box").hidden = true;
    text("demo-name", scenarios[demo.name]);
    const url = new URL(location.href);
    url.searchParams.set("demo", selected);
    history.replaceState({ page: state.page, depth: state.depth }, "", url);
    await refreshPage();
    await loadPrefs();
  });
}

function wire() {
  $("back").onclick = back;
  $("refresh").onclick = () => operation(() => refreshPage(true));
  document.querySelectorAll("[data-nav]").forEach((el) => {
    el.onclick = () => {
      if (el.id === "diagnostic-log-link") setSource("diag", false);
      navigate(el.dataset.nav);
    };
  });
  document.querySelectorAll("[data-action]").forEach((el) => {
    el.onclick = () => serviceAction(el.dataset.action);
  });
  document.querySelectorAll("[data-probe]").forEach((el) => {
    el.onclick = () => probe(el.dataset.probe);
  });
  Object.keys(SWITCHES).forEach((id) => {
    $(id).onchange = () => togglePref(id);
  });
  $("btn-hostname").onclick = setHostname;
  $("btn-login").onclick = login;
  $("btn-logout").onclick = logout;
  $("btn-dns-refresh").onclick = () => probe("dns-refresh");
  $("routes-refresh").onclick = () => operation(loadRoutes);
  $("btn-src-daemon").onclick = () => setSource("daemon");
  $("btn-src-diag").onclick = () => setSource("diag");
  $("btn-log-refresh").onclick = () => operation(loadOutput);
  $("btn-copy").onclick = () => copyOutput($("out").textContent);
  $("btn-clear").onclick = clearLogs;
  $("demo-picker").onclick = selectDemo;
  $("dialog-close").onclick = () => closeModal();
  $("dialog-cancel").onclick = () => closeModal();
  $("overlay").onclick = (event) => {
    if (event.target === $("overlay")) closeModal();
  };
  $("dialog-confirm").onclick = () => {
    if (!state.modal) return;
    if (state.modal.options.output) {
      copyOutput(state.modal.options.output);
      return;
    }
    if (state.modal.options.input) {
      const value = $("in-hostname").value.trim();
      if (!/^[A-Za-z0-9.-]+$/.test(value)) {
        text("input-error", "仅允许字母、数字、点与短横线。");
        $("input-error").hidden = false;
        $("in-hostname").focus();
        return;
      }
      closeModal(value);
    } else closeModal(true);
  };
  document.addEventListener("keydown", (event) => {
    if (!state.modal) return;
    if (event.key === "Escape") {
      event.preventDefault();
      closeModal();
    }
    if (event.key === "Tab") {
      const focusable = Array.from(
        $("dialog").querySelectorAll(
          "button:not(:disabled), input, pre[tabindex]",
        ),
      ).filter((el) => !el.hidden && el.getClientRects().length);
      if (!focusable.length) return;
      const first = focusable[0],
        last = focusable[focusable.length - 1];
      if (
        event.shiftKey &&
        (document.activeElement === first ||
          document.activeElement === $("dialog"))
      ) {
        event.preventDefault();
        last.focus();
      } else if (
        !event.shiftKey &&
        (document.activeElement === last ||
          document.activeElement === $("dialog"))
      ) {
        event.preventDefault();
        first.focus();
      }
    }
    if (event.key === "Enter" && document.activeElement === $("in-hostname")) {
      event.preventDefault();
      $("dialog-confirm").click();
    }
  });
  window.addEventListener("popstate", (event) => {
    if (state.modal) finishModal(state.modal.value);
    // Forward into a dismissed overlay is harmless: strip its marker.
    const saved = event.state || {};
    state.depth = saved.depth || 0;
    if (saved.modal)
      history.replaceState(
        { page: saved.page, depth: state.depth },
        "",
        location.href,
      );
    const page = saved.page || location.hash.slice(1) || "home";
    if (page !== state.page) showPage(page);
  });
  document.querySelector(".skip-link").onclick = (event) => {
    event.preventDefault();
    $("page-title").focus();
  };
  const theme = matchMedia("(prefers-color-scheme: dark)");
  if (theme.addEventListener) theme.addEventListener("change", renderStatus);
  else theme.addListener(renderStatus);
  document.addEventListener("visibilitychange", () => {
    if (!state.busy && document.visibilityState === "visible") refreshPage();
  });
}

wire();
if (demo) {
  $("demo-bar").hidden = false;
  text("demo-name", scenarios[demo.name]);
}
const initialPage = Object.prototype.hasOwnProperty.call(
  PAGES,
  location.hash.slice(1),
)
  ? location.hash.slice(1)
  : "home";
history.replaceState({ page: initialPage, depth: 0 }, "", "#" + initialPage);
showPage(initialPage, false);
updateDisabled();
(async () => {
  await refreshStatus();
  await loadPrefs();
})();
// Preserve the existing 15-second status cadence. Log reads run only while the
// selected log page is visible. No background DNS probe or route scan is added.
setInterval(() => {
  if (!state.busy && document.visibilityState === "visible") refreshStatus();
}, 15000);
setInterval(() => {
  if (
    !state.busy &&
    !state.modal &&
    state.page === "logs" &&
    $("auto").checked &&
    document.visibilityState === "visible"
  )
    loadOutput();
}, 5000);
