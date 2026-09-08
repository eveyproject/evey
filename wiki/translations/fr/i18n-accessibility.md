---
source_sha: 507ac22fa3a3e19be004b58f4eb8c59130fdf70e75855b01aae6d4b67fc7e4cd
---

# Langues et accessibilité

Un état des lieux sincère de la situation d'Evey en matière
d'internationalisation (i18n) et d'accessibilité (a11y) : ce qui marche, comment
cela marche, et ce qui manque encore. C'est un projet auto-hébergé développé et
maintenu par une seule personne - tenez l'accessibilité en particulier pour un
*effort de bonne foi, pas pour une certification*.

---

## Internationalisation (i18n)

### Ce qui est traduit, et ce qui ne l'est pas

- **Traduit :** l'habillage de l'interface - boutons, menus, réglages, messages
  d'erreur, texte des courriels et des notifications poussées.
- **Non traduit par Evey :** les métadonnées des médias (titres, résumés,
  genres) viennent de votre fournisseur de métadonnées (TMDB, OMDb) dans la
  **langue des métadonnées** que vous configurez (`EVEY_METADATA_LANGUAGE`),
  laquelle est indépendante de la langue de l'interface. Vos propres
  bibliothèques, vos noms de fichiers et tout ce que vous saisissez sont
  conservés tels quels.

L'interface peut donc être en français pendant que le synopsis d'un film reste
en anglais, ou l'inverse : ce sont deux réglages distincts.

### Langues d'interface prises en charge

Six : **anglais (en)**, **français (fr)**, **allemand (de)**, **espagnol (es)**,
**chinois (zh)**, **hindi (hi)**. L'anglais est la langue source et le repli :
toute chaîne absente d'une autre langue s'affiche en anglais, plutôt que de
laisser un blanc ou d'exposer une clé.

### Comment cela fonctionne

- **Une seule source de vérité :** [`/i18n/<locale>/*.yaml`](../i18n), à la
  racine du dépôt, découpée en espaces de noms (`common`, `auth`, `media`,
  `player`, `admin`, `errors`, `notifications`, `mobile`, `downloads`,
  `requests`). Le même catalogue alimente toutes les surfaces.
- Le **serveur** le charge, le sert sur `GET /api/v1/i18n/{locale}`, et s'en
  sert directement pour traduire les messages d'erreur de l'API et le texte des
  notifications.
- Les **applications web, mobile et TV** chargent les traductions depuis ce
  point d'accès (i18next et http-backend), les mettent en cache localement, et
  tiennent `<html lang>` en phase avec la langue active pour que les
  technologies d'assistance annoncent la bonne langue.
- La langue est reprise de la préférence de votre compte, à défaut de celle du
  navigateur ou de l'appareil (`Accept-Language` est analysé, la région retirée :
  `fr-FR` → `fr`), et en dernier recours de l'anglais.
- L'interpolation et la gestion du pluriel passent par i18next (`{{count}}`,
  etc.).

### Les limites, sans fard

- **La complétude varie.** L'anglais fait autorité et reste toujours complet ;
  les cinq autres sont tenus à parité à la main, avec un script de vérification,
  mais une fonctionnalité toute neuve peut arriver d'abord en anglais et se
  replier en attendant sa traduction.
- **Pas encore de langue écrite de droite à gauche.** Ni arabe, ni hébreu, ni
  persan : la mise en page n'a été ni conçue ni testée pour le sens
  droite-gauche, et en ajouter une est un vrai chantier (propriétés logiques
  CSS, gestion de `dir`), pas seulement un fichier de traduction.
- **Les traductions ne sont pas relues par des professionnels.** Elles sont de
  qualité communautaire ou mainteneur. Les corrections sont bienvenues.
- **Le formatage sensible à la locale est partiel.** Les dates et les nombres
  s'appuient sur `Intl` et sur la locale de la plateforme, et ne sont pas câblés
  uniformément sur tous les écrans.

### Ajouter ou corriger une langue

