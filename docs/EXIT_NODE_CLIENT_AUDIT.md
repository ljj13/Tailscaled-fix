# Exit Node Client：只读审计与最小预检

日期：2026-10-07。基于 main、GOOS=linux / osrouter、Android netd 与现有 Clash 共存设计。未新增 WebUI 开关，未启用任何设备的 exit node，未改变路由、DNS、身份或服务器角色。

以上描述的是本轮只读审计。随后已执行 [Exit Node Client 真机闭环验收](testing/EXIT_NODE_CLIENT_ACCEPTANCE.md)：原生双栈转发、outer bypass 与恢复通过，但 Android DNS / FlClash Fake-IP 共存失败，尚不能宣布完整支持。

## 核心结论

| 问题 | 源码与现有规则给出的结论 |
|---|---|
| `tailscale set --exit-node=<peer>` 新增什么？ | Linux router 在 **table52** 安装 `default dev tailscale0` / `::/0 dev tailscale0`。还可能增加本地网段的 tunnel route 或 throw route，取决于 LAN access 和接口分类。通常复用既有 5210/5230/5250/5270 rules，不需要新增一套默认公网 rule。缺一个地址族时也补默认路由，避免泄漏；不代表出口已成功转发。 |
| 抢 main default 吗？ | IP_MULTIPLE_TABLES 在这台 Redmi 已生效，osrouter 默认路由写 table52，不替换 main。模块继续维护 main 的 physical IPv4 default，outer IPv6 用 5209 命中动态 physical table。公网流量可能被较早的 table52 lookup 接管，**无需修改 main**。 |
| 与 table52 / 1099 冲突吗？ | table52 正是正常 exit client 路由所在。5270 lookup52 早于模块 5300 的目标前缀 lookup1099；已有 Tailnet 更具体路由仍有效。正常的 1099 Tailnet/subnet 路由没有表号冲突。但手工/自动文件若包含 `/0`，就有关闭 exit 后仍抓公网的风险，不能自动清理外来配置。 |
| DNS 随 exit node 改变吗？ | 分三层，不能一概而论，见下节。选择 exit 不等于自动开启 `--accept-dns`，也不等于配置 Android netd/FlClash DNS。 |
| FlClash 与 exit 谁接管公网？ | 对正常非 Tailscale-bypass 流量，5270/table52 default 早于 Android VPN/netId rules（真机常见 11000/16000/24000 等），因此 Tailscale 通常先抓到它。FlClash 自身上游连接也可能被送进 exit，形成代理→exit 链路。不能保证“所有公网最终仍归 FlClash”，也不能根据一条 root route get 推断所有应用；更早规则、socket mark/绑定接口、OUTPUT/TPROXY/REDIRECT 都要分别查。 |
| 关闭后完全恢复吗？ | 原生 osrouter 根据新 Config 对 table52 routes/throw routes 做差量删除，保留一般 Tailnet/subnet 及基础 rules；main physical route / outer bypass 继续工作。不会自动删除模块 routes/routes.auto 中的 `/0`，也不承诺 Android VPN/DNS 无残留。**本轮无可用 exit，真实启用→关闭恢复尚未验收**。 |

控制/outer 标记仍是 `0x10020000/0x1e020000`。IPv4 在 5210 查询 main；IPv6 的 module-owned 5209 查询当前 netd physical table。它们早于 5270/table52，避免 WG outer/control 流量再次被默认 tunnel route 捕获。Tailscale client 和 advertise-exit-node server 是不同角色：客户端不需要通告出口、增加代理层或自行开 IP forwarding。

## DNS 三层

1. **bootstrap/control DNS**：Go resolver 固定读取私有 bootstrap-resolv.conf，DefaultResolver.Dial 打 bypass mark；现有 Android discovery/probe/watchdog 继续提供物理网络 DNS。选择 exit 不改它，也不应以 exit DNS 替换控制面引导。这是有意保留的物理网络通道，不是“所有 DNS 都走 exit”。
2. **Tailscale resolver / quad100**：`CorpDNS=false` 时 upstream 的 dnsConfigForNetmap 提前返回，不建立 exit DNS 默认 resolver；true 时会优先使用允许用于 exit 的 tailnet resolvers，否则可选 exit peerAPI DoH proxy，以及旧 exit 的 fallback 情形。具体受控制端策略和出口能力影响。
3. **Android / FlClash 应用 DNS**：本模块 DNS manager 只管理私有 os-resolv.conf，不接管 Android netd，也不修改 FlClash DNS。Android 原有 DNS 的地址可能不变，但 DNS 包的实际路由可能进 exit；运营商 DNS 未必接受出口机器的来源。即使开启 accept-dns，也不能声称 Android 系统已应用 quad100 配置。

