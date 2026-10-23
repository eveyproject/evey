---
source_sha: 822f2e4e87b78c5d8f5b5509badc1a7c70b7d5fe0c6a2b84d574662a66b8ba95
reviewed: true
---
# Référence de configuration { #configuration-reference }

Où se trouve chaque réglage d'un serveur Evey, quelle valeur l'emporte quand un
réglage existe à la fois dans le panneau d'administration et dans
l'environnement, et ce qui demande un redémarrage. Les variables elles-mêmes
sont listées par thème dans les dix pages liées plus bas ; cette page
s'adresse à l'administrateur qui édite le fichier compose ou se demande
pourquoi une variable semble ignorée.

## Deux endroits, une règle { #two-places-one-rule }

**Ce qui doit exister avant le démarrage du serveur** - secrets, chemins de
stockage, adresse publique, informations sur le reverse proxy - va dans le
fichier compose, sous forme de variables d'environnement. **Ce qu'un
administrateur ajuste ensuite** - langues, limites de transcodage,
accélération matérielle, planifications, limites des plugins - se trouve
dans le panneau d'administration, est stocké en base, est conservé lors de la
mise à jour du conteneur et s'applique le plus souvent sans redémarrage.

Pour un réglage qui existe aux deux endroits, la priorité est toujours la
même :

1. **La valeur enregistrée dans le panneau l'emporte.** À chaque démarrage,
   le serveur lit d'abord l'environnement, puis remplace chaque champ par la
   valeur stockée en base quand il y en a une. Une variable ne remplace
   jamais une valeur enregistrée.
2. **La variable s'applique tant que rien n'est enregistré.** Le panneau
   l'affiche comme valeur par défaut courante ; **Réinitialiser** supprime
   la valeur enregistrée, et la variable s'applique de nouveau à partir du
   démarrage suivant.
3. **La valeur par défaut intégrée** s'applique quand ni l'un ni l'autre
   n'est défini.

Les secrets font exception : le panneau n'affiche jamais un mot de passe ou
une clé qu'il a stockés, et une variable comme `EVEY_TMDB_API_KEY` est
copiée en base à la première utilisation, pour que le panneau puisse
indiquer qu'une clé personnalisée est en service.

## Ce qui demande un redémarrage { #what-needs-a-restart }

- Une modification d'une **variable d'environnement** demande toujours de
  recréer le conteneur (`docker compose up -d`) : le processus lit son
  environnement une seule fois.
- Une modification dans le **panneau** s'applique immédiatement pour la
  plupart des réglages (nom du serveur, niveau de journalisation, limites de
  requêtes, accélération matérielle, SMTP, authentification unique, accès
  distant, limites des plugins, durées de conservation au prochain nettoyage
  quotidien).
- Le panneau signale par *appliqué au redémarrage* les réglages **lus au
  démarrage** : la surveillance des fichiers et l'intervalle d'analyse, le
  budget du cache de transcodage, la carte **Limites avancées** du
  transcodage, les sources de détection des intros, et les options
  d'opérateur connues parmi les lignes des **Entrées de configuration** (une
  ligne pour toute autre clé n'affiche pas ce repère). Enregistrer conserve le choix, redémarrer
  l'applique.

Chaque tableau des pages par thème a une colonne **Panneau** : le chemin du
champ (à partir de **Admin**) et s'il s'applique immédiatement ou au
redémarrage, ou `aucun` quand la variable n'a pas d'équivalent dans le
panneau.

| Page | Contenu |
| --- | --- |
| [Serveur et réseau](environment/server.md) | Adresse publique, ports, reverse proxy, accès distant, limites de requêtes, utilisateur du conteneur |
| [Base de données et recherche](environment/database-search.md) | Postgres et Meilisearch intégrés ou externes |
| [Comptes et connexion](environment/accounts.md) | Durée des sessions, clé de signature, authentification unique |
| [Bibliothèques et métadonnées](environment/libraries.md) | Scan, clés de métadonnées, détection des intros |
| [Transcodage et matériel](environment/transcoding.md) | Carte graphique, capacité, limites de qualité, cache, extrait de calibrage |
| [Diffusion de la lecture et téléchargements](environment/playback-delivery.md) | Options d'empaquetage des flux, téléchargements préparés, boutons de saut du lecteur |
| [Tâches de fond](environment/background.md) | Passes en arrière-plan et leur rythme |
| [Plugins et sandbox](environment/plugins.md) | Mode de sandbox, utilisateur séparé, limites de ressources, signatures |
| [E-mail et notifications](environment/notifications.md) | SMTP, clés de notification push |
| [Sauvegardes, logs et supervision](environment/logs-monitoring.md) | Sauvegardes, log, durée de conservation des historiques, métriques et traces |