1. Copiez `i18n/en/` vers `i18n/<votre-locale>/` et traduisez les valeurs du
   YAML (gardez les clés).
2. Ajoutez la locale à la liste d'autorisation dans
   [`internal/pkg/i18n/i18n.go`](../internal/pkg/i18n/i18n.go)
   (`supportedLocales`).
3. Gardez un jeu de clés identique d'une locale à l'autre, faute de quoi le
   contrôle de parité échoue.

---

## Accessibilité (a11y)

### Où en est le projet

Evey repose sur du HTML sémantique, des libellés ARIA et les propriétés
d'accessibilité de React Native, et plusieurs des points ci-dessous sont
délibérés. Mais il n'a **pas** été audité formellement au regard des WCAG, la
suite de tests ne comporte **aucun test d'accessibilité automatisé**, et il n'a
**pas** été éprouvé systématiquement au lecteur d'écran. Certaines parties sont
sans doute utilisables avec une technologie d'assistance, d'autres sans doute
pas. Si vous en dépendez, attendez-vous à des aspérités, et signalez des cas
précis.

### Ce qui est en place

- **Mouvement réduit :** `prefers-reduced-motion: reduce` est respecté partout -
  animations et transitions sont de fait désactivées dès que votre système
  d'exploitation le demande.
- **Langue pour les technologies d'assistance :** `<html lang>` suit la langue
  d'interface active.
- **Clavier (lecteur) :** commandes clavier complètes, avec une surcouche
  affichant les raccourcis (`?`) ; les raccourcis sont neutralisés pendant la
  saisie dans un champ.
- **Contrôles libellés :** les boutons réduits à une icône et les curseurs
  portent un `aria-label` sur le web, un `accessibilityLabel` et un
  `accessibilityRole` sur mobile (la plus grande part des contrôles interactifs,
  pas la totalité).
- **Thèmes :** cinq thèmes sont livrés d'origine (sombre par défaut, AMOLED,
  rouge sombre, clair par défaut, bleu clair) ; d'autres s'ajoutent par les
  extensions de thème.
- **Android TV :** gestion du focus à la croix directionnelle, pour la
  navigation à la télécommande.

### Les manques, sans fard (pas encore faits)

- **Aucune passe au lecteur d'écran.** Les parcours NVDA, VoiceOver et TalkBack
  ne sont pas vérifiés ; certains composants sur mesure (menus, fenêtres
  modales, barre de progression, sélecteurs de piste) risquent de ne pas
  annoncer correctement leur état ou leur rôle.
- **La gestion du focus n'est pas auditée.** Le style de focus visible et
  l'ordre de tabulation dans les boîtes de dialogue et les surcouches sont
  inégaux par endroits.
- **Le contraste n'est mesuré que pour les thèmes intégrés.** Un test unitaire
  (`frontend/packages/theme/src/contrast.test.ts`) tient chaque thème livré au
  ratio WCAG AA de 4,5:1 pour l'accent, les trois niveaux de texte et le jeu de
  couleurs de danger, face au fond comme aux deux surfaces ; l'application TV
  revérifie ses propres jetons dérivés. Les extensions de thème et les couleurs
  qu'une extension injecte à l'exécution ne sont pas contrôlées : un thème sur
  mesure peut donc encore échouer par endroits. Le niveau AAA n'est pas visé.
- **Aucune mise à l'échelle du texte dans l'application**, au-delà du zoom du
  navigateur ou du système, et pas de mode contraste élevé dédié.
- **La mise en forme des sous-titres est limitée sur mobile** (une contrainte du
  lecteur `expo-video` sous-jacent), même si les pistes de sous-titres et de
  sous-titrage codé se lisent, elles, sans problème.

### Signaler un problème

Indiquez la page ou l'écran, le contrôle précis, ce que vous attendiez, ainsi
que la technologie d'assistance employée et sa version. Un signalement concret
finit corrigé ; « ce n'est pas accessible » ne permet d'agir sur rien. Les
correctifs d'accessibilité sont traités au mieux.
