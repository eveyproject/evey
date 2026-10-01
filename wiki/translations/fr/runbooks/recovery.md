---
source_sha: 64f12b2cc77ed7069bf33f581c2d91e3b13b6ff04cc4a3bc62db72c3607b46fe
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

- Toutes les sessions de l'utilisateur prennent fin : ses jetons de
  rafraîchissement sont supprimés et ses jetons d'accès révoqués. Une
  application ouverte s'en aperçoit en 30 secondes environ et redemande la
  connexion.
- Le mot de passe en clair n'est jamais journalisé. Si vous perdez la sortie, il
  faut relancer la commande.

## rotate-jwt-key

Remplace la clé RSA qui signe les jetons d'accès.

```sh
evey admin rotate-jwt-key
```

Comportement :

- Le fichier de clé actuel (chemin : `EVEY_JWT_PRIVATE_KEY_PATH`, par défaut
  `/data/jwt.key`) est renommé en `<chemin>.rotated-<horodatage UTC>`.
- Au démarrage suivant du serveur, `LoadOrGenerateRSAKey` génère une nouvelle
  clé RSA de 2048 bits.
- Toutes les sessions prennent fin immédiatement, sans attendre le
  redémarrage : la commande supprime tous les jetons de rafraîchissement et
  révoque tous les jetons d'accès en base. Les applications ouvertes s'en
  aperçoivent en 30 secondes environ et tout le monde se reconnecte.
- Si la base est injoignable, la clé est quand même mise de côté et la
  commande le signale : relancez-la quand la base répond.

Quand l'utiliser :

- Fuite de clé soupçonnée : plus personne ne peut fabriquer de jeton d'accès
  valide avec l'ancienne clé.
- Coupure de toutes les sessions. Pour un seul compte, le désactiver dans
  **Admin → Utilisateurs**, ou `reset-password`, suffit.

## restore-backup

Applique un `pg_dump.gz` produit par la sauvegarde d'avant migration (ou par
toute autre source) sur la base de données configurée. **Écrase les données
existantes.** Le fichier doit se terminer par `.sql.gz` : les sauvegardes
`.tar.gz` de **Admin → Sauvegarde BDD et restauration** se restaurent depuis le panneau,
pas par cette commande.

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
Le dump le plus récent de chacune des 3 dernières versions du schéma est
conservé ; les autres sont supprimés.
