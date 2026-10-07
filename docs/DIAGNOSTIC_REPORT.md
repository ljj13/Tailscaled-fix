# 脱敏诊断报告

网络与诊断页的“导出脱敏诊断报告”生成纯文本，可复制或保存。管理器不支持下载时可复制/长按文本。
也可运行 `su -c '/data/adb/tailscale/scripts/tailscaled.service report'`；总采集deadline15秒，各子命令最多9秒，输出有上限。

## 安全边界

- 所有采集结果、stderr、错误和日志通过report.go的同一最终脱敏函数，新增采集项也不能直接输出。
- status JSON只投影现有typed Node/endpoints/netcheck/network字段，prefs严格白名单；未知prefs/Persist丢弃。
- 固定公开metadata/status文件及固定日志，拒绝symlink/特殊文件；不读取tailscaled.state、settings原文或socket内容。
- 识别并隐藏auth/login/OAuth URL、auth/API token、密钥、Cookie/Authorization、秘密字段、多行/损坏JSON与PEM。
- Tailscale state dump若意外写入日志，识别_machinekey/_profiles后隐藏整个对象；已识别秘密值的重复裸文本也隐藏。
- IP、hostname、物理接口、DERP、route默认保留。额外秘密值每行一个，在WebUI内存中统一遮盖后才预览/复制/保存；不持久化、不传入shell。
- 未知无标记的任意字符串不能凭空识别为秘密；可用额外隐藏内容明确指定，分享前可检查全文。

## 报告内容

头部：名称、模块版本、build revision、UTC生成时间、redaction: enabled。
含模块/build元数据、daemon/backend、CLI status、安全JSON摘要、prefs安全字段、netcheck、endpoints/peers、physical/VPN underlying、DNS、outer双栈route、ip rule、table52/1099、proxyexemptions、UDPlistener以及最近三类日志。
单项失败、权限不足、deadline或输出过大写为 `<unavailable: reason>`，其余项继续；不触发ping、DNS刷新、路由写入或daemon重启。

## 阶段2验证

- netdiag Go测试新增7项报告测试，总16项，race与vet通过；canary覆盖字段/URL/密钥/headers/state dump及最终完整报告，零泄漏。
- WebUI显式秘密值canary与不支持报告的拒绝测试3组；browser验证预览、复制、保存和旧helper不可用。
- service集成新增2项，验证timeout/损坏JSON/缺helper不影响服务或身份。
- 22条WebUI命令×3种PATH=66次；原Peers及WebUI16页面、5场景、26截图继续通过。

报告在分享时仍保留对排障有用的地址和设备名；它不是匿名报告。不会自动上传到任何服务器。
