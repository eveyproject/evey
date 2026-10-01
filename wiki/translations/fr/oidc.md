---
source_sha: 2edc135c644943052453572d9223c7834031131c057cd92f3e44e3a9ae72cc8c
reviewed: true
---
# Authentification unique (OIDC)

Evey prend en charge l'authentification unique via OpenID Connect : Keycloak,
Authentik, Authelia, Google ou tout autre fournisseur OIDC. La page de connexion
des applications web et Android affiche alors **Se connecter avec SSO** à côté
du formulaire de mot de passe.

## 1. Déclarer Evey chez le fournisseur

Créez un client (ou une application) chez le fournisseur :

| Champ | Valeur |
|-------|--------|
| Client ID | `evey`, ou n'importe quel nom |
| Secret client | à générer (obligatoire) |
| URI de redirection | `https://<votre adresse>/api/v1/auth/oidc/callback` |
| URI de redirection après déconnexion | `https://<votre adresse>/login` |
| Type d'autorisation | Authorization Code |
| Scopes | `openid email profile` |

## 2. Configurer Evey

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

`EVEY_OIDC_ISSUER` est l'URL de base du fournisseur, sans
`/.well-known/openid-configuration`. La liste complète, dont
`EVEY_OIDC_REDIRECT_URL`, se trouve dans [Comptes et connexion](environment/accounts.md).

## 3. Redémarrer

```bash
docker compose up -d
```

Le serveur contacte le fournisseur au démarrage. Si l'émetteur est injoignable,
ou si le secret client est vide, **le serveur ne démarre pas** : les journaux en
donnent la cause.

## Qui peut se connecter

La première connexion SSO crée un compte Evey, sans mot de passe et avec un
profil. Son nom d'utilisateur est le `preferred_username` du fournisseur, à
défaut la partie de l'e-mail avant `@`. Le compte est un utilisateur ordinaire
et, comme tout compte sans restriction, voit toutes les bibliothèques :
restreignez-le ensuite dans **Admin → Utilisateurs** (voir
[Utilisateurs](administration/users.md)).

**Limitez les inscriptions avec `EVEY_OIDC_ALLOWED_EMAIL_DOMAINS`.** Vide,
quiconque possède un compte chez le fournisseur en obtient un sur votre serveur,
ce qui, avec un fournisseur public comme Google, veut dire n'importe qui. Avec
des domaines, seules ces adresses sont acceptées, et le fournisseur doit marquer
l'adresse comme vérifiée.

Un compte est reconnu par le fournisseur et son identifiant d'utilisateur, pas
par l'e-mail. Une personne qui a déjà un compte local avec le même e-mail ou le
même nom d'utilisateur ne peut pas créer de compte SSO : la connexion échoue
avec « Échec de la connexion SSO ». Utilisez une autre adresse chez le
fournisseur, ou supprimez d'abord le compte local.

Un compte désactivé est refusé à la connexion, comme d'habitude.

## Se déconnecter

Quand le fournisseur publie un point de déconnexion, se déconnecter d'Evey
ferme aussi la session chez le fournisseur, puis revient à la page de connexion
d'Evey.
