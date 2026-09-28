---
source_sha: 9fca0896aed2edad83b383a5c8dc68a4c1946e29c664cda6b017db17d93c7dc0
---
# Administration

Le panneau d'administration (menu de l'avatar, puis **Admin**) est l'endroit
d'où un administrateur fait tourner le serveur au quotidien. Presque tout y
est enregistré en base de données : cela survit à une mise à jour du
conteneur, et la plupart des réglages s'appliquent immédiatement. Le panneau
indique quand un réglage ne prend effet qu'après un redémarrage
(`docker compose restart`).

Cette page suit le menu du panneau et s'arrête sur les réglages qui comptent.
Pour ce qui doit être décidé avant le démarrage du serveur (URL publique, proxy
de confiance, propriétaire des fichiers), voir la
[configuration](../configuration.md).

## Tableau de bord et système

### Tableau de bord et En cours de lecture

Le **Tableau de bord** affiche la version, la machine et ce qui se passe.
**En cours de lecture** liste chaque flux en cours : qui, quoi, sur quel
appareil, et comment - **Lecture directe** (le fichier tel quel), **Remux**
(réempaqueté, image intacte) ou **Transcodage** (converti) - piste par piste,
avec le moteur et la carte graphique utilisés. Vous pouvez arrêter une session
ou envoyer un message au spectateur.

### Accès distant

Qui peut se connecter depuis l'extérieur de votre réseau domestique.

| Réglage | Ce qu'il fait |
| --- | --- |
| **Activer l'accès distant** | Désactivé : quiconque n'a pas une adresse de votre réseau local reçoit une erreur sur chaque page. Activé : il peut se connecter comme n'importe qui. |
| **URL externe** | L'adresse publique, quand elle diffère de celle avec laquelle le serveur a démarré. |
| **Réseaux LAN** | Les plages d'adresses considérées comme « la maison ». Les plages privées y figurent par défaut ; ajoutez ici la plage d'un VPN (Tailscale, WireGuard) si ses utilisateurs doivent compter comme la maison. |
| **Débit distant maximal** | Un plafond de débit pour les spectateurs hors de la maison, en kbit/s (0 = aucun). Une adresse privée n'est jamais plafonnée. |

