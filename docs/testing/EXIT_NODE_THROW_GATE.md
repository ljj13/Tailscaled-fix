# Exit Node scoped throw：Gate 1 实测

日期：2026-10-07（Asia/Singapore）。main基线 `4c86996`。遵循用户的新方案：废弃goto，不创建5271 sentinel，不更换goto target；两项门槛全通过才实现产品机制。

**结果：Gate 1 路由部分PASS，新域名解析/访问门槛FAIL。按停止条件不实现产品机制；Gate 2、route proto ownership验证及A–F未执行。**

## 前置

- Redmi USB连接，当前原生prefs没有选择exit、LAN access=false，FlClash基线OFF。
- 临时启用FlClash并实际确认当前VPN netId113、iface tun0、underlying111/ccmni1。
- 再次只提取当前VPN owner运行配置的DNS安全字段，确认enable=true、fake-ip模式、明确CIDR规范化为198.18.0.0/16。该值用于本次测试，不是全局默认。
- 临时让Fog通告exit，实际确认Redmi `ExitNodeOption=true` 后启用exit，保持LAN access=false。
- 先确认table52没有同前缀外来条目。临时脚本trap按完整目的前缀/type/table签名撤销，并清空exit；没有添加VPN DNS前缀throw、carrier DNS规则或其他辅助策略。

## 精确临时条目及内核结果

```sh
ip route add throw 198.18.0.0/16 table 52
```

| 检查 | 真机结果 |
|---|---|
| 添加前 `ip route get 198.18.0.4` | tailscale0 table52，src100.118.66.106 |
| 添加后同目标 | tun0 table tun0，src172.19.0.1 |
| 添加后 `ip route get 1.1.1.1` | tailscale0 table52，src100.118.66.106 |
| 添加后直接ping198.18.0.4 | 成功，0.640ms；仅证明Fake-IP合成ICMP，不等于真实网站HTTP成功 |
| 完整签名删除throw后 | 198.18.0.4恢复tailscale0 table52 |

这证明当前内核中table52 throw确实结束当前表查找，后续Android/netd VPN policy接管，不会把普通公网default一起绕开。不同于上一轮的未解析goto。

## 新域名功能检查与抓包

在throw存在、Exit ON/FlClash ON时执行：

```sh
timeout 12 ping -c 1 -W 2 example.net
```

结果：`ping: unknown host example.net`，命令退出码2，没有得到新的Fake-IP，无法继续声称该域名访问成功。

同一窗口抓包：

```text
23:21:28 tailscale0 Out 100.118.66.106:15958 → 172.19.0.2:53
           transaction45041 A? example.net
23:21:33 tailscale0 Out 相同查询重试
```

同时 `ip route get 172.19.0.2`仍是tailscale0/table52。**失败发生在VPN DNS查询，不能归因为Fake-IP throw无效，也不能用现存Fake-IP的ping成功替代新域名门槛。**

本次严格验证指定的单一Fake-IP throw。没有偷偷补172.19.0.0/30 throw来让门槛成功；没有改LAN access或bootstrap。physical carrier DNS规则也不会解决发往VPN本地DNS的这一条查询路径。

后续若调整门槛，需要明确将“当前LinkProperties证明的VPN DNS本地前缀throw”作为新域名解析前置，再分别验证Fake-IP数据路径和真实网站访问。当前报告不将这一尚未执行的组合视为通过。

## 清理与交付

- 已按 `ip route del throw 198.18.0.0/16 table52`撤销临时条目，行为恢复检查通过。
- Redmi清空exit、LAN access=false，FlClash恢复OFF；table52没有exit default或本次throw残留。
- Fog撤销exit通告；原管理端批准记录保留。
- 没有安装5266/5267、没有新goto或sentinel。既有5209、5210/5230/5250/5270、main、fwmark、DNS helper、routes文件均未修改。daemon仍为PID6665。
- 原始脚本/输出及抓包元数据保留在忽略目录 `build/exit-node-acceptance/`。未提交代理配置、state或秘密字段。
- 本轮只有文档，链接与git diff检查通过。没有产品代码、模块安装包、WebUI selector或Release；不满足进入selector门槛。
