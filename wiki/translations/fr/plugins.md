---
source_sha: 641ed9ad2a04551e1c5e1db07a26cb739f2708ce4ec146e2a5590265cd4216ca
---

# Système d'extensions

Les extensions d'Evey enrichissent le serveur : fournisseurs de métadonnées,
traitement des médias, webhooks, notifications, modules d'interface et thèmes
CSS. Une extension s'exécute dans un **sous-processus isolé** qui dialogue avec
l'hôte en gRPC sur une socket de domaine Unix ; l'interface qu'elle embarque, le
cas échéant, est rendue dans une **iframe en bac à sable** et parle à l'hôte au
travers d'un pont `postMessage`.

Cette page est une carte. Le détail se trouve dans trois documents dédiés :

| Document | Ce qu'on y trouve |
|---|---|
| **[plugins/dev-setup.md](plugins/dev-setup.md)** | Écrire sa première extension : prérequis, squelette, la boucle installer/itérer, la signature, la soumission au catalogue, le dépannage. **Commencez ici.** |
| **[plugins/api-reference.md](plugins/api-reference.md)** | Le contrat : schéma du manifeste, IPC et magasin de données côté serveur, le pont `evey.*` côté interface, la liste des emplacements d'insertion, et chaque capacité (`metadata.provider`, `media.read/write`, `notification.sender`, `webhook.receiver`, `public`, `prefs`). |
| **[plugins/security-model.md](plugins/security-model.md)** | Modèle de menace et couches de défense : séparation d'UID, la liste noire d'appels système (noyau et LD_PRELOAD), les modes d'isolation (`auto` se résout en landlock → bubblewrap → basic ; `strict` ajoute la bibliothèque LD_PRELOAD à bubblewrap), le moniteur de ressources en espace utilisateur, l'analyse statique, l'assainisseur CSS, la signature et le versionnage, la liste blanche des emplacements, et les limites connues. |

Le modèle d'autorisation qui sous-tend les jetons d'extension (refus par défaut,
portées verrouillées en dur, un pont qui ne laisse jamais le jeton entrer dans
l'iframe) est décrit dans
[adr/0012-plugin-token-deny-by-default.md](adr/0012-plugin-token-deny-by-default.md) ;
les plafonds de ressources le sont dans [adr/0013-userspace-plugin-resource-monitor.md](adr/0013-userspace-plugin-resource-monitor.md).

## Architecture

```
+------------------+      gRPC (Unix socket)      +-------------------+
|   Evey Server    | <--------------------------> |  Plugin Process   |
|                  |   /tmp/evey/plugins/{id}/    |                   |
|  HostService     |   plugin.sock                |  PluginService    |
|  (serves RPCs)   |                              |  (implements)     |
+------------------+                              +-------------------+
        |                                                  |
        v                                                  v
+------------------+                              +-------------------+
|  Permission /    |                              |  SDK stubs        |
|  scope checker   |                              |  (multi-language) |
+------------------+                              +-------------------+
        |
        v
+------------------+     iframe sandbox           +-------------------+
|  Frontend        | <--------------------------> |  Frontend module  |
|  PluginSlot      |   postMessage bridge         |  (in srcdoc)      |
+------------------+                              +-------------------+
```

Une extension peut s'écrire dans **n'importe quel langage que l'`entrypoint:`
sait lancer** - le lanceur ne l'impose pas. Des stubs gRPC pré-générés sont
fournis pour Python, Node.js, PHP, Ruby, Dart, Java et Swift (`sdk/<lang>/`,
recopiés automatiquement à l'installation) ; Go n'a pas de stubs livrés,
générez-les avec `protoc-gen-go` et `protoc-gen-go-grpc` ; les langages
interprétés dépourvus de stubs fournis (Bash, Lua, Perl, …) fonctionnent en
invoquant directement l'interpréteur, et les binaires compilés (Go, Rust, …)
grâce à la permission `native-binary`. Mise en place langage par langage :
[plugins/dev-setup.md](plugins/dev-setup.md#per-language).

Les trois piliers :

- **Capacités et portées** - une extension déclare dans son manifeste les
  capacités dont elle a besoin ; l'administrateur les lui accorde à
  l'installation. L'hôte les fait respecter à chaque appel, en refusant par
  défaut. Catalogue complet dans
  [plugins/api-reference.md](plugins/api-reference.md#capabilities).
- **Bac à sable** - les processus serveur des extensions tournent isolés
  (confinement du système de fichiers, des appels système et du réseau) sous la
  surveillance d'un moniteur de ressources en espace utilisateur ; le code
  d'interface, lui, s'exécute dans une iframe verrouillée. Détails dans
  [plugins/security-model.md](plugins/security-model.md).
- **Confiance** - les extensions sont épinglées par empreinte SHA256 et peuvent
  être signées avec Cosign ; le serveur peut être réglé pour exiger une
  signature. Voir
  [plugins/security-model.md](plugins/security-model.md#update-workflow).

## Activer le sous-système

Les extensions sont actives par défaut (`EVEY_PLUGINS_ENABLED=true`). Le mode
d'isolation se règle par `EVEY_PLUGINS_SANDBOX_MODE` (`auto` par défaut, qui se
résout en Landlock sur l'image officielle). Les plafonds de ressources
(`EVEY_PLUGINS_MEMORY_MB`, `_PIDS_MAX`, `_DISK_MAX_MB`, `_CPU_NICE`) sont
modifiables par l'administrateur depuis **Admin → Plugins**.
Voir [configuration.md](configuration.md) et [plugins/dev-setup.md](plugins/dev-setup.md#7-resource-limits-and-sandbox).
