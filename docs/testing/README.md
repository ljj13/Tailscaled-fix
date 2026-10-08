# 测试与验收记录

| 范围 | 报告 |
|---|---|
| Android真实系统浅/深主题、live/cold启动及稳定发布门槛收口 | [系统主题验收](ANDROID_THEME_ACCEPTANCE.md) |
| 稳定 main 真机 WebUI、网络回归与 Exit DRAFT 安全封存 | [稳定 WebUI 验收](STABLE_WEBUI_ACCEPTANCE.md) |
| 暂停中的 Exit IPv4 TLS 缺段/握手停顿 | [独立 unresolved 记录](EXIT_NODE_IPV4_TLS_UNRESOLVED.md) |
| dnsfix.1 / dnsfix.2 本地验证和 Redmi Note 8 Pro 真机验收 | [DNS 测试报告](DNS_TEST_RESULTS.md) |
| Miuix WebUI 页面、交互、bridge 和 mock 检查 | [WebUI 设计与测试](../WEBUI_MIUIX.md) |
| Android 默认设备名、并发保护与升级保留 | [设备名初始化](../ANDROID_HOSTNAME.md) |
| WebUI 1 正式版本验证汇总 | [发布说明](../releases/v1.102.5-dnsfix.2-webui.1.md) |
| 网络诊断增强与 Redmi ↔ EAIDK direct 排查 | [P2P 排查记录](P2P_DIAGNOSIS.md) |
| outer IPv6 marked routing 修复与 direct 真机恢复 | [IPv6 修复验收](IPV6_MARKED_ROUTING.md) |
| 版本化备份、Release状态机与GitHub只验证运行 | [备份/CI验证](BACKUP_RELEASE_CI.md) |
| 两次真实覆盖安装与首次tag发布/feed事务 | [真实事务验收](RELEASE_TRANSACTION.md) |
| Peers、脱敏报告与切网收敛（main，未发布） | [三阶段验收](PEERS_REPORT_CONVERGENCE.md) |
| Exit Node Client 原生闭环：转发/恢复通过，Android DNS与Fake-IP共存失败 | [Exit Node真机验收](EXIT_NODE_CLIENT_ACCEPTANCE.md) |
| Exit scoped policy只读模型与goto5271内核门槛失败 | [策略建模/内核验证](EXIT_NODE_POLICY_MODEL.md) |
| scoped throw内核路由通过，新域名门槛被VPN DNS阻塞 | [throw Gate 1实测](EXIT_NODE_THROW_GATE.md) |
| VPN local+Fake-IP联合门槛：同前缀osrouter route导致add冲突 | [throw Gate 1B](EXIT_NODE_THROW_GATE_1B.md) |
| VPN DNS host+Fake-IP throw与单carrier DNS policy静态门槛通过 | [Gate 1C / Gate 2](EXIT_NODE_THROW_GATE_1C.md) |

报告分别注明本地测试、助手 ADB 观测和用户确认的真机验收。
第一轮 P2P 报告保留未恢复时的证据；后续 IPv6 报告记录 ADB 实测的 direct 恢复与自动切换验收。

[文档索引](../README.md)
