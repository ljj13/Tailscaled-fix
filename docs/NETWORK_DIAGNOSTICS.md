# Tailscale 网络诊断（main，尚未发布）

本轮在现有 GOOS=linux / osrouter 模块上增加只读诊断。没有改变 DNS helper、
`0x10020000` fwmark、main 默认路由、table 52 / 1099 或 Clash/Mihomo 豁免，
也没有增加代理层或强制 direct。当前公开 Release 保持原样。

## 入口

```sh
su -c 'tailscaled.service netdiag'
su -c 'tailscaled.service diag'
su -c 'tailscaled.service selftest'
# 可选：指定本轮实际要验证的 peer，避免默认选择其他在线节点
su -c 'tailscaled.service selftest 100.72.239.86'
```

`netdiag` 返回 schema 1 JSON，`diag` / `selftest` 提供可读摘要和原始输出。
WebUI 的「网络与诊断」按需加载，可刷新、查看和复制原始 JSON；首页不增加调试字段。
原有服务管理与偏好 API 保留。

## 采集内容与含义

| 内容 | 数据来源与解释 |
|---|---|
| Self HostName / OS / Tailnet IPv4、IPv6 | `tailscale status --json` 的 Self；缺失字段保持未知。 |
| Home DERP code / region name | `Self.Relay` 与 `tailscale debug derp-map`，使用当前地图解析，不硬编码地区。 |
| 发布的 endpoints | `Self.Addrs`；区分 IPv4 / IPv6、Public / Private / CGNAT / Link-local。 |
| endpoint 对应接口 | 与本机接口地址精确匹配；NAT 公网映射只能推断物理出口，会明确标注 inferred。 |
| UDP listener | `ss -H -u -l -n -p` 按 daemon PID / tailscaled 进程归属筛选，显示实际端口，可能不是 41641。 |
| peer 在线状态、home Relay、连接路径 | `Online`、`Active`、`CurAddr`、`PeerRelay` 与 `Relay`。 |
| netcheck | UDP、IPv4、IPv6、MappingVariesByDestIP、UPnP/PMP/PCP、PreferredDERP、GlobalV4/V6、CaptivePortal。 |
| Android physical / VPN underlying | 复用 `android-dns --network`，失败时缓存明确标为可能过期；不刷新或写入 DNS。 |
| outer route | 同时保留 IPv4 / IPv6 main 表、规则和带 `0x10020000` 的 route get；已有 CurAddr 的对端也查询目的地址路由。 |
| 原始证据 | 有界 stdout / stderr、超时/错误/截断状态和 daemon 最近日志。 |

`Relay` 是 home DERP，不是单独的“正在走 DERP”证据。Online 描述控制平面连接，
与数据路径分别展示：有近期活动且有 CurAddr 时显示 direct；有 PeerRelay 时显示
peer-relay；有近期活动、无 direct / peer-relay 且有 Relay 时显示 DERP。
控制平面暂时离线也可能仍有有效数据路径。没有近期活动的离线节点标为 offline，
其他无近期活动节点标为 idle，字段缺失标为 unknown。
路径为状态快照，最终应结合指定对端的实际 `tailscale ping`。

Public 只描述地址范围，不能证明可达。Tailnet IPv6（`fd7a:...`）不等于公网 IPv6。
Self.Addrs 中其他接口的 IPv6 候选也不能证明当前 marked route 能使用它。
netcheck 使用临时 UDP socket，STUN 成功不等于 daemon 的 UDP listener 可以接收入站。
CaptivePortal 仅作为线索，不单独作为根因结论。

## 隔离与失败处理

采集器是独立的 `android-netdiag` 静态 arm64 helper，使用 Go 标准库解析 JSON；
不依赖 Android 上的 jq / Python。不放入 startup / watchdog，不修改 Tailscale state、
偏好、resolver、规则、路由或代理配置。只有 selftest 会发送有界 ping。

单命令最多 9 秒，总预算默认 15 秒，输出每流最多 128 KiB。超时终止子进程，
输出过长明确标记截断；失败字段保持未知，不冒充 false / 成功。helper 缺失也会给出
诊断错误，selftest / diag 继续执行。UI 读取失败保留上次快照并提醒核对时间。

`debug prefs` 原始内容可能含 Persist 私钥。采集器只保留 RouteAll；无论成功、
格式错误还是截断，都不会输出该命令的原始 stdout / stderr。
其他原始诊断可能包含公网 IP、节点名和登录链接，分享前请按需检查。

## 构建与测试

```sh
python3 scripts/build-netdiag.py
cd tools/android-netdiag
go test -race ./...
go vet ./...
```

从仓库根目录运行：

```sh
python3 -m unittest discover -s tests -p 'test_*.py' -v
node tests/network-ui.test.cjs
node tests/webui-command.test.cjs
node tests/webui.test.cjs
```

全量构建、复用验收二进制的打包器和覆盖安装器均包含新增 helper，升级仍保留配置与身份。
安装器在停止旧服务前检查完整 payload；没有 helper 的旧版本仍可正常运行。

## 参考范围

参考 [Magisk-Tailscaled 的 Troubleshooting](https://github.com/anasfanani/Magisk-Tailscaled#faq--troubleshooting)
中的服务状态、Tailnet ping 和原始日志分层排查方式。本轮没有移植其
userspace-networking / hev-socks5-tunnel 实现。

连接语义来自固定 v1.102.5 的
[PeerStatus](https://github.com/tailscale/tailscale/blob/v1.102.5/ipn/ipnstate/ipnstate.go)
与 [netcheck Report](https://github.com/tailscale/tailscale/blob/v1.102.5/net/netcheck/netcheck.go)。
