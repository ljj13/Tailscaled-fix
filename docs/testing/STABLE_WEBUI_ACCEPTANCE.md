# 稳定 main：WebUI 真机验收与 Exit DRAFT 封存

日期：2026-10-08。设备：Redmi Note 8 Pro；KernelSU v3.3.0，真实模块 WebView。

**发布门槛：PENDING。** 稳定功能、安装和网络回归通过；系统设置实际切换深色模式后，WebView 跟随主题这一项仍待人工补验。本轮没有 tag、Release 或 update.json 修改。

## 代码范围与实验隔离

从干净的 main `36969b9a8e0ea468e6702528e5be93671f9493cf` 开始；pull --ff-only 确认已是远端最新。仅提交稳定 WebUI 修复 `8999673347c4f43aa63488d53264de2bbf25038d`：

- `webroot/peers.js`：忽略 Go 的零值活动时间；缺失时显示未提供，保留有效时间回退。
- `webroot/report.js`、`webroot/app.js`：真实 KernelSU 的 Blob 下载未落盘，改为通过已有串行 exec bridge 保存用户明确导出的脱敏文本；桌面 demo 保留浏览器下载。
- `tests/peers-ui.test.cjs`、`tests/report-ui.test.cjs`：零时间、UTF-8、失败清理和外来文件保护回归测试。

未修改 DNS helper、网络规则、daemon、fwmark、outer IPv6、main default、table52/1099、exemptions、版本号或 Release CI。

### DRAFT 双重封存

外部归档目录：`D:\Project\Tailscaled-fix-draft-archives\exit-scoped-DRAFT-20261008-155455`。

- `draft-source.zip` SHA256：`3f652016b13e09f4e37982498a7c43b9ea0c6e948b8c01d0dfdaa39fee10f85d`。
- `manifest.json` 逐文件记录路径、字节数、SHA256、tracked 标志及基线 commit。
- 精确 stash：`b653b24e47c6e2f97064d6398a400cf365cdb06e`，仍保留，未 pop/drop。
- ZIP 的 20 个源文件逐项解包读回校验，并与 stash 的 tracked/untracked blob 对照；源码可独立从 ZIP 无损恢复。
- NTFS 权限只允许当前用户和 SYSTEM；归档不包含 state、登录密钥、socket、日志、二进制或抓包。现有 build 目录中的运行证据未加入源码归档或提交。
- `RESTORE.md` 提供完整恢复步骤。在兼容的干净工作区使用 `git stash apply --index <精确 stash>`；后续 main 有变化时需要正常处理冲突，不能直接覆盖当前稳定修复。

封存文件清单：

```text
docs/design/EXIT_NODE_DNS_HANDOFF_STATE_MACHINE_DRAFT.md
docs/testing/EXIT_NODE_CANDIDATE_DNS_GATE_3.md
docs/testing/EXIT_NODE_C_CONNECTIVITY_FAILURE.md
docs/testing/EXIT_NODE_IMPLEMENTATION_COMPARISON.md
docs/testing/EXIT_NODE_SCOPED_POLICY_DRAFT.md
docs/testing/README.md
scripts/package-webui.py
scripts/release-ci.py
tailscale/scripts/exit-policy.sh
tailscale/scripts/tailscaled.service
tests/test_exit_policy.py
tests/test_shell.py
tests/test_watchdog_events.py
tests/test_webui_package.py
tools/android-netdiag/candidate.go
tools/android-netdiag/main.go
tools/android-netdiag/policy.go
tools/android-netdiag/policy_test.go
tools/android-netdiag/report.go
tools/android-netdiag/watch.go
```

归档后恢复干净 main，重新从稳定源码构建 helper；没有复用忽略目录中的 DRAFT helper。最终安装没有 exit-policy.sh、5264–5267 或实验 throws。Exit 问题单独记录在 [IPv4 TLS unresolved](EXIT_NODE_IPV4_TLS_UNRESOLVED.md)。

## 真实 WebUI 结果

