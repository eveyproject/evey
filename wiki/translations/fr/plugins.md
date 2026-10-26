---
source_sha: 3ef56ca6884cb437782bd60a1bb1314f62db8602db661c29fca9d5f8cab4b4de
reviewed: true
---
# Plugins { #plugins }

Cette page s'adresse à l'administrateur : comment installer un plugin ou un
thème, ce que vous acceptez en le faisant, et ce que le serveur lui permet
de faire sur votre machine. Tout se passe dans **Administration → Plugins** ; les pages
destinées aux auteurs de plugins sont listées [à la fin](#writing-a-plugin).

Un plugin ajoute à Evey ce qu'il ne fait pas lui-même : une section sur la
page d'accueil, une surcouche dans le lecteur, une source de métadonnées, un
thème. Chaque plugin est un processus à part, qui communique avec le serveur par
un socket local, confiné par une sandbox et limité aux permissions que déclare
son manifeste. Son interface, quand il en a une, s'exécute dans un cadre isolé
de la page.

<div class="evey-shot-pair" markdown>

<figure class="evey-shot" markdown="span">
  ![L'accueil sans plugin](images/home-no-plugin.webp){ loading=lazy }
  <figcaption>Sans plugin.</figcaption>
</figure>

<figure class="evey-shot" markdown="span">
  ![L'accueil avec le plugin météo](images/home.webp){ loading=lazy }
  <figcaption>Avec le plugin météo d'exemple : une section en haut, affichée dans son propre cadre.</figcaption>
</figure>

</div>

## Installer un plugin { #install-a-plugin }

1. Dans **Administration → Plugins**, ouvrez **Installer un plugin** et choisissez le
   **Fichier plugin (.zip)**. Si l'auteur fournit une signature, ajoutez-la
   dans **Fichier signature (.zip.sig, optionnel)**. L'onglet **Depuis une
   URL** installe à partir d'un lien ; ajoutez `#sha256:<empreinte>` à l'URL
   pour faire vérifier le téléchargement.
2. **Aperçu et installation** analyse le paquet sans l'installer. L'aperçu
   montre les **Permissions** et les **Capacités** que demande le plugin, le
   résultat de l'**Analyse statique** et, quand une signature a été fournie,
   le verdict **Signature** avec l'éditeur. Les emplacements et les hôtes sont
   montrés à la mise à jour et dans le **Tableau de bord sécurité**.
3. Lisez cette liste comme un contrat : ce sont les seules choses que le plugin
   pourra faire. Confirmez, et le plugin démarre.

<figure class="evey-shot" markdown="span">
  ![Admin → Plugins sur le serveur de démo](images/admin-plugins.webp){ loading=lazy }
  <figcaption><strong>Admin → Plugins</strong> : les environnements d'exécution disponibles, les limites de ressources, et une carte par plugin.</figcaption>
</figure>

Un paquet dont l'analyse relève un motif critique est refusé, quelle que soit
votre réponse. Les avertissements sont affichés et laissés à votre
appréciation. Le langage du plugin doit figurer parmi les **Environnements
d'exécution disponibles** du serveur : l'image par défaut embarque
Python, Node.js, PHP, Ruby et Lua ; l'image `-nodejs` garde Python et
Node.js et l'image `-lean` ne garde que Python.

!!! tip "Plugins signés"
    L'auteur d'un plugin peut signer le ZIP avec [cosign](https://docs.sigstore.dev/cosign/installation/) ;
    sa clé publique voyage dans le paquet. Evey vérifie la signature seul (pas
    de binaire `cosign`) et affiche **Signé par <éditeur>** quand la clé est
    dans votre liste d'**Éditeurs de confiance**, **Signé, clé inconnue** avec
    son empreinte sinon (un clic lui fait confiance), **Non signé** ou
    **Invalide**. Une mise à jour qui change de clé, ou qui abandonne la
    signature, attend votre acceptation explicite. La politique décide du
    reste :

    ```sh
    EVEY_PLUGINS_COSIGN_POLICY=warn      # off | warn (défaut) | enforce
    ```

    Avec `warn`, le verdict est affiché et rien n'est bloqué ; avec
    `enforce`, seuls les éditeurs de confiance s'installent. Auteurs : voir
    [Signer son plugin](plugins/dev-setup.md#7-sign-your-plugin).

## Mettre à jour un plugin { #update-a-plugin }

Sur la carte du plugin, **Mettre à jour depuis la source…** prend le nouveau
ZIP (un chemin ou une URL) et affiche le même aperçu. Une mise à jour doit
porter une version supérieure à celle installée. Quand elle demande de
nouvelles permissions, capacités, emplacements ou hôtes, l'aperçu indique
**Cette mise à jour demande de nouvelles permissions** et liste la
différence : rien n'est appliqué tant que vous n'avez pas choisi **Approuver
et installer**. Le paquet analysé est le paquet installé : son empreinte est
vérifiée de nouveau au moment d'appliquer la mise à jour.

## Gérer les plugins installés { #manage-the-installed-plugins }

Chaque carte montre l'état du plugin (**Activé** ou **Désactivé**) et propose :

| Action | Effet |
| --- | --- |
| **Activé** / **Désactivé** | Démarre ou arrête le processus du plugin. Un plugin désactivé garde ses fichiers et sa configuration. |
| **Config** | Les réglages que le plugin déclare (une clé d'API, une ville, une unité...). Les champs secrets sont affichés masqués et ne sont jamais renvoyés au navigateur. |
| **Logs** | Les dernières lignes que le plugin a écrites par l'intermédiaire de l'hôte. |
| **Métadonnées** | Sur un plugin de métadonnées seulement : les champs qu'il remplace et la récupération de la bibliothèque existante ([plus bas](#metadata-plugins)). |
| **Désinstaller** | Supprime le processus, les fichiers et les données stockées par le plugin. |

**Limites de ressources**, en haut de la page, plafonne chaque processus de
plugin. Les valeurs du panneau sont les défauts ; la variable d'environnement
les fixe tant que le panneau n'a jamais été enregistré.

| Champ | Défaut | Variable d'environnement |
| --- | --- | --- |
| **Mémoire max (Mio)** | 256 | `EVEY_PLUGINS_MEMORY_MB` |
| **Processus max** | 64 | `EVEY_PLUGINS_PIDS_MAX` |
| **Disque max (Mio)** | 100 | `EVEY_PLUGINS_DISK_MAX_MB` |
| **Nice CPU (0-19)** | 10 | `EVEY_PLUGINS_CPU_NICE` |
| **Mode sandbox** | `auto` | `EVEY_PLUGINS_SANDBOX_MODE` |

La mémoire, les processus et le disque s'appliquent au prochain balayage du
moniteur, qui arrête un plugin qui les dépasse ; la priorité processeur et le
mode sandbox s'appliquent aux plugins démarrés ensuite. `0` rétablit la
valeur par défaut pour la mémoire et les processus ; seule la limite de disque traite `0` comme « désactivée ».

Le **Tableau de bord sécurité** conserve, par plugin, les constats de
l'analyse, le résultat de la signature et la dernière erreur. Sa ligne
**Sandbox de l'hôte** indique le mode de sandbox réellement appliqué sur cette
machine : s'il affiche `basic`, l'hôte n'offre aucune isolation du système de
fichiers par chemin, et [Mise en place de la sandbox des
plugins](plugins/sandbox-deployment.md) explique quoi changer.

Les plugins sont activés par défaut. `EVEY_PLUGINS_ENABLED=false` désactive tout
le sous-système.

## Plugins de métadonnées { #metadata-plugins }

Un plugin de métadonnées (capacité `metadata.provider`) fournit son propre
texte pour vos films et séries, depuis une autre source que TMDB. Il ne
remplace pas TMDB : TMDB identifie toujours chaque titre et fournit sa
distribution, ses visuels, ses notes, ses collections et ses traductions. Le
texte du plugin est conservé à côté de celui de TMDB, et affiché à sa place
champ par champ.

Ce que lit un spectateur, du plus faible au plus fort :

1. le texte de TMDB, dans la langue du spectateur quand TMDB l'a ;
2. le texte du plugin dans cette langue, pour les champs que le plugin a le
   droit de remplacer ;
3. votre propre correction du titre (**Modifier les métadonnées** sur sa page),
   qui l'emporte toujours.

**Métadonnées**, sur la carte du plugin, choisit les champs qu'il remplace
parmi **Titre**, **Synopsis** et **Accroche**. Tant que vous n'avez rien
choisi, un plugin remplace le synopsis et l'accroche, jamais le titre. Un
changement s'applique tout de suite à ce que le plugin a déjà fourni ; rien
n'est récupéré à nouveau.

Le plugin est interrogé sur chaque film et chaque série au moment de son
identification ou de son actualisation. **Récupérer pour toute la
bibliothèque**, sur le même panneau, l'interroge sur les titres déjà présents,
l'un après l'autre, montre où il en est et, à la fin, combien il en a trouvé
et combien sont restés sans réponse. Une actualisation du titre depuis TMDB
n'écrase jamais le texte du plugin ; désactiver le plugin masque ce texte, le
désinstaller le supprime.

Les saisons et les épisodes gardent le texte de TMDB, et la recherche trouve
toujours les titres de TMDB.

## Ce qu'un plugin peut faire sur votre machine { #what-a-plugin-can-do-on-your-host }

Quoi que déclare son manifeste, un plugin :

- voit son propre dossier, pas les données du serveur ni les fichiers média ;
- n'a aucun accès direct au réseau : un plugin qui a la permission `network`
  n'atteint que les hôtes qu'il a déclarés, par l'intermédiaire du serveur, en
  `GET` et `HEAD` ;
- ne peut pas lancer de processus enfants sans déclarer `subprocess` ;
- tourne sous les limites de ressources ci-dessus et, quand l'hôte le permet,
  sous un utilisateur distinct.

Son interface est confinée aux emplacements que déclare le manifeste : une
section ou la zone de bannière de l'accueil, un panneau latéral dans une
bibliothèque ou sur une fiche, une surcouche du lecteur, les réglages
d'apparence, le bas de la barre de navigation, ou une entrée du menu
d'administration qui ouvre la page du plugin lui-même. Un plugin ne peut
jamais rien afficher dans les écrans de connexion, d'installation, de sécurité ni
dans les écrans d'administration existants. Dans son cadre, il n'a accès ni à
la page, ni à vos cookies, ni à votre session : ce qu'il lit sur le serveur
passe par un jeton de courte durée limité aux deux permissions qu'il peut
demander (`public` pour une projection sûre des titres et affiches, `prefs`
pour les préférences de l'utilisateur courant dans son propre espace).

Les permissions qu'un plugin peut demander sont listées dans
[Permissions et emplacements](plugins/security-model.md). Le fonctionnement
de la sandbox, et ce qu'elle ne peut toujours pas empêcher, sont décrits dans
[Sandbox des plugins et modèle de menace](SECURITY.md).

!!! warning "Le serveur contrôle les hôtes que contacte un plugin, pas ce qu'il leur envoie"
    Une fois qu'un hôte figure dans la liste d'un plugin, le serveur ne peut
    pas inspecter ce que le plugin lui envoie. Examinez un plugin tiers avant
    de l'installer, comme n'importe quel logiciel, et considérez sa liste
    d'hôtes comme la liste des tiers qui peuvent recevoir les titres de votre
    bibliothèque.

## Thèmes { #themes }

Un thème est un paquet de `kind: theme` sans processus : une feuille de style
et, en option, un script confiné dans un cadre caché. Il s'installe depuis la
même fenêtre **Installer un plugin** ; la feuille de style est assainie à
l'installation. Chaque utilisateur le choisit ensuite dans **Paramètres →
Apparence → Thème**. Ce qu'un thème peut contenir est décrit dans
[Thèmes](themes.md).

## Écrire un plugin { #writing-a-plugin }

| Page | Contenu |
| --- | --- |
| [Écrire un plugin](plugins/dev-setup.md) | Un premier plugin pas à pas, la boucle d'installation, la signature, la distribution. À lire en premier. |
| [Référence de l'API des plugins](plugins/api-reference.md) | Le manifeste, les appels backend et le stockage de données, le pont `evey.*` de l'interface. |
| [Permissions et emplacements](plugins/security-model.md) | Ce qu'un manifeste peut déclarer, ce qui est vérifié à l'installation, les règles d'accès externe, les emplacements. |
| [Thèmes](themes.md) | Modifier l'apparence de l'interface avec du CSS et un script isolé. |
| [Webhooks et clés d'API](webhooks-api-keys.md) | Relier Evey à d'autres outils sans écrire de plugin. |

<!-- sources
install dialog, from URL with #sha256: i18n admin.plugins.installPlugin, pluginFileZip, sigFileOptional, fromUrl, installFromUrlHint, previewInstall, staticAnalysis ; internal/app/plugin/install.go - downloadIfURL
critical findings refuse, warnings: internal/app/plugin/install.go - validateStagedPlugin, checkEscalatedWarnings ; i18n admin.plugins.blockedByAnalysis, installAnyway
runtimes: internal/adapters/driven/pluginrunner/runtime_availability.go - runtimeBinaries ; i18n admin.plugins.availableRuntimes
cosign policy: internal/config/config.go - CosignPolicy, CosignPublicKey ; internal/adapters/driven/pluginrunner/cosign.go
update, version higher, escalation, hash re-checked: internal/app/plugin/install.go - resolveExistingPlugin ; internal/app/plugin/update.go - ApplyUpdate ; internal/app/plugin/permissions.go - DiffPermissions ; i18n admin.plugin.update, admin.plugins.permissionEscalation.*
card actions: i18n admin.plugin.enabled, disabled, config, logs, uninstall ; secrets masked: internal/domain/plugin/model.go - IsSecretConfigField ; internal/adapters/driving/grpcapi/admin_plugin.go
resource limits defaults and env: internal/config/config.go - Plugins (MemoryMB, PIDsMax, DiskMaxMB, CPUNice, SandboxMode) ; i18n admin.plugins.resourceLimits.*
security dashboard, host sandbox: i18n admin.plugins.security.* ; internal/app/plugin/security_dashboard_service.go
plugins enabled flag: internal/config/config.go - PluginsEnabled
fetch broker GET/HEAD: internal/adapters/driving/pluginhost/fetch.go - Fetch
subprocess: docs/plugins/security-model.md (Permissions catalog)
slots: internal/domain/plugin/slots.go - AllowedSlots
scoped token, public/prefs: docs/adr/0012-plugin-token-deny-by-default.md ; internal/pkg/auth/plugin_jwt.go - AllowedPluginScopes
theme selection: frontend/apps/web/src/features/user/ThemeSettings.tsx ; i18n common.appearance, common.theme
-->
