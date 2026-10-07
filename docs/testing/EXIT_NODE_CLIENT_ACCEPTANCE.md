# Exit Node Client 真机闭环验收

日期：2026-10-07（Asia/Singapore）。基线：main `4cf4853`。设备为 ROOT Redmi Note 8 Pro 与 Windows 11 Fog。未安装新模块、未修改功能代码、未发布 Release。

**结果：PARTIAL / NOT ACCEPTED。原生双栈转发、outer bypass、LAN access 和退出恢复通过；Android DNS 与 FlClash Fake-IP 共存失败。不能宣告 `NO_CODE_CHANGE_NEEDED`，不进入 WebUI Exit Node 选择器阶段。**

后续 [scoped policy建模与内核门槛验证](EXIT_NODE_POLICY_MODEL.md) 确认当前配置的Fake-IP实际为198.18.0.0/16（此前/15为假设），并发现goto5271未解析，按用户要求停止实现。

## 前置与测量边界

- Fog `100.79.33.7` 临时执行 `tailscale set --advertise-exit-node`，用户在管理端批准。Redmi status JSON 实际出现 `ExitNodeOption=true` 后才首次启用客户端。
- Redmi 使用原生 `tailscale set --exit-node=100.79.33.7`；关闭使用 `set --exit-node= --exit-node-allow-lan-access=false`。没有手工添加 `/0`，没有修改 routes/routes.auto。
- USB 序列号固定为 `pnq47xf6899t4huc`，未操作另一台 ADB 设备或重启 ADB server。按用户要求保留 USB 充电常亮。
- 手机安装版尚没有 `tailscaled.service exit-audit`，实际采集使用上一轮临时 `exit-audit-helper --format exit-audit`，未替换模块 service/helper。
- 临时 Linux/arm64 HTTPS 探针显式使用 `1.1.1.1:53`，加载 Android 系统 CA，保留 TLS 校验。普通与 `0x10020000` socket 分别测量。它不代表 Android netd DNS，也不代表普通 App。
- 普通 App 使用 Chrome；同时采集 `ss -ntpe` 的 UID/mark、iptables OUTPUT、`tcpdump -i any` 的接口和 DNS/UDP/TCP 元数据。所有原始测量保留在忽略目录 `build/exit-node-acceptance/`，没有提交 state、密钥或完整 prefs。
- Fog 本机默认 curl 使用环境代理，不能据此判定出口。无代理 IPv6 HTTPS 返回 `2001:250:3c00:3487:a964:98cc:81ea:184`。无代理 IPv4返回 `218.17.40.113`；出口客户端返回 `218.17.207.83`，两者不同，不能宣称 IPv4 字符串相等。客户端进入 tailscale0、双栈 exit default 和匹配 Fog 的 IPv6 是独立转发证据；未完成 Windows 侧逐流抓包归因。

## A–D 矩阵

| 阶段 | 真实观测 | 判定 |
|---|---|---|
| A：移动数据，FlClash OFF，Exit OFF | main 经 ccmni1；table52 无 exit default；到 Fog 初始 DERP(sin)。普通 IPv6 探针出口为 Redmi 公网地址 `2408:8456:3243:fa46:1:2:5190:d2f7`；首次 IPv4站点测量超时，后续 marked 物理出口为 `112.97.55.41` / `112.97.51.41`。Android 对 www.example.com 可解析并 ping 成功。 | 基线已采集；首次 IPv4普通探针不能标 PASS。 |
| B：FlClash OFF，Exit ON(Fog) | 普通探针 IPv4 `218.17.207.83`、IPv6 Fog 的 `…:184`；marked 探针仍走 Redmi 的 `112.97.x.x` / `2408:8456:…`。table52 双栈 default，main 未被 exit 替换。到 Fog IPv6 direct 56 ms。 | 转发/outer bypass PASS；Android 原生 DNS失败，见下节。 |
| C：FlClash ON，Exit ON(Fog) | 明确存在 Chrome→127.0.0.1:7890→FlClash；FlClash 上游源地址为 `100.118.66.106`、mark `0x60000`，经 exit。普通显式公共 DNS 探针仍返回 Fog 双栈出口。Android VPN DNS被送到 tailscale0且重试。Chrome 的新出口页面未取得可靠成功结果。 | 共存 FAIL；不能把探针成功当成所有 App 成功。 |
| D：FlClash ON，Exit OFF | table52 双栈 exit default 撤销；table1099 无 /0。普通 IPv4探针及实际 Chrome 页面返回 FlClash 代理出口 `52.221.249.202`。Android DNS重新返回 Fake-IP，ping 成功。到 Fog IPv6 direct 36 ms。FlClash 当前无普通 IPv6可用路径，未将此现象归因于 exit 残留。 | 原行为/关闭恢复 PASS。 |

