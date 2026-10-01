---
source_sha: 49645a52c7506bc78947f31b763d5b78de15eaf577542b073330d83230b45467
reviewed: true
---
# Comptes et connexion

Durée des sessions, clé de signature et authentification unique.

## Sessions

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_JWT_PRIVATE_KEY_PATH` | `/data/jwt.key` | La clé qui signe les jetons d'accès, générée au premier démarrage. Si elle est perdue, le démarrage suivant en génère une nouvelle et les applications renouvellent leurs jetons : personne n'est déconnecté. |
| `EVEY_JWT_ACCESS_TTL` | `15m` | Durée de vie d'un jeton d'accès. Les applications le renouvellent sans rien demander. |
| `EVEY_JWT_REFRESH_TTL` | `168h` (7 jours) | Durée de vie d'une session de connexion : sans utiliser une application pendant ce temps, il faut se reconnecter. |
| `EVEY_JWT_REFRESH_LONG_TTL` | `4320h` (180 jours) | Durée de vie d'une session qu'une application demande à conserver, comme une TV connectée par Connexion rapide. |

## Authentification unique (OIDC)

| Variable | Défaut | Effet |
| --- | --- | --- |
| `EVEY_OIDC_ENABLED` | `false` | Active l'authentification unique. Redémarrez après l'avoir modifiée. |
| `EVEY_OIDC_ISSUER` | vide | L'URL de l'émetteur du fournisseur, par exemple `https://auth.example.com/realms/main`. |
| `EVEY_OIDC_CLIENT_ID` | vide | Le client enregistré pour Evey chez le fournisseur. |
| `EVEY_OIDC_CLIENT_SECRET` | vide | Son secret. |
| `EVEY_OIDC_REDIRECT_URL` | vide | L'adresse de retour envoyée au fournisseur : `https://<votre adresse>/api/v1/auth/oidc/callback`. Vide : aucune n'est envoyée, et le fournisseur utilise celle enregistrée pour le client. |
| `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` | vide (tous) | Les domaines d'e-mail autorisés à créer un compte à leur première connexion, séparés par des virgules. Le fournisseur doit avoir vérifié l'adresse. |

Configurer un fournisseur : [Authentification unique (OIDC)](../oidc.md).
