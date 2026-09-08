---
source_sha: b5ebe9aa528d6b195b210870c763195df806015fe80273fefda50e5e3927aad7
---

# Mise en place d'OIDC / SSO

Evey prend en charge l'authentification unique par OpenID Connect. Vos utilisateurs se connectent avec Google, Keycloak, Authentik, Authelia, ou n'importe quel fournisseur conforme à OIDC.

## Configuration

```env
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Mise en place côté fournisseur

### 1. Déclarer Evey comme client OIDC

Dans votre fournisseur d'identité, créez un client (ou une application) :

| Champ | Valeur |
|-------|-------|
| Client ID | `evey` |
| Client Secret | à générer |
| Redirect URI | `{EVEY_SERVER_BASE_URL}/api/v1/auth/oidc/callback` |
| Grant Type | Authorization Code |
| Scopes | `openid email profile` |

### 2. Configurer Evey

Renseignez dans votre `.env` les variables ci-dessus. `EVEY_OIDC_ISSUER` doit valoir l'URL de base du point de découverte de votre fournisseur OIDC, sans le suffixe `/.well-known/openid-configuration`.

### 3. Redémarrer

```bash
docker compose restart evey
```

La page de connexion affiche alors un bouton « Se connecter avec SSO ».

## Comment cela fonctionne

1. L'utilisateur clique sur « Se connecter avec SSO » depuis la page de connexion
2. Evey le redirige vers le fournisseur OIDC
3. L'utilisateur s'authentifie auprès du fournisseur
4. Le fournisseur le renvoie vers Evey, muni d'un code d'autorisation
5. Evey échange ce code contre des jetons, puis crée ou met à jour le compte

## Restriction par domaine

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` limite les domaines de courriel autorisés à s'inscrire. Laissez la variable vide pour les accepter tous.

## Création automatique des comptes

Un utilisateur qui se connecte par OIDC pour la première fois voit son compte créé automatiquement, avec :
- un nom d'utilisateur dérivé de `preferred_username`, ou à défaut du préfixe de l'adresse de courriel
- l'adresse de courriel issue du jeton OIDC
- un rôle non administrateur
- un profil par défaut

Un utilisateur OIDC ne peut pas se donner de mot de passe local : il s'authentifie toujours par l'intermédiaire du fournisseur.
