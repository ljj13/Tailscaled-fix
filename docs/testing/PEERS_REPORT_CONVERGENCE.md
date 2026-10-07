# Peers / 脱敏报告 / 网络收敛三阶段验收

日期：2026-10-07。串行基于 main 开发，未创建分支、tag 或 Release，版本与 update.json 未变。

## 阶段 1：设备页面

提交 `79ac48a`。改动：webroot 的 app.js / index.html / style.css / demo.js，新增 peers.js；tests 的 peers-ui.test.cjs、webui.test.cjs、webui-command.test.cjs；docs/PEERS.md。

独立页面按在线、离线、未知分组，排除 Self；展示 hostname、双栈 Tailnet IP、OS、当前路径/endpoint/DERP、最近活跃及 Exit/Subnet 能力。支持复制、详情和有界 ping/RTT。idle/offline 不把 Home DERP 当作当前数据路径，缺字段与旧 JSON 不产生假成功。

验收：8 组 Peers 单元测试；21 命令 × 3 PATH = 63 次命令执行；浏览器 16 个页面/主题组合、5 个状态场景、26 张截图，含 native timeout/invalid JSON/缓存/XSS 与 ping/copy/details。既有网络 UI 与打包回归通过。

## 阶段 2：脱敏报告

提交 `a93fcfb`。改动：tools/android-netdiag 的 report.go / report_test.go / main.go；service 仅新增 report API；webroot 新增 report.js 并调整 app.js / demo.js / index.html / style.css；tests 新增 report-ui.test.cjs，更新 netdiag/browser/command tests；docs/DIAGNOSTIC_REPORT.md。

所有 section、stderr、日志、错误汇合后通过同一个最终脱敏器。status JSON 和 prefs 白名单投影；不读取 state/socket/settings 原文。识别 auth/login/OAuth URL、keys/tokens/headers、损坏或多行秘密字段、PEM 和完整 state dump；UI 可额外明确隐藏秘密值。IP、hostname、接口、DERP 和 route 保留。报告本地预览/复制/保存，不上传。

验收：7 项新增 Go 报告测试（阶段总 16 项），9 类基础 canary，另含 malformed/multiline/PEM/full-state/final-report 测试，零 canary 泄漏；3 组 UI 显式秘密值测试；2 项新增 service 故障测试；22 命令 × 3 PATH = 66 次；完整浏览器通过。

真机补测：生成约 66 KB 报告，包含可用网络摘要和 2 个 `<unavailable: no data>`，其余项继续。补测发现 module.prop 版本被 DNS 专用解析器过滤，阶段 3 收口时修正为只读取精确 `version=`，并将版本断言加入现有完整报告测试。复测头部显示实际已安装 `v1.102.5-dnsfix.2-webui.2`，recognized credential 扫描无泄漏。该显示修正不触及网络。

## 阶段 3：先测量，后最小优化

环境：Redmi USB `pnq47xf6899t4huc`；移动数据有公网 IPv6；保存 Wi-Fi 为 IPv4-only；对端 EAIDK-310，Tailnet IPv4 `100.72.239.86`。四种切换均先采基线，再复测。只重启 watchdog 部署观察器，tailscaled PID 全程 **6693**，不重启 daemon、不主动重绑 listener。

采样在手机上用单调时间进行，Android discovery 约 400 ms、rule 约 250 ms、ping 约 1.5 s 一次。表内为观测值，不是精确协议耗时或多次平均。首次 direct 取新 Android selection 后完成的 ping，保留 DERP fallback。一次 strace 辅助样本明显拖慢 shell，完全排除出正式对比。

| 切换 | before | after（最终代码） | 说明 |
|---|---|---|---|
| Wi-Fi → mobile | 操作→direct **17.862 s**；physical→direct **16.515 s** | **6.829 s**；**5.857 s** | IPv6 direct，首个 RTT 34 ms |
| mobile → Wi-Fi | physical→撤销旧 IPv6 rule **9.973 s** | 慢建链样本 **12.563 s** | IPv4-only：direct N/A；周期 fallback 正确完成 |
| mobile + FlClash OFF → ON | VPN selection→下一次 direct **0.407 s** | **1.164 s** | 两边均保持 direct；采样差异，不是重连耗时 |
| mobile + FlClash ON → OFF | **0.364 s** | **0.186 s** | 两边均保持 direct |

补充样本：最初优化样本 Wi-Fi→mobile 操作→direct 8.548 s，重复样本 6.927 s；较快 mobile→Wi-Fi 样本 physical→撤销 IPv6 rule 3.614 s。保留慢 Wi-Fi 样本，不宣称所有切换均改善。

