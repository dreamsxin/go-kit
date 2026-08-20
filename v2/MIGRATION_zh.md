# 升级说明

[English](MIGRATION.md) | 简体中文

go-kit v2 遵循语义化版本。本页记录当前版本所需的升级动作；每个版本的
完整变更列表见 [CHANGELOG.md](CHANGELOG_zh.md)。

## 升级到 v2.3.0

`v2.3.0` 完全向后兼容：全部为新增能力与行为修复，无需修改源码。

升级时值得复核的两个行为变化：

- `endpoint.TracingMiddleware` 生成的 trace ID 为 32 位小写十六进制字符
  （W3C Trace Context 格式），不再是 16 位。把 ID 当作不透明字符串的
  调用方不受影响。
- `endpoint.ErrBackpressure` 与新增的 `endpoint.ErrBulkheadFull` 在 HTTP
  中编码为 429，不再是 500。

## 兼容性策略

- 补丁版本修复行为；次版本新增能力。两者在 `/v2` 内均向后兼容。
- 不兼容变更需要新的主 module 版本。
- v2 不保留废弃转发 API。早期版本的文档仍可通过不可变的发布标签获取。
