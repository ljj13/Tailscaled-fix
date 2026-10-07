# 开发与目录说明

从仓库根目录执行下列命令。安装和使用见 [中文 README](../README.md)，
相关技术报告见 [文档索引](README.md)。

## 目录职责

| 路径 | 职责 |
|---|---|
| `META-INF/`、`customize.sh`、`service.sh`、`uninstall.sh` | 模块安装、启动与卸载入口，根管理器依赖这些路径。 |
| `system/bin/` | 安装到模块中的 CLI / daemon / service 包装器。 |
| `tailscale/` | 首次安装的配置模板与持久服务脚本。 |
| `webroot/` | 纯 HTML / CSS / JS WebUI 和本地 demo 数据。 |
| `tools/android-dns/` | Android 网络 / DNS 发现 helper 与 Go 测试。 |
| `tools/android-netdiag/` | 独立只读网络诊断 helper。 |
| `tools/android-hostname/` | 默认设备名初始化 helper。 |
| `patches/` | Tailscale / Go DNS 与 fwmark 构建补丁。 |
| `scripts/` | 固定版本构建、测试准备和 ZIP 打包。 |
| `tests/` | DNS、脚本、升级、hostname、WebUI 与打包测试。 |
| `.github/workflows/` | 分支构建上传 artifact；Release tag 工作流测试并发布，手动运行只验证。 |
| `docs/dns/`、`docs/testing/`、`docs/releases/` | DNS 技术报告、测试记录与发布说明。 |
| `docs/screenshots/webui/` | 已选入文档的截图和本地图库。 |

模块安装入口保持管理器要求的路径。构建补丁统一放在 `patches/`，
根目录只保留双语 README 和模块必要文件，专项报告统一归入 `docs/`。
构建工作流支持手动运行，无需通过修改占位文件触发。

## 本地产物

以下目录由构建或测试生成，已经加入 `.gitignore`：

- `build/`：上游 checkout、测试 overlay、浏览器工具和生成截图。
- `files/`：打包所需二进制与构建来源文件。
- `dist/`：ZIP 和 SHA256 文件；正式包作为 GitHub Release 附件发布。

这些目录不会随源码提交。本地已验收 ZIP 可能被复现打包流程使用，请保留需要的基线包。
仅把选定的文档截图复制到 `docs/screenshots/webui/`；截图中的 UI 保持中文即可。

## 构建与复现

Linux / WSL 全量构建需要 Go 1.26.6 和 Python 3：

```sh
sh scripts/build.sh
```

正式 WebUI 2 包复用已验收 dnsfix.2 的 daemon / DNS helper。
该打包路径需要在 `dist/` 放置原始 `tailscaled-v1.102.5-dnsfix.2-arm64.zip`，
并使用 Go 1.26.6 构建 hostname helper：

```sh
python3 scripts/build-hostname.py
python3 scripts/build-netdiag.py
python3 scripts/package-webui.py --release
```

打包器会验证基线 ZIP hash。输出 ZIP 与 `.zip.sha256` 位于 `dist/`。
这些命令生成本地安装资产；正式发布另行执行。

tag Release CI 使用同一正式打包器；固定输入、完整检查与手动验证步骤见
[Release CI](RELEASE_CI.md)。安装器快照行为见[版本化备份](UPGRADE_BACKUPS.md)。

## 检查

Linux / WSL 下运行 Python 检查：

```sh
python3 -m unittest discover -s tests -p 'test_*.py' -v
```

其中依赖编译 helper / resolver 的检查需要相应构建产物和固定 Go 工具链；
只看到跳过结果不代表已验证完整构建。详细条件见 [DNS 测试报告](testing/DNS_TEST_RESULTS.md)。

WebUI 测试先安装本地测试工具，然后执行：

```sh
npm install --prefix build/browser-tools playwright@1.63.0 acorn@8.15.0 --no-audit --no-fund
node tests/webui-command.test.cjs
node tests/network-ui.test.cjs
node tests/webui.test.cjs
```

Windows 默认使用已安装的 Edge；Linux 设置 `WEBUI_BROWSER` 为已安装的
Chromium 兼容浏览器绝对路径。浏览器工具不会打包进模块。

静态检查可使用仓库 CI 中的 ShellCheck 命令：

```sh
shellcheck -s sh -S warning -e SC1090,SC1091,SC2034,SC2086,SC2154 customize.sh service.sh uninstall.sh tailscale/scripts/* system/bin/* scripts/build.sh
```

诊断时不要提交 `tailscaled.state`、私钥、登录 token 或未经筛选的真机原始转储。
本地采集文件可放到被忽略的 `build/` 下，再将必要证据整理成文档。