Si vous n'y touchez pas, le serveur décide au démarrage : l'accès distant est
activé si vous lui avez donné une adresse publique, désactivé sinon. Si le
serveur est joignable depuis Internet alors que l'accès distant est coupé, la
page vous prévient en rouge. Derrière un reverse proxy, cela ne fonctionne que
si le proxy transmet la vraie adresse du client - voir le
[reverse proxy](../reverse-proxy.md#real-client-ips-and-the-remote-access-gate).

L'accès distant est un choix sur qui peut atteindre le serveur, pas une
protection : c'est la connexion qui protège la bibliothèque.

### Configuration du serveur

La page regroupe les réglages propres au serveur en cartes :

| Carte | Les réglages à connaître |
| --- | --- |
| **Langue et région** | Langue par défaut de l'interface, pays dont les classifications d'âge s'appliquent, et langue des titres et des résumés. |
| **Flux d'accueil** | Les rangées affichées sur la page d'accueil, dans quel ordre, et combien d'éléments chacune montre. Les extensions peuvent ajouter des rangées. |
| **E-mail / SMTP** | Le courrier sortant pour les invitations et les réinitialisations de mot de passe. Voir [Notifications](../notifications.md). |
| **Miniatures Trickplay** | Génère les images d'aperçu de la barre de progression. Activé par défaut. |
| **Puissance allouée** | La part du processeur que le travail de fond peut prendre, tout compris : **25 %** (en douceur, par défaut) ou **50 %** (coup de fouet, quand personne ne regarde). Une tâche lourde (analyse des nouveaux médias, miniatures, détection des génériques, classification de complexité) peut recevoir sa propre part. S'applique à l'élément suivant. Le travail de fond se met en pause quand quelqu'un regarde. |
| **Profil mémoire** | **Faible** suspend les extensions inactives pour économiser la mémoire ; **Auto** le choisit sur les machines de moins de 3 Go. |
| **Travaux en arrière-plan** | Met en pause et cadence les passes de fond (index des images clés, niveaux de codec). Lu au démarrage : redémarrez après un changement. |
| **Téléchargement des artworks** | Garde affiches et fonds sur le serveur, convertis en WebP, au lieu de les charger depuis Internet à chaque fois. |
| **Jetons d'analyse** | Des étiquettes de release et des noms d'édition supplémentaires que l'analyseur de noms de fichiers doit retirer, quand vos fichiers portent des étiquettes qu'il ne connaît pas. |
| **Clés API** | TMDB (une clé partagée est intégrée), OMDb, OpenSubtitles, TheIntroDB. |
| **Sources de détection des intros** | Utilise des bases communautaires (TheIntroDB, puis IntroDB) pour les repères de générique, de résumé et de générique de fin avant de lancer la détection du serveur lui-même. Redémarrez après l'avoir basculé. Peut aussi renvoyer à TheIntroDB les détections sûres du serveur. |
| **Paramètres du serveur** | Nom du serveur, limites de connexion et globales, niveau de log, surveillance des fichiers et intervalle de scan, **Saut automatique des intros** (désactivé par défaut : une fois activé, chaque lecteur saute seul les intros détectées), **Passer les segments d'aperçu**, durée de conservation de l'historique et des statistiques, **budget du cache de transcodage** (10 Go par défaut), et plafond de l'AV1 logiciel. |
| **Mises à jour** | Vérifie toutes les 6 heures si une image plus récente est publiée et vous le signale. Evey ne se met jamais à jour tout seul. |

Sous les cartes, **Entrées de configuration** liste chaque réglage brut
clé-valeur, pour le cas rare où une clé documentée n'a pas de carte à elle.

### Transcodage

Comment le serveur convertit la vidéo.

- **Calibrage de l'encodage** montre ce que le serveur a mesuré sur lui-même :
  la plus haute résolution qu'il sait convertir en temps réel, en logiciel et
  avec la carte graphique, et les codecs que la carte gère. **Accélération
  matérielle** choisit la carte (**Auto** prend la meilleure détectée,
  **Désactivée** force le processeur). **Recalibrer** après avoir ajouté une
  carte graphique ou déplacé le conteneur sur une autre machine. Ici aussi :
  **Sortie AV1** (matériel uniquement), l'**encodeur basse consommation**
  d'Intel, et un **plafond de débit vidéo**, avec un plafond séparé pour les
  spectateurs hors de la maison.
- **Capacité de lecture en direct** est le nombre de vidéos qui peuvent être
  converties en même temps. Elle est mesurée ; vous ne pouvez que l'abaisser.
  Un flux qui ne convertit que le son ne compte pas, et un spectateur qui
  change de qualité garde une seule place.
- **Tâches de transcodage en arrière-plan** limite les conversions en file
  d'attente, séparément de la lecture en direct.
- **Limites avancées** plafonne sessions et résolutions sur du matériel
  faible. Lues au démarrage.
- **Test de charge** simule de vrais spectateurs pour mesurer ce que le serveur
  tient. À lancer uniquement quand personne ne regarde.

Mettre en place une carte graphique dans Docker :
[Transcodage et matériel](../transcoding.md).

### Cache de transcodage et Santé du système

**Cache de transcodage** montre ce que les conversions ont laissé sur le disque
et permet de le vider. Après une mise à jour qui change la façon dont les
vidéos sont converties, le vider garantit que personne ne reçoit une ancienne
conversion. **Santé du système** affiche le processeur, la mémoire, la bande
passante, le stockage et la capacité de conversion utilisée.

## Médias

- **Bibliothèques** : ajouter, modifier, scanner, planifier ; **Matcher les
  orphelins**, **Re-matcher tout sur TMDB** et **Réparer les sagas** pour les
  métadonnées. Voir les [premiers pas](first-steps.md#add-your-libraries).
- **Cache artworks** : les affiches et les fonds gardés sur le serveur.
- **Santé des médias** : un scan en lecture seule de chaque fichier MKV et WebM
  rend l'un de deux verdicts. **Réparable** (index de recherche manquant,
  passage endommagé, `.mkv` qui est en réalité un MP4) se corrige en une fois
  avec **Tout réparer**, et chaque fichier réparé garde son original en
  sauvegarde. **Pour information** (téléchargement incomplet, décalage audio...)
  liste ce qui ne peut pas être corrigé ici. Réparer écrit dans vos dossiers de
  médias, qui doivent donc être montés en lecture-écriture. Plus de détails
  dans [Santé des médias](../runbooks/media-health.md).

Sur un titre, un administrateur peut aussi **modifier les métadonnées**,
changer l'affiche, et ajuster les **segments du média** (générique, résumé,
générique de fin) à la main ou relancer leur détection.

