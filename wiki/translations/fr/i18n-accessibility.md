---
source_sha: aacd60e164be25536f5b7ca3fe0a88655d153900e00d2521c8d0795dd2769d24
reviewed: true
---
# Langues et accessibilité

Où en est Evey sur l'internationalisation (i18n) et l'accessibilité (a11y) :
ce qui est traduit et comment le catalogue atteint chaque client, puis ce
qui est en place pour les technologies d'assistance et ce qui ne l'est pas.
Le catalogue se trouve dans `i18n/<locale>/*.yaml` à la racine du dépôt, servi
par `internal/pkg/i18n` et chargé par le module i18n de chaque application.
C'est un projet auto-hébergé maintenu par une seule personne : considérez
l'accessibilité en particulier comme faite au mieux, pas certifiée.

## Internationalisation (i18n)

### Ce qui est traduit et ce qui ne l'est pas

| Traduit par Evey | Non traduit par Evey |
| --- | --- |
| les textes de l'interface : boutons, menus, réglages, messages d'erreur, texte des notifications push, e-mails de notification (dans la langue du destinataire) | les métadonnées des médias (titres, résumés, genres), qui viennent du fournisseur de métadonnées dans la **Langue des métadonnées** choisie dans l'assistant de configuration ou dans **Administration → Configuration du serveur → Langue et région** (`EVEY_METADATA_LANGUAGE` n'est que la valeur par défaut) |
| | vos propres bibliothèques, les noms de fichiers et tout ce que vous saisissez, stockés tels quels |

Les e-mails de réinitialisation de mot de passe sont envoyés en anglais. La
langue de l'interface et la langue des métadonnées sont deux réglages
distincts : l'interface peut être en français pendant que le synopsis d'un
film est en anglais, ou l'inverse.

### Langues d'interface prises en charge

Six : **anglais (en)**, **français (fr)**, **allemand (de)**, **espagnol
(es)**, **chinois (zh)**, **hindi (hi)**. L'anglais est la langue source et
la langue de repli : toute chaîne manquante dans une autre langue retombe sur
l'anglais plutôt que d'afficher un blanc ou une clé.

### Comment une chaîne arrive à l'écran

- **Une seule source de vérité :** [`/i18n/<locale>/*.yaml`](../i18n) à la
  racine du dépôt, découpé en espaces de noms (`common`, `auth`, `media`,
  `player`, `admin`, `errors`, `notifications`, `mobile`, `downloads`,
  `requests`). Le même catalogue alimente toutes les surfaces.
- Le **serveur** le charge et le sert sur `GET /api/v1/i18n/{locale}`, et
  l'utilise directement pour les messages d'erreur traduits de l'API et le
  texte des notifications.
- L'**application web** charge les traductions depuis ce point d'accès
  (i18next avec le backend HTTP), en garde une copie dans le stockage local
  du navigateur pour le prochain démarrage, et maintient `<html lang>` en
  phase avec la langue active pour que les technologies d'assistance
  annoncent la bonne langue. Les **applications mobile et TV** chargent
  depuis le même point d'accès via le module i18n natif partagé et
  embarquent des textes de base en anglais (`fallback.en.json`, `offlineStrings.ts`) pour
  les écrans affichés avant qu'un serveur ne réponde.
- La langue est choisie d'après la préférence du compte, puis la langue du
  navigateur ou de l'appareil (`Accept-Language` est analysé, région
  retirée : `fr-FR` devient `fr`), puis la **Langue du serveur** de **Administration →
  Configuration du serveur → Langue et région** (qui fixe aussi la langue des
  nouveaux comptes), puis l'anglais.
- L'interpolation et le pluriel passent par i18next (`{{count}}`, etc.).

### Ce qui n'est pas encore fait

- **La complétude varie.** L'anglais fait référence et est toujours complet ;
  les cinq autres langues sont maintenues à parité à la main, avec un test
  (`internal/pkg/i18n/parity_test.go`) qui échoue quand un jeu de clés
  diverge des autres, mais une toute nouvelle fonctionnalité peut arriver d'abord en
  anglais et se replier dessus jusqu'à sa traduction.
- **Pas de langue de droite à gauche.** Il n'y a ni arabe, ni hébreu, ni
  persan, et la mise en page n'a été ni conçue ni testée pour le RTL : en
  ajouter une est un vrai travail (propriétés CSS logiques, gestion de
  `dir`), pas seulement un fichier de traduction.
- **Les traductions ne sont pas relues par des professionnels.** Elles sont
  faites par le mainteneur. Les corrections sont bienvenues.
- **Le formatage selon la locale est partiel.** Les dates et les nombres
  reposent sur `Intl` de la plateforme ou la locale du navigateur et ne sont
  pas appliqués uniformément sur chaque écran.

### Ajouter ou corriger une langue

1. Copiez `i18n/en/` vers `i18n/<votre-locale>/` et traduisez les valeurs
   YAML (gardez les clés).
2. Ajoutez la locale à la liste d'autorisation dans
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   (`supportedLocales`).
3. Gardez le jeu de clés identique entre les locales : le test de parité
   échoue sinon, et `TestI18nPlaceholdersAgreeAcrossLocales` vérifie aussi
   que les `{{espaces réservés}}` de chaque chaîne concordent.

