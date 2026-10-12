---
source_sha: 5f2dd5bf7c21906e243c0cece2ecff83e306d69d5b99a193541b9e574db7a4c2
reviewed: true
---
# Authentification unique (OIDC)

Permettez aux utilisateurs de se connecter avec le fournisseur d'identité que vous avez
déjà (Keycloak, Authentik, Authelia, Google ou tout autre fournisseur OpenID
Connect) : la page de connexion des applications web et Android affiche alors
**Se connecter avec SSO** à côté du formulaire de mot de passe. Deux étapes
pour l'administrateur : déclarer Evey chez le fournisseur, puis remplir une
carte du panneau d'administration.

## 1. Déclarer Evey chez le fournisseur

Créez chez le fournisseur un client (ou une application) avec le type
d'autorisation Authorization Code, les scopes `openid email profile`, un secret
client, et les deux adresses que la carte **Authentification unique (SSO)**
affiche sous **Adresses à déclarer chez votre fournisseur** :

| Champ | Valeur |
| --- | --- |
| **URI de redirection** | `https://<votre adresse>/api/v1/auth/oidc/callback` |
| **URI de redirection après déconnexion** | `https://<votre adresse>/login` |

Les deux sont construites à partir de l'adresse publique du serveur (**Administration →
Accès distant**) : réglez-la d'abord si l'on accède à Evey par une autre
adresse.

## 2. Configurer Evey

**Administration → Configuration du serveur → Authentification unique (SSO)** :

| Champ | Effet |
| --- | --- |
| **Activer l'authentification unique** | Affiche le bouton SSO sur les écrans de connexion. |
| **URL de l'émetteur (issuer)** | L'adresse du fournisseur ; Evey y lit `/.well-known/openid-configuration`. |
| **ID client**, **Secret client** | Le client déclaré plus haut. Le secret est gardé sur le serveur et n'est plus jamais affiché ; laisser les points le conserve. |
| **Domaines e-mail autorisés** | Séparés par des virgules. S'ils sont renseignés, seules ces adresses sont acceptées, et le fournisseur doit confirmer l'adresse. |
| **Libellé du bouton** | Facultatif, remplace « Se connecter avec SSO ». |
| **Relier aux comptes existants par e-mail vérifié** | Activé par défaut. Voir [Qui peut se connecter](#who-can-sign-in). |
| **Créer un compte pour les nouveaux utilisateurs SSO** | Désactivé sauf si des domaines e-mail sont autorisés. Voir [Qui peut se connecter](#who-can-sign-in). |
| **Accès aux bibliothèques des nouveaux comptes SSO** | **Toutes les bibliothèques**, ou **Seulement les bibliothèques choisies**. |

**Tester la connexion** vérifie que l'émetteur répond. **Enregistrer les
réglages SSO** s'applique aussitôt, sans redémarrage. La ligne **État** indique
si le fournisseur est joignable, avec la dernière erreur s'il ne l'est pas.

Les variables `EVEY_OIDC_*` de
[Comptes et connexion](environment/accounts.md#single-sign-on-oidc) servent de
repli : la carte signale quand ses valeurs en viennent, et l'enregistrer les
remplace.

!!! note "Le fournisseur n'est jamais sur le chemin critique du serveur"
    Le serveur démarre même quand le fournisseur est arrêté, le bouton SSO
    reste masqué tant que le fournisseur est injoignable, et la connexion par
    mot de passe continue de fonctionner. Le fournisseur est recontacté à la
    tentative de connexion suivante, au plus toutes les 30 secondes.

## Qui peut se connecter

1. Un compte qui s'est déjà connecté avec ce fournisseur se reconnecte, sauf
   s'il est désactivé.
2. Sinon, avec **Relier aux comptes existants par e-mail vérifié** activé, un
   compte existant ayant le même e-mail est relié au fournisseur et connecté,
   mais seulement si le fournisseur confirme l'adresse. Le compte garde son mot
   de passe. Les comptes administrateurs ne sont jamais reliés ainsi.
3. Sinon, avec **Créer un compte pour les nouveaux utilisateurs SSO** activé, un
   nouveau compte est créé, avec un profil et l'accès aux bibliothèques choisi
   dans la carte. Son nom d'utilisateur est le `preferred_username` du
   fournisseur, à défaut la partie de l'e-mail avant `@` ; un nom déjà pris
   reçoit un court suffixe.

!!! warning "Pourquoi la création de compte est désactivée par défaut"
    Avec un fournisseur public comme Google, quiconque y possède un compte
    pourrait sinon en obtenir un sur votre serveur. La création n'est activée
    par défaut que lorsque **Domaines e-mail autorisés** restreint qui y a
    droit. Désactivée, seuls les comptes existants peuvent se connecter par
    SSO.

Quand une connexion est refusée, la page de connexion en donne la raison : le
domaine de l'e-mail n'est pas autorisé, aucun compte ne correspond et la
création est désactivée, le compte est désactivé, le compte existe mais n'a pas
pu être relié, ou le fournisseur est indisponible.

## Se déconnecter

Quand le fournisseur publie un point de déconnexion, se déconnecter d'Evey après
une connexion SSO ferme aussi la session chez le fournisseur, puis revient à la
page de connexion d'Evey. La carte indique si le fournisseur le permet.

<!-- sources
settings and precedence (panel keys oidc.* win over EVEY_OIDC_*): internal/app/user/oidc_settings.go - ResolveOIDCConfig, AllowSignup default from AllowedEmailDomains
lazy discovery, 30 s backoff, never at boot: internal/app/user/oidc_service.go - oidcRetryBackoff ; cmd/evey/wire.go
account resolution (identity, verified-email link, sign-up, suffix, default libraries, admin never linked): oidc_service.go findOrCreateUser ; tests oidc_policy_test.go
admin routes GET/PUT /api/v1/admin/sso, POST /api/v1/admin/sso/test: internal/adapters/driving/http/sso_admin_handler.go
login error reasons: internal/adapters/driving/http/auth_handler.go (oidc_error=<reason>) ; i18n auth.login.ssoError.*
logout endSessionUrl: internal/adapters/driving/http/server.go
mobile flow: internal/adapters/driving/http/oidc_app_flow.go ; frontend/apps/mobile/app/oidc-callback.tsx
UI: frontend/apps/web/src/features/admin/serverConfig/SsoSettingsCard.tsx ; i18n admin.sso.* ; auth.login.signInWithSSO
-->
