# Android 系统主题：稳定版本发布前收口

日期：2026-10-08。`RELEASE_READY = YES`（当前 Redmi / KernelSU 稳定版本范围）。

设备：Redmi Note 8 Pro，KernelSU v3.3.0，Android System WebView 140.0.7339.207。APatch 未安装，不声称完成 APatch 真机验收。

使用现有 main；保留 `8999673` 和 `a38525d`，仅增加主题修复 `c346ebee529540e0feb0c831330ed8421e95e1c2`。无新分支、tag、Release 或 update.json 改动。Exit scoped-policy DRAFT 的 ZIP/manifest/stash 保持封存；[Exit IPv4 TLS](EXIT_NODE_IPV4_TLS_UNRESOLVED.md) 仍 unresolved。

## 发现与最小修复

用户通过手机「设置 → 显示与亮度」真实开启深色。UiModeService 明确报告 `mComputedNightMode=true`，但 KernelSU WebView 在返回前台和冷启动后仍报告 `prefers-color-scheme: light`。模块原本仅依赖该媒体查询，因此保持浅色。

已定位到系统主题与宿主 WebView 媒体查询之间的传递层，不推断为网络或 daemon 问题。[Android 官方说明](https://developer.android.com/develop/ui/views/layout/webapps/dark-theme)指出 WebView 的媒体查询受宿主 isLightTheme 属性影响；本轮没有修改宿主 APK/ROM。

修复限于 `webroot/app.js`、`style.css`、新增 `theme.js` 和对应测试：

- 原 CSS 媒体查询保留；真实 manager 中，只读 `/system/bin/timeout 2 /system/bin/dumpsys uimode`，严格读取实际 `mComputedNightMode`。字段缺失、冲突、命令失败不猜测。
- 通过根元素 data-theme 统一应用设计 tokens 与 color-scheme；violet 图标和关闭态 Switch 一并归入 tokens，避免宿主媒体查询与实际 Android 状态不一致时出现混搭。
- 初次打开、返回前台、focus/pageshow、媒体查询变化时同步；仅可见页面增加5秒只读 fallback，忙于用户操作时不做周期查询。原15秒状态/网络和日志 cadence 未改。
- 保留最近有效系统状态；没有有效状态时跟随媒体查询。单次等待2.5秒、single-flight，不因桥接异常不断堆积查询。
- 主题查询与服务命令队列隔离。丢失可选 theme callback 不阻塞 status、prefs、日志或服务管理；桌面 demo 不调用 root bridge。

未修改 DNS helper、fwmark、main default、5209/5210–5270、table52/1099、outer IPv6、proxy exemption、osrouter、daemon 或 FlClash 配置。

## 真实手机页面与截图

这里所有截图均为 ADB screencap 的完整1080×2340手机屏幕，含状态栏和底部手势区。系统主题由手机设置实际选择，没有使用 browser mock 或 CDP media emulation。调试连接只用于导航、读取计算样式和记录事件。

| 页面 | 浅色 | 深色 |
|---|---|---|
| 首页 | [截图](screenshots/webui-theme-20261008/light-home.png) | [截图](screenshots/webui-theme-20261008/dark-home.png) |
| Peers | [截图](screenshots/webui-theme-20261008/light-peers.png) | [截图](screenshots/webui-theme-20261008/dark-peers.png) |
| 设置 | [截图](screenshots/webui-theme-20261008/light-settings.png) | [截图](screenshots/webui-theme-20261008/dark-settings.png) |
| 网络 | [截图](screenshots/webui-theme-20261008/light-network.png) | [截图](screenshots/webui-theme-20261008/dark-network.png) |
| DNS | [截图](screenshots/webui-theme-20261008/light-dns.png) | [截图](screenshots/webui-theme-20261008/dark-dns.png) |
| 路由 | [截图](screenshots/webui-theme-20261008/light-routing.png) | [截图](screenshots/webui-theme-20261008/dark-routing.png) |
| Daemon 日志 | [截图](screenshots/webui-theme-20261008/light-logs.png) | [截图](screenshots/webui-theme-20261008/dark-logs.png) |
| Diagnostics 日志 | [截图](screenshots/webui-theme-20261008/light-diagnostics-log.png) | [截图](screenshots/webui-theme-20261008/dark-diagnostics-log.png) |
| 报告 BottomSheet | [截图](screenshots/webui-theme-20261008/light-report.png) | [截图](screenshots/webui-theme-20261008/dark-report.png) |
| 关于 | [截图](screenshots/webui-theme-20261008/light-about.png) | [截图](screenshots/webui-theme-20261008/dark-about.png) |
| hostname 弹窗 | [截图](screenshots/webui-theme-20261008/light-hostname-dialog.png) | [截图](screenshots/webui-theme-20261008/dark-hostname-dialog.png) |
| 危险操作确认（未执行停止） | [截图](screenshots/webui-theme-20261008/light-danger-dialog.png) | [截图](screenshots/webui-theme-20261008/dark-danger-dialog.png) |

两个主题的8个页面均无全局横向溢出，实际 viewport 和 scrollWidth 同为392 CSS px。详情页、长日志和报告可滚动；卡片、边框、按钮、状态颜色、状态栏和安全区域均已核对。未把锁屏/黑屏或系统设置窗口的早期采集当作模块截图。

| 对比度 | 浅色 | 深色 |
|---|---|---|
| 主文字 / 页面背景 | 14.64:1 | 16.72:1 |
| 副标题 / 卡片 | 5.57:1 | 7.28:1 |
| 主要按钮文字 / 背景 | 5.12:1 | 7.50:1 |

均超过普通文字4.5:1。白色 Switch 滑块为组件既有设计；关闭态轨道会正确跟随主题，不将白色滑块误记为浅色卡片混搭。

## 打开状态与冷启动

- 深色→浅色：PASS。系统设置点击浅色后，同一文档 MutationObserver 记录 data-theme=light 和背景 rgb(244,244,244)，opened 时间不变；返回前台时 Android 报告将原 task 带回，没有重建或刷新页面。
- 浅色→深色：PASS（补充验证使用 Android 原生 `cmd uimode night yes`，不是 browser/CDP 模拟，也不替代上面已完成的两种系统设置 GUI 验收）。同一文档记录 data-theme=dark / 背景 rgb(16,16,16)，opened 时间不变，未刷新或重建页面。随后用原生接口恢复原始浅色并核对实际系统状态。
- 深色冷启动：PASS。force-stop/重新打开 KernelSU（不是 tailscaled），新宿主 PID21731，系统模式2，页面有效深色、背景 rgb(16,16,16)，全部页面完成截图。
- 浅色冷启动：PASS。新宿主 PID29945，系统模式1，有效浅色、背景 rgb(244,244,244)，首页正常连接。

没有精确打点用户点击时刻，不捏造毫秒级切换耗时；5秒轮询仅为缺失宿主事件时的 fallback。

## 回归测试与安装一致性

本轮必要测试：

- theme-ui：真实字段解析、auto/custom 的 computed 状态、字段缺失/冲突、权限失败、宿主媒体查询不一致、有效主题保留、超时、coalescing、late result 与 demo 隔离通过。
- WebUI 浏览器：5个 mock 场景、16个页面/主题组合、26张自动截图；原 Peers/report/bridge/navigation 场景通过。
- 新增 native bridge 浏览器回归：computed Android dark 覆盖 light-only host，live token/violet/switch 和恢复通过；可选主题 callback 永不返回时，状态命令仍能执行。
- 22个原生命令 × 3种 PATH，共66次契约执行通过；network-ui 缺字段/offline/IPv6缺失/timeout 测试通过。
- 5项 WebUI package 测试通过；ES2019解析、git diff --check通过。主题修复不涉及 Go/helper，未无意义重复其完整测试；上轮完整稳定验收仍见 [主报告](STABLE_WEBUI_ACCEPTANCE.md)。
- violet 错色和挂起队列问题均先由回归测试失败复现，再修复到通过。独立只读复核确认问题已解决。

固定来源本地预览 ZIP：`build/theme-acceptance/tailscaled-theme-preview-arm64.zip`。

```text
source: c346ebee529540e0feb0c831330ed8421e95e1c2
SHA256: 381d03ef5e81c882b0abbfd98d5d5caeaea58d59eae1703f4bca01843e92663a
```

13个受保护 core payload 保持相同。此次仅将规范 ZIP 的 WebUI 层同步到上轮已安装的稳定预览模块，没有重装模块或重启 daemon；20个最终实际安装文件与 ZIP 逐项 SHA256 匹配，含 UI manifest、稳定 helper、daemon 和 service。未把 hot-patch 称为重新覆盖安装。

## 服务、网络与恢复

FlClash ON 实测：physical107/ccmni2，VPN108 underlying107；DNS reachable=true，main_default=ok，pre/out/nat exemptions=OK，outer IPv6 active。Fog 从初始 DERP 收敛到 IPv6 direct 73/76ms；Android 原生 DNS 返回 VPN Fake-IP。Exit始终 OFF，没有开启新的 Exit 实验。

最终恢复核对：PASS。FlClash OFF 后 VPN/underlying 清空，DNS reachable=true，main/exemptions/outer IPv6 正常，Fog IPv6 direct 113/46ms。tailscaled PID9359 与本轮基线相同；Self ID、Tailnet 双栈地址、hostname 不变；settings.ini/routes/hostname marker 三份哈希与测试前相同，state 保持0600。

系统夜间模式恢复原值1（浅色，实际页面背景 rgb(244,244,244)），USB保持亮屏恢复原值2。KernelSU临时 debugging 配置逐字节恢复原文件，专用9223转发撤销；临时私有配置备份删除。Fog未作为Exit server，手机Exit仍OFF，没有恢复Exit实验。

源修复只在 main，两个既有稳定 commit 为祖先。推送前再次 fetch 核对远端；常规 fast-forward push，不 force push。发布 workflow 只匹配 tag/手动触发，main push 不创建 Release。本轮不创建 tag，不更新 update.json，不发布。