## Accessibilité (a11y)

### Où en est-on

Evey est construit avec du HTML sémantique, des libellés ARIA et les
propriétés d'accessibilité de React Native, et quelques points ci-dessous
sont délibérés. Mais il n'a **pas** été formellement audité au regard des
WCAG, il n'y a **aucun test d'accessibilité automatisé** dans la suite, et il
n'a **pas** été testé systématiquement avec des lecteurs d'écran. Des parties
sont probablement utilisables avec une technologie d'assistance ; d'autres
probablement pas. Si vous en dépendez, attendez-vous à des défauts et
signalez des cas précis.

### Ce qui est en place

| Domaine | Ce qui est fait |
| --- | --- |
| Mouvement réduit | `prefers-reduced-motion: reduce` est respecté globalement (`frontend/apps/web/src/styles/main.css`) : les animations et transitions sont en pratique désactivées quand votre système le demande |
| Langue pour les technologies d'assistance | `<html lang>` suit la langue active de l'interface |
| Clavier (lecteur) | commandes clavier complètes avec un aide-mémoire des raccourcis à l'écran (`?`, `KeyboardShortcutsOverlay.tsx`) ; les raccourcis sont suspendus pendant que vous saisissez dans un champ |
| Commandes libellées | les boutons à icône seule et les curseurs portent `aria-label` sur le web et `accessibilityLabel`/`accessibilityRole` sur mobile (la majeure partie des commandes interactives, pas toutes) |
| Thèmes | cinq thèmes livrés d'origine (sombre par défaut, AMOLED, dark-red, light-default, light-blue) ; d'autres via des plugins de thème |
| Android TV | gestion du focus au pavé directionnel pour la navigation à la télécommande |

### Ce qui n'est pas encore fait

- **Aucune vérification au lecteur d'écran.** Les parcours NVDA/VoiceOver/TalkBack
  ne sont pas vérifiés ; certains composants personnalisés (menus, boîtes de
  dialogue, barre de progression, sélecteurs de pistes) peuvent ne pas
  annoncer correctement leur état ou leur rôle.
- **La gestion du focus n'est pas auditée.** Le style du focus visible et
  l'ordre de focus dans les boîtes de dialogue et les surcouches sont
  incohérents par endroits.
- **Le contraste n'est mesuré que pour les thèmes intégrés.** Un test
  unitaire (`frontend/packages/theme/src/contrast.test.ts`) impose à chaque
  thème livré le niveau WCAG AA 4,5:1 pour l'accent, les trois niveaux de
  couleur du texte et les couleurs de danger contre le fond et les deux surfaces ; l'application
  TV revérifie ses jetons dérivés (`ThemeProvider.contrast.test.ts`). Les
  plugins de thème et les couleurs qu'un plugin injecte à l'exécution ne sont
  pas vérifiés : un thème personnalisé peut encore échouer par endroits. Le
  niveau AAA n'est pas visé.
- **Pas de mise à l'échelle du texte dans l'application** au-delà du zoom du
  navigateur ou du système, et pas de mode contraste élevé dédié.
- **Le style des sous-titres sur mobile est limité** (une contrainte du
  lecteur `expo-video` sous-jacent), même si les pistes de sous-titres et de
  légendes elles-mêmes sont lues.

### Signaler un problème

Indiquez la page ou l'écran, la commande précise, ce que vous attendiez, et
la technologie d'assistance et sa version. Les signalements concrets sont
corrigés ; « ce n'est pas accessible » ne permet pas d'agir. Les corrections
d'accessibilité sont traitées au mieux.

<!-- sources
catalogue: i18n/en, de, es, fr, hi, zh ; internal/pkg/i18n/i18n.go - supportedLocales, FallbackLocale, ParseLocale ; internal/pkg/i18n/parity_test.go ; internal/contractguard - TestI18nPlaceholdersAgreeAcrossLocales, TestServerEmittedI18nKeysExistInCatalogue
endpoint: internal/adapters/driving/http/server.go - GET /api/v1/i18n/{locale}
loaders: frontend/apps/web/src/i18n/index.ts - HttpBackend, getCached, documentElement.lang ; frontend/packages/client/src/native/i18n.ts ; frontend/apps/mobile/src/i18n/fallback.en.json ; frontend/apps/tv/src/i18n/offlineStrings.ts
mails: internal/adapters/driven/email/password_reset_emailer.go ; internal/adapters/driven/email/notification_emailer.go - WithNotificationLocale
labels: i18n admin.serverConfig.languageRegion, serverLanguage, metadataLanguage
a11y: frontend/apps/web/src/styles/main.css - prefers-reduced-motion ; frontend/apps/web/src/features/playback/KeyboardShortcutsOverlay.tsx ; frontend/packages/theme/src/themeVars.ts ; frontend/packages/theme/src/contrast.test.ts ; frontend/apps/tv/src/providers/ThemeProvider.contrast.test.ts ; frontend/apps/mobile/package.json - expo-video
-->
