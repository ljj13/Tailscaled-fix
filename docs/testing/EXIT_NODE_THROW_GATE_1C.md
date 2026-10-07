# Gate 1C 与 Gate 2 真机验证

日期：2026-10-07（Asia/Singapore）；main基线 `31c778f`。Redmi Note 8 Pro、Fog。仅临时策略验证；无产品代码、WebUI、安装升级或Release。

**Gate 1C = PASS；Gate 2 = PASS。** 可继续设计产品机制；尚不满足发布或进入WebUI selector条件。动态发现/更新、ownership/proto、A–F尚未验收。

## 实时发现与作用域

- VPN由ConnectivityService的VPN transport识别，iface tun0、OwnerUid10251、underlying physical netId111。重复测试期间VPN重建，netId随之变化，不作为固定配置。
- 全部当前VPN LinkProperties DNS地址只有172.19.0.2；IPv6 DNS不存在。校验其位于明确LinkAddresses/local prefix172.19.0.0/30内，不从VPN default推导。
- 按实时OwnerUid查询唯一包映射，验证生成运行配置的文件owner UID，只提取DNS模式/CIDR安全字段。当前fake-ip模式明确声明198.18.0.1/16，规范化为198.18.0.0/16。配置原文没有保存/打印。
- 本轮地址是实时测试输入，未写入产品默认值。IPv6 /128 throw未实测，不能扩大PASS到未出现的IPv6 VPN DNS。
- Fog临时通告后，Redmi status JSON确认ExitNodeOption=true；原生选择Fog，全程LAN access=false。

## Gate 1C：DNS host + Fake-IP scoped throw

在添加前逐一确认table52没有相同host/Fake-IP前缀的外来route。原生osrouter的 `172.19.0.0/30 dev tailscale0` 保持原样。

本轮精确新增条目：

```sh
ip -4 route add throw 172.19.0.2/32 table 52
ip -4 route add throw 198.18.0.0/16 table 52
```

| 目标 | 添加前 | 添加后 |
|---|---|---|
| VPN DNS172.19.0.2 | tailscale0/table52 | tun0/table tun0，src172.19.0.1 |
| 新Fake-IP198.18.0.5 | tailscale0/table52 | tun0/table tun0 |
| 普通公网1.1.1.1 | tailscale0/table52 | tailscale0/table52 |
| osrouter172.19.0.0/30 | dev tailscale0 | 原条目未变 |

### DNS与实际HTTPS

- Android原生解析 `www.example.org` 得到新的198.18.0.5；合成ICMP成功（0.398ms），仅作为辅助证据。
- HTTPS探针显式使用当前VPN DNS，实际TCP连接 `172.19.0.1:39908 → 198.18.0.5:443`；加载Android系统CA，保持TLS验证，返回HTTP200与Example Domain页面。
- UDP抓包：tun0查询/响应172.19.0.2:53；Fake-IP443的握手与TLS数据均在tun0，没有同目标被tailscale0捕获。
- TCP DNS实连 `172.19.0.1:57312 → 172.19.0.2:53`，查询example.net成功得到新的198.18.0.6。该TCP实连及结果记录与host route命中tun0一致；UDP包与Fake-IP TCP包另有接口抓包。
- 探针IPv6对该域名无AAAA结果；通用GOOS=linux默认resolver仍因没有resolv.conf报localhost错误。它们不等于Android netd DNS失败，不隐去诊断中的这些预期边界。

### Chrome与FlClash upstream

- Chrome打开新URL `https://example.net/?gate=1C`，实际页面成功显示；随后重复测试新URL `https://example.com/?gate=1C-final` 也成功。UI XML在Exit ON/throws存在时采集。
- Chrome UID10255实际连接127.0.0.1:7890，socket mark记录为0x73；本地proxy端为FlClash UID10251。这里不宣称Chrome自身TCP直接连接Fake-IP，当前Chrome使用的是显式localhost proxy。
- 同窗口VPN DNS的example.net A查询由tun0发出并返回198.18.0.6；透明Fake-IP分支的真实TLS网站访问由上述独立探针验证。两种入站路径分开记录，不能用ping替代Chrome页面加载。
- FlClash新upstream socket具有mark0x60000，例如Tailnet源100.118.66.106连接51.75.77.164:6001、109.61.127.36:6001；DNS/DoH upstream也有Tailnet源地址。全协议元数据抓包确认Clash upstream经tailscale0，localhost proxy经lo。
- 另有启用exit前建立的旧socket仍显示物理源地址；部分抓到的这类旧源数据也从tailscale0发出。因此源地址或一条route get都不能单独决定所有既存连接路径。没有把所有socket归并成同一链路。
- 成功的DNS、HTTPS与Chrome请求没有出现本地VPN/exit循环；此静态窗口不证明所有应用、所有未来切网时序均无loop。

