---
source_sha: 1a764dc349466d08546e1872dd0168db702f96a926dc5a0f771fabb1cd60921b
reviewed: true
---
# Langues et accessibilité

Un état des lieux honnête d'Evey sur l'internationalisation (i18n) et
l'accessibilité (a11y) : ce qui fonctionne, comment, et ce qui ne fonctionne pas
encore. C'est un projet auto-hébergé construit et maintenu par une seule
personne : considérez l'accessibilité en particulier comme *faite au mieux, pas
certifiée*.

---

## Internationalisation (i18n)

### Ce qui est traduit, et ce qui ne l'est pas

- **Traduit :** l'interface elle-même : boutons, menus, réglages, messages
  d'erreur, textes des notifications push. Les e-mails (notifications,
  réinitialisations de mot de passe) partent en anglais.
- **Pas traduit par Evey :** les métadonnées des médias (titres, résumés,
  genres) viennent de votre source de métadonnées (TMDB/OMDb), dans la **Langue
  des métadonnées** choisie dans l'assistant de configuration ou dans **Admin →
  Configuration du serveur → Langue et région** (`EVEY_METADATA_LANGUAGE` n'est
  que la valeur par défaut), indépendante de la langue de l'interface. Vos bibliothèques, vos noms de fichiers et tout ce
  que vous saisissez sont conservés tels quels.

L'interface peut donc être en français et le synopsis d'un film en anglais (ou
l'inverse) : ce sont deux réglages distincts.

### Langues de l'interface

Six : **anglais (en)**, **français (fr)**, **allemand (de)**, **espagnol
(es)**, **chinois (zh)**, **hindi (hi)**. L'anglais est la langue source et la
langue de repli : un texte absent dans une autre langue s'affiche en anglais
plutôt que vide ou sous forme de clé.

### Comment ça fonctionne

- **Une seule source de vérité :** [`/i18n/<locale>/*.yaml`](../i18n) à la racine
  du dépôt, découpé en espaces de noms (`common`, `auth`, `media`, `player`,
  `admin`, `errors`, `notifications`, `mobile`, `downloads`, `requests`). Le même
  catalogue alimente toutes les surfaces.
- Le **serveur** le charge et le sert à `GET /api/v1/i18n/{locale}`, et s'en sert
  directement pour traduire les messages d'erreur de l'API et le texte des
  notifications.
- Les **applications web, mobile et TV** chargent les traductions depuis ce point
  d'accès (i18next + http-backend), les gardent en cache localement, et tiennent
  `<html lang>` à jour avec la langue active, pour que les aides techniques
  annoncent la bonne langue.
- La langue vient de la préférence de votre compte, sinon de la langue du
  navigateur ou de l'appareil (`Accept-Language` est lu, sans la région :
  `fr-FR` → `fr`), sinon de la **Langue du serveur** de **Admin → Configuration
  du serveur → Langue et région** (qui fixe aussi la langue des nouveaux
  comptes), sinon de l'anglais.
- L'interpolation et les pluriels passent par i18next (`{{count}}`, etc.).

### Limites, en toute honnêteté

- **La complétude varie.** L'anglais fait foi et est toujours complet ; les cinq
  autres langues sont tenues à parité à la main, avec un script de contrôle, mais
  une toute nouvelle fonction peut arriver d'abord en anglais et s'afficher en
  anglais en attendant sa traduction.
- **Pas encore de langue écrite de droite à gauche.** Ni arabe, ni hébreu, ni
  persan, et la mise en page n'a été ni construite ni testée pour ce sens
  d'écriture : en ajouter une est un vrai chantier (propriétés CSS logiques,
  gestion de `dir`), pas un simple fichier de traduction.
- **Les traductions ne sont pas relues par des professionnels.** Elles sont de
  qualité communautaire ou mainteneur. Les corrections sont les bienvenues.
