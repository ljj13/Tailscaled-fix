# Exit Node scoped policy：只读模型与 goto 内核门槛

日期：2026-10-07（Asia/Singapore）。main 基线 `9c36fc2`。复用 [前轮闭环证据](EXIT_NODE_CLIENT_ACCEPTANCE.md)。

**结果：STOP_BEFORE_IMPLEMENTATION。Redmi 实测 `goto 5271` 未能跳过 table52。按用户门槛停止，没有实现/安装 physical DNS 或 VPN/Fake-IP 自动策略，没有开展新策略的 A–F 矩阵。不能进入 WebUI selector 阶段。**

## 当前只读模型

| 项目 | 当前真机依据 |
|---|---|
| physical network | 现有 android-dns `--network` 与 ConnectivityService 一致：netId111、CELLULAR、ccmni1。 |
| physical DNS | 所选父 LinkProperties：2408:8888:0:8888::8、2408:8899:0:8899::8、120.80.80.80、221.5.88.88；不是 public fallback 列表。 |
| physical policy | 原生 netd rule 的 fwmark0x1006f/0x1ffff 与 netId111一致、lookup ccmni1；实际表默认路由与接口地址均验证为 ccmni1。`/data/misc/net/rt_tables` 当前将该表映射为1003；没有根据 netId算表号。 |
| VPN | ConnectivityService 显式 VPN transport，netId112，OwnerUid10251，underlying=[111]，interface tun0。 |
| VPN DNS/本地前缀 | VPN LinkProperties 与接口地址：172.19.0.2、172.19.0.1/30；当前 tun0 / tun0_local table也有172.19.0.0/30。 |
| VPN 默认路由 | VPN LinkProperties/table有IPv4 default；IPv6 default为unreachable。不能把default整体当成“VPN-owned local prefix”。 |
| Fake-IP | LinkProperties、VPN表与其他route表没有显式Fake-IP前缀。当前VPN owner的生成运行配置 dns.enable=true、enhanced-mode=fake-ip、fake-ip-range=198.18.0.1/16；规范化后为198.18.0.0/16。 |

Fake-IP字段读取只在内存中提取合法CIDR/模式，没有保存或输出整个代理配置。这里记录的实际包/接口/网段是证据，**不能成为全局默认或产品硬编码**。后续可靠发现需要绑定当前VPN OwnerUid→包→运行配置及进程/配置有效性；配置不存在、语法不支持或无法确认其正在运行时必须 unavailable/conflict。

与前轮报告中的198.18.0.0/15假设不同：路由到198.18.0.4的失败不能证明CIDR长度；只有本轮明确配置声明证明当前实际/16。仅凭DNS返回一个198.18.x.x地址也不能推导配置范围。

## Exit OFF 时的真实路径

VPN OFF：

```text
120.80.80.80 → ccmni1 table ccmni1
2408:8888:0:8888::8 → fe80::5 ccmni1 table ccmni1
198.18.0.4 → ccmni1 table ccmni1（并非本地VPN范围）
```

VPN ON：

```text
172.19.0.2 → tun0 table tun0
198.18.0.4 → tun0 table tun0
```

顺序：5270 lookup52 → 5300模块Tailnet前缀规则 → Android10000/11000/16000等规则 → 24000 VPN selector → physical/default policy。Exit OFF时table52没有公网default，因此进入后续policy；Exit ON时5270先命中双栈default。现有5209与5210/5230/5250使outer/control提前绕过。

## goto5271的真机内核验证

本轮仅为测试临时让Fog通告（此前管理端批准仍有效），确认Redmi `ExitNodeOption=true` 后原生启用exit。FlClash保持OFF。选用保留文档地址192.0.2.123，仅做内核FIB查询，不发送业务流量。

先确认5268无外来规则，添加完整目的地址/iif/priority/action签名，使用trap按同一签名删除并清空exit。没有修改table52、main、DNS、默认路由或现有规则。

```sh
ip -4 rule add pref 5268 to 192.0.2.123/32 iif lo goto 5271
```

实际输出：

```text
BEFORE:
192.0.2.123 dev tailscale0 table52 src100.118.66.106

WITH-GOTO:
5268: from all to192.0.2.123 iif lo goto5271 [unresolved]
192.0.2.123 dev tailscale0 table52 src100.118.66.106

MARKED（WITH-GOTO期间）:
192.0.2.123 dev ccmni1 src10.167.171.85 mark0x10020000

REMOVED:
192.0.2.123 dev tailscale0 table52 src100.118.66.106
```

这不是仅验证ip语法：`ip route get`发起实际内核路由查询，加入goto前后都命中table52，未进入后续netd表。原bypass mark路径没有改变。

Linux4.14上游 [fib_rules.c](https://github.com/torvalds/linux/blob/v4.14/net/core/fib_rules.c) 为goto寻找**相同priority**的target；target未解析时继续下一条rule。本机没有5271规则，观测的unresolved与此实现一致。它不等于“goto完全不受支持”，但已经否决**直接跳转到不存在的5271**的当前方案。尚未验证跳转到现存目标或其他替代方案。

## 仍须满足的重新设计条件

- 不能默默把goto5271替换成其他目标继续部署。须重新设计并重新真机验证。
- physical DNS的5266/5267意图仍是绑定经netId+iface证明的实际物理表；切网先撤销旧owned rules。它们本轮没有安装。
- VPN策略只接受明确本地前缀与当前运行配置的Fake-IP CIDR；绝不从default扩大推导，不猜本轮/16。
- 任何替代跳转必须有真实、稳定、可验证的目标及删除/替换后的失效处理；不能因目标消失又静默回落至5270。
- 私有ledger、完整signature、冲突拒绝接管、Exit OFF/VPN OFF/切网清理与A–F完整矩阵仍是实现验收前提。本轮未把临时探针称为产品实现。

## 清理结果

临时5268规则已经按完整签名删除；5266/5267没有安装。检查无5266/5267/5268残留、无table52 exit default；Redmi ExitNodeID/IP为空、LAN access=false、FlClash OFF。Fog AdvertiseRoutes恢复为空。daemon仍为PID6665；main工作区在文档记录前干净。

原始只读模型、测试脚本与输出保留在忽略目录 `build/exit-node-acceptance/`。本轮仅提交文档；没有新功能代码、模块安装包、WebUI修改或Release。文档相对链接与 `git diff --check` 验证通过，不以未运行A–F声称策略可用。
