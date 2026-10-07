# Android Tailscale（KernelSU / Magisk 模块）

**简体中文** | [English](README.en.md)

由 **FogPurification** 维护。当前发布版本：
[v1.102.5-dnsfix.2-webui.2](https://github.com/ljj13/Tailscaled-fix/releases/tag/v1.102.5-dnsfix.2-webui.2)。
基于 Tailscale `v1.102.5`，包含已通过 Redmi 真机验收的 Android DNS 修复、
Miuix 风格 WebUI、网络诊断、Android 默认设备名初始化与 outer IPv6 路由修复。

相关文档：[使用指南](docs/USAGE.md)、[发布说明](docs/releases/v1.102.5-dnsfix.2-webui.2.md)、
[DNS 审计与真机测试](docs/dns/DNS_FIX.md)、[WebUI 设计与测试](docs/WEBUI_MIUIX.md)、
[设备名初始化](docs/ANDROID_HOSTNAME.md)、[文档索引](docs/README.md)。

本版包含[网络诊断增强](docs/NETWORK_DIAGNOSTICS.md)：endpoint、DERP 地区、
peer 路径、UDP / NAT 与 outer route，集中展示在 WebUI 网络详情页。

Redmi Note 8 Pro 移动数据 + FlClash OFF/ON 已恢复 **IPv6 direct，实测37–80 ms**。
Wi-Fi/移动数据切换由 watchdog 自动更新物理网络策略；测试中的 IPv4-only Wi-Fi
没有 IPv6 出口，自动撤销旧移动规则并保留 DERP fallback，不强制 direct。
原 fwmark、IPv4 main route、table52/1099、DNS、proxy exemptions 与节点身份保持不变。
详见[IPv6 修复与真机验收](docs/testing/IPV6_MARKED_ROUTING.md)。

本模块在已 ROOT 的 Android 设备上运行独立的 `tailscaled`，通过内核网络接口
让浏览器和其他应用访问 Tailnet，以及其他节点通告的子网。
模块采用 `GOOS=linux` 构建，并针对 Android 的 DNS、路由和 fwmark 做了适配。

项目沿用 [mgksu/tailscaled](https://github.com/mgksu/tailscaled) 的模块实现；
该项目源自 [anasfanani/Magisk-Tailscaled](https://github.com/anasfanani/Magisk-Tailscaled)。
本仓库使用的 `v1.102.5` 基线来自 [keweiya/tailscaled](https://github.com/keweiya/tailscaled)。

## 安装与升级

1. 从[本仓库 Releases](https://github.com/ljj13/Tailscaled-fix/releases)
   下载 `tailscaled-<版本>.zip`。发布附件同时提供 `<文件名>.zip.sha256`，安装前请核对 SHA256。
2. 在 KernelSU / Magisk / APatch 中安装 ZIP，然后重启设备。
3. 通过 WebUI 登录，或执行：

```sh
su -c 'tailscale login'
```

升级时直接覆盖安装新 ZIP，再重启即可。安装器保留现有 Tailscale state、登录身份、
`settings.ini`、手工路由和设备名保护标记，无需退出登录或删除节点。

WebUI 的 MagicDNS 开关用于调整 Tailscale DNS 偏好。Android DNS helper 会获取
物理网络的 DNS，追溯 VPN 的底层网络，并排除 VPN / Tailscale 接口。
它使用独立的 resolver 文件，因此 `/etc/resolv.conf` 缺失、`[::1]:53` 没有监听
不会阻止 daemon 启动，也不会替换 Android / netd 的全局 DNS 配置。

## WebUI

模块提供纯 HTML / CSS / JavaScript WebUI，适用于 KernelSU / APatch，以及兼容的
Magisk WebUI 宿主。支持 WebUI 的管理器可从模块卡片打开界面。

界面参考 Miuix / HyperOS 设置页：大标题、圆角分组卡片、偏好设置行、开关和带返回按钮的
二级页面。主题跟随系统浅色 / 深色模式，适配安全区域，所有资源和字体均使用本地资源或系统字体。

| 页面 | 内容 |
|---|---|
| 首页 | 连接状态、设备 / Tailnet / 账号信息、启动 / 停止 / 重启和登录。 |
| 设置 | 接受子网路由、MagicDNS、Shields up、通告出口节点、设备名和登录 / 退出登录。 |
| 网络详情 | 物理接口、Android VPN 底层网络、selftest 和高级诊断入口。 |
| DNS 诊断 | DNS 来源、网络 / transport、选中与排除的接口、可达性和带 fwmark 的探测结果；保留全部 dnsfix.2 字段。 |
| 路由详情 | main 默认路由、table 52、自动发现 / 手工路由和代理豁免状态。 |
| 日志 | Daemon 与诊断日志，支持刷新、复制和清空。 |
| 关于 | 模块版本、作者、构建信息和功能说明。 |

WebUI 保留现有 service / CLI API。Bridge 使用实际安装路径调用命令，并保留自定义 socket
设置，不依赖系统 overlay 中的命令查找。页面隐藏时暂停状态轮询。

以下截图使用**桌面浏览器 mock 数据**，不是手机实拍。
截图保留中文界面，中英文 README 使用相同图片。
更多页面见[完整截图库](docs/screenshots/webui/README.md)。

| 首页 · 浅色 | 首页 · 深色 | 设置 |
|---|---|---|
| <img src="docs/screenshots/webui/light-home.png" width="260" alt="中文首页，浅色主题，模拟 Wi-Fi 与 FlClash"> | <img src="docs/screenshots/webui/dark-home.png" width="260" alt="中文首页，深色主题，模拟 Wi-Fi 与 FlClash"> | <img src="docs/screenshots/webui/light-settings.png" width="260" alt="中文设置页，浅色主题"> |

| 网络详情 | DNS 诊断 | 路由详情 |
|---|---|---|
| <img src="docs/screenshots/webui/light-network.png" width="260" alt="中文网络详情页"> | <img src="docs/screenshots/webui/light-dns.png" width="260" alt="中文 DNS 诊断页"> | <img src="docs/screenshots/webui/light-routing.png" width="260" alt="中文路由详情页"> |

无需连接手机也可以预览：

```sh
python -m http.server 8765 --bind 127.0.0.1 --directory webroot
```

打开 `http://127.0.0.1:8765/?demo=wifi`。
其他模拟场景为 `cellular`、`needs-login`、`failure` 和 `stopped`。
强制 demo 模式不会调用 native root bridge。

## 常用命令与诊断

以下命令需在 root shell 中运行，也可通过 `su -c '命令'` 调用。

```sh
# 服务管理
tailscaled.service start|stop|restart|status
tailscaled.service routes            # 已安装路由（手工 + 自动发现）
tailscaled.service routes-reload     # 重新读取路由文件并应用
tailscaled.service routes-sync       # 自动发现 Tailnet 子网并应用
tailscaled.service diag              # 完整诊断信息
tailscaled.service dns               # DNS 来源与可达性
tailscaled.service dns-refresh       # 重新获取 Android DNS
tailscaled.service selftest          # 构建、DNS、路由与 ping 诊断
tailscaled.service selftest <peer-ip> # 可选：指定要验证的节点
tailscaled.service netdiag           # 结构化网络诊断 JSON（main 新增）
tailscaled.service webstatus         # WebUI 使用的机器可读状态
tailscaled.service prefs             # 机器可读的 Tailscale 偏好
tailscaled.service log {runs|service|tailscaled|diag}

# 偏好设置白名单：accept-routes、accept-dns、shields-up、
# advertise-exit-node、advertise-routes、hostname、auto-update
tailscaled.service set-pref accept-routes on
tailscaled.service set-pref accept-dns off
tailscaled.service set-pref advertise-routes 192.168.1.0/24
tailscaled.service logout

# Tailscale CLI
tailscale status
tailscale ip
tailscale ping <peer>
```

遇到异常时先运行 `tailscaled.service diag`。它会显示实际 / 预期二进制 hash、daemon 和
watchdog PID、已安装路由与规则、代理的 `DIVERT` 跳转，以及近期日志。
DNS 问题可进一步查看 `tailscaled.service dns`，或在 WebUI 中打开 DNS 诊断页。

## 配置与数据目录

模块目录由管理器安装，升级时会替换；持久数据目录保存配置和节点身份，升级时保留。

```text
/data/adb/modules/tailscaled/              模块目录（升级时替换）
├── module.prop                            id / 名称 / 版本；描述显示运行状态
├── system/bin/tailscale                   CLI 包装器（阻止 tailscale update）
├── system/bin/tailscaled                  daemon 包装器
├── system/bin/tailscaled.service          服务命令包装器
└── META-INF/ customize.sh                 安装器

/data/adb/service.d/tailscaled_service.sh  开机入口（由 customize.sh 安装）

/data/adb/tailscale/                       持久数据目录（升级时保留）
├── settings.ini           路径、TUN 名称、路由表和规则优先级
├── routes                 可选手工前缀（常规路由由 osrouter 管理）
├── bin/
│   ├── tailscale          合并二进制（CLI）
│   ├── tailscaled         合并二进制（daemon）
│   ├── tailscaled.orig    二进制保护机制使用的已知可用副本
│   ├── tailscaled.sha256
│   ├── android-dns        物理网络 / DNS 发现 helper
│   ├── android-hostname   一次性设备名初始化 helper
│   └── android-netdiag    只读网络诊断 helper（main 新增）
├── scripts/               start.sh、tailscaled.service、tailscaled.inotify
└── run/                   state 与日志
    ├── tailscaled.state   节点身份与 Tailscale 偏好（包含登录状态）
    ├── tailscaled.sock
    ├── tailscaled.log     daemon 日志
    ├── diag.log           路由操作诊断日志
    ├── runs.log / service.log
    └── tailscaled.pid / watchdog.pid
```

`settings.ini` 和 `routes` 仅在首次安装时复制，用户修改会在升级后保留。
删除它们后，后续安装会恢复默认文件。

| 要调整的内容 | 配置位置 |
|---|---|
| 手工指定进入隧道的网段 | `/data/adb/tailscale/routes`，修改后执行 `tailscaled.service restart`。 |
| TUN 名称 / 路由表 / 规则优先级（高级） | `/data/adb/tailscale/settings.ini`。 |
| MagicDNS、设备名、Shields up、通告出口节点、接受子网路由 | Tailscale 偏好保存在 `run/tailscaled.state`；通过 WebUI 或 `tailscale set` 修改，例如 `tailscale set --hostname=...`、`tailscale set --accept-routes`。 |
| 代理设置 | 由代理自身管理，参见下方共存说明。 |

启用 `--accept-routes` 后，osrouter 将已接受的子网路由写入 table 52。
手工 `routes` 文件仅作为可选补充，普通子网访问无需手工添加路由。

当 `Prefs.Hostname` 为空时，模块会一次性初始化设备名：优先使用 Android 用户设备名，
其次尝试 marketname / model / device，并规范化为合法的小写 DNS label。
已设置的名称会保留；用户通过 WebUI 或模块 CLI 手工设置设备名后，自动初始化永久停用。
由于 daemon 使用 `GOOS=linux`，Tailscale 管理后台显示的系统仍为 Linux。

## 访问其他节点通告的子网

在本机启用接受子网路由：

```sh
su -c 'tailscale set --accept-routes'
```

也可在 WebUI 中打开**设置 → 接受子网路由**。
同时需要在 Tailscale 管理后台批准对端通告的路由。
批准后，浏览器和其他应用即可访问对应子网，例如 `http://192.168.100.1`。

osrouter 自动把这些路由安装到 table 52。运行 `tailscaled.service routes` 查看，例如：

```text
mode:                    osrouter (linux build) - tailscaled owns the routing
default route in main:   default via 10.20.30.1 dev rmnet_data3
tailnet in table 52:
  100.64.0.0/10 dev tailscale0
  192.168.100.0/24 dev tailscale0
```

以上地址和接口名仅为示例，实际输出取决于当前网络。
如需手工补充前缀，仍可使用 `/data/adb/tailscale/routes`。

## 与代理共存（Surfing / Clash / Mihomo 等）

模块保留现有 Tailnet 路由和代理豁免逻辑。常规共存无需修改代理配置。
模块在启动时、此后每 15 秒检查一次，确保以下三条规则位于相应链的第 1 条：

```sh
iptables -t mangle -I PREROUTING 1 -i tailscale0 -j RETURN   # 隧道入站流量
iptables -t mangle -I OUTPUT     1 -o tailscale0 -j RETURN   # 隧道出站流量
iptables -t nat    -I OUTPUT     1 -o tailscale0 -j RETURN   # REDIRECT 类代理豁免
```

规则只匹配隧道接口，不依赖特定代理名称。未安装代理时也可保留。
`tailscaled.service diag` 可查看豁免是否到位。

### 为什么需要入站豁免

Surfing 的 TPROXY 会在 mangle `PREROUTING` 第 1 条插入 `DIVERT` 跳转：

```sh
iptables -t mangle -I PREROUTING -p tcp -m socket -j DIVERT
```

这条规则只匹配 TCP，却没有接口条件。`DIVERT` 设置 `0x1000000` mark 并 `ACCEPT` 数据包，
导致经 `tailscale0` 返回的 TCP SYN-ACK 被路由到代理的 TPROXY 端口，本地 socket 收不到回复，
TCP 握手无法完成。ICMP 不匹配，所以可能出现“ping 正常，但浏览器 / curl 卡住”的现象。

豁免必须放在 `PREROUTING` 链本身的第 1 条。只在 `BOX_EXTERNAL` 内添加无效，
因为 `DIVERT` 已先执行。

如果在代理配置中使用 Surfing 的 `ignore_out_list=("tailscale0")`，它只能处理出站方向，
不能替代模块的入站豁免。

## 为什么采用 GOOS=linux

本模块需要独立 daemon 和内核级路由。在当前使用的 Tailscale 基线中，
`wgengine/router` 通过 `router.HookNewUserspaceRouter` 获取路由实现；
`osrouter/router_linux.go` 带有 `//go:build !android`，因此 `GOOS=android`
构建不会注册这个实现，独立 daemon 会报错退出：

```text
wgengine.NewUserspaceEngine(tun "tailscale0") error: creating router: unsupported OS "android"
getLocalBackend error: createEngine: creating router: unsupported OS "android"
```

官方 Android App 通过自己的 Java `VpnService` 路由实现注册该 hook。
`--tun=userspace-networking` 虽能启动独立 daemon，但不会提供本模块需要的内核路由，
其他应用无法直接通过它访问 Tailnet 子网。

因此，本模块采用 `GOOS=linux` 并对 Android 做以下适配。

### main 路由与 fwmark

Linux osrouter 为 tailscaled 自身的 socket 设置 bypass mark，规则将其导向 `lookup main`。
Android 的 netd 通常将默认路由保存在每个网络的独立路由表中，`main` 没有默认路由时，
控制平面和 DERP 流量会报 `network is unreachable`，节点无法连接。

模块在 `main` 中维护默认路由及源子网的直连路由，并在 netd 改写路由表后重新检查。
同时处理 fwmark 和代理规则冲突：

| 问题 | 处理方式 |
|---|---|
| 原始 `0x40000` / `0x80000` mark 与 Android netd fwmark 的 permission 位（18 / 19）冲突。 | 构建时通过 `patches/linuxfw-mark.patch` 改为 `0x8000000` 和 `0x10020000`。 |
| TPROXY 的 `DIVERT` 抢先匹配隧道 TCP 回复，造成 ping 正常但浏览器卡住。 | 保持 mangle `PREROUTING` 的 `-i tailscale0 -j RETURN`，以及 mangle / nat `OUTPUT` 的 `-o tailscale0 -j RETURN` 为第 1 条，每 15 秒检查。 |

osrouter 的规则优先级为 5210–5270，位于 netd 的 11000 之前。
它将 Tailnet 前缀和已接受的子网路由安装到 table 52，无需手工管理常规路由。

Android DNS 缺失或误选 VPN DNS 的处理细节见 [DNS 修复文档](docs/dns/DNS_FIX.md)。

## 支持范围与限制

- 仅提供 **arm64** 安装包。
- 不支持 **Tailscale SSH**，构建使用 `ts_omit_ssh`。
- 支持通告本机为出口节点；**不支持选择其他节点作为本机出口节点**，沿用现有服务限制。
- 二进制不使用 UPX 压缩。部分 ROM / SELinux 策略不允许 UPX 所需的可执行匿名内存映射。

## 仓库结构

完整目录职责与开发命令见 [开发说明](docs/DEVELOPMENT.md)。

```text
META-INF/                 安装器
customize.sh              安装到 /data/adb/tailscale，并记录二进制保护信息
service.sh                开机入口，等待启动完成后执行 start.sh
system/bin/               tailscale / tailscaled / tailscaled.service 包装器
webroot/                  KernelSU / APatch WebUI（HTML / CSS / JS）
tailscale/settings.ini    路径、前缀和路由表配置，安装到 /data/adb/tailscale/
tailscale/routes          可选手工路由，安装到 /data/adb/tailscale/
tailscale/scripts/        start.sh、tailscaled.service、tailscaled.inotify
uninstall.sh              停止 daemon 并移除模块路由
```

## 构建与测试

在 Linux / WSL 中使用 Go 1.26.6 和 Python 3，运行 `sh scripts/build.sh`。
构建固定使用 Tailscale `v1.102.5`，应用现有 fwmark 与 Android DNS 补丁，执行相关测试，
并生成 `dist/tailscaled-v1.102.5-dnsfix.2-webui.2-arm64.zip`。
分支构建工作流只生成 artifact，不会自动修改 main 或发布 Release。
模块不接入上游自动更新，避免其他构建替换本模块的 DNS 修复。

正式 WebUI 2 ZIP 复用经过验收的 dnsfix.2 daemon 和 DNS helper 二进制。
严格复现已发布版本时请使用对应 tag。
如需复现该打包流程，先将已验收的发布 ZIP 放入 `dist/`，再在 Linux / WSL 中执行，
并确保 Go 1.26.6 位于 PATH：

```sh
python3 scripts/build-hostname.py
python3 scripts/build-netdiag.py
python3 -m unittest discover -s tests -p 'test_*.py' -v
node tests/webui-command.test.cjs
node tests/network-ui.test.cjs
node tests/webui.test.cjs
python3 scripts/package-webui.py --release
```

打包器检查已验收 ZIP 的 hash、helper 源码 / 二进制 hash、静态 arm64 ELF、Unix 权限、
模块元数据和 ZIP CRC，并生成 `.zip.sha256` 校验文件，在包内记录 UI / helper 构建来源。

浏览器测试工具位于忽略提交的 `build/browser-tools/`。
运行 Node 测试前，在该目录通过 npm 安装 `playwright` 和 `acorn`。
Windows 默认使用已安装的 Edge；Linux 需通过 `WEBUI_BROWSER` 指定已安装的
Chromium 兼容浏览器的绝对路径。这些工具仅用于测试，不随模块发布。

## 致谢

- [keweiya/tailscaled](https://github.com/keweiya/tailscaled)：本模块的 v1.102.5 基线。
- [anasfanani/Magisk-Tailscaled](https://github.com/anasfanani/Magisk-Tailscaled)：原始模块。
- [mgksu/tailscaled](https://github.com/mgksu/tailscaled)：前身 fork。
- [Tailscale](https://tailscale.com)：BSD-3-Clause。
