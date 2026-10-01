---
source_sha: 07ef437cfda80f904ab9b75af363b1bc953955e06ee27538265b4d7564457422
reviewed: true
---
# Dépannage

Partez du symptôme. Chaque entrée indique quoi vérifier d'abord et où se
trouve la procédure détaillée. Les procédures d'incident vers lesquelles
renvoie cette page sont pour l'instant en anglais.

## Premières vérifications

Deux commandes répondent à la plupart des questions sur un serveur qui se
comporte mal :

```bash
docker compose logs --tail=200 evey
docker exec evey-server evey admin doctor
```

`evey admin doctor` vérifie la base de données et son schéma, la clé de
signature, l'index de recherche, les montages de médias (y compris la
possibilité pour le serveur d'écrire dans chaque bibliothèque), les dossiers
de cache, ffmpeg, l'accès distant et l'espace disque libre. Il affiche une
ligne par vérification avec un remède, et se termine en erreur quand l'une
d'elles échoue.

Dans le panneau d'administration, **Logs & Debug** affiche les mêmes logs,
avec recherche et filtre par domaine (lecture, transcodage, scan...).

## Le serveur ne démarre pas

| Ce que disent les logs | Ce que cela signifie |
| --- | --- |
| Un message de validation de la configuration | Avec une base externe, le serveur refuse le mot de passe par défaut `evey` ; avec un Meilisearch externe, une clé vide ou factice. Corrigez la valeur nommée dans le message. |
| Des lignes commençant par `evey:` à propos de Postgres | La base intégrée a refusé de démarrer : une base d'une autre version majeure de Postgres ([montée de version majeure](advanced-deployments.md#postgres-major-upgrade)), un dossier de données qui n'est pas une base Postgres, un disque plein, ou `./evey-data/postgres` sur un partage réseau. |
| `permission denied` sur `/cache/...`, en boucle de redémarrage | Le volume `evey-cache` appartient à un autre utilisateur que celui sous lequel tourne le serveur. Mettez l'image à jour ; sur une image antérieure au 17/09/2026, lancez une fois `docker run --rm -v <projet>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

Mot de passe administrateur perdu, clé de signature compromise ou base
endommagée : [Restauration](../runbooks/recovery.md).

## Le serveur est injoignable depuis l'extérieur

Le serveur répond à la maison mais pas ailleurs, ou les applications
affichent un message indiquant que l'accès distant est désactivé : voir
[Accès distant et HTTPS](../administration/remote-access.md). La page
d'administration **Accès distant** signale en rouge un serveur joignable depuis
Internet alors que l'accès distant est désactivé.

Rien ne répond, même à la maison :
[Server not reachable](../runbooks/incident-runbook.md#1-server-not-reachable).

## Tout le monde est déconnecté, ou plus aucune connexion ne fonctionne

[All logins return 401](../runbooks/incident-runbook.md#2-all-logins-return-401).
Les causes habituelles sont la base de données (chaque requête vérifie le compte
auprès d'elle et est refusée quand elle ne répond pas ; le journal le dit) et un
reverse proxy absent des **Proxys de confiance**, qui place tout le monde sous
une seule limite de connexions. Une clé de signature perdue ou remplacée ne
déconnecte personne à elle seule : les applications renouvellent leurs jetons.

## Un titre est transcodé alors que vous attendiez une lecture directe

Ouvrez les **Infos du flux** dans le lecteur : elles indiquent le mode de
lecture et ses raisons. Pour les lire, voir
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).

## La lecture s'arrête, met en mémoire tampon ou ne démarre pas

- Un seul titre concerné : ouvrez **Admin → Santé des médias**. Un fichier
  sans index de navigation, un passage endommagé ou des pistes rangées par
  blocs expliquent les coupures et les déplacements lents dans le film, et la
  plupart se réparent sur place. Voir [Santé des médias](../runbooks/media-health.md).
- Plusieurs titres, quand plusieurs personnes regardent : le serveur est
  peut-être au bout de sa capacité de transcodage. Voir
  [Transcodage et capacité du serveur](../playback/transcoding.md).
- La lecture transcodée échoue franchement :
  [Transcoding fails](../runbooks/incident-runbook.md#3-transcoding-fails--videos-wont-play-transcoded).
- Ça coupe et les logs ne disent pas pourquoi :
  [la procédure de diagnostic](../runbooks/incident-runbook.md#3c-it-buffers--it-cut-out-and-the-logs-do-not-say-why).
- **Admin → Qualité streaming** montre, pour les vrais spectateurs sur le
  dernier jour ou la dernière semaine, à quelle fréquence la lecture a
  attendu, et quels titres et appareils sont concernés.

## Pas de son, ou son désynchronisé

- Dans un navigateur uniquement, avec une piste Dolby Digital (AC-3/E-AC-3) :
  un reverse proxy qui impose sa propre Content-Security-Policy peut bloquer
  le décodeur utilisé par le lecteur web. Voir
  [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Désynchronisé sur un titre : **Admin → Santé des médias** détecte un audio
  qui démarre après l'image et des pistes en désaccord entre elles.

## Les sous-titres ne s'affichent pas

Voir [Sous-titres](../playback/subtitles.md) : quels formats les lecteurs
dessinent, et lesquels le serveur doit incruster dans l'image.

## Des affiches manquent ou sont cassées

Les affiches et les fonds sont copiés sur le serveur pendant les scans, dans
`./evey-data/artworks`, et les manquants sont retentés toutes les heures.
**Admin → Cache artworks** indique combien n'ont pas encore de copie locale ;
**Télécharger les artworks manquants** les récupère tout de suite, et **Purger
les variantes corrompues** supprime les affiches redimensionnées laissées vides
par un redimensionnement raté, qui sont alors reconstruites à la demande.

## La recherche ne renvoie rien

Reconstruisez l'index depuis la base :
`docker exec evey-server evey admin reindex-search`. L'index peut toujours être
reconstruit, rien n'est perdu.

## Une diffusion s'est figée ou est devenue muette

[A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#8-a-cast-froze-went-silent-or-the-phone-keeps-spinning).

## Un plugin ne fonctionne plus

[Plugin crashed](../runbooks/incident-runbook.md#7-plugin-crashed). Ses
propres logs se trouvent sur sa carte dans **Admin → Plugins**.

## Le disque se remplit

Le cache de transcodage en est la cause habituelle :
[Transcode cache disk full](../runbooks/incident-runbook.md#6-transcode-cache-disk-full).
Il peut être vidé à tout moment depuis **Admin → Cache de transcodage**.
