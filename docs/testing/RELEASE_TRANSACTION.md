# 真实升级备份与 Release CI 事务验收

日期：2026-10-07（Asia/Singapore）。本轮仅验收与正式发布整理；不修改网络、WebUI、备份格式或 CI 架构，不新建分支。

## 真机安装

Redmi Note 8 Pro，KernelSU 3.3.0 CLI。ADB 全程指定 Redmi 序列号，未重启 ADB server、未操作另一设备。
从干净 main b500f38 的 canonical release-ci/package-webui 规则生成未发布构建，版本元数据仍为 webui.2。
它不是旧 webui.2 Release 的资产，未覆盖旧正式资产。

使用 `/data/adb/ksud module install` 连续覆盖安装两次，每次都经 modules_update 正常激活并重启。
不手动执行安装器片段替代真实安装，不先卸载，不删除 state 或重新登录。

| 检查 | 实测 |
|---|---|
| 首次备份 | `/data/adb/tailscale/backups/unknown/00000001/` |
| 第二次备份 | `/data/adb/tailscale/backups/v1.102.5-dnsfix.2-webui.2/00000002/` |
| 首次快照不被覆盖 | 十个文件 SHA256 全部相同 |
| 权限/归属 | 备份树所有目录0700、所有文件0600、uid/gid0；30个条目核对通过 |
| 内容 | settings.ini、routes、hostname marker、旧 scripts、build-info、boot-service、快照信息及完整标记；第二份另有 installed-module.prop |
| 禁止复制内容 | 无 tailscaled.state、登录密钥、socket、日志、二进制或特殊文件 |

旧模块 module.prop 为 dnsfix.2，但此前安装没有 installed-module.prop 版本跟踪文件。
因此机制以 unknown 标记首份来源是正确行为；没有人为补写版本或猜测已部署脚本版本。
第一次安装记录新版本后，第二份正确使用 webui.2 来源与全局序号2。

在真机 `/data/local/tmp` 独立 fixture 中提取原 backup_upgrade 函数，配置/服务路径全部指向 fixture。
生成7份完整快照后仅保留3–7，外来 `foreign/00000000/keep` 内容仍为 foreign。
测试未访问真实 state 或真实配置。首次 fixture 在 set-e 下因无 .sequence 提前结束，重新用条件调用函数（与安装器调用方式相同）完成验证；未改生产实现。

## 身份、配置与网络

两次安装后、重启后 state/settings/routes/hostname marker SHA256 均与安装前相同：

| 文件 | SHA256 |
|---|---|
| run/tailscaled.state | `23525e43714936d9e1c23b876008f038bc87f120642485493f802e64b93295e0` |
| settings.ini | `8a0278d0bcdd67411232cb5fcd2c73a071c1522d1898e15a20d63987dce23463` |
| routes | `96a2391d2a7d39dbf0f33139a5aede6598be9ade0c6e336bbd64773dc4c3a641` |
| hostname-initialized | `9721bbc51964ed4d8b36d4a0de55b9bc4cf8a377f723d87f4419beff8fc862d4` |

只读取文件哈希，未把敏感 state 或登录密钥复制到本机/备份/仓库。
Self.ID `nGSD4XDqF611CNTRL`、hostname `redmi-note-8-pro`、IPv4 `100.118.66.106`、
IPv6 `fd7a:115c:a1e0::ce2e:426b` 均不变。reported OS 保持 linux。
settings/routes 继续保持旧安装权限，本轮没有更改升级语义；上述0700/0600只描述备份树。

| 阶段 | 结果 |
|---|---|
| 升级前，移动+FlClash ON | Running，DNS reachable，exemptions全OK；既有 debug restun 后 IPv6 direct59ms |
| 第一次重启，FlClash OFF | 自动启动；netcheck UDP/IPv4/IPv6可用；IPv6 direct75ms |
| 第一次 FlClash ON | VPN106 / physical105 / ccmni1，DNS与exemptions正常；IPv6 direct35ms |
| 第二次重启，FlClash OFF | 自动启动；state/settings/routes/marker哈希不变，双栈netcheck可用 |
| 第二次 FlClash ON | VPN106 / underlying105 / ccmni1，Running，DNS reachable，IPv6 direct32ms |
| 最后 kernel ping EAIDK | 3/3成功，36.6–120.6ms |
| 最后 marked IPv6 route | mark0x10020000，经ccmni1真实policy表与RA网关，无unreachable |
| main/table52/1099 | main_default=ok，table52含EAIDK等Tailnet路由，ip rule保留1099目的范围规则 |

