---
source_sha: 4609c608404f7d7614f31c97a0e6a7ce865e34c4d562caf6ca04af7ced28096a
---
# 公开 API

面向用户的接口，所有已认证用户均可使用。

## 规范

| 规范 | 说明 |
|------|-------------|
| [library.swagger.json](library.swagger.json) | 媒体库、媒体条目、搜索、人物 |
| [playback.swagger.json](playback.swagger.json) | 串流、进度跟踪、观看历史、续播 |
| [user.swagger.json](user.swagger.json) | 认证、个人资料、API 密钥、快速连接 |
| [collection.swagger.json](collection.swagger.json) | 智能合集与手动合集 |
| [favorite.swagger.json](favorite.swagger.json) | 收藏 |
| [feed.swagger.json](feed.swagger.json) | 主页信息流的各个区块（继续观看、最近添加等） |
| [filter.swagger.json](filter.swagger.json) | 保存的筛选条件 |
| [playlist.swagger.json](playlist.swagger.json) | 播放列表（创建、排序、分享） |
| [rating.swagger.json](rating.swagger.json) | 用户评分 |
| [share.swagger.json](share.swagger.json) | 公开分享链接 |
| [syncplay.swagger.json](syncplay.swagger.json) | 同步播放房间 |
| [notification.swagger.json](notification.swagger.json) | 应用内通知 |
| [push.swagger.json](push.swagger.json) | Web 推送订阅 |
| [music.swagger.json](music.swagger.json) | 音乐库（艺术家、专辑、曲目） |
| [photo.swagger.json](photo.swagger.json) | 照片库（相册、照片） |

## 认证

除 `/api/v1/auth/login` 和 `/api/v1/setup/*` 外，所有接口都需要通过 `Authorization: Bearer <token>` 提供有效的 JWT 令牌。
