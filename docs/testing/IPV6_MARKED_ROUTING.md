# Redmi outer IPv6 marked routing 修复与真机验收

2026-10-07，基于 main；不新建分支、不发布 Release。EAIDK IPv4 STUN 排查暂停。

## 根因与真机证据

Redmi 的真实移动接口具有公网 IPv6，Android 将 RA/default 路由保存在该物理网络的
policy table 中。Tailscale GOOS=linux socket 使用 `0x10020000`，被 osrouter 的
5210/5230/5250 规则先送入 main/default，再判定 unreachable；IPv6 main 没有出口。
问题不只是缺少 netId：保留原 bypass 并补上 netd mark，仍命中这些更早的规则。

在 netId **112**、interface **ccmni3** 的同一轮采集中，用真实 netd fwmark socket
API 的 SELECT_NETWORK 设置测试 socket，再读 SO_MARK，得到 **0xf0070**。
没有凭猜测构造 Android permission、explicit 或 protected 位。netd rules 同时包含：

```text
16000: from all fwmark 0x10070/0x1ffff iif lo lookup ccmni3
23000: from all fwmark 0x70/0x1ffff iif lo lookup ccmni3
5210:  from all fwmark 0x10020000/0x1e020000 lookup main
5230:  from all fwmark 0x10020000/0x1e020000 lookup default
5250:  from all fwmark 0x10020000/0x1e020000 unreachable
```

ConnectivityService 将 FlClash VPN 的 underlying 指向 network 112；该网络的
LinkProperties 是 CELLULAR / ccmni3，含公网 IPv6 与 IPv6 default route。
实际内核默认路由是 `via fe80::5 dev ccmni3 table ccmni3`。

| 同一公网 IPv6 目标的查询/测试 | 修复前结果 |
|---|---|
| unmarked route get | No route to host，普通未选网调用受当前 VPN/policy 影响 |
| 指定真实物理 oif | 找到 ccmni3 policy table 的 IPv6 出口 |
| 当前 0x10020000 | Network is unreachable |
| netd SELECT_NETWORK 返回的 0xf0070 | IPv6 UDP connect 成功；单个选定 STUN server 未回包，不单独作为可达证明 |
| 保留 bypass、组合成 0x100f0070 | 仍 Network is unreachable |