| 项目 | 结果与证据 |
|---|---|
| Peers | PASS：2 个 peer，Self 不重复；hostname、OS、双栈地址、在线分组、详情与路径均显示；无活跃路径时保守显示 Unknown |
| Ping / RTT | PASS：Fog 实测 IPv6 direct；展示真实 endpoint/RTT，超时不会伪装成功 |
| 复制地址 | PASS：真实系统剪贴板 IPv4 为 100.79.33.7 |
| 报告预览 / 复制 | PASS：完整脱敏报告；剪贴板与预览逐字一致 |
| 报告保存 | PASS：真实 /sdcard/Download/tailscaled-diagnostic-20261008T085005653Z.txt 与预览逐字节相同；未将原始报告提交仓库 |
| 保存失败 | PASS：真实 Android 注入 decoder 失败后，私有临时文件与本次 public .part 清除；外来文件不删除 |
| 停止 / 启动 / 重启 | PASS：停止后 daemon 消失，启动恢复 Running；重启 PID 21405→23568，身份不变 |
| 浅色 | PASS：真实 KernelSU WebView |
| 深色 CSS / WebView 渲染 | PASS：真实 WebView 经 CDP 模拟 prefers-color-scheme: dark；桌面浅/深主题浏览器测试通过 |
| 实际系统主题跟随 | PENDING：cmd uimode night yes 返回开启，但此 ROM 的 WebView 仍报告浅色；不能用模拟结果替代真实系统设置切换 |

报告导出先检查 redaction/header，再分块 base64 传入既有 exec；私有暂存目录 0700，不读取 tailscaled.state。固定导出目录与受限文件名，避免原始报告进入 shell 语法。公有 Download 文件是用户明确选择保存的脱敏报告；仍保留排障所需的 IP、hostname、接口和 DERP 信息。

截图来自真实 KernelSU WebView：

- [Peers 浅色](screenshots/webui-stable-20261008/peers-light.png)
- [Peer 详情](screenshots/webui-stable-20261008/peer-details.png)
- [Ping / RTT](screenshots/webui-stable-20261008/peer-ping.png)
- [脱敏报告预览](screenshots/webui-stable-20261008/report-preview.png)
- [停止服务](screenshots/webui-stable-20261008/service-stopped.png)
- [深色渲染（CDP 模拟，非系统主题验收）](screenshots/webui-stable-20261008/peers-dark-emulated.png)

## 网络回归矩阵（Exit 始终 OFF）

| 状态 | physical / VPN underlying | DNS / 路由 / exemption | Tailnet 路径 |
|---|---|---|---|
| 移动 + FlClash ON | net106 / ccmni2；VPN107 underlying106 | PASS；DNS reachable，main 正常，pre/out/nat 正常 | IPv6 direct 55–71 ms，初始 DERP 后收敛 |
| IPv4-only Wi-Fi + FlClash ON | net108 / wlan0；VPN107 underlying108 | PASS；撤销旧 outer IPv6 5209，保留 fallback | Fog 172.31.180.136:41641 IPv4 direct 2–4 ms |
| Wi-Fi→移动 + FlClash ON | 新 net109 / ccmni3；VPN107 underlying109 | PASS；outer IPv6 更新为当前网络，无旧 netId/table 残留 | IPv6 direct 41–51 ms |
| 移动 + FlClash OFF（稳定后） | net109 / ccmni3；无 VPN | PASS；Android 原生 DNS，main/table52/1099/exemption 正常 | IPv6 direct 48–80 ms |

切网期间不重启 daemon，PID 23568 不变。DNS 的短暂缓存不当作稳定 OFF 快照，等待 watchdog 更新后核对 ConnectivityService 和 dns_active_vpn。

原生 Android resolver 实测：VPN ON 得到 Fake-IP 并可达；VPN OFF 得到正常公网域名地址。普通 kernel Tailnet ping 通过；netcheck 与显式、校验证书的公共双栈 HTTPS 探针成功，IPv4 HTTP200（1.153秒），IPv6 HTTP200（0.761秒）。这些测试没有启用 Exit，也不代表 unresolved Exit TLS 问题已解决。