实际 direct endpoint为 `[2001:250:3c00:3487:add3:4002:9c55:b820]:41641`。
升级前初次空闲探测暂经DERP；既有 debug restun 恢复direct，重启/公网地址变化后的ping也有短暂DERP收敛。
未改路由或强制direct，DERP fallback保留。本轮验证双栈netcheck和IPv6 direct，未声称IPv4 direct。
未重新切Wi-Fi，历史切网及IPv4-only Wi-Fi fallback参考 [IPv6报告](IPV6_MARKED_ROUTING.md)。

实际 KernelSU WebUI 首页已打开，刷新显示已连接、原设备名/IP及移动数据VPN共存。
刚打开时出现过exemption提醒，刷新后消失；同期root webstatus为pre/out/nat全OK，最终截图无警告。
未为了消除提醒改UI或网络。安装ZIP的WebUI资源与main源一致。
按用户明确要求永久设置USB充电保持亮屏，不恢复原值；不使用或记录锁屏密码。

## 发布事务

版本 `v1.102.5-dnsfix.2-webui.3` / code110200505，准备提交8004b13。
真实tag触发 [Actions37598757410](https://github.com/ljj13/Tailscaled-fix/actions/runs/37598757410)。

真实执行结果：**build / publish 均 PASS，attempt1，无失败或人工重试**。未手工创建/上传Release，未绕过校验、未force push。

- tag/source：`8004b131cf4d1e42ef67a9be1d40fae3eb5443b2`；Go1.26.6、Node22.14.0及锁定基线按现有CI执行。
- 63项Python全部通过；三个helper Go race/vet、上游DNS/dnscache/netns/osrouter及bootstrap race、resolver热重载、ShellCheck全部通过。
- WebUI19条命令×3种PATH通过；防御性字段/离线/超时测试通过；浏览器14个主题页面、5场景、24截图与native bridge测试通过。
- [正式Release](https://github.com/ljj13/Tailscaled-fix/releases/tag/v1.102.5-dnsfix.2-webui.3)于2026-10-07T09:14:19Z公开，draft=false、prerelease=false，`releases/latest`返回webui.3。
- 正式资产仅ZIP及SHA256文件，均uploaded；ZIP大小16,939,671字节，SHA256文件113字节。
- 下载ZIP与正式tag提交上的本地canonical构建逐字节一致，ZIP CRC和源码revision通过。
- ZIP SHA256及GitHubasset digest均为 `81f49001c642828798ec627826032cd2c7e8ef9de3cae9172a9e0b65d84ab07e`。
- SHA256文件本身的GitHub digest与下载文件SHA256均为 `891c0e4ac08f766f10c106a53a6c0c6ca564064f5af231a982946b3bdaa8b8c9`。
- CI自动feed提交 `925c114`，version=webui.3、versionCode=110200505，zipUrl与tag固定changelog均正确；本地ff-only拉取核对通过。
- 发布前Latest仍为webui.2；完整资产校验后才切换Release/feed。本次无失败，真实恢复分支未触发；重试/上传失败/feed失败等状态机由现有自动化测试覆盖，不虚构故障演练。

手机保留两次验收所安装的未发布webui.2元数据构建；正式webui.3仅调整版本/文档，功能代码未变。
本轮没有第三次刷正式包或再次重启，正式ZIP完整测试与字节复现由CI/下载核对验证。

## 最少复查命令

```sh
su -c 'find /data/adb/tailscale/backups -name snapshot.info -exec cat {} \;'
su -c '/data/adb/tailscale/scripts/tailscaled.service selftest 100.72.239.86'
```

原始ADB安装日志、哈希/stat、netcheck/status与截图留在本机忽略目录 `build/real-upgrade-acceptance/`，不提交账号或密钥数据。