- **La mise en forme selon la langue est partielle.** Les dates et les nombres
  s'appuient sur `Intl` et la langue du navigateur ou de la plateforme, et ne
  sont pas branchés uniformément sur tous les écrans.

### Ajouter ou corriger une langue

1. Copiez `i18n/en/` vers `i18n/<votre-locale>/` et traduisez les valeurs YAML
   (gardez les clés).
2. Ajoutez la locale à la liste autorisée dans
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   (`supportedLocales`).
3. Gardez exactement le même jeu de clés dans toutes les langues : le contrôle de
   parité échoue sinon.

---

## Accessibilité (a11y)

### Où en est Evey

Evey est construit avec du HTML sémantique, des libellés ARIA et les propriétés
d'accessibilité de React Native, et quelques points ci-dessous sont délibérés.
Mais il **n'a pas** été audité formellement au regard des WCAG, la suite de tests
ne contient **aucun test d'accessibilité automatisé**, et il **n'a pas** été
testé systématiquement avec des lecteurs d'écran. Certaines parties sont
probablement utilisables avec des aides techniques ; d'autres probablement pas.
Si vous en dépendez, attendez-vous à des aspérités et signalez-les précisément.

### Ce qui est en place

- **Mouvements réduits :** `prefers-reduced-motion: reduce` est respecté partout :
  animations et transitions sont en pratique désactivées quand votre système le
  demande.
- **Langue pour les aides techniques :** `<html lang>` suit la langue active de
  l'interface.
- **Clavier (lecteur) :** des commandes clavier complètes, avec un aide-mémoire
  des raccourcis à l'écran (`?`), et les raccourcis sont suspendus pendant la
  saisie dans un champ.
- **Contrôles libellés :** les boutons et curseurs réduits à une icône portent un
  `aria-label` sur le web et un `accessibilityLabel`/`accessibilityRole` sur
  mobile (pour la majeure partie des contrôles interactifs, pas tous).
- **Thèmes :** cinq thèmes sont fournis (sombre par défaut, AMOLED, rouge sombre,
  clair par défaut, bleu clair) ; d'autres via les plugins de thème.
- **Android TV :** gestion du focus à la croix directionnelle pour la navigation
  à la télécommande.

### Ce qui manque encore

- **Pas de passage au lecteur d'écran.** Les parcours NVDA, VoiceOver et
  TalkBack ne sont pas vérifiés ; certains composants sur mesure (menus,
  fenêtres, barre de progression, choix des pistes) peuvent ne pas annoncer
  correctement leur état ou leur rôle.
- **La gestion du focus n'est pas auditée.** Le style de focus visible et l'ordre
  du focus dans les fenêtres et superpositions sont par endroits incohérents.
- **Le contraste n'est mesuré que pour les thèmes intégrés.** Un test unitaire
  (`frontend/packages/theme/src/contrast.test.ts`) impose à chaque thème fourni
  le niveau WCAG AA 4,5:1 pour la couleur d'accent, les trois niveaux de texte
  et les couleurs d'alerte, sur le fond et les deux surfaces ; l'application TV
  revérifie ses propres couleurs dérivées. Les plugins de thème et les couleurs
  qu'un plugin injecte en cours d'exécution ne sont pas vérifiés : un thème
  personnalisé peut encore échouer par endroits. Le niveau AAA n'est pas visé.
- **Pas de réglage de taille du texte dans l'application**, au-delà du zoom du
  navigateur ou du système, et pas de mode contraste élevé dédié.
- **Le style des sous-titres sur mobile est limité** (une contrainte du lecteur
  `expo-video` sous-jacent), même si les pistes de sous-titres elles-mêmes sont
  lues.

### Signaler un problème

Indiquez la page ou l'écran, le contrôle précis, ce que vous attendiez, et l'aide
technique utilisée avec sa version. Un signalement concret se corrige ; « ce
n'est pas accessible » ne permet pas d'agir. Les corrections d'accessibilité
sont traitées au mieux.
