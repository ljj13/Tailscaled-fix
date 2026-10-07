# Gate 1B：VPN local prefix + Fake-IP 双 throw

日期：2026-10-07（Asia/Singapore）；main基线 `1cdb872`。只执行联合门槛，不进入Gate 2或产品实现。

**结果：FAIL_INSTALL_CONFLICT。指定的VPN local prefix throw无法添加；按用户停止条件立即终止联合测试。不能标记scoped throw mechanism=PASS。**

## 实时发现

- ConnectivityService明确VPN transport，当前netId114、iface tun0、OwnerUid10251、underlying111。
- 当前VPN LinkProperties：LinkAddresses172.19.0.1/30，DNS172.19.0.2，明确local route172.19.0.0/30。没有从default推导local prefix。
- 按OwnerUid查包映射，校验该owner生成配置文件的UID，只在内存中提取DNS安全字段。当前enable=true、enhanced-mode=fake-ip，明确声明的CIDR规范化为198.18.0.0/16。没有保存/打印整份代理配置。
- 两个前缀是本轮实时输入，不是功能代码默认值。临时发现脚本与安全摘要保留于忽略目录 `build/exit-node-acceptance/`。
- 临时启用Fog通告并确认Redmi `ExitNodeOption=true`，再原生选择Fog，保持LAN access=false、FlClash ON。

## 新发现：osrouter已经占用同一前缀

启用exit后的实际table52包含：

```text
default dev tailscale0
10.0.0.0/8 dev tailscale0
100.72.239.86 dev tailscale0
100.79.33.7 dev tailscale0
100.100.100.100 dev tailscale0
throw 127.0.0.0/8
172.19.0.0/30 dev tailscale0
```

执行用户指定命令：

```sh
ip route add throw 172.19.0.0/30 table 52
```

结果：`RTNETLINK answers: File exists`，退出码2。同前缀原route仍为 `172.19.0.0/30 dev tailscale0`；`ip route get 172.19.0.2`仍命中table52/tailscale0。

这与前轮Fake-IP-only测试不同：Fake-IP前缀没有同前缀Tailscale route，能够直接添加throw；VPN local前缀已由原生osrouter安装，不能将它当成模块可直接接管的空前缀。

**没有执行replace或删除Tailscale原route，也没有安装第二条Fake-IP throw来掩盖第一条失败。**没有扩大网段、启用LAN access兜底、添加goto或修改其他规则。

## 未完成项与后续边界

由于联合临时条目无法安装，新域名解析、Fake-IP实际网站访问、Chrome完整链路、联合策略公网/Tailnet/mark验证均未执行；不能用先前单throw的PASS代替这些结果。

下一次设计必须先解决“同前缀现有osrouter非throw route”的冲突，并明确如何保留外来/原生route所有权。任何替代签名、替换操作或更具体prefix策略都需要独立验证；本轮没有自动变更指定门槛。

Gate 2、route proto ownership、动态维护、A–F与WebUI selector均未开展。

## 恢复核对

- 第一个add失败，没有模块临时throw需要删除；没有删除原生172.19.0.0/30 route。
- 原生清空exit后，osrouter自行撤销exit default和相关local routes；table52恢复原Tailnet/quad100条目，无本轮throw残留。
- ExitNodeID/IP为空、LAN access=false；FlClash OFF，Fog通告撤销。
- daemon PID6665，BackendState=Running；节点ID、hostname、双栈Tailnet IP不变。
- settings.ini、routes、hostname-initialized SHA256与前轮保存的基线逐项一致。未读取/复制state原文。
- 没有功能代码、安装包或Release。本轮只提交证据文档；相对链接和 `git diff --check` 验证通过。