`--exit-node-allow-lan-access` 默认 false。true 时 table52 的本地网段 throw routes 允许后续物理规则接管；Android netd 还要求对应 policy/network 可用。本地 SSH/无线 ADB、网关及运营商 DNS 的影响必须另外验证，不能只看 Tailnet ping。

## 源码证据（固定版本）

实际缓存 HEAD：`5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115`，与 v1.102.5 构建锁定版本一致。

- [routerConfigLocked](https://github.com/tailscale/tailscale/blob/5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115/ipn/ipnlocal/local.go#L6511)：补双栈 default，处理 LAN access / LocalRoutes。
- [Linux osrouter](https://github.com/tailscale/tailscale/blob/5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115/wgengine/router/osrouter/router_linux.go#L465)：cidrDiff、addRoute/routeTable、throw routes、baseIPRules；本仓库另外应用已验收的 fwmark patch。
- [DNS config](https://github.com/tailscale/tailscale/blob/5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115/ipn/ipnlocal/node_backend.go#L1480)：CorpDNS gate、exit DoH、UseWithExitNode resolvers。
- 本仓库 [prepare-build.py](../scripts/prepare-build.py)：bootstrap 与 os-resolv 分离；[resolver mark](../patches/android_dns_linux.go)。
- [官方 Exit Nodes 文档](https://tailscale.com/docs/features/exit-nodes)：出口需通告并经管理端批准；客户端需获访问公网的策略授权。通告能力、在线状态、ACL 许可和真实转发成功是不同证据。

## 最小实现：exit-audit

新增只读 CLI：

```sh
su -c 'tailscaled.service exit-audit'
```

helper 对应 `--format exit-audit`。复用既有有界脱敏 report pipeline，额外采集 unmarked IPv4/IPv6 route、mangle/nat OUTPUT；增加 ExitNodeID/IP/LAN access 的 prefs 白名单、当前候选/选中节点摘要、table1099 默认路由残留检查。原有 report 也保留这些安全 prefs 字段。

所有内容经过同一个最终脱敏器；不读取 state/socket 原文，不执行 set/up/logout/restart，不写 route/rule/DNS。状态缺失、JSON 损坏、timeout/权限错误显示 unknown/unavailable，不把 prefs enabled、Online 或 ExitNodeOption 当作已成功转发。netcheck 仍会发 STUN 探测，但不会改变 exit 选择。`restoration_live_test=not-performed` / `internet_forwarding=not-tested` 明示只读限制。

没有新增 setter：原生 `tailscale set --exit-node=...` 已存在；本轮先提供预检，不能绕过候选/审批/DNS 证据就加 WebUI 开关。

## 两端实测（当前状态）

- 电脑 **Fog / 100.79.33.7 / Windows** 在线，`ExitNodeOption=false`。
- 手机 **redmi-note-8-pro / 100.118.66.106** 在线，`ExitNodeOption=false`；当前无选中 exit，accept-dns=false。`RouteAll=true` 不代表选了 exit，接受子网路由与 exit prefs 独立。
- 手机看到电脑也为 ExitNodeOption=false，候选列表为空。两端登录不能替代出口通告/批准，故未对手机调用 set --exit-node。
- 当前采集时 FlClash **OFF**，physical ccmni1；未切换它。此前 ON 的已验收规则/underlying 时间线只作为已有证据，不冒充本轮 exit+FlClash ON 实测。
- 真机只运行临时 helper，未覆盖安装/更改现有 service/watchdog。预检约 63 KB，configured=disabled，table1099 双栈 defaults absent。
- unmarked IPv4 / IPv6 命中 ccmni1 physical table；marked IPv4 main/ccmni1、marked IPv6 ccmni1 table 均可路由。采集前后 state/settings/routes SHA256 与 daemon PID 完全相同。

后续若使用电脑作出口，需先让电脑通告并在管理端批准，再以 USB 保持手机可恢复通道，做 enable→OFF/ON→disable 四组差量采集。未执行通告或后台批准；本轮不改变电脑服务器角色。

## 验证

- 新增 3 项 Go tests：缺字段/disabled/selected/offline、候选安全白名单、自身去重、table1099 默认/blackhole/throw/失败判定。
- 新增 1 项 service 集成测试：invalid JSON/timeout 不改变身份，仍输出只读/未验收标志及 route sections。
- 全套 netdiag 23 项 race tests 与 vet、service 5 项、ShellCheck、Python 全套 69 项（含升级/备份/打包）全部通过。
- 上游 Linux `TestRouterStates` 用仓库既有 **test overlay** 通过，包含 default/throw route 状态转换。未使用 overlay 的一次试跑按 stock fwmark golden strings 失败；应用现有 Android-mark fixture overlay 后通过，没有改断言或生产规则。

**尚未证明**：电脑作为出口的真实公网出口 IP、IPv6 转发、手机应用 DNS、Exit+FlClash OFF/ON 优先级与真实关闭恢复。本轮结论分清源码推导、当前规则观测和未执行的启用验收。
