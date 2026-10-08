# Exit Node IPv4 TLS：独立 unresolved 记录

状态：暂停。日期：2026-10-08。不与稳定 WebUI 发布验收混合，不继续 A–F，不修改网络规则。

已有旧实现与 scoped-policy DRAFT 的对照都出现 IPv4 Exit HTTPS：TCP 连接成功，TLS 握手停顿；DRAFT 在 FlClash 开启前已经出现。同期 Tailnet 和 IPv6 Exit 可成功，不能将其等同于历史 C/E 的整条 dataplane 失联。

一次 httptrace 记录 TCP 约673ms完成、TLSHandshakeStart 后直到8秒期限没有 TLSHandshakeDone。仅 header metadata 观察到 ClientHello 序号1:1506；尾段1229:1506先到，服务端 ACK1/SACK1229:1506，前段约9秒后重传才 ACK1506。证明缺段/延迟重传，不能定位到 Redmi、outer 链路、Fog 转发或公网；不据此猜测 MTU/MSS/GSO、修改 scoped policy。

```text
IPv4 TLS partial failure layer = exit-transport
packet-loss location / root cause = unresolved
historical C/E full connectivity failure = unresolved
```

此前固定移动 C 中首次观测失败层是 Tailscale dataplane；当时 policy signature 和 route-get 正确，只关 FlClash未恢复，随后关 Exit 后首次复测恢复。关闭 Exit 同时撤销多个机制，时间相关性不能证明其中某一条 policy 的因果责任。旧实现/DRAFT 对照未复现这次全失联，两个问题保持独立。

完整设计、源码、测试和历史报告保留在外部20文件归档及精确 stash，详见 [稳定 WebUI 验收的封存记录](STABLE_WEBUI_ACCEPTANCE.md#draft-双重封存)。运行日志/metadata 不加入源码归档，不提交 state、密钥或抓包。

本轮只恢复稳定 main 并验收 Exit OFF 的已有功能；这些双栈 HTTPS 成功不代表 Exit TLS 已修复。下一次若获授权继续，应采集 Redmi/Fog 同流、同时间 TCP header 对照来定位缺段位置。当前不重试实验，不扩大规则，不合入 DRAFT，不做 Exit WebUI selector。