## Utilisateurs et accès

- **Utilisateurs** : créer, modifier, désactiver ou supprimer des comptes ;
  restreindre chacun à certaines bibliothèques ; plafonner son débit de
  streaming, son nombre de profils et d'appareils par profil ; consulter son
  historique ; **Se connecter en tant que** l'utilisateur pour voir ce qu'il
  voit (un bandeau vous le rappelle, **Quitter** vous ramène).
- **Liens d'invitation** : un lien avec, au choix, une expiration, un nombre
  d'utilisations, des bibliothèques, une limite de profils, et le rôle
  d'administrateur.
- **Demandes de média** : la file des titres demandés par les utilisateurs
  (accepter, refuser ou reporter, avec une note), et qui peut demander : tout
  le monde par défaut, avec des exceptions par utilisateur.

## Intégrations

- **Plugins** : installer depuis un fichier ZIP. Avant
  l'installation, le panneau montre les permissions que l'extension déclare et
  ce que son analyse statique a trouvé. Par extension : activer, mettre à jour,
  limites de ressources (mémoire, processus, disque, priorité), logs. Voir les
  [extensions](../plugins.md).
- **Webhooks** : appeler une URL à vous quand quelque chose se produit (un
  titre ajouté, une lecture lancée...), pour tous les événements ou quelques-uns
  choisis, signé avec un secret si vous en donnez un. **Tester** envoie un ping.
- **Clés API** : des clés pour vos propres scripts et intégrations, chacune
  limitée aux portées que vous cochez (lire les médias, piloter la lecture,
  gérer les utilisateurs...). Une clé n'est affichée qu'une fois, à sa
  création. **Documentation de l'API** liste ce que chaque portée ouvre et
  permet de l'essayer.

## Supervision et maintenance

- **Analytique** : lectures du jour et de la semaine, utilisateurs actifs, les
  titres les plus vus des 30 derniers jours.
- **Activité** : l'historique des lectures et des actions d'administration, et
  le travail de fond en cours.
- **Logs & Debug** : chercher et suivre en direct les logs du serveur, filtrés
  par domaine (lecture, conversion, scan...). Le premier arrêt quand un
  utilisateur signale un problème.
- **Qualité streaming** : comment la lecture s'est passée pour de vrais
  spectateurs sur le dernier jour ou la dernière semaine - combien de fois ils
  ont attendu la vidéo, les titres qui calent le plus, et les types d'appareils
  qui peinent.
- **Sauvegarde BDD et restauration** : créer une sauvegarde, la télécharger, en
  restaurer une, ou restaurer depuis un fichier. La planification et la
  conservation sont traitées dans [Sauvegardes](../backups.md).

Quand c'est le panneau lui-même qui ne répond plus, la même maintenance est
disponible en ligne de commande : [Ligne de commande](../runbooks/cli.md).
