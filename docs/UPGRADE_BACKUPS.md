# 版本化升级备份

本机制只辅助回滚配置，不改变现有升级保留规则，也不自动回滚 daemon 或路由。
安装器先检查必需二进制完整，再做备份；成功后才停止旧服务并替换安装文件。
备份失败、锁冲突、非法版本或符号链接/特殊文件会在停止服务前中止安装。

## 内容与位置

```text
/data/adb/tailscale/backups/<旧版本>/<全局序号>/
  settings.ini
  routes
  hostname-initialized        # 如存在
  hostname-user-set/          # 如存在
  scripts/
  installed-module.prop      # 如存在
  build-info.json            # 如存在
  boot-service.sh            # 如存在
  snapshot.info
  .complete
```

没有历史配置/脚本的首次安装不创建快照。旧版本来自 `installed-module.prop`；
首次从此前版本升级时尚无该文件，记为 `unknown`，不猜测旧 daemon 的模块版本。
安装完成后记录新 `module.prop`，供下次升级准确标记来源。
重复安装同一版本仍产生独立序号，不覆盖旧快照。

**不备份 `run/tailscaled.state`、登录密钥、socket、日志、resolver 缓存或二进制。**
live state、settings、routes、hostname marker 的内容和权限沿用原安装语义，
原地保留，不从备份“恢复”或重建身份。备份中的配置也可能包含敏感参数，需留在 root 私有目录。

## 权限、并发与清理

- 备份根目录和所有新快照目录为 root:root /0700；文件为 root:root /0600。
  快照脚本故意不具备执行权限。
- 使用内核 flock，适配 Android toybox `flock -n` 和 mksh 的 fd9 显式传递。
  安装器并发争用时失败，不冒险继续覆盖。需要系统提供 flock/find/sort/awk 等工具。
- 先写私有 `.pending-<pid>`，完成后原子改名；持锁时清除残留暂存目录。
- 全局最多保留5份完成快照，按递增序号删除旧代次；只清理数字目录且带有本机制
  `.complete` 标记的快照，不清理自建目录，不跟随符号链接。
- 不更改 live state/config/marker 权限；备份清理不接触 live `run/`。
  不要手工修改 `.sequence` 或 `.complete`。完整卸载仍按原语义删除整个模块状态目录及备份。

## 回滚辅助

先在 root shell 中查看 `snapshot.info`，确认 source_version 和 target_version。
优先覆盖安装所需的旧正式 ZIP 并重启，保留节点身份；如果还需回滚用户配置，
停止服务后按需恢复单个 settings/routes，并沿用恢复前 live 文件权限。
不要整目录覆盖 `/data/adb/tailscale`，也不要从别的设备复制 state。

hostname-user-set 是用户手工命名保护标记，hostname-initialized 是初始化记录；
不要为了改名删除它们，使用 WebUI 或 `tailscale set --hostname=...`。
备份脚本只是供比较旧改动；不要直接执行0600的旧脚本，更不要混装旧脚本与不匹配的二进制。

查看位置（只读）：

```sh
su -c 'ls -la /data/adb/tailscale/backups'
su -c 'find /data/adb/tailscale/backups -name snapshot.info -type f'
```

本地回归覆盖首次安装、重复升级、5份限制、旧配置/脚本/marker、live identity 和
权限保留、符号链接/FIFO 拒绝、残留暂存清理与外来目录保护。测试不操作手机或主机路由。
