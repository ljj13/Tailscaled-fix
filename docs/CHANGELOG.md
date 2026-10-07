# 更新记录

完整功能说明和验证范围见 [发布说明](releases/README.md)。

## v1.102.5-dnsfix.2-webui.2 · 2026-10-07

- 修复 outer IPv6：按当前 physical netId 验证并引用真实 netd IPv6 表，保留原 fwmark 与 IPv4 路由；网络变化时自动撤销/更新规则和 Re-STUN。Redmi 移动数据 + FlClash OFF/ON 已恢复 IPv6 direct，见 [验收报告](testing/IPV6_MARKED_ROUTING.md)。
- 增加只读 Tailscale 网络诊断采集器，统一 selftest / diag 与 WebUI 网络详情。
- 展示 endpoints、DERP 地区、peer 状态与路径、UDP listener、netcheck 和 outer route。
- 区分 home DERP、实际路径和未知状态；补充 JSON 缺字段、离线、超时与私钥保护测试。

[设计与用法](NETWORK_DIAGNOSTICS.md)

[发布说明与真机验证范围](releases/v1.102.5-dnsfix.2-webui.2.md)

## v1.102.5-dnsfix.2-webui.1 · 2026-10-07

- 重构 Miuix / HyperOS 风格 WebUI，提供七个页面、浅色 / 深色主题和本地 mock。
- 修复管理器 shell 的命令查找，bridge 使用实际安装路径并保留自定义 socket。
- 增加 Android 默认设备名初始化，仅在 hostname 未设置时执行，永久尊重手工命名。
- 作者更新为 FogPurification；复用已验收 dnsfix.2 daemon / DNS helper。

[完整说明](releases/v1.102.5-dnsfix.2-webui.1.md)

## v1.102.5-dnsfix.2 · 2026-10-06

- 追溯 Android VPN underlying network，排除 tun / Tailscale 接口。
- 使用物理网络 LinkProperties 维护 main 路由，切换期间保留同网络 verified DNS。
- 分离网络发现与探测预算，增加选网和 marked route 诊断。
- Redmi 移动数据、VPN 切换和 Wi-Fi + FlClash 真机验收通过。

[设计与根因](dns/DNS_FIX_2.md) · [验收记录](testing/DNS_TEST_RESULTS.md)

## v1.102.5-dnsfix.1

- 修复缺少 `/etc/resolv.conf` 和本地 DNS listener 时的 daemon 启动解析。
- 增加 Android DNS 发现、marked probe、独立 resolver 文件和 watchdog 刷新。
- 升级保留节点身份、配置和 state 权限，增加 DNS 诊断。

[初始审计](dns/DNS_FIX.md)

## v1.102.5 基线

GOOS=linux / osrouter、Android fwmark 适配、main 默认路由、代理豁免、二进制保护、
升级和 selftest 的历史说明见 [原始发布记录](releases/HISTORY.md)。