## 安装与配置保留

最终预览包已通过 ksud module install 覆盖安装并重启。重启后：

- BackendState=Running，Self ID、登录身份、hostname `redmi-note-8-pro` 不变。
- Tailnet IPv4 `100.118.66.106`；IPv6 `fd7a:115c:a1e0::ce2e:426b` 不变。
- settings.ini、routes、hostname-initialized 的 SHA256 与安装前相同；state 仍 0600。state 正常运行会写入，不要求其内容哈希不变，也未读取/归档原文。
- 最终 ZIP 中 19 个实际安装文件逐字节哈希匹配；包括 WebUI/manifest、service 脚本、稳定 helper 和 daemon。customize.sh 由安装器消费，service.sh 按原设计迁往 service.d，module.prop 可由管理器追加字段。
- 最终装包再核对 Peers、复制和保存；DNS reachable=true，main/exemptions 正常，marked IPv6 仍在当前物理接口，重启后从 DERP 收敛回 IPv6 direct。

## 自动化与一致性

- Python unittest：69 项通过。
- Go：android-dns 20 项、android-netdiag 23 项，fresh -count=1 -race 和 go vet 通过；android-hostname 无独立 Go tests，由 Python 覆盖。
- 固定 Tailscale 源码的 net/dns、net/dnscache、net/netns、wgengine/router/osrouter 四包通过；bootstrap race 和两个 resolver fallback 场景通过。
- ShellCheck、git diff --check、ES2019 兼容检查通过。
- WebUI 浏览器：5 个 mock 场景，16 个页面/主题组合、26 张自动截图；Peers、report、缓存、字段缺失、offline、无 IPv6/DERP、timeout 和 HTML 安全通过。
- 命令契约：22 个命令 × 3 种 PATH，共 66 次执行通过。
- 脱敏 canary 泄漏测试通过；真实导出报告未出现测试扫描的 key/auth 标记。保存失败清理在 POSIX shell 实测，Windows 不伪称执行 POSIX 文件系统测试。
- 两项修复均先观察回归测试失败，再修复到通过；另经独立只读复核。

本地 release-ci test 首次在缺 shellcheck、随后缺 Linux node 时停止；安装工具后，剩余检查分别执行通过。浏览器使用 Windows Edge。这里报告完整检查组件的结果，不声称一次 Linux CI 入口调用全绿，也未启动发布事务。

## 预览包

代码来源：`8999673347c4f43aa63488d53264de2bbf25038d`；后续提交仅为本文和截图，不改变安装代码。版本仍为现有 v1.102.5-dnsfix.2-webui.3，没有增加正式版本/tag。

本地路径：`build/stable-webui-acceptance/tailscaled-stable-main-preview-arm64.zip`；同目录 .zip.sha256。

```text
7657fb0d9a8e1b9185c75187e8fe56cd718b8b005bec0e3aae14aa4128396ef1
```

使用固定版本源码/二进制、规范 Linux 打包。Windows 和 Linux 容器压缩结果可能不同；已核对解压后的 payload 相同，以本次 Linux 构建作为预览包标准。

## 最少补验与收口

1. 手机系统设置开启深色模式，重新打开 KernelSU → Tailscale WebUI，确认背景/卡片/文字切换；再恢复浅色。
2. 在设备页 Ping Fog，确认显示实际路径/RTT；网络页导出报告，点击复制/保存。
3. 必要时终端确认：`su -c 'tailscaled.service selftest'` 和 `su -c 'tailscale ping 100.79.33.7'`。

USB 保持亮屏恢复为用户要求的永久值2；系统夜间模式恢复原值no；KernelSU 临时 WebView debugging 配置已逐字节恢复原件，专用9223转发撤销。手机最终移动数据、FlClash OFF、Exit OFF，服务 Running；Fog AdvertiseRoutes=null，保持原角色。

Exit DRAFT 没有合入；主题补验前发布门槛保留 PENDING，之后仍等待用户确认发布。
