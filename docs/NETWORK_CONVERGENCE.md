# 网络切换收敛

本轮只缩短现有 watchdog 的等待时间，不改变 DNS、main route、outer IPv6 rule、fwmark、table52/1099 或代理豁免的算法。tailscaled 不重启，UDP listener 不主动重绑。

## 设计

`android-netdiag --watch-parent <watchdog-pid> --dir <install-dir>` 是内部可选模式：被 watchdog 启动，监听 Linux NETLINK_ROUTE 的 link/address/route 事件。只能通知自己的实际父进程，父进程退出后自动结束；watchdog 退出也主动清理观察器。

事件先合并：静默 750 ms 或连续事件达到 2 s 后检查，检查间隔至少 2 s。复用 `android-dns --network` 的只读 ConnectivityService/underlying discovery，比较真实 physical netId、interface、地址和该接口 IPv6 routes；忽略 VPN 展示字段、TUN/Tailscale routes、路由 expires 倒计时及排序差异。暂时没有有效物理网络时不通知空选择，最多尝试 10 s，后续事件可以重试。

只有指纹实际改变才发 SIGUSR1。watchdog 中的 sleep 可中断，收到事件后仍顺序执行原来的 route repair → outer IPv6 sync → DNS refresh；执行中收到的事件合并为一个后续 pass。Re-STUN 继续由原来的 outer IPv6 sync 决定，不由观察器发起。相同 physical network 的 FlClash 启停不额外执行 Re-STUN。

15 s 周期检查始终保留，以 `/proc/uptime` 的单调时间计算，事件不会推迟周期截止时间。自动 subnet routes 和 hostname retry 仍按周期检查执行。缺 helper、netlink 不支持、观察器退出或消息丢失时，周期检查负责最终一致性。

## 边界

内核事件不等于 ConnectivityService 已选定新网络。当前 ROM 的一次较慢 Wi-Fi 建链中，网络选择明显晚于首批 netlink 事件，最后由周期检查完成；这是保留 fallback 的实际理由。此机制减少常见切换等待，不保证每次切换都加速，也不保证 IPv4-only Wi-Fi 能 direct。

真机时间线、重复样本、原始 endpoint 变化与测试见 [三阶段验收](testing/PEERS_REPORT_CONVERGENCE.md)。
