# Release CI 与本地正式打包

`.github/workflows/release.yml` 在 `v1.102.5-*` tag push 时运行。
`workflow_dispatch` 只验证和生成 Actions artifact，**publish job 永远跳过**，
即使手动选择 tag 也不会发布。普通 main push 不触发该 Release workflow。

## 固定输入与同一打包规则

- checkout 精确使用事件 SHA，正式 tag 必须匹配 module.prop 的 version，并属于 main 历史。
  对应 `docs/releases/<tag>.md` 必须存在；versionCode 为有效递增整数。
- Go 固定1.26.6，Node 固定22.14.0，Actions 固定 commit SHA。
  Playwright1.63.0 / Acorn8.15.0 使用提交的 npm lockfile。
- `scripts/release-config.json` 锁定仓库、dnsfix.2 基线 tag、附件文件名和 SHA256，
  同时记录保留条目的哈希与权限。HTTPS 下载必须通过证书验证，不使用 latest 或跳过证书检查。
- 基础 daemon / DNS helper 不重编译；复用验收二进制。
  hostname/netdiag helper 从当前 tag 的源码以静态 linux/arm64、Go1.26.6 构建。
- 上游测试源码固定为 commit `5fb2a81b065b0a0bbbfc67ab20a0d9c6a1108115`。
  只在独立测试副本应用现有补丁，生成 resolver/test overlay。
- 本地和 CI 都调用 `scripts/package-webui.py --release`；版本和文件名来自 module.prop。
  ZIP 条目时间固定、排序固定、脚本规范化 LF，helper 禁用 VCS build stamp。
  同一提交、固定输入和工具链的重复打包应得到相同 SHA256。

不要在新版本发布前把 main 的 update.json 指向尚未存在的资产。
先修改 module.prop 的 version/versionCode、对应发布说明和相关 README，再提交并推 tag。
发布完成后 CI 按实际 tag 生成并提交 update.json；当前已发布版本的 feed 保持有效。
首次真实 tag 发布与 feed 更新已通过，见 [真实事务验收](testing/RELEASE_TRANSACTION.md)。

## 测试与发布边界

1. 只有 contents:read 的 build job 准备固定输入，运行全部 Python、Go race/vet、
   上游 DNS/netns/osrouter、resolver 重载、ShellCheck、WebUI 浏览器和命令测试。
2. 全部通过后构建 arm64 ZIP + `.zip.sha256`，验证 CRC、文件清单、模式、版本、
   tagged source 的 script/WebUI 字节和 helper 来源。仅上传这两个 Actions artifact。
3. 独立 contents:write publish job 再验证下载的 bundle。
4. 以草稿创建 Release 并上传资产；既有草稿可重试上传。
   GitHub 返回的完整文件清单、uploaded 状态、大小与 SHA256 digest 必须都匹配。
5. 校验后才转为公开正式 Release。旧版本延迟完成时不覆盖较新版本的 GitHub Latest。
6. 最后通过 GitHub Contents API 更新 main/update.json，使用现有文件 SHA 做并发检查；
   不覆盖更新的 versionCode，同码不同 tag 视为冲突。

草稿与上传行为参见 [gh release create](https://cli.github.com/manual/gh_release_create)，
job 权限参见 [GitHub workflow syntax](https://docs.github.com/en/actions/reference/workflows-and-actions/workflow-syntax)。

### 失败与重试

测试、打包、上传或远端校验失败，不公开半成品、不更新 feed；已上传内容最多留在草稿。
重新运行相同 tag 工作流会检查并补齐草稿。已公开的资产不能被重传或覆盖，重试需字节完全一致。
如果 Release 已完整公开但 feed 写入失败，保留完整 Release，工作流报失败；
重跑会重新核对资产并只补 feed，不重复发布。

自动写 main 需要仓库 Actions 写权限，以及分支保护允许该 bot 更新 update.json。
否则 feed 更新会明确失败，不绕过保护、不 force push；调整仓库授权后重跑即可。
全局 release-main-feed concurrency 排队，避免 tag 发布任务互相取消。

## 本地只验证（不发布）

Linux/WSL 需要固定 Go1.26.6、Node22.14.0、Python3、Git、curl、ShellCheck：

```sh
# TAG 换成 module.prop 当前 version；untagged 允许验证 main 的未发布变化
python3 scripts/release-ci.py prepare --tag TAG --untagged
mkdir -p build/browser-tools
cp tests/browser-tools/package*.json build/browser-tools/
npm ci --prefix build/browser-tools --ignore-scripts --no-audit --no-fund
build/browser-tools/node_modules/.bin/playwright install --with-deps chromium
python3 scripts/release-ci.py test
python3 scripts/release-ci.py build --tag TAG --bundle build/release-validation
python3 scripts/release-ci.py verify --tag TAG --bundle build/release-validation
```

`prepare/test/build/verify` 不创建远端 Release、tag 或修改 feed。
当前 module.prop 可能仍使用上次正式版本号，验证产物只应作为测试 artifact，不能冒充该 tag 的正式资产。
publish 命令仅用于 tag-triggered CI。本轮用 mock 覆盖发布失败/重试状态机，
不会为了测试创建真实草稿或正式 Release。

实际 workflow_dispatch 全流程通过、publish跳过，本地与hosted ZIP字节一致；
详见[本轮验证报告](testing/BACKUP_RELEASE_CI.md)。