## Comment les valeurs sont lues { #how-values-are-read }

- Les booléens acceptent `1`, `t`, `true`, `0`, `f`, `false`. Les durées
  exigent une unité : `30s`, `5m`, `2h` (un `30` nu est ignoré). Les listes
  sont séparées par des virgules.
- Une valeur que le serveur ne sait pas lire est **ignorée avec un
  avertissement** dans le log (`environment variable ignored, value could
  not be parsed`) et la valeur par défaut s'applique. Relisez les premières
  lignes du log après avoir modifié le fichier compose.
- `EVEY_DB_PASSWORD` et `MEILI_MASTER_KEY` peuvent aussi venir d'un fichier
  nommé par la même variable suffixée `_FILE` ; une valeur explicite prime
  sur le fichier.

## Secrets { #secrets }

Le Postgres et le Meilisearch intégrés génèrent leurs secrets au premier
démarrage, dans `./evey-data/keys` (`postgres.password`, `meili.key`, mode
0600). Le serveur les lit via `EVEY_DB_PASSWORD_FILE` et
`MEILI_MASTER_KEY_FILE`, définies dans l'image ; c'est aussi pourquoi les
commandes `evey admin` n'ont pas besoin de mot de passe. Ne renseignez
`EVEY_DB_PASSWORD` ou `MEILI_MASTER_KEY` que pour une base ou un moteur de
recherche externe.

Une clé TMDB est intégrée. `EVEY_TMDB_API_KEY` fournit la vôtre ; la clé
saisie dans **Administration → Configuration du serveur → Clés API** l'emporte.

## Variables inutiles { #variables-you-do-not-need }

Des lignes souvent reprises d'anciennes installations, et pourquoi elles ne
changent rien :

| Ligne | Pourquoi elle est inutile |
| --- | --- |
| `EVEY_LOG_DIR`, `EVEY_TRANSCODING_CACHE_DIR`, `EVEY_TRICKPLAY_CACHE_DIR` | L'image les définit déjà sur `/data/logs`, `/cache/transcode`, `/data/trickplay`. Les répéter ne change rien. |
| `EVEY_FORWARDED_PROTO` | Obsolète : déduite du schéma de `EVEY_SERVER_BASE_URL`. La définir ne fait qu'afficher un avertissement. |
| `EVEY_KEYFRAME_BACKFILL=on` | Toute valeur autre que `off` l'active, y compris l'absence de valeur. |
| `LIBVA_DRIVER_NAME` | libva choisit le pilote d'après l'identifiant PCI du GPU. Ne l'imposez que si votre carte a besoin d'un pilote que libva ne choisit pas (vérifiez avec `docker exec evey-server vainfo`). |
| `EVEY_PLUGINS_SANDBOX_MODE` | `auto` choisit le confinement le plus robuste que permet la machine. Ne la définissez explicitement que pour une garantie stricte : `landlock` refuse de lancer un plugin qu'il ne peut pas confiner, là où `auto` passe à un mode moins strict avec un avertissement. |
| un `group_add` pour `/dev/dri/card0` | Evey n'ouvre que des nœuds de rendu (`/dev/dri/renderD*`, quel que soit celui de l'hôte), jamais `card*`. Vérifiez avec `ls -ln /dev/dri` et n'ajoutez que le groupe du nœud de rendu. |

## Propriétaire des fichiers (PUID et PGID) { #file-ownership-puid-and-pgid }

**Par défaut : automatique.** Quand `PUID`/`PGID` ne sont pas définis,
l'entrypoint regarde au démarrage le propriétaire de vos montages `/media*` et
lance le serveur sous cet utilisateur, pour que les fonctions qui *écrivent*
dans votre bibliothèque (la réparation de l'index de lecture reconstruit
les fichiers sur place) fonctionnent d'emblée, sans `chown` ni configuration.
Le log de démarrage indique le choix fait :

