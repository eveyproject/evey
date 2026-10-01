---
source_sha: 58e220cb177619c27f3d55f0ae3d1c6a3b4df50ab6ace660df93aa8ccf6e6df9
reviewed: false
---
# 公开 API

面向用户的端点，所有经过身份验证的用户都可以使用。

## 规范

| 规范 | 说明 |
|------|-------------|
| [library.swagger.json](library.swagger.json) | 媒体库、媒体条目、搜索、人物 |
| [playback.swagger.json](playback.swagger.json) | 流媒体、进度跟踪、观看记录、继续播放 |
| [user.swagger.json](user.swagger.json) | 身份验证、个人资料、API 密钥、快速连接 |
| [collection.swagger.json](collection.swagger.json) | 智能合集和手动合集 |
| [favorite.swagger.json](favorite.swagger.json) | 收藏 |
| [feed.swagger.json](feed.swagger.json) | 首页区块（继续观看、最近添加等） |
| [playlist.swagger.json](playlist.swagger.json) | 播放列表，手动创建或按规则生成 |
| [rating.swagger.json](rating.swagger.json) | 用户评分 |
| [share.swagger.json](share.swagger.json) | 公开分享链接 |
| [syncplay.swagger.json](syncplay.swagger.json) | 同步播放房间 |
| [notification.swagger.json](notification.swagger.json) | 应用内通知 |
| [push.swagger.json](push.swagger.json) | 网页推送订阅 |
| [music.swagger.json](music.swagger.json) | 音乐库（艺术家、专辑、曲目） |
| [photo.swagger.json](photo.swagger.json) | 照片库（相册、照片） |
| [media_request.swagger.json](media_request.swagger.json) | 媒体请求：搜索、提交、策略；管理员的答复和请求权限 |

## 身份验证

各端点接受 `Authorization: Bearer <token>`：登录获得的会话令牌，或 API 密钥（见 [Webhook 与 API 密钥](../../webhooks-api-keys.md)）。无需令牌即可访问的有：登录和续期会话、通过邀请注册、Quick Connect、密码重置、设置向导及其状态，以及公开下载链接。
