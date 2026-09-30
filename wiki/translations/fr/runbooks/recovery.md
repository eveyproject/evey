---
source_sha: 75f950590ae90c9a9a77cfcea37f93cbbe8eb70a0dbea1df56d154a205d2fc12
reviewed: true
---
# Récupération

Les procédures en ligne de commande pour les cas où l'on ne peut plus se
connecter, où la clé JWT est peut-être compromise, ou quand la base de données
doit revenir à une sauvegarde d'avant migration.

Toutes les commandes s'exécutent sous la forme `evey admin <sous-commande>` et
lisent leur configuration dans les mêmes variables `EVEY_*` que le serveur. Elles
ne démarrent **pas** le serveur, et la plupart peuvent tourner sans risque
pendant qu'il est arrêté.

## reset-password

Génère un nouveau mot de passe pour un utilisateur existant. Il est affiché une
seule fois sur la sortie standard : notez-le immédiatement.

```sh
evey admin reset-password --user admin@example.com
```

Sortie :
```
user:     admin@example.com
user_id:  usr-xxxxxxxx
password: <random 24-byte base64url>
```

Remarques :

- Le champ `updated_at` de l'utilisateur est mis à jour, rien d'autre ne
  change.
- Les sessions existantes restent valides jusqu'à leur expiration normale. Pour
  aussi déconnecter l'utilisateur partout, lancez ensuite `rotate-jwt-key`.
- Le mot de passe en clair n'est jamais journalisé. Si vous perdez la sortie, il
  faut relancer la commande.

## rotate-jwt-key

Invalide de force toutes les sessions actives en remplaçant la clé de signature
RSA.

```sh
evey admin rotate-jwt-key
```

Comportement :

- Le fichier de clé actuel (chemin : `EVEY_JWT_PRIVATE_KEY_PATH`, par défaut
  `/data/jwt.key`) est renommé en `<chemin>.rotated-<horodatage UTC>`.
- Au démarrage suivant du serveur, `LoadOrGenerateRSAKey` génère une nouvelle
  clé RSA de 2048 bits.
- Tous les jetons d'accès émis auparavant échouent à la vérification de
  signature : tous les utilisateurs sont déconnectés.
- Les jetons de rafraîchissement sont des JWT signés avec la même clé : ils sont
  morts eux aussi, sans nettoyage séparé.

Quand l'utiliser :

- Fuite de clé soupçonnée.
- Besoin de couper toutes les sessions d'un coup (réponse à un incident, départ
  d'un prestataire).

## restore-backup

Applique un `pg_dump.gz` produit par la sauvegarde d'avant migration (ou par
toute autre source) sur la base de données configurée. **Écrase les données
existantes.**

```sh
evey admin restore-backup /data/backups/pre-migration-15-20260418T220130Z.sql.gz
```

Déroulement :

1. La commande attend 5 secondes avant de faire quoi que ce soit : Ctrl+C
   l'interrompt.
2. Le fichier passe par `gunzip | psql` vers la base configurée, avec
   `--single-transaction` et `ON_ERROR_STOP=1`.
3. À la moindre erreur, psql annule la transaction : la base reste dans son état
   précédent.

Prérequis :

- `psql` et `gunzip` doivent être dans le PATH (tous deux sont livrés dans
  l'image de production, avec `pg_dump`).
- La base cible doit exister ; le dump ne contient que les DROP/CREATE des
  tables, pas la base elle-même.
- L'utilisateur `EVEY_DB_USER` configuré doit être propriétaire de la base
  cible.

Les sauvegardes d'avant migration sont produites automatiquement avant chaque
`migrate.Up()` et rangées dans `EVEY_BACKUP_DIR` (par défaut `/data/backups`).
Les 3 plus récentes sont conservées ; les plus anciennes sont supprimées.
