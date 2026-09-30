---
source_sha: bba2d9d0a70621281091516a01462cd4178130e57b6654f467dfe62130069342
reviewed: true
---
# Authentification unique (OIDC)

Evey prend en charge l'authentification unique (SSO) via OpenID Connect. Les
utilisateurs peuvent se connecter avec Google, Keycloak, Authentik, Authelia ou
tout fournisseur compatible OIDC.

## Configuration

```env title=".env"
EVEY_OIDC_ENABLED=true
EVEY_OIDC_ISSUER=https://auth.example.com/realms/main
EVEY_OIDC_CLIENT_ID=evey
EVEY_OIDC_CLIENT_SECRET=your-client-secret
EVEY_OIDC_ALLOWED_EMAIL_DOMAINS=example.com,company.org
```

## Configurer le fournisseur

### 1. Déclarer Evey comme client OIDC

Dans votre fournisseur d'identité, créez un nouveau client (ou application) :

| Champ | Valeur |
|-------|-------|
| Client ID | `evey` |
| Client Secret | à générer |
| URI de redirection | `https://<votre adresse>/api/v1/auth/oidc/callback` |
| Type d'autorisation | Authorization Code |
| Scopes | `openid email profile` |

### 2. Configurer Evey

Renseignez les variables d'environnement ci-dessus dans votre `.env`.
`EVEY_OIDC_ISSUER` est l'URL de base du point de découverte de votre
fournisseur, sans `/.well-known/openid-configuration`.

### 3. Redémarrer

```bash
docker compose restart evey
```

La page de connexion affiche alors un bouton **Se connecter avec SSO**.

## Comment ça fonctionne

1. L'utilisateur clique sur **Se connecter avec SSO** sur la page de connexion.
2. Evey le redirige vers le fournisseur OIDC.
3. L'utilisateur s'authentifie auprès du fournisseur.
4. Le fournisseur le renvoie vers Evey avec un code d'autorisation.
5. Evey échange ce code contre des jetons, puis crée ou met à jour le compte.

## Restreindre les domaines

`EVEY_OIDC_ALLOWED_EMAIL_DOMAINS` limite les domaines d'adresse e-mail autorisés
à s'inscrire. Laissez-la vide pour accepter tous les domaines.

## Création automatique des comptes

À sa première connexion par OIDC, un utilisateur est créé automatiquement avec :

- un nom d'utilisateur tiré de `preferred_username`, ou à défaut du début de son
  adresse e-mail ;
- l'adresse e-mail fournie par le jeton OIDC ;
- un rôle non administrateur ;
- un profil par défaut.

Un compte OIDC ne peut pas définir de mot de passe local : il s'authentifie
toujours via le fournisseur.
