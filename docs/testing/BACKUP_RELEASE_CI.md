# 升级备份与 Release CI 验证

日期：2026-10-07。功能提交 `f4a6f988f16f9068bf9edc8357b2ba4da6d1a584`，基于 main。
本轮没有新建分支、tag、草稿或正式 Release，也没有安装到手机。

## GitHub Actions 实际验证

[只验证运行 37590650522](https://github.com/ljj13/Tailscaled-fix/actions/runs/37590650522)
通过 workflow_dispatch 在该提交运行：

- build job /全部步骤：**PASS**。
- publish job：**SKIPPED**，不是发布失败，也没有实际调用 Release/Contents 写入 API。
- 固定源码、基线下载与哈希、Go1.26.6、Node22.14.0、npm lockfile、Linux Chromium：PASS。
- 63项 Python 测试全部通过、无跳过，包含原40项、8项升级备份与15项 Release 回归。
- helper Go race/vet、上游 DNS/dnscache/netns/osrouter、bootstrap race 与 resolver重载：PASS。
- ShellCheck、完整 WebUI 两种主题/五场景/24截图与57次 shell命令调用：PASS。
- Canonical arm64 ZIP构建、完整 bundle核对与 artifact上传：PASS。

## 可复现性

同提交的 Windows工作区/WSL本地打包与 GitHub Ubuntu24.04产物字节一致：

```text
3df541423bfe970bd1e8f27e35d4572782ff45f6475db4de6fd1ee823cbe6062
```

下载 CI artifact 后再次运行 `release-ci.py verify`：PASS。
另外，同一未提交工作区的两次打包也得到相同 SHA256，验证排序、LF和固定ZIP时间的确定性。
daemon/DNS基线二进制保持字节一致；当前 runtime、WebUI、module.prop、update.json相对前一版无差异。

artifact 中的 ZIP仍使用当前 module.prop 的旧版本名，因为这次是验证 main 的未发布安装器。
**它不是 `v1.102.5-dnsfix.2-webui.2` 的正式资产，不能替换已有 Release。**
Actions artifact保留14天；已有正式 Release及其SHA256保持不变。

## 备份与故障路径

真实安装脚本在隔离目录执行，升级框架/服务行为使用 fixture，未调用主机路由或手机：

- 首装不产生无用快照，live identity持续保留。
- 配置、旧脚本、hostname marker与版本信息进入快照；不复制state。
- 所有 owned备份目录0700、文件0600；live state和settings原模式保持。
- 8次重复升级仅保留5份独立快照；跨版本全局保留、外来目录不被误删。
- source/root/嵌套符号链接、FIFO、非法版本、copy失败和锁争用均在stop之前中止。
- 残留暂存目录安全清理，快照完成后原子发布。

Release状态机使用mock覆盖：上传/远端哈希失败不公开、不更新feed，完整公开Release的
feed失败可重试而不重传，较新版本指针及GitHub Latest不被旧任务降级，同码冲突失败，
损坏ZIP、额外文件、自洽但篡改的WebUI、二进制变化及module.prop权限异常被拒绝。
真实 tag-triggered发布和 Android管理器安装未在本轮执行，不将mock结果当作实机发布验收。

## CI race夹具修正

无缓存测试暴露既有 `TestAndroidBootstrapMissedRegistrationEvent` 偶发在watcher退出前
恢复全局test hook。只在 `android_bootstrap_test.go` 的defer中补 `synctest.Wait()`，
确保取消后drain再cleanup；production DNS代码和验收二进制未修改。
本地 `go test -count=30 -race -run TestAndroidBootstrap ./net/dns`：PASS。
独立只读审查提出的Latest单调性与ZIP/tag字节比对问题已修正，并加了回归。

[备份操作说明](../UPGRADE_BACKUPS.md) · [Release CI操作说明](../RELEASE_CI.md)
