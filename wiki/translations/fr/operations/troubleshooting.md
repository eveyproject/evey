---
source_sha: 58ae53ccc12bc8dbe6e691e81203c38189833f7fe97b8f072c82a6641ae48c30
reviewed: true
---
# Dépannage

Quelque chose ne fonctionne pas ? Cherchez ce que vous voyez dans le tableau
ci-dessous. La colonne du milieu indique où regarder en premier ; la dernière
mène à la solution. Si cela ne suffit pas, les logs et `evey admin doctor`
(plus bas) donnent plus de détails.

## Où regarder, selon ce que vous voyez

| Ce que vous voyez | Regarder d'abord | Ensuite |
| --- | --- | --- |
| Rien ne répond, même sur le réseau domestique | Evey tourne-t-il ? `docker compose ps`, puis les dernières lignes qu'il affiche : `docker compose logs --tail=200 evey` | [Le serveur ne démarre pas](#the-server-does-not-start) ci-dessous ; [Le serveur ne répond pas](../runbooks/incident-runbook.md#the-server-does-not-answer) |
| L'application affiche **Impossible de joindre le serveur** ou **Connexion échouée** | L'adresse saisie dans l'application | [Connecter les applications](../getting-started/apps.md#the-server-address-to-enter) ; hors de chez vous seulement : [Accès distant et HTTPS](../administration/remote-access.md) |
| Tout le monde est déconnecté, ou personne ne peut se connecter | **Administration → Accès distant**, les proxys de confiance ; puis les logs | [Personne ne peut se connecter](#nobody-can-sign-in) ci-dessous |
| Un film n'a pas d'image, ne démarre pas, ou s'arrête | **Infos du flux** dans le lecteur, puis **Administration → Santé des médias** pour ce titre | [Problèmes de lecture](#playback-problems) ci-dessous |
| La lecture saccade ou charge sans arrêt | **Administration → Qualité streaming** (quels titres et appareils attendent) et **Administration → Santé du système** (le serveur est-il trop occupé) | [Transcodage et capacité du serveur](../playback/transcoding.md) |
| Mauvaise affiche, mauvais titre, ou un film n'est pas reconnu | **Modifier les métadonnées** sur le titre | [Corriger un titre faux ou manquant](../administration/libraries.md#fix-a-wrong-or-missing-title) |
| Des affiches manquent ou sont cassées | **Administration → Cache artworks** | [Bibliothèque et affiches](#library-and-posters) ci-dessous |
| Les nouveaux fichiers n'apparaissent pas | Le résultat du scan sur la ligne de la bibliothèque, dans **Administration → Bibliothèques** | [Bibliothèque et affiches](#library-and-posters) ci-dessous |
| Les e-mails n'arrivent pas | **Envoyer un e-mail de test** dans **Administration → Configuration du serveur → E-mail / SMTP** | [Les e-mails n'arrivent pas](#e-mails-do-not-arrive) ci-dessous |
| Un plugin échoue | Ses **Logs**, sur sa carte dans **Administration → Plugins** | [Un plugin a planté](../runbooks/incident-runbook.md#a-plugin-crashed) |
| Une diffusion s'est figée ou est devenue muette | **Administration → En cours de lecture** : la lecture est-elle encore dans la liste ? | [Une diffusion s'est figée, est devenue muette, ou le téléphone reste en chargement](../runbooks/incident-runbook.md#a-cast-froze-went-silent-or-the-phone-keeps-spinning) |
| Le disque se remplit | **Administration → Cache de transcodage** | [Le disque de cache est plein](../runbooks/incident-runbook.md#the-cache-disk-is-full) |

## Les logs

Les logs sont le journal d'Evey : chaque erreur y est notée, avec
l'heure.

**Le plus simple : Administration → Logs & Debug.** Vous y voyez les logs
avec une zone de recherche, une vue **Direct** qui suit les nouvelles lignes,
et un filtre **Zone** pour un seul domaine (lecteur et conversion,
sous-titres, détection d'intro, Chromecast, connexion...). Quand quelqu'un
signale un problème, regardez ici en premier, autour de l'heure qu'on vous
donne.

??? info "Où sont les fichiers de logs"

    - Les fichiers sont dans `./evey-data/logs/`, à côté de votre fichier
      compose (`/data/logs/evey-YYYY-MM-DD.log` dans le conteneur). Un
      fichier par jour, en JSON, gardé 90 jours par défaut
      (`EVEY_LOG_RETENTION_DAYS`).
    - `docker compose logs evey` montre autre chose : ce que le conteneur
      affiche pendant qu'il démarre (les lignes `evey:` du script de
      démarrage, la base de données, la bannière). Lisez-le quand le serveur
      ne démarre pas. Une fois qu'il tourne, les logs sont dans les fichiers
      et dans le panneau.

## `evey admin doctor`

Une seule commande vérifie tout le serveur. Elle affiche une ligne par
vérification, `ok`, `WARN` ou `FAIL`, avec ce qu'il faut faire :

```bash
docker exec evey-server evey admin doctor
```

??? info "Ce qu'elle vérifie"

    La base de données et son schéma, la clé de signature, l'index de
    recherche (et s'il correspond à la base de données), les dossiers de
    médias (y compris si Evey peut écrire dans chaque bibliothèque), les
    dossiers de cache et de logs, `ffmpeg` et `mkvgo`, les proxys de
    confiance, l'accès distant, et la place libre sur le disque. `--repair`
    n'applique que les corrections sans risque (créer les dossiers qui
    manquent et leur donner le bon propriétaire). Elle se termine avec un
    code d'erreur quand une vérification échoue. Les autres commandes sont
    dans [Ligne de commande](../runbooks/cli.md).

## Le serveur ne démarre pas

| Ce que dit `docker compose logs evey` | Ce que cela signifie |
| --- | --- |
| Un message de validation de la configuration | Avec une base externe, le serveur refuse le mot de passe par défaut `evey` ; avec un Meilisearch externe, une clé vide ou factice. Corrigez la valeur nommée dans le message. |
| Des lignes commençant par `evey:` à propos de Postgres | La base intégrée a refusé de démarrer : une base d'une autre version majeure de Postgres ([montée de version majeure](advanced-deployments.md#move-to-a-newer-postgres-major)), un dossier de données qui n'est pas une base Postgres, un disque plein, ou `./evey-data/postgres` sur un partage réseau. |
| `permission denied` sur `/cache/...`, en boucle de redémarrage | Le volume `evey-cache` appartient à un autre utilisateur que celui sous lequel tourne le serveur. Mettez l'image à jour ; sur une image antérieure au 17/09/2026, lancez une fois `docker run --rm -v <projet>_evey-cache:/c alpine chown -R <PUID>:<PGID> /c`. |

Mot de passe administrateur perdu, clé de signature compromise ou base
endommagée : [Restauration](../runbooks/recovery.md).

## Personne ne peut se connecter

Deux causes habituelles :

- **La base de données ne répond pas.** Evey vérifie chaque connexion
  auprès d'elle, et refuse quand il ne peut pas. Les logs l'indiquent.
- **Un reverse proxy manque dans les proxys de confiance** (**Administration
  → Accès distant**). Tous les visiteurs semblent alors venir de l'adresse du
  proxy, et ils partagent tous une seule limite de tentatives de connexion.

La procédure pas à pas :
[Toutes les connexions renvoient 401](../runbooks/incident-runbook.md#every-login-returns-401).

!!! note
    Une clé de signature perdue ou remplacée ne déconnecte personne à elle
    seule : les applications renouvellent leurs jetons. Pour mettre fin à
    toutes les sessions volontairement, c'est `evey admin rotate-jwt-key`,
    voir [Restauration](../runbooks/recovery.md#rotate-the-key-that-signs-access-tokens).

Le serveur répond sur le réseau domestique mais pas ailleurs, ou les
applications indiquent que l'accès distant est désactivé :
[Accès distant et HTTPS](../administration/remote-access.md). La page
**Administration → Accès distant** signale en rouge un serveur joignable depuis Internet
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
- **indisponible** : le fournisseur est injoignable ; l'**État** de la carte
  et **Tester la connexion** montrent l'erreur.

Une page d'erreur affichée par le fournisseur lui-même signifie le plus souvent
que l'URI de redirection déclarée chez lui n'est pas celle qu'affiche la carte.
Voir [Authentification unique (OIDC)](../oidc.md).

## Les e-mails n'arrivent pas

**Envoyer un e-mail de test** dans **Administration → Configuration du serveur → E-mail /
SMTP** envoie un message à votre propre adresse avec les réglages enregistrés,
et affiche la réponse du serveur de messagerie en cas d'échec. Un mode TLS qui
ne correspond pas au port (`starttls` avec 587, `ssl` avec 465) est l'erreur
habituelle. Un hôte enregistré dans la carte remplace toutes les variables
`EVEY_SMTP_*`. Les envois de notifications qui échouent sont journalisés comme
`email: send notification`. Voir [Notifications](../notifications.md).

## Problèmes de lecture

Commencez par les **Infos du flux** dans le lecteur. Elles indiquent comment le
film est envoyé (tel quel, recopié dans un autre format, ou converti), piste
par piste, et pourquoi. Un titre converti alors que vous attendiez qu'il soit
lu tel quel : voir
[Comment Evey choisit le mode de lecture](../playback/how-playback-is-decided.md).

- **Un seul titre** s'arrête, charge sans arrêt, ou met du temps à sauter à
  un autre moment : ouvrez **Administration → Santé des médias**. Les causes
  habituelles sont un fichier sans index de lecture, un passage abîmé, ou des
  pistes rangées en gros blocs. La plupart peuvent être réparées. Voir
  [Santé des médias](../runbooks/media-health.md).
- **Plusieurs titres, quand plusieurs personnes regardent** : le serveur est
  peut-être au maximum de ce qu'il peut convertir. **Administration → Santé
  du système** montre à quel point il est occupé. Voir
  [Transcodage et capacité du serveur](../playback/transcoding.md).
- La lecture transcodée échoue complètement :
  [La lecture transcodée échoue](../runbooks/incident-runbook.md#transcoded-playback-fails).
- La lecture se met en mémoire tampon et les logs n'indiquent pas pourquoi :
  [la procédure de diagnostic](../runbooks/incident-runbook.md#it-buffers-or-cuts-out-and-the-log-does-not-say-why).
- Pas de son dans un navigateur uniquement, avec une piste Dolby Digital
  (AC-3/E-AC-3) : un reverse proxy qui impose sa propre Content-Security-Policy
  peut bloquer le décodeur utilisé par le lecteur web. Voir
  [Audio](../playback/audio.md#dolby-digital-in-a-browser).
- Son désynchronisé sur un titre : **Administration → Santé des médias** détecte un
  audio qui démarre après l'image et des pistes en désaccord entre elles.
- Les sous-titres ne s'affichent pas : [Sous-titres](../playback/subtitles.md)
  indique quels formats les lecteurs affichent eux-mêmes et lesquels le serveur doit
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
**Administration → Cache artworks** indique combien n'ont pas encore de copie locale ;
**Télécharger les artworks manquants** les récupère tout de suite, et **Purger
les variantes corrompues** supprime les affiches redimensionnées laissées vides
par un redimensionnement échoué, qui sont alors reconstruites à la demande.

Une affiche ou un titre qui appartient à un autre film relève de
l'identification des métadonnées, pas des affiches :
[Corriger un titre faux ou manquant](../administration/libraries.md#fix-a-wrong-or-missing-title).

Un nouveau fichier apparaît normalement en quelques minutes (voir
[Prise en compte des nouveaux fichiers](../administration/libraries.md#how-new-files-are-picked-up)).
Sinon, appuyez sur **Scanner** sur la ligne de la bibliothèque, dans
**Administration → Bibliothèques**, et lisez le résultat.

Un scan se termine par **Scan partiel - ces dossiers n'ont pas pu être lus :**
suivi de chemins : Evey ne peut pas lire ces dossiers (droits, partage réseau
déconnecté). Le reste de la bibliothèque est analysé, et les titres d'un
dossier illisible sont conservés plutôt que retirés. Vérifiez le montage et le
propriétaire (`PUID`/`PGID`, voir [Installation](../installation.md#common-variations)),
puis relancez le scan.

La recherche ne renvoie rien : reconstruisez l'index de recherche à partir de
la base de données avec `docker exec evey-server evey admin reindex-search`.
Rien n'est perdu : l'index peut toujours être reconstruit.

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
(**Administration → Logs & Debug**, voir [Les logs](#the-logs)) et le rapport de
[`evey admin doctor`](#evey-admin-doctor). Les logs nomment des comptes et
des adresses réseau : relisez-les avant de les envoyer.

Une faille de sécurité ne se signale pas dans un endroit public : écrivez à
**security@eveyproject.org**, et laissez le temps de la corriger avant d'en
parler.
