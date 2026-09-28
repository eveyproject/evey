---
source_sha: 69eb69dc75f4051d291795ad196ce5ee4b61038c94cbe8a97d9b7ee7d7470a69
---

# Sauvegardes

Evey sait sauvegarder votre base de données tout seul, à intervalle régulier.

## Configuration

```env title=".env"
EVEY_BACKUP_DIR=/data/backups          # Emplacement des sauvegardes
EVEY_BACKUP_INTERVAL_HOURS=24          # Intervalle des sauvegardes automatiques (0 = désactivé)
EVEY_BACKUP_RETENTION_DAYS=30          # Supprimer les sauvegardes plus anciennes que cela (0 = conserver indéfiniment)
```

## Ce qui est sauvegardé

La sauvegarde couvre l'ensemble des données utilisateur :
- Comptes, profils, rôles, permissions
- Bibliothèques, collections, listes de lecture, favoris
- Progression de visionnage, historique de lecture
- Extensions, configuration du serveur, tâches planifiées
- Notifications, webhooks, journaux d'audit

**Hors sauvegarde** : les fichiers multimédias (ils vous appartiennent) et le cache de transcodage (régénéré automatiquement).

## Format d'une sauvegarde

Chaque sauvegarde est une archive `.tar.gz` qui contient :
- `manifest.json` - métadonnées, version, empreinte d'intégrité SHA256
- `dump.sql` - le dump PostgreSQL complet

## Sauvegarde manuelle

Une sauvegarde se déclenche depuis le panneau d'administration ou par l'API :

```bash
curl -X POST -H "Authorization: Bearer <token>" http://localhost:9091/api/v1/admin/backups
```

## Restauration

La restauration depuis une sauvegarde passe par l'API d'administration. L'intégrité de l'archive est vérifiée par son empreinte SHA256 avant toute application.

## Stockage persistant

Assurez-vous que `EVEY_BACKUP_DIR` pointe vers un volume persistant ou un chemin de l'hôte. La valeur par défaut, `/data/backups`, se trouve à l'intérieur du volume Docker `evey-data`.

Pour ranger les sauvegardes sur l'hôte :
```yaml title="docker-compose.yml"
services:
  evey:
    volumes:
      - /path/on/host/backups:/data/backups
```
