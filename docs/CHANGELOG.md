# 更新记录

完整功能说明和验证范围见 [发布说明](releases/README.md)。

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