上述比较同时覆盖 EAIDK 公网 IPv6 和公网 STUN IPv6 目标。
netId 的低 16 位含义由 [AOSP Fwmark.h](https://android.googlesource.com/platform/system/netd/+/master/include/Fwmark.h)
以及真机 rule selectors / netd socket 返回值交叉确认；没有把 112 写进功能代码。

## 最小设计

保留 daemon/DNS helper 的 socket mark 和 IPv4 处理，新增一条 **IPv6-only** policy rule：

```text
5209: from all fwmark 0x10020000/0x1e020000 iif lo lookup <verified physical IPv6 table>
```

优先方案已验证：只补 netId 不能越过现有 osrouter bypass 规则；只换成 native mark
又会丢失 Tailscale bypass 的路由隔离语义，并使长期 UDP socket 持有容易过期的 netId。
因此采用动态引用已有 netd 物理表，比修改 netns/socket 生命周期或复制 IPv6 default
到 main 更小。netId 用于验证表的归属，socket mark 保持常量，没有旧 socket netId 要清理。

选表过程：

1. 复用 `android-dns --network` 的只读 Android discovery，跟随 active VPN underlying。
2. 读取实际 `ip -6 rule`，要求 native selector 的 netId 与选中 physical netId 一致。
3. 验证候选表的 IPv6 default 出口就是该物理接口，且接口有公网 IPv6。
4. 排除 VPN/Tailscale、main/default/local、table52/1099 和配置的手工 tunnel table。
5. startup / watchdog 更新 5209；没有 verified 物理 IPv6 时撤销旧规则，保留 IPv4/DERP。
6. 规则或 physical netId 变化时调用一次有界 `debug restun`。不 rebind、不重启 daemon。

最初只恢复路由时 netcheck 已 IPv6=true，但 direct 尚未建立。tcpdump 显示手机从
IPv6 listener **51975** 向 EAIDK 发 UDP，而对端仍探测旧 local candidate **53276**。
Re-STUN 后新增真实 `[mobile-GUA]:51975 (stun)` endpoint，随即恢复 direct **40 ms**。
这是恢复路由后的 endpoint 更新需求；没有修改 daemon、listener 端口或节点身份。

实际修复后的 route get 保持原 mark：

```text
ip -6 route get <EAIDK-GUA> mark 0x10020000
<EAIDK-GUA> via fe80::5 dev ccmni3 table ccmni3 ...
```

tcpdump 在真实 ccmni3 上观察到 magicsock IPv6 UDP 出站；netcheck 双向 STUN 和
实际 Tailnet direct ping 随后成功。EAIDK endpoint 为
`[2001:250:3c00:3487:add3:4002:9c55:b820]:41641`。

## 真机验收

| 环境/变化 | 结果 |
|---|---|
| 移动数据 + FlClash ON，physical netId116 / ccmni3 | netcheck IPv4/IPv6=true；direct 43 ms；marked route 正确 |
| 移动数据 + FlClash OFF，确认无 VPN network | netcheck IPv4/IPv6=true；direct 37 ms |
| FlClash 重新 ON，VPN103→117，physical 仍116 | underlying 正确；direct 70 ms |
| Wi-Fi→移动，physical116→119 / ccmni1 | watchdog 自动更新选表；IPv6=true；direct 80 ms |
| 移动→Wi-Fi，physical119→120 / wlan0 | watchdog 自动撤销5209；不残留旧移动表；Running、IPv4正常 |
| 再回移动，physical120→121 / ccmni3，FlClash ON | watchdog 自动恢复；netcheck UDP/IPv4/IPv6=true；direct 75 ms |

当前测试 Wi-Fi 是 IPv4-only，没有公网 IPv6/default。这个状态下 IPv6 unavailable
是正确结果，不能从 IMS/另一个 SIM 借路伪造成功；DERP fallback 保留。

为了在 USB 上热加载修改过的 service，只重载了原 watchdog 进程一次；没有重启
tailscaled。后续 119→120 等切换使用新 watchdog 自动处理，没有手工同步规则。
daemon PID 一直是 6517，Self hostname 和 Tailnet 地址保持不变。

最后一次切回移动数据后，selftest 的前两次 kernel ping 在路径收敛期间丢包，
同次 Tailscale ping 随后建立 IPv6 direct。再次 kernel ping 为3/3成功，
RTT 48.8–68.9 ms；没有把切换瞬间的丢包记为全程无损。
最终 main_default=ok、DNS reachable=true、pre/out/nat exemption=OK；
IPv4 的5210–5270、table52与5300→1099规则仍在。手机留在移动数据 + FlClash ON，
USB测试临时亮屏设置已恢复为原值0。

## 故障与清理

`outer-ipv6-status` 以0600原子写入，记录 network/interface/table/state/reason/mark。
`webstatus`、`diag`、`selftest` 可查看；原始诊断仍包含 IPv6 rules 与 marked route。

Android toybox flock 不支持 `-w`，mksh 还会将 shell descriptor 标为 CLOEXEC。
实现用 `flock -n` 有界重试并显式传递 fd9，避免并发 start/stop/watchdog 竞争。
Stop 只清理完整 owned 签名；遇到同表 foreign rule 时保守跳过并报告 conflict，
避免 Linux 删除规则时的通配匹配误删其他规则。规则添加失败和 Restun 超时均不停止 daemon。

## 本地检查

- 40 项 Python 测试通过，其中10项新增 IPv6 policy 回归：netId/table变化、IPv4-only
  Wi-Fi、VPN/错误表、发现失败、外来规则、同表删除歧义、添加失败、stop、Restun超时、toybox锁。
- ShellCheck / shell语法 / git diff --check 通过。
- 19条 WebUI命令在3种PATH环境中通过，保留 custom socket，共57次检查。
- 安装升级身份保留与 ZIP 二进制隔离测试通过；DNS helper、fwmark patch 和 daemon 未修改。

## 最少复核命令

在有公网 IPv6 的移动网络下：

```sh
su -c 'cat /data/adb/tailscale/outer-ipv6-status'
su -c '/data/adb/tailscale/scripts/tailscaled.service selftest 100.72.239.86'
su -c 'ip -6 route get 2001:250:3c00:3487:add3:4002:9c55:b820 mark 0x10020000'
```

检查 selftest 的 IPv6 netcheck、实际 direct endpoint 和 DNS/route/exemption。
在 IPv4-only Wi-Fi 下应看到 unavailable，不能要求该网络凭空具有 IPv6。

[前一轮排查](P2P_DIAGNOSIS.md) · [网络诊断设计](../NETWORK_DIAGNOSTICS.md)