公网地址会变化；`52.221.249.202` 是当前代理出口，不是 Fog 或 Redmi 的物理出口。

## 路由与 mark 差量

```text
Exit OFF → ON，table52 IPv4:
+ default dev tailscale0
+ 当前外部接口的本地前缀 dev tailscale0（LAN access=false）
+ throw 127.0.0.0/8

table52 IPv6:
+ default dev tailscale0
+ 当前外部接口的 IPv6 本地前缀 dev tailscale0

main IPv4（移动数据）:
  default via 10.167.171.85 dev ccmni1
  未被 exit default 替换

table1099:
  100.64.0.0/10 dev tailscale0
  未新增 /0

ON → OFF:
- 上述 exit default / 本地 tunnel 与 throw 条目
  保留原 Tailnet / quad100 路由
```

5210/5230/5250 的 marked bypass 和 5270 lookup52 继续存在；没有修改规则算法。网络切换期间 netd netId/接口有正常变化，5209随现有机制更新。实际控制 TCP socket 为物理源地址、`fwmark:0x10020000`；到 Fog 的 IPv6 UDP direct 成功，不是 exit 套娃。

IPv4-only Wi-Fi 下，普通 exit 探针仍获得 Fog 的 IPv6；同条件 marked IPv6探针 `network is unreachable`，说明测试的普通 IPv6未从该物理 Wi-Fi 泄漏。**未人为关闭 Fog 的 IPv6，也未验收所有 UID/socket 的完整无泄漏保证**；现有 bootstrap/control 的物理绕过是有意设计。

## DNS 与 FlClash 的失败证据

### 无 FlClash：Android carrier DNS没有随 exit 切换

Redmi 基线 `CorpDNS=false`。选择 exit 不会改变此 prefs，也不会把 Linux resolver 配置写入 Android netd。

补测 B 时保持候选可用、原生启用成功后，Android `ping www.iana.org` 在 10 秒内未完成解析。抓包显示：

```text
22:32:08 tailscale0 Out fd7a:…:426b → 2408:8888:0:8888::8:53
22:32:15 tailscale0 Out 100.118.66.106 → 120.80.80.80:53 A? www.iana.org
```

撤销 exit 后，同一待完成查询的重试改走 ccmni1，`221.5.88.88` 返回响应。这里确认了 carrier DNS经过远端出口时不响应；**未在 Fog 侧抓包证明究竟是运营商源地址限制还是远端沿途过滤**。

同期 controlplane 的 bootstrap DNS仍从 ccmni1 发往 carrier DNS并收到响应，`dns_reachable=true`。所以 bootstrap 正常不等于 Android App DNS正常。独立公共 DNS探针成功，也不能消除上述 netd DNS故障。

### 有 FlClash：VPN DNS与 Fake-IP 被 exit default 捕获

```text
tailscale0 Out 100.118.66.106 → 172.19.0.2:53
  A / AAAA? icanhazip.com，随后同 transaction 重试

ip route get 172.19.0.2:
  dev tailscale0 table52

ip route get 198.18.0.4:
  dev tailscale0 table52
```

补测新名称 `exit-audit-20261007.example.com`：Exit ON 时 12秒内没有解析结果；Exit OFF 后返回 `198.18.0.4`，Fake-IP ping 恢复。

原生 `--exit-node-allow-lan-access=true` 的对照：table52 新增 `throw 172.19.0.0/30`，VPN DNS恢复到 tun0，新名称解析为 `198.18.0.5`；但是 Fake-IP ping 仍失败。**LAN access不是完整的 FlClash 共存修复**，不能默认开启它后宣称通过。

