---
source_sha: 2f1c2cafd1c1e20b2e8d011ec2457b1f01bde072b96b93d077d7694cf6537497
---

# 备份

Evey 可以按计划自动备份数据库。

## 配置

```env
EVEY_BACKUP_DIR=/data/backups          # 备份的存放位置
EVEY_BACKUP_INTERVAL_HOURS=24          # 自动备份的间隔（0 = 关闭）
EVEY_BACKUP_RETENTION_DAYS=30          # 删除早于此天数的备份（0 = 永久保留）
```

## 备份包含哪些内容

备份涵盖全部用户数据：
- 用户、配置文件、角色、权限
- 媒体库、合集、播放列表、收藏
- 观看进度、播放历史
- 扩展、服务器配置、计划任务
- 通知、Webhook、审计日志

**不在备份范围内**：媒体文件（这些本来就属于你）以及转码缓存（会自动重新生成）。

## 备份格式

每份备份都是一个 `.tar.gz` 归档，其中包含：
- `manifest.json` - 元数据、版本号、SHA256 完整性校验值
- `dump.sql` - 完整的 PostgreSQL 转储

## 手动备份

可以从管理面板触发备份，也可以调用 API：

```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:9091/api/v1/admin/backups
```

## 恢复

通过管理 API 即可从备份恢复。应用备份之前，服务器会先用 SHA256 校验值验证其完整性。

## 持久化存储

请确认 `EVEY_BACKUP_DIR` 指向一个持久卷或宿主机路径。默认的 `/data/backups` 位于 `evey-data` 这个 Docker 卷内部。

若要把备份存放在宿主机上：
```yaml
services:
  evey:
    volumes:
      - /path/on/host/backups:/data/backups
```
