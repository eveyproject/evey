# Evey — Feature Plan

## P6 — Logging structuré

### Logger système (`internal/pkg/logger/`)
- Interface : `*slog.Logger` injecté partout par DI, attrs `component` sur chaque sous-logger
- **Handler prod** : JSON → fichier via `lumberjack`
  - Max size : 50 MB (configurable `EVEY_LOG_MAX_SIZE_MB`)
  - Rotation quotidienne
  - Rétention : 7 jours (configurable `EVEY_LOG_RETENTION_DAYS`)
  - Fichier : `logs/evey.log`
- **Handler dev** : texte colorisé human-readable, avec `source file:line`, activé par `EVEY_LOG_LEVEL=debug`
- Les deux handlers actifs simultanément en dev (console + fichier)
- `request_id` propagé via context sur chaque appel gRPC (intercepteur)

### Logger FFmpeg (`internal/adapters/driven/ffmpeg/`)
- Goroutine lit stderr du process ffmpeg en streaming ligne par ligne
- Parsing : lignes `frame=`, `fps=`, `time=`, `bitrate=` → progress structuré ; `error`, `warning` → level correspondant
- Fichier dédié : `logs/ffmpeg.log` (même rotation que système)
- Injecte le logger dans `Transcoder` et `HWDetector`

### Logger plugin
- Chaque plugin : fichier dédié `logs/plugins/<plugin_id>.log`
- Runner redirige stdout/stderr du process → fichier plugin
- `HostService.Log` RPC écrit dans le fichier du plugin appelant (level + fields structurés)
- **Mode dev plugin** (`EVEY_PLUGIN_DEBUG=true`) : intercepteur gRPC sur le socket host qui log chaque req/resp en JSON (payload complet) — les devs voient exactement ce qui transit entre plugin et host
- Dashboard admin : lecture des N dernières lignes par plugin via API (`GET /api/v1/admin/plugins/{id}/logs`)

### Audit enrichi
- Migration : ajout `user_agent TEXT`, `device_name TEXT` sur `audit_log`
- Intercepteur gRPC admin extrait `user-agent` et `x-device-name` du metadata gRPC → enrich chaque `AuditLogEntry`
- L'IP est déjà capturée, s'assurer qu'elle passe bien via `x-forwarded-for` derrière proxy

---

## P7 — Notifications in-app + Webhooks

### Domaine `notification`

**Modèle `Notification`**
```
id, profile_id, type, title, message, media_item_id,
level, created_at, expires_at, delivered_at, read_at
```

**`NotificationType`** : `NewMovie`, `NewEpisode`, `NewSeason`, `NewSeries`, `NewAlbum`, `LibraryScanned`, `PluginError`, `Custom`

**`NotificationLevel`** : `Disabled` / `All` / `Relevant` / `HighlyRelevant`
- `Relevant` : genres favoris du profil matchent
- `HighlyRelevant` : série actuellement en cours de visionnage

**`NotificationSettings`** (par profil, héritera des presets admin)
```
profile_id
movie_level      NotificationLevel
series_level     NotificationLevel
music_level      NotificationLevel
```
Admin pose les defaults globaux ; chaque profil peut override.

### Port `WatchHistoryAnalyzer`
```go
GetFavoriteGenres(ctx, profileID string, minCount int) ([]string, error)
HasWatchedSeries(ctx, profileID, mediaItemID string) (bool, error)
IsCurrentlyWatchingSeries(ctx, profileID, mediaItemID string) (bool, error)
CalculateRelevanceScore(ctx, profileID string, genres []string) (int, error)
```
Implémentation postgres : requêtes sur `play_sessions` + `watch_progress` + `media_items`.
Réutilisée également par P8 (recommandations).

