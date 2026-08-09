# Task Intent Draft

目标：在 L4D2 Control Panel 内实现不依赖 Throttle 的 Accelerator 兼容崩溃报告接收端，并以用户提供的 minidump 完成验证。

范围：协议预提交、dump/metadata 接收、可选 symbol 接收、二进制上传拒绝、token 鉴权、内容去重、90 天默认可配置保留、认证查询和下载 API。

非目标：首版不做 minidump stackwalk、符号解析 UI、Crash Signature 聚合、自动删除 API、任意模块二进制存储或独立前端页面。

主要风险：公开上传端点的滥用、大文件和磁盘耗尽、敏感 metadata 泄露、临时文件残留，以及错误地把 crash dump 放进实例 Overlay 生命周期。
