---
source_sha: d5c0534aa9aa7e7c333f76023148a199fc8b6bf2f16cee4f6c80face79daf6b2
reviewed: true
---
# 公开 API

所有已登录用户都能调用的端点：应用用它们来浏览媒体库、播放影片、管理收藏和播放列表，以及请求新的媒体。
每个服务一个文件。

| 规范 | 说明 |
|------|-------------|
| [library.swagger.json](library.swagger.json) | 媒体库、媒体条目、搜索、人物；插件界面可以读取的 `public` 投影 |
| [playback.swagger.json](playback.swagger.json) | 串流、进度跟踪、观看记录、继续播放 |
| [user.swagger.json](user.swagger.json) | 身份验证、个人资料、API 密钥、快速连接 |
| [collection.swagger.json](collection.swagger.json) | 智能合集和手动合集 |
| [favorite.swagger.json](favorite.swagger.json) | 收藏的条目 |
| [feed.swagger.json](feed.swagger.json) | 首页的各个区块（继续观看、最近添加……） |
| [playlist.swagger.json](playlist.swagger.json) | 播放列表，手动创建或按规则生成 |
| [rating.swagger.json](rating.swagger.json) | 用户评分 |
| [share.swagger.json](share.swagger.json) | 公开分享链接 |
| [syncplay.swagger.json](syncplay.swagger.json) | 同步播放房间 |
| [notification.swagger.json](notification.swagger.json) | 应用内通知 |
| [push.swagger.json](push.swagger.json) | 网页推送订阅 |
| [music.swagger.json](music.swagger.json) | 音乐库（艺术家、专辑、曲目） |
| [photo.swagger.json](photo.swagger.json) | 照片库（相册、照片） |
| [media_request.swagger.json](media_request.swagger.json) | 媒体请求：搜索、提交、规则；管理员的答复，以及请求功能的使用权限 |

## 身份验证

这些端点接受 `Authorization: Bearer <token>`：登录得到的会话令牌，或者在其权限范围内的 API 密钥（[服务器 API](../README.md#authentication)）。
不带凭据也能访问的有：登录和续期会话、凭邀请注册、快速连接、重置密码、设置向导及其状态，以及公开下载链接。

<!-- sources
auth: internal/adapters/driving/grpcapi/interceptors/auth.go ; public methods: internal/adapters/driving/grpcapi/interceptors/rbac.go - PublicMethods
public projection: proto/evey/v1/library.proto - GetPublicMediaItem, ListPublicMediaItems
-->