### Event handlers (via event bus)
- `MediaAddedHandler` : souscrit `library.MediaItemCreated`
  - Déduplication 5 min par série/saison (évite le spam lors d'un scan massif)
  - Filtre par `NotificationLevel` du profil
  - Crée une `Notification` par profil éligible
- Rétention : 30 jours par défaut (`EVEY_NOTIFICATION_RETENTION_DAYS`)
- Cleanup job : purge les notifications expirées (lancé au démarrage + toutes les 24h)

### Repository postgres
Table `notifications` + table `notification_settings`.
Méthodes : Create, ListByProfile (unread filter + pagination), MarkRead, MarkAllRead, UnreadCount, DeleteAll (clear), CleanupExpired.

### API (`NotificationService`)
```
GET  /api/v1/notifications?profile_id=&unread_only=&limit=&offset=
POST /api/v1/notifications/{id}/read
POST /api/v1/notifications/read-all
GET  /api/v1/notifications/count?profile_id=
DELETE /api/v1/notifications?profile_id=          ← "vider tout"

GET  /api/v1/users/{id}/profiles/{pid}/notification-settings
PUT  /api/v1/users/{id}/profiles/{pid}/notification-settings

Admin defaults :
GET  /api/v1/admin/notification-settings
PUT  /api/v1/admin/notification-settings
```

### Webhooks sortants
**`WebhookConfig`** : `id, name, url, events[]string, secret, headers map[string]string, enabled, created_at`

- Table `webhook_configs`
- Signature : `X-Evey-Signature: sha256=<HMAC-SHA256(secret, body)>`
- Delivery : async (goroutine), retry 3x avec backoff exponentiel (1s, 5s, 30s)
- Log de chaque delivery (succès/échec) dans `webhook_deliveries` (id, webhook_id, event, status_code, error, sent_at)
- Admin-only CRUD : `POST/GET/PUT/DELETE /api/v1/admin/webhooks`

---

## P8 — Feeds & Recommandations

### Concept "Feed"
Un feed = une liste ordonnée d'items média avec un type et des paramètres.

**Types disponibles**
| ID | Description |
|----|-------------|
| `recently_added` | Items ajoutés récemment |
| `continue_watching` | Films/séries en cours |
| `recommended` | Recommandations personnalisées |

### FeedConfig (admin presets + override user/profil)

**Admin** (`feed_admin_config`) : preset global
```
feed_type, enabled, display_order, limit, params JSONB
```

**Profil** (`feed_profile_config`) : override partiel
```
profile_id, feed_type, enabled, display_order, limit, params JSONB
```
- Un profil ne peut pas activer un feed que l'admin a désactivé
- Un profil peut désactiver, réordonner, changer les limites
- Bouton "reset aux defaults admin" par feed

### Feed : Continuer à regarder

**Films** : `watch_progress.position_ms > 0 AND completed = false`
→ retourne le film directement, avec `position_ms`

**Séries** : trouver l'épisode le plus récent en cours OU le prochain épisode non vu
- En cours = `position_ms > 0 AND completed = false` → cet épisode
- Dernier épisode vu completé → épisode suivant dans la séquence (S01E03 → S01E04, etc.)
- L'item retourné est toujours l'épisode (pas la série), avec `position_ms` et référence à la série parente
- Trié par `last_played_at DESC`
- Pas de mélange : un seul item par série dans le feed (le plus récent/pertinent)
- Configurable : `max_items` (default 20), `include_movies` (default true), `include_series` (default true)

### Feed : Récemment ajouté
- `SELECT * FROM media_items ORDER BY added_at DESC WHERE added_at > NOW() - INTERVAL 'N days'`
- Paramètre `days_window` (default 30), `limit` (default 20)
- Filtrable par type (`movie`, `series`, `episode`, `album`)
- Configurable par l'admin : types inclus, fenêtre temporelle
- Override profil : peut réduire la fenêtre ou exclure des types

### Feed : Recommandations

**Algorithme content-based** (pas de ML, SQL pur) :
1. `GetFavoriteGenres(profileID)` → genres vus ≥ `min_watch_count` fois (default 3)
2. Score = nb de genres favoris matchant l'item
3. Filtre : `score >= min_score` (default 1), non vus par ce profil
4. Tri : score DESC, puis community_rating DESC

**`GetRecommendedItems(profileID, type, limit)`** → films ou séries

**`GetSimilarItems(mediaItemID, profileID, limit)`** → même genres/réalisateur, non vus

**Override profil** :
- Peut ajuster `min_score` (plus ou moins strict)
- Peut exclure des genres
- Peut reset aux defaults admin
- Ne peut pas modifier `min_watch_count` (paramètre admin uniquement)

### API feeds
```
GET /api/v1/feeds/{profile_id}                          ← tous les feeds activés dans l'ordre
GET /api/v1/feeds/{profile_id}/continue-watching
GET /api/v1/feeds/{profile_id}/recently-added
GET /api/v1/feeds/{profile_id}/recommended
GET /api/v1/feeds/{profile_id}/similar/{media_item_id}

GET /api/v1/feeds/{profile_id}/config                   ← config profil (merged avec admin)
PUT /api/v1/feeds/{profile_id}/config                   ← override profil
DELETE /api/v1/feeds/{profile_id}/config/{feed_type}    ← reset un feed aux defaults admin

Admin :
GET /api/v1/admin/feeds/config
PUT /api/v1/admin/feeds/config
```

---

## Ordre d'implémentation

1. **P6 Logging** — fondation, tout le reste bénéficiera des logs dès le départ
2. **P7 WatchHistoryAnalyzer** — port + impl postgres (partagé P7/P8)
3. **P7 Notifications in-app** — domain, repo, event handlers, API
4. **P7 Webhooks** — domain, repo, delivery service, API admin
5. **P8 Continue watching** — feed SQL + API
6. **P8 Recently added** — feed SQL + API
7. **P8 Recommendations** — algo content-based + API
8. **P8 FeedConfig** — admin presets + profile overrides + API
9. **Proto** — ajout des nouveaux services (NotificationService, FeedService) + regen
10. **Tests** — unit (fakes) + intégration postgres pour chaque nouveau repo

---

## Questions ouvertes

- Aucune bloquante. On peut démarrer par P6.