```
evey: media mounts owned by 1026:100 (detected on /media) - running the
server as that user so write features (seek-index repair) work out of the box
```

Seul un `/media*` effectivement monté compte : le `/media` vide de l'image
elle-même (toute image Debian en a un) est ignoré, donc une bibliothèque rangée
ailleurs, sous `/data` par exemple, démarre sans aucun avertissement de
propriété.

Des montages appartenant à **des utilisateurs différents** sont gérés au
mieux : le serveur adopte l'utilisateur propriétaire du plus grand nombre de
montages et rejoint les groupes des autres en groupes secondaires, de sorte
que les réparations y fonctionnent si leurs fichiers sont modifiables par le
groupe (l'avertissement de démarrage liste les montages qui pourraient encore
refuser l'écriture). La détection automatique n'est abandonnée (repli sur
`1000:1000` avec un avertissement) que si tous les montages appartiennent à
root, ou à l'uid réservé `1001` tant que les plugins sont activés : attribuez vos médias à un utilisateur
ordinaire, ou définissez `PUID`/`PGID` explicitement, sous `environment:`
dans le fichier compose :

```yaml title="docker-compose.yml"
services:
  evey:
    environment:
      PUID: "1026"   # lancer le serveur sous cet uid  (sur l'hôte : `id -u <propriétaire-des-médias>`)
      PGID: "100"    # ...et ce gid
```

Des valeurs explicites l'emportent toujours sur la détection.

Fonctionnement : le conteneur démarre en root le temps de remapper son
utilisateur interne (et de réattribuer les dossiers propres à Evey, `/data`
et `/cache`, **jamais** vos montages de médias), puis abandonne ses
privilèges. Les groupes secondaires accordés via `group_add:` dans le compose
(par exemple le groupe render de l'hôte pour le transcodage matériel via
`/dev/dri`) sont conservés. Le processus du serveur ne tourne jamais en root.

Sans aucun passage en root : si même ce passage transitoire est inacceptable
pour vous, définissez `user: "UID:GID"` sur le service dans votre fichier
compose. L'entrypoint détecte le démarrage sans root et saute le remappage (y
compris la détection du propriétaire des médias) : c'est alors à vous de
garder `./evey-data` modifiable par cet uid.

!!! note
    `PUID=1001` est refusé tant que les plugins sont activés : c'est l'utilisateur interne `evey-plugin`,
    séparé pour que les plugins ne partagent jamais l'identité du serveur.
    Les médias sont montés en lecture-écriture par défaut pour que les
    réparations fonctionnent ; ajoutez `:ro` au volume si vous préférez une
    bibliothèque en lecture seule (les réparations échouent alors avec une
    erreur explicite).

## Variables que le serveur ne lit pas { #variables-the-server-does-not-read }

Certaines variables figurent dans des fichiers compose sans être lues par le
serveur :

- `EVEY_HTTP_PORT`, `EVEY_DOMAIN`, `MEDIA_DIR` : utilisées par le
  `docker-compose.yml` du dépôt (port publié, domaine du Caddy intégré, chemin
  des médias), pas par Evey. `EVEY_HTTP_PORT` est uniquement le port de l'hôte :
  le conteneur écoute toujours sur 9091 et le compose publie
  `${EVEY_HTTP_PORT:-9091}:9091`. `EVEY_VERSION` y est le tag de l'image ; le
  serveur ne l'affiche sur `/healthz` que si son binaire ne porte pas de
  version.
- `EVEY_WEB_DIR` (`/srv`, l'emplacement des fichiers de l'interface web), ainsi
  que `EVEY_DB_PASSWORD_FILE`, `MEILI_MASTER_KEY_FILE`, `EVEY_IMAGE_REPO`,
  `EVEY_IMAGE_RUNTIMES` : correspondent à l'organisation de l'image. Les
  modifier n'est pas pris en charge.
- `EVEY_SUPERVISED`, `EVEY_COMPANION_PIDS`, `EVEY_RESTART_EXIT_CODE`,
  `EVEY_LAUNCHER_BIN`, `EVEY_PLUGIN_LAUNCHER` : définies par l'entrypoint et le
  lanceur de plugins pour leur propre usage.
- `EVEY_BANNER=off` masque seulement la bannière de démarrage dans le log ;
  `NO_COLOR` en retire les couleurs.
