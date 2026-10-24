---
source_sha: 9ec00e9464587406ecdffdd125543fdaf6988a5989c62a715f8a88417d946b9f
reviewed: true
---

# Limites connues { #known-limitations }

Ce qu'Evey ne fait pas encore, dit simplement, pour que personne ne le
découvre le soir où il voulait regarder un film. Chaque point dit quoi
utiliser à la place.

## Pas d'application pour iPhone, iPad ni Apple TV { #no-app-for-iphone-ipad-or-apple-tv }

Il existe une application Evey pour les téléphones et tablettes Android, et
une pour Android TV et Google TV. Il n'y en a aucune pour iPhone, iPad ou
Apple TV. Sur ces appareils, ouvrez l'adresse du serveur dans le navigateur
web.

Pour aller plus loin : [Connecter les applications](apps.md).

## Les applications Android attendent la validation de Google { #the-android-apps-are-waiting-for-googles-review }

Les applications Android et Android TV ont été soumises à Google Play. Tant
que la validation n'est pas passée, la fiche publique n'est pas visible et
seuls les comptes invités au test peuvent les installer ; le lien de la page
[Applications](apps.md) devient actif à ce moment-là. L'application web et la
page `/tv` dans le navigateur d'une télévision ne demandent aucune
installation et fonctionnent pour tous.

Pour aller plus loin : [Connecter les applications](apps.md).

## Regarder sans connexion : application Android seulement { #watching-without-a-connection-android-app-only }

Seule l'application Android garde une copie d'un film ou d'un épisode sur
l'appareil (**Téléchargements**), lisible pendant 30 jours sans contacter le
serveur. Le lecteur web, l'application Android TV et la page `/tv` lisent
depuis le serveur et ont besoin de la connexion du début à la fin.

Pour aller plus loin : [Ce que chaque application ajoute](apps.md#what-each-app-adds).

## Pas de notifications sur le téléphone quand l'application est fermée { #no-notifications-on-the-phone-while-the-app-is-closed }

Les applications Android et Android TV affichent leurs notifications dans
leur écran **Notifications**, quand elles sont ouvertes. Elles ne sonnent pas
sur le téléphone ni sur la TV quand elles sont fermées. Les notifications qui
vous parviennent en dehors de l'application viennent du navigateur web (une
fois activées, en `https://`) ou par e-mail, quand la personne qui fait
tourner Evey l'a configuré.

Pour aller plus loin : [Où apparaissent les notifications](../notifications.md#where-notifications-appear).

## Les profils se gèrent sur le web ou sur le téléphone { #profiles-are-managed-on-the-web-or-on-the-phone }

L'application Android TV passe d'un profil à l'autre et change certains
réglages de celui en cours, mais elle ne peut ni créer ni supprimer un
profil. Faites-le dans l'application web ou dans l'application Android.

Pour aller plus loin : [Utilisateurs, profils et contrôle parental](../administration/users.md#profiles-kids-profiles-and-the-pin).

## Caster : depuis Chrome ou l'application Android { #casting-from-chrome-or-the-android-app }

Le bouton **Caster** existe dans le lecteur web sous Chrome et dans
l'application Android. Firefox n'en a pas, et l'application Android TV non
plus, puisqu'elle est déjà sur la TV. Safari, et Chrome sur un téléphone
Android, passent par la lecture à distance du navigateur lui-même, qui
fonctionne autrement.

Pour aller plus loin : [Diffusion (Chromecast)](../playback/casting.md).

## Evey ne se met jamais à jour tout seul { #evey-never-updates-itself }

Le serveur vérifie si une version plus récente existe et le signale sur le
tableau de bord d'administration, mais il n'installe rien de lui-même : la
personne qui le fait tourner met à jour quand cela lui convient, en deux
commandes. Les applications Android affichent un message quand elles sont plus
anciennes que le serveur.

Pour aller plus loin : [Mettre à jour Evey](../backups.md#updating-evey) et
[Garder les applications à jour](apps.md#keeping-the-apps-up-to-date).