实际拓扑按 socket 区分：

- Chrome 已观察到显式连接本地 HTTP 代理7890；FlClash 的代理/DoH上游 socket（UID10251、mark0x60000）从 Tailnet IP 发出，说明这一层经 Fog transport，再到实际远端代理/目标。最终网站看到的 IP可能仍是远端代理 IP，不能一概说网站出口一定是 Fog。
- 普通无代理探针直接进入 exit。
- Android VPN DNS / Fake-IP 被错误地送进 exit，发生超时/访问失败。
- Tailscale control为物理源地址、mark0x10020000；Tailnet direct依然工作。

已检查 OUTPUT/nat/mangle 与模块 exemption，未修改规则；本轮不能只凭上述 route get 认定所有 App 拓扑。socket 证据、DNS抓包及出口探针提供不同层面的验证。

## LAN access 真机对照

Wi-Fi `192.168.137.249/24`，网关 `192.168.137.1`。该网关不响应所测 ICMP，因此改用实际 UDP DNS访问验证：

| 状态 | 网关 DNS查询 | 路由 |
|---|---|---|
| Exit OFF | 返回 example.com 的 A/AAAA | 本地 Wi-Fi |
| Exit ON、LAN access=false | 超时 | table52 tunnel |
| Exit ON、LAN access=true | 返回同一域名 A/AAAA | throw192.168.137.0/24 → Android wlan0 table |

这里证明实际 LAN DNS可访问，不把它扩大为所有 LAN服务/协议都已验收。

## 恢复检查

- Redmi ExitNodeID/IP清空、ExitNodeAllowLANAccess=false。Hostname、CorpDNS、RouteAll、WantRunning 与基线安全 prefs对比一致。
- Self ID与双栈 Tailnet IP保持不变，hostname仍为 redmi-note-8-pro。没有读取/复制 state原文；不能将正常 prefs写入后的 state文件字节变化等同于身份变化。
- settings.ini、routes、hostname-initialized 的 SHA256逐项一致；routes.auto与 hostname-user-set基线不存在，未生成/删除用户配置。
- daemon PID全程6665；未执行 restart。最终 BackendState=Running。
- table52退出 defaults完全撤销；main回到 ccmni1 physical default；table1099只有原Tailnet前缀。最终 DNS来源 android-linkproperties、CELLULAR、dns_reachable=true。
- 最终 selftest：main default ok，exemptions `pre=OK out=OK nat=OK`，Fog仍为IPv6 direct。
- Wi-Fi恢复关闭、FlClash恢复关闭。USB充电常亮按用户要求保留。
- Fog执行 `set --advertise-exit-node=false`，AdvertiseRoutes恢复为空，Redmi再次看到 ExitNodeOption=false。**管理端批准记录仍保留**；未擅自更改管理端授权，Fog不再通告或提供本次 exit角色。

## 后续边界

本次交付真实失败与恢复报告，未以手工 /0、改 DNS/helper 或改规则顺序掩盖失败。需要进一步设计 Android netd DNS、VPN本地 DNS与 Fake-IP 的出口客户端策略；仅添加一个硬编码网段或默认开启 LAN access不能解决全部问题。没有足够依据将这种策略变更称为已验证的最小修复，因此本轮没有提交网络补丁或提前实现 UI。

最少恢复后复查（在 root shell）：

```sh
TS=/data/adb/tailscale/bin/tailscale
SOCK=/data/adb/tailscale/run/tailscaled.sock
$TS --socket="$SOCK" status
$TS --socket="$SOCK" ping --c=2 --timeout=4s 100.79.33.7
ip route show table 52; ip -6 route show table 52
ip route show table 1099; ip route show table main
/data/adb/tailscale/scripts/tailscaled.service selftest
```

这些是恢复检查，不是通过验收后可直接公开推荐的 Exit Node启用步骤。

文档提交前验证：5项 service/诊断集成测试通过；android-netdiag Go race测试通过；文档相对链接与 `git diff --check` 通过。未修改功能代码，因此没有构建或发布新的安装包。
