---
source_sha: e3a9611908da29d559070f9745961b1f31fad6154f252e1304481c5e371f2c0c
reviewed: true
---
# Comptes et connexion

Les variables derrière les sessions et l'authentification unique : la durée
d'une connexion, l'emplacement de la clé de signature et le fournisseur
OIDC. L'authentification unique se configure normalement dans le panneau,
comme décrit dans [Authentification unique (OIDC)](../oidc.md) ; les
variables de cette page en sont le repli.

## Sessions

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | La clé qui signe les jetons d'accès, générée au premier démarrage. Si elle est perdue, le démarrage suivant en génère une nouvelle et les applications renouvellent leurs jetons : personne n'est déconnecté. | aucun |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Durée de vie d'un jeton d'accès. Les applications le renouvellent sans rien demander. | aucun |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 jours) | Durée de vie d'une session de connexion : sans utiliser une application pendant ce temps, il faut se reconnecter. | aucun |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 jours) | Durée de vie d'une session qu'une application demande à conserver, comme une TV connectée par Connexion rapide. | aucun |

## Authentification unique (OIDC)

Chaque champ enregistré dans la carte **Authentification unique (SSO)** de
**Admin → Configuration du serveur** remplace sa variable, sans redémarrage.

| Variable | Défaut | Effet | Panneau |
| --- | --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Active l'authentification unique. | Configuration du serveur → Authentification unique (SSO), immédiat |
| `EVEY_OIDC_ISSUER` | vide | L'URL de l'émetteur du fournisseur, par exemple `https://auth.example.com/realms/main`. | même carte, immédiat |
| `EVEY_OIDC_CLIENT_ID` | vide | Le client enregistré pour Evey chez le fournisseur. | même carte, immédiat |
| `EVEY_OIDC_CLIENT_SECRET` | vide | Son secret. Activée sans lui, le serveur avertit au démarrage. | même carte, immédiat |
| `EVEY_OIDC_REDIRECT_URL` | vide | L'adresse de retour envoyée au fournisseur : `https://<votre adresse>/api/v1/auth/oidc/callback`. Vide : construite à partir de l'adresse publique du serveur (**Admin → Accès distant**). | aucun |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | vide (tous) | Les domaines d'e-mail autorisés à se connecter, séparés par des virgules. Le fournisseur doit avoir vérifié l'adresse. | même carte, immédiat |
| `EVEY_OIDC_ALLOW_SIGNUP` | vide | `true` ou `false` : créer un compte pour les nouveaux utilisateurs SSO. Vide : activé seulement si des domaines e-mail sont renseignés. | même carte, immédiat |
