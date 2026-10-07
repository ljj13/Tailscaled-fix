# 设备 / Peers

WebUI 首页“设备”进入独立页面，沿用Miuix Card与浅/深色主题。列表只在进入/刷新时读取 `tailscale status --json`，不新增后台网络探测。
在线、离线、状态未知分组；Self按ID、公钥或Tailnet地址去重。详情包含OS、Tailnet双栈地址、最近活跃、ExitNodeOption和路由前缀；缺字段显示未知/未提供。

Direct仅在Active且CurAddr存在、非明确offline时显示；活跃Relay且无CurAddr/PeerRelay才显示DERP。Home DERP不能证明实际路径，空闲/离线/不完整数据均Unknown。
只读Ping使用校验后的Tailnet地址，3次、每次3秒；结果显示实际路径与RTT，并保留原始输出。
复制名称/IPv4/IPv6和详情沿用现有BottomSheet。不包含节点修改、删除或ACL管理。

## 阶段1验证

- `node tests/peers-ui.test.cjs`：8组适配/去重/路径/能力/目标安全/解析检查。
- `node tests/webui-command.test.cjs`：21条命令×3种PATH=63次；保留自定义socket。
- `node tests/webui.test.cjs`：原交互回归、8页×2主题、5场景；新增Peers分组、详情、复制、ping结果与native超时/缺字段/缓存/HTML安全。
- `node tests/network-ui.test.cjs`：原诊断适配回归。

本地预览：在webroot启动静态服务器，打开 `/?demo=cellular#peers` 或 `/?demo=wifi#peers`。
截图由browser测试生成在忽略目录build/webui-screenshots/light-peers.png与dark-peers.png。
本阶段仅WebUI、测试与本文档，未修改DNS、fwmark、table52/1099、outer IPv6或Clash exemption。
