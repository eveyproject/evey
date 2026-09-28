---
source_sha: 3ba52125e2272c947b09a156c62f232a40194d67c79158ea4d06ec6075d320b4
---
# Evey API 文档

Evey 媒体服务器的 OpenAPI（Swagger 2.0）规范。

## 结构

| 部分 | 说明 |
|---------|-------------|
| [公开 API](public/README.md) | 面向用户的接口 - 浏览媒体、播放、合集、播放列表等。 |
| [管理 API](admin/README.md) | 服务器管理 - 系统配置、转码、备份、元数据等。 |
| [扩展 API](plugins/README.md) | 扩展管理与 webhook 集成。 |

## 用法

每个文件夹都包含独立的 `.swagger.json` 文件，可以导入任何兼容 OpenAPI 的工具（Swagger UI、Postman、Insomnia 等）。

## 重新生成

这些规范由 protobuf 定义通过 `buf generate` 生成。更新方法：

```bash
buf generate --template buf.gen.local.yaml
python3 docs/api/sync.py
```
