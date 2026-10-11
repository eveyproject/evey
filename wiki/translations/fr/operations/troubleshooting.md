---
source_sha: 03e396675cd3de69ac6a467253bd4e526fd42614bb5aedfc7904f1acc60be5e2
reviewed: true
---
# Dépannage

Quelque chose ne va pas et vous voulez savoir où regarder en premier.
Trouvez votre symptôme dans le tableau, ouvrez la page ou la section du
panneau qu'il indique, et revenez ici pour les logs et les procédures de
restauration quand cela ne suffit pas.

## Où regarder, selon le symptôme

| Symptôme | Regarder d'abord | Ensuite |
| --- | --- | --- |
| Rien ne répond, même à la maison | `docker compose ps`, puis `docker compose logs --tail=200 evey` | [Le serveur ne démarre pas](#the-server-does-not-start) ci-dessous ; [Server not reachable](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| L'application dit **Impossible de joindre le serveur** ou **Connexion échouée** | L'adresse saisie dans l'application : [Applications et appareils](../getting-started/apps.md#the-server-address-to-enter) | Hors de chez vous seulement : [Accès distant et HTTPS](../administration/remote-access.md) |
| Tout le monde est déconnecté, plus aucune connexion ne passe | **Admin → Accès distant → Proxys de confiance**, et la base de données dans les logs | [Personne ne peut se connecter](#nobody-can-sign-in) ci-dessous |
| Pas d'image, la lecture ne démarre pas, ou elle s'arrête | **Infos du flux** dans le lecteur, puis **Admin → Santé des médias** pour ce titre | [Problèmes de lecture](#playback-problems) ci-dessous |
| Ça saccade ou ça charge sans arrêt | **Admin → Qualité streaming** (quels titres et appareils attendent) et **Admin → Santé du système** (le serveur est-il à sa capacité) | [Transcodage et capacité du serveur](../playback/transcoding.md) |
| Mauvaise affiche, mauvais titre, un film non reconnu | **Modifier les métadonnées** ou **Réidentifier** sur le titre | [Corriger un titre faux ou manquant](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| Des affiches manquent ou sont cassées | **Admin → Cache artworks** | [Bibliothèque et affiches](#library-and-posters) ci-dessous |
| Un plugin échoue | Ses **Logs** sur sa carte dans **Admin → Plugins** | [Plugin crashed](../runbooks/incident-runbook.md#a-plugin-crashed) |
| Une diffusion s'est figée ou est devenue muette | **Admin → En cours de lecture** (la session est-elle encore là) | [A cast froze, went silent, or the phone keeps spinning](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| Le disque se remplit | **Admin → Cache de transcodage** | [Transcode cache disk full](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## Les logs

Le serveur écrit son journal dans `/data/logs/evey-AAAA-MM-JJ.log` à
l'intérieur du conteneur, c'est-à-dire `./evey-data/logs/` à côté de votre
fichier compose. Un fichier par jour, en JSON, conservé 90 jours par défaut
(`EVEY_LOG_RETENTION_DAYS`).

**Admin → Logs & Debug** lit ces mêmes fichiers : recherche plein texte, suivi
en **Direct**, et un filtre **Zone** qui restreint à un domaine (lecteur et
transcodage, index de lecture, sous-titres, détection d'intro, Chromecast,
authentification, infrastructure...). Quand un spectateur signale un problème,
c'est le premier endroit où regarder.

`docker compose logs evey` montre ce que le conteneur affiche pendant qu'il
démarre : les lignes `evey:` du script de démarrage, la base de données
intégrée et la bannière. C'est là qu'il faut lire quand le serveur ne se lance
pas ; une fois qu'il tourne, le journal applicatif est dans le fichier et dans
le panneau.

## `evey admin doctor`

Une seule commande vérifie tout le serveur et affiche une ligne par
contrôle, `ok`, `WARN` ou `FAIL`, avec un remède :

```bash
docker exec evey-server evey admin doctor
```

Elle couvre la base de données et son schéma, la clé de signature, l'index de
recherche et son écart avec la base, les montages de médias (y compris la
possibilité pour le serveur d'écrire dans chaque bibliothèque), les dossiers de
cache et de logs, `ffmpeg` et `mkvgo`, les proxys de confiance, l'accès distant
et l'espace disque libre. `--repair` n'applique que les corrections sûres
(créer et réattribuer les dossiers d'écriture manquants). Elle se termine en
erreur quand un contrôle échoue. Les autres commandes sont dans
[Ligne de commande](../runbooks/cli.md).

## Le serveur ne démarre pas

| Ce que dit `docker compose logs evey` | Ce que cela signifie |
| --- | --- |
| Un message de validation de la configuration | Avec une base externe, le serveur refuse le mot de passe par défaut `evey` ; avec un Meilisearch externe, une clé vide ou factice. Corrigez la valeur nommée dans le message. |
| Des lignes commençant par `evey:` à propos de Postgres | La base intégrée a refusé de démarrer : une base d'une autre version majeure de Postgres ([montée de version majeure](advanced-deployments.md#move-to-a-newer-postgres-major)), un dossier de données qui n'est pas une base Postgres, un disque plein, ou `./evey-data/postgres` sur un partage réseau. |
| `permission denied` sur `/cache/...`, en boucle de redémarrage | Le volume `evey-cache` appartient à un autre utilisateur que celui sous lequel tourne le serveur. Mettez l'image à jour ; sur une image antérieure au 17/09/2026, lancez une fois `docker run --rm -v <projet>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

Mot de passe administrateur perdu, clé de signature compromise ou base
endommagée : [Restauration](../runbooks/recovery.md).

## Personne ne peut se connecter

Chaque requête vérifie le compte auprès de la base de données et est refusée
quand elle ne répond pas ; le journal le dit. L'autre cause habituelle est un
reverse proxy absent de **Admin → Accès distant → Proxys de confiance** : tous
les visiteurs partagent alors l'adresse du proxy et une seule limite de
tentatives de connexion. La procédure est
[All logins return 401](../runbooks/incident-runbook.md#every-login-returns-401).

!!! note
    Une clé de signature perdue ou remplacée ne déconnecte personne à elle
    seule : les applications renouvellent leurs jetons. Pour mettre fin à
    toutes les sessions volontairement, c'est `evey admin rotate-jwt-key`,
    voir [Restauration](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens).

Le serveur répond à la maison mais pas ailleurs, ou les applications disent
que l'accès distant est désactivé :
[Accès distant et HTTPS](../administration/remote-access.md). La page
**Admin → Accès distant** signale en rouge un serveur joignable depuis Internet
alors que l'accès distant est désactivé.

### La connexion SSO échoue

La page de connexion en donne la raison :

- **domaine non autorisé** : l'adresse est hors des domaines e-mail autorisés
  de la carte ;
- **aucun compte ne correspond** : la création de comptes est désactivée ;
  activez **Créer un compte pour les nouveaux utilisateurs SSO** ou créez le
  compte ;
- **compte désactivé** ;
- **le compte existe mais n'a pas pu être relié** : le fournisseur n'a pas
  confirmé l'e-mail, la liaison est désactivée, ou le compte est relié à une
  autre identité ;
- **indisponible** : le fournisseur est injoignable ; le **Statut** de la carte
  et **Tester la connexion** montrent l'erreur.

Une page d'erreur affichée par le fournisseur lui-même signifie le plus souvent
que l'URI de redirection déclarée chez lui n'est pas celle qu'affiche la carte.
Voir [Authentification unique (OIDC)](../oidc.md).

## Les e-mails n'arrivent pas

**Envoyer un e-mail de test** dans **Admin → Configuration du serveur → E-mail /
SMTP** envoie un message à votre propre adresse avec les réglages enregistrés,
et affiche la réponse du serveur de messagerie en cas d'échec. Un mode TLS qui
ne correspond pas au port (`starttls` avec 587, `ssl` avec 465) est l'erreur
habituelle. Un hôte enregistré dans la carte remplace toutes les variables
`EVEY_SMTP_*`. Les envois de notifications qui échouent sont journalisés comme
`email: send notification`. Voir [Notifications](../notifications.md).

## Problèmes de lecture

Les **Infos du flux** dans le lecteur nomment le mode de lecture (envoyé tel
quel, copié ou réencodé, piste par piste) et ses raisons. Un titre transcodé
alors que vous attendiez une lecture directe s'explique là : voir
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).

- Un seul titre s'arrête, charge ou se déplace lentement : ouvrez **Admin →
  Santé des médias**. Un fichier sans index de lecture, une zone endommagée ou
  des pistes rangées par blocs l'expliquent, et la plupart se réparent sur
  place. Voir [Santé des médias](../runbooks/media-health.md).
- Plusieurs titres, quand plusieurs personnes regardent : le serveur est
  peut-être au bout de sa capacité de transcodage. **Admin → Santé du système**
  montre la part utilisée. Voir
  [Transcodage et capacité du serveur](../playback/transcoding.md).
- La lecture transcodée échoue franchement :
  [Transcoding fails](../runbooks/incident-runbook.md#transcoded-playback-fails).
- Ça charge et les logs ne disent pas pourquoi :
  [la procédure de diagnostic](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why).
- Pas de son dans un navigateur uniquement, avec une piste Dolby Digital
  (AC-3/E-AC-3) : un reverse proxy qui impose sa propre Content-Security-Policy
  peut bloquer le décodeur utilisé par le lecteur web. Voir
  [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Son désynchronisé sur un titre : **Admin → Santé des médias** détecte un
  audio qui démarre après l'image et des pistes en désaccord entre elles.
- Les sous-titres ne s'affichent pas : [Sous-titres](../playback/subtitles.md)
  dit quels formats les lecteurs dessinent et lesquels le serveur doit
  incruster dans l'image.
- **Sous-titres en préparation…** reste affiché plusieurs minutes : le fichier
  n'a pas d'index pour cette piste et le serveur le lit en entier, une fois.
  Voir [La première fois qu'un sous-titre s'affiche](../playback/subtitles.md#the-first-time-a-subtitle-is-shown).
- Des titres transcodés refusent de démarrer ou se bloquent alors que les
  titres déjà lus fonctionnent, ou les administrateurs reçoivent **Cache des
  sous-titres plein** : le disque est passé sous sa marge d'espace libre. Voir
  [Le cache de transcodage](../playback/transcoding.md#the-transcoding-cache).

## Bibliothèque et affiches

Les affiches et les fonds sont copiés sur le serveur pendant les scans, dans
`./evey-data/artworks`, et les manquants sont retentés toutes les heures.
**Admin → Cache artworks** indique combien n'ont pas encore de copie locale ;
**Télécharger les artworks manquants** les récupère tout de suite, et **Purger
les variantes corrompues** supprime les affiches redimensionnées laissées vides
par un redimensionnement raté, qui sont alors reconstruites à la demande.

Une affiche ou un titre qui appartient à un autre film relève de
l'identification des métadonnées, pas des affiches :
[Corriger un titre faux ou manquant](../administration/libraries.md#fix-a-wrong-or-missing-title).

Un scan se termine par **Scan partiel - ces dossiers n'ont pas pu être lus :**
suivi de chemins : Evey ne peut pas lire ces dossiers (droits, partage réseau
déconnecté). Le reste de la bibliothèque est analysé, et les titres d'un
dossier illisible sont conservés plutôt que retirés. Vérifiez le montage et le
propriétaire (`PUID`/`PGID`, voir [Installation](../installation.md#common-variations)),
puis relancez le scan.

La recherche ne renvoie rien : reconstruisez l'index depuis la base avec
`docker exec evey-server evey admin reindex-search`. L'index peut toujours être
reconstruit, rien n'est perdu.

## Signaler un problème

Écrivez à **contact@eveyproject.org**. Un signalement exploitable indique :

- ce que vous avez fait, ce que vous attendiez, et ce qui s'est passé à la
  place ;
- quand c'est arrivé, à la minute près : c'est par l'heure qu'on retrouve les
  lignes dans les logs ;
- l'appareil et l'application : **Paramètres → À propos → Version de l'app**
  dans l'application Android, le navigateur et sa version pour l'application
  web ;
- la version du serveur : `docker exec evey-server evey version` ;
- pour un problème de lecture, le titre et une capture des **Infos du flux**
  (voir [Lire les Infos du flux](../playback/how-playback-is-decided.md#reading-stream-info)) :
  elles nomment le mode de lecture choisi et ses raisons, soit l'essentiel du
  diagnostic.

Un administrateur peut y ajouter les lignes de log autour de ce moment
(**Admin → Logs & Debug**, voir [Les logs](#the-logs)) et le rapport de
[`evey admin doctor`](#evey-admin-doctor). Les logs nomment des comptes et
des adresses réseau : relisez-les avant de les envoyer.

Une faille de sécurité ne se signale pas dans un endroit public : écrivez à
**security@eveyproject.org**, et laissez le temps de la corriger avant d'en
parler.