### Tailnet/control与清理

- 到Fog仍为IPv6 direct `[2001:250:3c00:3487:a964:98cc:81ea:184]:41641`，45ms。
- Tailscaled control socket保持物理源10.167.171.85、mark0x10020000；IPv6 UDP listener的mark也为0x10020000。daemon仍PID6665。
- 只按throw/type/精确prefix/table52删除本轮两条条目，没有删除osrouter /30。两次重复测试的删除前后table52均经cmp验证 `TABLE52_EXACTLY_RESTORED`。
- 删除throws后，DNS和Fake-IP重新命中table52，确认临时作用确实撤销；再原生清空exit并关闭FlClash/Fog通告。

## Gate 2：单个physical carrier DNS scoped policy

Gate 1C通过并清理后，重新只读验证：现有android-dns discovery选择physical netId111、ccmni1。所选父LinkProperties明确包含carrier DNS120.80.80.80。

原生fwmark selector与netId111一致、lookup ccmni1；该表default dev ccmni1与接口一致。读取netd的 `/data/misc/net/rt_tables` 得到实际numeric table1003；没有用netId/ifindex猜表号。安装前再次检查netId+iface未变，5266/5267无外来规则。

FlClash OFF、Exit ON、LAN access=false，新增：

```sh
ip -4 rule add pref 5266 to 120.80.80.80/32 iif lo lookup 1003
ip -4 rule add pref 5267 to 120.80.80.80/32 iif lo unreachable
```

| 检查 | 真机结果 |
|---|---|
| 添加前carrier DNS route get | tailscale0/table52 |
| 添加后carrier DNS route get | via10.167.171.85，dev ccmni1/table ccmni1，src10.167.171.85 |
| 实际carrier DNS查询 | ccmni1发往120.80.80.80:53，收到example.com的A/AAAA响应 |
| 普通1.1.1.1 | 仍tailscale0/table52 |
| 1.1.1.1 mark0x10020000 | 仍ccmni1物理路径 |
| 普通HTTPS公网出口 | IPv4 218.17.40.113；IPv6 Fog的2001:250:…:184；两族源地址为Tailnet地址 |
| 按完整signature删除5266/5267后 | carrier DNS重新命中table52；无规则残留 |

Gate 2只验证一个当前IPv4 carrier DNS；没有通过破坏physical table来测试5267故障分支，也没有声称所有DNS/IPv6/failover已经完成产品验收。

## 最终恢复与后续门槛

- Redmi exit ID/IP为空、LAN access=false、FlClash OFF；Fog AdvertiseRoutes恢复为空，管理端批准记录保留。
- table52只有原Tailnet/quad100条目；无host/Fake-IP throws、exit defaults或5266/5267/5268残留。
- daemon PID6665、BackendState=Running，节点ID/hostname/双栈Tailnet IP不变；安全prefs与保存基线一致。
- settings.ini、routes、hostname-initialized哈希一致；未读取/复制state原文。
- 所有临时条目仅用于静态门槛，测试ledger位于root私有临时目录；没有将其视为已完成产品级ownership。route proto标记、osrouter同前缀重写后ownership丢失、foreign-satisfied、原子更新、动态清理尚未验证。
- 后续产品候选范围仅为：physical DNS5266/5267、VPN LinkProperties DNS对应/32或/128 throw、明确运行配置Fake-IP CIDR throw。**不对整个VPN local prefix加throw，不使用goto。**
- A–F全部PASS之前不实现WebUI selector、不发布。原始安全摘要、临时脚本、接口元数据和Chrome UI XML在忽略目录 `build/exit-node-acceptance/`；本轮提交只包含文档，链接与git diff检查通过。
