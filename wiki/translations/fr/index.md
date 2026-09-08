---
source_sha: aedd9ba6244ba2dbc7e348c929241546a233301b93c88350c78c524424a8728f
hide:
- navigation
---

<pre class="evey-wordmark" aria-hidden="true"><span>███████╗</span><span class="v">██╗   ██╗</span><span>███████╗██╗   ██╗</span>
<span>██╔════╝</span><span class="v">██║   ██║</span><span>██╔════╝╚██╗ ██╔╝</span>
<span>█████╗  </span><span class="v">██║   ██║</span><span>█████╗   ╚████╔╝ </span>
<span>██╔══╝  </span><span class="v">╚██╗ ██╔╝</span><span>██╔══╝    ╚██╔╝  </span>
<span>███████╗</span><span class="v"> ╚████╔╝ </span><span>███████╗   ██║   </span>
<span>╚══════╝</span><span class="v">  ╚═══╝  </span><span>╚══════╝   ╚═╝   </span></pre>

# Evey

Serveur multimédia auto-hébergé pour vos films, séries, musiques et photos. Un
conteneur, votre matériel, vos fichiers.

[Installer Evey](installation.md){ .md-button .md-button--primary }
[Voir le code source](https://github.com/Evey-project/evey){ .md-button }

## Par où commencer

<div class="grid cards" markdown>

-   __Installer__

    ---

    Un seul fichier compose, aucun secret à inventer. Postgres et l'index de
    recherche tournent dans le conteneur et génèrent leurs propres accès au
    premier démarrage.

    [Guide d'installation](installation.md)

-   __Configurer__

    ---

    Tout ce qu'un administrateur peut changer, et où ça se règle. La quasi
    totalité se trouve dans le panneau d'administration, pas dans des variables
    d'environnement.

    [Référence de configuration](configuration.md)

-   __Passer derrière un proxy__

    ---

    Caddy, nginx, Traefik et Apache, avec les en-têtes dont la diffusion et la
    lecture par WebSocket ont réellement besoin.

    [Guide du reverse proxy](reverse-proxy.md)

-   __Protéger vos données__

    ---

    Ce que le serveur sauvegarde tout seul, ce qu'il ne sauvegarde pas, et
    comment restaurer une base qui refuse de démarrer.

    [Sauvegardes](backups.md) · [Restauration](runbooks/recovery.md)

</div>

## Ce qu'il fait

- **Il lit ce qui vous appartient.** Lecture directe dès que le client sait
  ouvrir le fichier tel quel, transcodage matériel ou logiciel quand il ne sait
  pas, et un flux HLS qui change de piste audio et de qualité en cours de route.
- **Il vous suit d'un appareil à l'autre.** Web, Android, Android TV et
  Chromecast, avec les reprises de lecture, l'état de visionnage et les
  téléchargements hors ligne.
- **Il est fait pour un foyer.** Des profils avec leur thème et leur langue, un
  contrôle parental appliqué côté serveur, et le visionnage à plusieurs avec
  SyncPlay.
- **Il s'étend sans recompilation.** Les extensions tournent isolées, avec des
  jetons restreints et des permissions refusées par défaut, et les thèmes sont
  des données plutôt que du code.

## Contribuer

La page [architecture](ARCHITECTURE.md) explique comment le serveur est
découpé, et les [décisions d'architecture](adr/README.md) racontent pourquoi
chaque choix structurant a été fait. Les auteurs d'extensions commencent par la
[mise en place pour développer une extension](plugins/dev-setup.md).

Evey est distribué sous licence [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html).
Les signalements de sécurité vont à `security@eveyproject.org` - voir le
[modèle de menace](SECURITY.md).