### Wi-Fi → mobile 时间线（采样起点后 ms）

| 事件 | before | after（最终代码） |
|---|---:|---:|
| 发出切换操作 | 5024 | 5001 |
| Android 选到 mobile physical | 6371 | 5973 |
| watchdog 首次处理 | 18538 | 7040 |
| main physical route 修复日志 | 19341 | 8447 |
| outer IPv6 rule 可见 | 20611 | 9424 |
| outer sync / Re-STUN 分支日志 | 20752 | 9653 |
| DNS refresh 完成日志 | 21154 | 10054 |
| 首次 direct ping 完成 | 22886 | 11830 |

Re-STUN 时间为未修改 sync_outer_ipv6 在调用前的日志边界；实际调用未单独 ptrace 计时，发生在该日志至随后 DNS 完成之间。Self.Addrs snapshot 的变化保存在数据附件；这些候选 endpoint 有时早于正确 marked route，不能把“出现 endpoint”当作已可达。基线 physical→watchdog 12.167 s，主要可减少的等待来自 polling，后续 route/STUN/disco 仍需要时间。

最终 direct endpoint `[2001:250:3c00:3487:add3:4002:9c55:b820]:41641`。稳定采样约 32–49 ms，最终另一次 idle 后 ping 为 53 ms。空闲后短暂 DERP 仍可能出现，继续 ping 可恢复 direct；本优化不强制 direct、不替换 Tailscale 握手逻辑。

证据：[结构化时间线](data/direct-convergence-2026-10-07.json)，含 11 组未附加 strace 的样本、netId/interface/VPN underlying、outer status/rule、DNS、Self.Addrs 与 magicsock endpoint 日志。

### 不回退与 fallback

- settings、routes、tailscaled.state 的升级前/后 SHA256 一致；节点/Tailnet IP `100.118.66.106` 和 hostname 不变。
- 最终 mobile + FlClash ON：Backend Running，physical `115/ccmni1`，VPN `116` underlying `115`，DNS reachable；main default、table52/1099、pre/out/nat exemptions 正常。
- marked IPv6 route 命中 ccmni1；netcheck UDP=true、IPv4=yes、IPv6=yes。
- 终止观察器后，DNS 检查时间仍从 10:07:58Z 推进到 10:08:14Z，watchdog 周期检查继续；随后恢复观察器。测试也覆盖 helper 缺失/退出、sleep 唤醒、burst 和退出清理。
- DNS helper、routing/exemption/start/stop 函数与阶段 2 提交逐字比较一致；只改 watchdog 调度。变化是同步可提前发生，不改变规则或流量选择算法。

## 最终检查

- Python 全套 **68 tests PASS**（含 3 项新增 watchdog tests）。
- android-netdiag **20 Go tests PASS**，race/vet PASS；android-dns race PASS。
- Peers 8 组、报告 UI 3 组、命令 66 次、网络 UI 回归 PASS。
- 浏览器 **16 page/theme combinations、5 scenarios、26 screenshots PASS**。
- ShellCheck（仓库既有 exclusions）、shell syntax、打包一致性与受保护函数比对 PASS。
- 本地构建 `build/tailscaled-peers-report-convergence-preview-arm64.zip` 与 SHA256，13 项原始核心文件逐字保留；仅预览包，不作为正式 Release/升级 feed。
- 没有改 DNS helper、daemon 二进制、fwmark、route/exemption 算法、版本、CI 或 update.json；未发布 Release。

## 预览 / 最少验证

桌面：`python -m http.server 8765 --directory webroot`，访问 `http://localhost:8765/?demo=cellular#peers`；网络页可导出 mock 报告。浅/深色截图由浏览器测试生成在 `build/webui-screenshots/`。

手机更新到包含本轮源码的构建后：

```sh
su -c 'tailscaled.service webstatus'
su -c 'tailscale ping --c=10 --timeout=3s 100.72.239.86'
su -c 'tailscaled.service report'  # 输出已脱敏，但保留地址/设备名
```

切网时观察 `diag.log` 的 `network-observer` / `watchdog: verified physical network event`；移动数据应重新 direct，IPv4-only Wi-Fi 允许 DERP。Peer 页面与导出交互本轮通过桌面/mock/native-fixture 浏览器验证；真机网络使用只更新 service/helper 的部署，不把旧 WebUI 视为新页面真机验收。
