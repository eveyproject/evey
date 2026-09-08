---
source_sha: 75f950590ae90c9a9a77cfcea37f93cbbe8eb70a0dbea1df56d154a205d2fc12
---

# Runbook de récupération

Les procédures en ligne de commande pour les cas où vous ne pouvez plus vous
connecter, où la clé JWT est soupçonnée compromise, ou encore où la base doit
revenir à une sauvegarde d'avant migration.

Toutes ces commandes s'invoquent en `evey admin <subcommand>` et lisent leur
configuration dans les mêmes variables `EVEY_*` que le serveur. Elles ne
démarrent **pas** le serveur : la plupart s'exécutent sans risque serveur
arrêté.

## reset-password

Génère un nouveau mot de passe pour un utilisateur existant. Le mot de passe
n'est affiché qu'une fois, sur la sortie standard : notez-le immédiatement.

```sh
evey admin reset-password --user admin@example.com
```

Sortie :
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

À savoir :
- L'`updated_at` de la fiche utilisateur est mis à jour ; rien d'autre ne change.
- Les sessions déjà ouvertes restent valides jusqu'à leur expiration naturelle.
  Pour déconnecter aussi l'utilisateur partout, enchaînez avec
  `rotate-jwt-key`.
- Le mot de passe en clair n'est jamais journalisé. Si vous perdez la sortie,
  relancez la commande.

## rotate-jwt-key

Invalide de force toutes les sessions actives, en remplaçant la clé de signature
RSA.

```sh
evey admin rotate-jwt-key
```

Comportement :
- Le fichier de clé courant (chemin = `EVEY_JWT_PRIVATE_KEY_PATH`, par défaut
  `/data/jwt.key`) est renommé en `<path>.rotated-<UTC-timestamp>`.
- Au démarrage suivant du serveur, `LoadOrGenerateRSAKey` régénère une clé RSA
  de 2048 bits toute neuve.
- Tous les jetons d'accès déjà émis échouent à la vérification de signature →
  tout le monde est déconnecté.
- Les jetons de rafraîchissement sont des JWT signés par la même clé : ils
  meurent avec elle, aucun nettoyage séparé n'est nécessaire.

Quand s'en servir :
- Fuite de clé soupçonnée.
- Besoin de tuer toutes les sessions d'un coup (réponse à incident, départ d'un
  prestataire).

## restore-backup

Applique dans la base configurée un `pg_dump.gz` produit par le mécanisme de
sauvegarde d'avant migration, ou par toute autre source. **Écrase les données
existantes.**

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Déroulé :
1. La commande marque une pause de 5 secondes avant d'agir : Ctrl+C interrompt.
2. Le fichier est passé en flux dans `gunzip | psql` contre la base configurée,
   avec `--single-transaction` et `ON_ERROR_STOP=1`.
3. À la moindre erreur, psql annule la transaction : la base reste dans son état
   antérieur.

Prérequis :
- `psql` et `gunzip` doivent être dans le PATH (tous deux sont livrés dans
  l'image de production, aux côtés de `pg_dump`).
- La base cible doit exister : le dump ne porte les DROP/CREATE que des tables,
  pas de la base elle-même.
- L'`EVEY_DB_USER` configuré doit être propriétaire de la base cible.

Les sauvegardes d'avant migration sont produites automatiquement avant chaque
`migrate.Up()` et résident sous `EVEY_BACKUP_DIR` (`/data/backups` par défaut).
Les 3 plus récentes sont conservées, les plus anciennes élaguées.
