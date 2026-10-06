# Redmi ↔ EAIDK direct 排查记录

日期：2026-10-07。诊断增强基于 main，未发布 Release。

## 已完成的采集

- Redmi Note 8 Pro：ROOT Android、GOOS=linux 1.102.5、FlClash 开启。
  通过无线 ADB 采集 Wi-Fi 和短暂切换到移动数据后的报告，随后恢复 Wi-Fi。
- EAIDK-310：Linux 6.18.54、Tailscale 1.102.4，经 SSH 采集。
- 两端使用新增 collector 获取 status JSON、netcheck、DERP map、endpoint、
  listener、规则、路由和日志；另外在 EAIDK eth0 抓取 22 秒 UDP 包头。
  不读取数据载荷，不提交原始账号、节点标识或完整网络转储。

## 关键证据

| 项目 | Redmi | EAIDK |
|---|---|---|
| BackendState | Running | Running |
| Wi-Fi netcheck | UDP=true、IPv4=true、IPv6=false | UDP=true、IPv4=false、IPv6=true |
| 移动数据 netcheck | UDP=true、IPv4=true、IPv6=false | 同上 |
| NAT 映射随目标变化 | true | 未知，IPv4 STUN 无成功结果 |
| 当前物理接口 | Wi-Fi 为 wlan0；移动数据快照为 ccmni3 | eth0 |
| IPv6 地址 | 移动数据物理接口具有公网 IPv6；Wi-Fi 无公网 IPv6 | eth0 具有公网 IPv6 |
| IPv6 outer route | 带 0x10020000 查询公网地址和 EAIDK 地址均 ENETUNREACH；main IPv6 表为空 | 带原生 Linux 0x80000 有 eth0 出口 |
| daemon UDP listener | IPv4 53276、IPv6 51975；不能假定使用 41641 | IPv4 / IPv6 均 41641 |
| 实际指定对端 ping | DERP(hkg)，437–889 ms，首次约 1 s | DERP(lax)，371–454 ms，首次约 1 s |

home DERP、netcheck PreferredDERP 和实际 ping 的 relay 含义不同。
Redmi 的 home DERP 为 lax，而它向 EAIDK 发起的 ping 显示 hkg；不能仅凭 Self.Relay
判断这对设备使用的路径。本轮未恢复 direct。

第二次移动数据采集的真实接口变为 ccmni1。对 EAIDK 公网 IPv6，
`ip -6 route get <peer> oif ccmni1` 找到 Android 的 ccmni1 表，出口经 fe80::5；
同一查询增加 `mark 0x10020000` 后立即 ENETUNREACH。
接口绑定的 ping6 仍报告 No route to host，因此这里只证明物理表中存在候选路由，
尚未证明该物理路径的端到端 IPv6 UDP 可达性，不能把 IPv6 修复预先标为成功。

EAIDK eth0 包头采集期间：IPv4 STUN 出站 90 包、入站 0 包；IPv6 STUN 出站 81 包、
入站 80 包。还观察到 EAIDK 与 Windows 的 IPv6 daemon UDP 往返及 direct endpoint。
这表明 EAIDK 的 IPv6 UDP 和 Tailscale direct 引擎可以工作；IPv4 STUN 回包未到达
本机抓包位置，其具体丢失点尚未定位，不能只归因于本机 INPUT 规则。

EAIDK 另有独立 inet nftables input base chain，默认 drop，只允许 established/related、
ICMP、DHCP、SSH 和 tailscale0，没有显式允许新入站 UDP 41641。
iptables 的 ts-input accept 不能抵消另一条 base chain 的 drop。
这是公网入站限制，但 hole punching 返回流量可能匹配 established；本轮未证明
此规则是这对设备 direct 失败的唯一原因，也未修改防火墙。

## 根因判断与边界

已确认当前 IPv6 direct 的本地阻塞点：Redmi 的 daemon bypass mark 经过 osrouter
5210/5230/5250 规则，查 main/default 后遇到 unreachable；main 没有 IPv6 出口。
现有 main route 维护使用不带 `-6` 的 `ip route`，只维护 IPv4 默认路由。
发布了公网 IPv6 endpoint 或存在 Tailnet IPv6，并不意味着带 daemon mark 的 IPv6
socket 能发包。移动数据下 DNS 的 IPv6 server probe 同样 ENETUNREACH，而 IPv4
DNS 正常，符合这个判断。

当前 Wi-Fi 自身也没有公网 IPv6；IPv4 备选路径又受到 Redmi 目的相关 NAT 映射和
EAIDK 无 IPv4 STUN 回包的限制，因此继续保留 DERP 是预期的安全退路。
没有发现 VPN underlying 误选、DNS 失败或 IPv4 main route 回退的证据。
没有历史失败发生时的完整路由快照，不能据此声称已经证明历史退化的全部原因。

本轮要求不修改 DNS helper、fwmark、main default route、table 52/1099 和豁免行为，
因此只提交诊断与证据文档，未新增 IPv6 main 路由、修改 daemon 端口、开放防火墙，
也未强制 direct。后续恢复 IPv6 direct 需单独验证真实 physical network 的 IPv6
默认路由，再设计限定到该网络的最小修复；不能复制 tun0 或其他 SIM/IMS 接口的路由。

采集结束后已恢复 Wi-Fi / FlClash 环境。Redmi 仍 Running，IPv4 main route 正常，
table 52/1099 和 tailscale0 正常；kernel Tailnet ping 2/2 成功。
最后一次指定 EAIDK 的 Tailscale ping 为 DERP(hkg)，稳定样本约 313–318 ms，
仍显示 direct connection not established。没有重启 daemon 或改变节点身份。

## 本地验证

- Go collector 单元测试和 race、go vet：通过。
- Python 30 项测试：通过，覆盖升级身份保留、不完整 payload、超时与缺 helper。
- WebUI 缺字段/离线/无 IPv6/无 DERP/超时展示、57 次命令环境检查、
  14 个页面/主题组合和 5 个 mock 场景：通过，生成 24 张本地截图。
- ShellCheck、shell 语法与 git diff --check：通过。
- 预览 ZIP 构建通过；已验收 daemon / DNS 二进制未替换，未发布 Release。

## 最少复核命令

安装包含本轮诊断的 main 预览包后，在 Redmi 执行：

```sh
su -c '/data/adb/tailscale/scripts/tailscaled.service netdiag'
su -c '/data/adb/tailscale/scripts/tailscaled.service selftest 100.72.239.86'
```

EAIDK 执行：

```sh
tailscale netcheck
tailscale ping --timeout=2s -c 5 100.118.66.106
```

检查 marked IPv6 route、实际 ping 路径和 DNS/route/exemption 结果，不能只看 home DERP。

[诊断设计与字段说明](../NETWORK_DIAGNOSTICS.md) · [测试索引](README.md)
