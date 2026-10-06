# v1.102.5-dnsfix.2：Android VPN / underlying network

仍以 upstream `v1.102.5` 为基线，工作分支为
`fix/android-dns-v1.102.5`，没有修改 main。

## 真机确认的根因

Redmi Note 8 Pro 的 ConnectivityService 显示：物理默认网络为
`106 / CELLULAR / ccmni1`；FlClash 为 `107 / CELLULAR|VPN / tun0`，
`UnderlyingNetworks: [106]`。普通 `ip route get` 返回 VPN `tun0`。
dnsfix.1 将这个接口作为 DNS 匹配条件，错过物理 LinkProperties DNS；
其 watchdog 也曾把 main 默认路由改为 `default dev tun0`，导致
`0x10020000` marked DNS / control-plane 流量再次进入代理。

自动刷新与手工探测的 SO_MARK 相同，都没有 SO_BINDTODEVICE。
但旧自动刷新将 discovery 与 probe 共用 18 秒预算，最多八次顺序
getprop 调用可能在发出 DNS 请求前耗尽预算；手工检查跳过 discovery。
这可以解释一部分自动/手工时间差，并非声称复现了历史每一次 timeout。
真机另见 osrouter 启动前 IPv6 probe 成功、启动后被 IPv6 bypass
unreachable 规则拒绝：startup 现在在 osrouter / add_routes 完成后再次探测。
helper 会清楚标出不可达的 IPv6 DNS，保留可达的物理网络 IPv4 DNS。

## 选择与保留规则

- 解析 Current Networks，排除历史请求记录；独立识别 VPN transport，
  包括 `CELLULAR|VPN`，排除 `tun*`、`tailscale*` 等虚拟接口。
- 先选择 Android active VPN，再考虑普通路由指向的 VPN；递归跟踪
  declared underlying networks，使用物理父 LinkProperties 的 DNS。
  `Null` 表示追踪系统默认；显式 `[]` 表示无 underlying，不任选 IMS / Wi-Fi。
  missing ID / 环路安全失败，多 underlying 按声明顺序选择存在的物理网络。
- 无 VPN 时优先 Android active default；只有缺少 active ID 的旧版输出
  才采用物理 route hint。全局旧 getprop DNS 不用于 VPN 场景。
- main IPv4 default 使用同一选择得到的物理 LinkProperties 默认路由。
  CLAT 的 IPv4 路由和源地址来自该物理网络自己的 stacked `v4-*` 链路。
  绝不将普通 VPN 默认路由复制到 main；没有发现新物理路由时暂缓修改。
- `dns-verified.json` 保存 network ID、父接口、resolver、来源、验证时间及
  boot ID。仍是同一 underlying network 时，新候选均失败不会覆盖上一份
  verified resolver，原 bootstrap 内容和 mtime 保持不变。
  短暂无法发现网络最多保留两分钟；明确不同网络、空 underlying、跨重启
  均失效。probe 前后重新检查网络，切换中不发布过时结果；首次启动则重试。
- Android DNS → 同网络 verified cache → 已配置 public fallback，均使用
  同一个 marked probe 实现。getprop 改为一次读取，discovery 最多六秒；
  每次 probe 使用独立预算，实际查询最多 2.5 秒，候选并行探测。
- 清空 public fallback 会同时撤销其缓存；无 Android 候选时撤销已发布的
  public resolver，并等待网络 DNS，避免 daemon 继续使用被禁用的公共地址。
- 诊断增加 Android network / transport / active VPN / underlying / parent
  interface / excluded interfaces / selection reason / route hint / physical
  route / retained / last verified / caller / mark / discovery 与 probe 耗时 /
  marked route 前后快照。DNS 错误显示实际探测 server，不误报 `[::1]`。

Tailscale state、身份、settings、routes、osrouter、fwmark 常量、table 52、
手工路由表 1099 与三处 exemption 的实现均保留。升级安装器未改变。

## 修改文件

- `tools/android-dns/{main,network,cache}.go` 与对应 Go tests。
- `tailscale/scripts/tailscaled.service`：物理 main 默认路由、统一 caller、
  daemon routing 就绪后的再次验证。
- `webroot/{index.html,app.js}`：新增网络选择与 retained 显示。
- `tests/test_shell.py`、`tests/test_dns_helper.py`、`tests/test_resolver.py`。
- `module.prop`、`scripts/{build.sh,package.py}`、CI workflow：dnsfix.2 构建。
- 本文、`DNS_FIX.md`、`TEST_RESULTS.md`、`RELEASE_NOTES.md`。

## 最少真机验证

覆盖安装 `tailscaled-v1.102.5-dnsfix.2-arm64.zip`，不要先卸载旧模块。
Redmi Note 8 Pro 的 dnsfix.2 真机验收已完成；后续安装请直接覆盖现有模块。
FlClash 开启和关闭各保持至少 30 秒，再执行一次：

```powershell
adb -s pnq47xf6899t4huc shell "su -c 'tailscaled.service dns; tailscaled.service selftest'"
```

预期：`dns_iface=ccmni1` 或当前 Wi-Fi 物理接口；VPN 开启时
`dns_underlying` 指向它，`dns_excluded` 包含 `tun0`；
`dns_reachable=true`、marked route 为物理接口；BackendState Running，
`default in main: ok`、三处 exemption OK，两种 ping 均成功。

2026-10-06，用户确认 Redmi Note 8 Pro 的最终 dnsfix.2 验收全部 PASS：

| 场景 | 真机结果 |
| --- | --- |
| 移动数据 + FlClash OFF | PASS |
| 移动数据 + FlClash ON | PASS |
| VPN ON→OFF | PASS |
| Wi-Fi + FlClash ON | PASS |

underlying network、DNS、main marked route、table52、exemptions、
tailscale ping、kernel ping 均正常。以上最终验收由用户确认；此前的
ADB 观测与本地测试分别保留在 `TEST_RESULTS.md`。其他 OEM、CLAT-only
网络和助手独立执行的完整覆盖安装/重启仍不在本次真机验证范围内。

正式 release 复用已验收的 dnsfix.2 ZIP，功能代码和二进制均不再修改。

Android underlying 语义来源：
[AOSP NetworkCapabilities](https://android.googlesource.com/platform/packages/modules/Connectivity/+/bed26e8f00/framework/src/android/net/NetworkCapabilities.java)、
[ConnectivityService](https://android.googlesource.com/platform/frameworks/base/+/8230b03102fd3de01986fed7f1a7e660e366d859/services/core/java/com/android/server/ConnectivityService.java)。
