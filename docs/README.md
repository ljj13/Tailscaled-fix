# 文档索引

项目使用与安装：[简体中文 README](../README.md) · [English README](../README.en.md)

## 发布与验证

| 文档 | 内容 |
|---|---|
| [发布说明](../RELEASE_NOTES.md) | 当前版本与历史版本入口。 |
| [更新记录](../CHANGELOG.md) | 各版本主要变化。 |
| [WebUI 1](releases/v1.102.5-dnsfix.2-webui.1.md) | 正式版本功能、兼容性、验证与安装资产。 |
| [历史发布说明](releases/HISTORY.md) | v1.102.5、dnsfix.1、dnsfix.2 的完整原始记录。 |
| [测试索引](../TEST_RESULTS.md) | 各子系统验证入口，区分本地检查与真机验收。 |
| [DNS 测试报告](testing/DNS_TEST_RESULTS.md) | dnsfix.1 / dnsfix.2 本地验证、ADB 观测与用户确认的 Redmi 验收。 |

## 实现与诊断

| 文档 | 内容 |
|---|---|
| [DNS 初始审计](dns/DNS_FIX.md) | 缺少 resolv.conf / 本地 DNS listener 的根因与初始修复。 |
| [dnsfix.2](dns/DNS_FIX_2.md) | VPN 底层网络、物理路由、verified DNS 缓存与诊断字段。 |
| [Miuix WebUI](WEBUI_MIUIX.md) | 页面结构、组件、bridge、mock、浏览器测试和历史预览构建。 |
| [Android 设备名](ANDROID_HOSTNAME.md) | 名称来源、规范化、手工命名保护和并发测试。 |
| [开发与目录说明](DEVELOPMENT.md) | 源码职责、构建入口、测试命令及本地产物约定。 |

## 截图

[WebUI 截图库](screenshots/webui/README.md) · [本地 HTML 图库](screenshots/webui/index.html)

截图保留中文界面，来源是桌面 mock，不作为新增真机验收证据。
根目录的 DNS / 测试文档保留为导航入口，兼容旧链接。
