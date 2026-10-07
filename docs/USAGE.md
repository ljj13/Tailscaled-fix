# 使用指南

适用于本仓库的 KernelSU / Magisk / APatch 模块。先完成模块登录，确认 WebUI
显示“已连接”。通信双方需加入同一 Tailnet，并获访问策略授权；目标服务也必须启动。

示例中的手机地址为 `100.101.102.103`，Linux 节点地址为 `100.101.102.104`，
均需换成自己的 Tailnet IP。手机地址可从 WebUI 首页或以下命令取得：

```sh
su -c 'tailscale ip -4'
```

启用并可解析 MagicDNS 时，也可用设备名称代替 IP。以下命令是手工操作示例，
安装模块不会自动开启 SSH、ADB 或 Web 服务。

## 1. SSH over Tailscale

这是通过 Tailnet 连接已有的 OpenSSH 服务。当前模块以 `ts_omit_ssh` 构建，
不提供 Tailscale SSH 服务；普通 `ssh` 仍可使用，不需要开启 `tailscale set --ssh`。
两者的区别见 [Tailscale SSH 官方说明](https://tailscale.com/kb/1193/tailscale-ssh)。

目标 Linux 节点需要运行 SSH 服务，并允许 Tailnet 来源访问 SSH 端口。从电脑或
Termux 连接，`alice` 换成目标机器的实际系统用户名：

```sh
ssh alice@100.101.102.104
# 非默认端口
ssh -p 2222 alice@100.101.102.104
```

登录使用目标 SSH 服务的密码或密钥，不使用 Tailscale 账号密码。
Termux 连接前可执行 `pkg install openssh`；在 Termux 使用 SSH 客户端无需 root。

## 2. Termux SSH

在手机 Termux 中执行，不要在 `su` shell 内安装或运行这些命令：

```sh
pkg install openssh
whoami
passwd
sshd -p 8022
```

`whoami` 输出如 `u0_a123`，它是 SSH 用户名。`passwd` 设置的是 Termux 登录密码。
Termux OpenSSH 通常使用8022端口，此处显式指定；见
[Termux OpenSSH 配置](https://github.com/termux/termux-packages/blob/master/packages/openssh/sshd_config.patch)。

在电脑连接，替换用户名和手机 Tailnet IP：

```sh
ssh -p 8022 u0_a123@100.101.102.103
```

成功后进入的是 Termux 用户环境，不会自动成为 root。常用连接可配置 SSH 公钥，
把电脑公钥放入 Termux 的 `~/.ssh/authorized_keys`，不要复制私钥到手机。
需要长时间保持服务时，可在 Termux 执行 `termux-wake-lock` 并调整其后台电池限制；
结束后执行 `termux-wake-unlock`。这不保证 Android 永远不回收进程。

连接失败时，先在 Termux 检查 `sshd -T` 的 port / listenaddress，确认进程仍在、
8022可达；不要先改模块路由。只有监听 `127.0.0.1` 的 SSH 服务不能直接从 Tailnet 访问。

## 3. ADB over Tailscale

电脑先安装 Android Platform Tools。手机仍需开启开发者选项、USB 调试并授权电脑。
Tailscale 只提供网络连接，不替代 ADB 配对或 RSA 授权。见
[Android ADB 官方文档](https://developer.android.com/tools/adb)。

**已有 USB 授权时的 TCP 示例**：在电脑执行，`USB_SERIAL` 换成 `adb devices`
列出的目标手机序列号。显式选择设备，避免操作其他手机：

```sh
adb devices
adb -s USB_SERIAL tcpip 5555
adb connect 100.101.102.103:5555
adb -s 100.101.102.103:5555 shell
```

手机可能再次弹出授权窗口。`5555` 是本例选定的 TCP 端口，不是 Android 无线调试
页面的动态端口。OEM 对 adbd 监听接口的限制可能不同，不能保证每台手机都支持
直接从 Tailnet 接入。TCP ADB 可能同时监听局域网，不要做公网端口转发；用完恢复 USB：

```sh
adb -s 100.101.102.103:5555 usb
adb disconnect 100.101.102.103:5555
```

Android 11+ 的“无线调试”使用独立的配对端口和连接端口。按系统页面完成
`adb pair IP:配对端口` 后，用连接端口 `adb connect IP:连接端口`；不要混用。
该功能通常依赖 Wi-Fi，切网后端口可能变化或自动关闭，不能把 Wi-Fi 地址直接换成
Tailnet IP 就假定可用。无法从 Tailnet 访问、但本机 adbd TCP 正常时，可通过已配置的
Termux SSH 转发：

```sh
# 电脑第一个终端保持连接；手机 adbd 必须已在本机5555监听
ssh -N -p 8022 -L 127.0.0.1:15555:127.0.0.1:5555 u0_a123@100.101.102.103
# 电脑另一个终端
adb connect 127.0.0.1:15555
adb -s 127.0.0.1:15555 shell
```

该方式仍需 ADB 授权。结束前可执行 `adb -s 127.0.0.1:15555 usb`，再断开 ADB
和 SSH 隧道；不要用 `adb kill-server` 影响其他设备的调试。

## 4. SCP / rsync

复用上面的 Termux SSH 服务。在电脑执行，`scp` 的端口选项是大写 `-P`：

```sh
# 上传到 Termux home
scp -P 8022 ./example.txt u0_a123@100.101.102.103:~/
# 下载
scp -P 8022 u0_a123@100.101.102.103:~/example.txt ./
```

访问手机共享存储，先在 Termux 执行 `termux-setup-storage` 并授予权限；
随后可使用 `~/storage/shared/Download/`。它不授予其他应用私有目录的访问权限。

rsync 需要通信双方都安装它，手机 Termux 执行 `pkg install rsync`；电脑可用
Linux、macOS 或 WSL 中的 rsync：

```sh
rsync -av -e 'ssh -p 8022' ./photos/ u0_a123@100.101.102.103:~/photos/
```

源目录末尾的 `/` 表示同步目录内容。本例没有 `--delete`，不会删除远端多余文件。
连接 Linux SSH 服务时按其实际端口调整即可。

## 5. 访问手机本地服务

服务监听手机 Tailnet IP 或 `0.0.0.0` 时，其他节点可通过 Tailnet IP 和服务端口
访问；仅监听 `127.0.0.1` 的服务需要 SSH 转发。监听所有接口也可能允许局域网访问。

例如在 Termux 临时分享一个专用目录，不要在 home 根目录启动文件服务器：

```sh
pkg install python
mkdir -p ~/tailnet-share
cd ~/tailnet-share
# 替换为手机自己的 Tailnet IPv4
python -m http.server 8080 --bind 100.101.102.103
```

另一台节点浏览器打开 `http://100.101.102.103:8080`，手机按 Ctrl+C 停止服务。
模块 WebUI 本身是管理器加载的本地界面，不会因安装模块就变成8080上的远程网站。

对已有的 localhost-only 服务，在电脑通过 Termux SSH 建隧道：

```sh
ssh -N -p 8022 -L 127.0.0.1:18080:127.0.0.1:8080 u0_a123@100.101.102.103
```

随后在电脑打开 `http://127.0.0.1:18080`。SSH 必须允许 TCP forwarding，目标服务
必须在手机本机8080运行；结束时关闭 SSH 隧道。

## 6. Subnet route 示例

例如 Linux 节点能访问 `192.168.50.0/24`，希望手机经该节点访问局域网设备。
该 Linux 节点需先按 [Tailscale subnet router 文档](https://tailscale.com/docs/features/subnet-routers)
开启 IP forwarding、配置转发防火墙，然后通告：

```sh
# 在 Linux subnet router 执行；设置的是完整通告列表
sudo tailscale set --advertise-routes=192.168.50.0/24
```

如果已有其他通告，需一并保留在逗号分隔列表中。之后在 Tailscale 管理后台批准路由，
并让 Tailnet 访问策略允许目标子网流量。在手机开启 WebUI“设置 → 接受子网路由”，
或执行：

```sh
su -c 'tailscale set --accept-routes'
su -c '/data/adb/tailscale/scripts/tailscaled.service routes'
```

手机浏览器即可尝试访问 `http://192.168.50.1`，具体服务需确实存在。
正常接受的路由由 osrouter 写入 table52，不需要手工改 table52/1099、fwmark 或默认路由。
若手机当前局域网与远端子网重叠，先解决地址冲突。此例让 Linux 当 subnet router，
不表示 Android 手机只通告一个 CIDR 就自动具备 LAN 转发能力。

## 7. Direct / DERP 判断

在手机执行，目标换成对端 Tailnet IP：

```sh
su -c 'tailscale ping 100.101.102.104'
su -c 'tailscale status'
su -c 'tailscale netcheck'
su -c '/data/adb/tailscale/scripts/tailscaled.service selftest 100.101.102.104'
```

| 输出 | 含义 |
|---|---|
| ping `via [公网IPv6]:端口` 或 `via IPv4:端口` | 本次 ping 使用 direct endpoint。 |
| ping `via DERP(hkg)` | 本次 ping 经 DERP 中继，hkg 是地区代码。 |
| status 中 peer 的 `direct ...` / `relay ...` | 当前活动路径；空闲或离线节点可能没有可判断的活动路径。 |
| netcheck `UDP: true`、`IPv6: yes` | 探测条件可用；不保证每个 peer 都能 direct。 |

最初通过 DERP，随后出现 direct 是正常建连过程。Home DERP / Nearest DERP 只表示
中继归属或探测选择，不等于当前 peer 流量必经该中继；也不能仅凭 RTT 判断路径。
更多解释见 [Tailscale connection types](https://tailscale.com/docs/reference/connection-types)。

WebUI“网络详情”可查看 endpoints、peer 路径、netcheck、physical interface / VPN
underlying 和原始诊断。需要定位移动 IPv6 出口时再查：

```sh
su -c 'cat /data/adb/tailscale/outer-ipv6-status'
```

有公网 IPv6 的移动网络通常应为 `active`；IPv4-only Wi-Fi 下 `unavailable`
是正常结果，保留 IPv4/DERP fallback。Redmi 的实测恢复与切网结果见
[IPv6 验收报告](testing/IPV6_MARKED_ROUTING.md)，不代表其他网络一定能 direct。

[返回项目说明](../README.md) · [网络诊断设计](NETWORK_DIAGNOSTICS.md)
