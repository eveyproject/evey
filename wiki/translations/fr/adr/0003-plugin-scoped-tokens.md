---
source_sha: d0ba9089365254cbfd08ddda42f3899ae2356726234271fb5793f3afc982f960
---

# ADR-0003 : jetons JWT limités à une extension

- Statut : acceptée
- Amendée par : [ADR-0012](0012-plugin-token-deny-by-default.md) (application)
- Date : 2026-02-20

## Contexte

Les extensions tournent dans des iframes isolées et appellent l'API d'Evey au nom de l'utilisateur. Leur remettre le jeton de session complet leur donnerait tous les privilèges du compte, y compris l'administration et l'écriture sur les bibliothèques, les utilisateurs et les réglages système. C'est inacceptable pour des extensions tierces ou communautaires.

## Décision

Introduire des JWT de courte durée limités à une extension :

- Algorithme : **RS256** (asymétrique, la clé publique est exposée pour la vérification).
- Durée de vie : **5 minutes**. Les extensions les renouvellent par l'hôte.
- Revendications : `{ sub, plugin_id, scopes, exp, iat, jti }`.
- Délivrés par `app/plugin.TokenService.Mint(ctx, userID, pluginID, requestedScopes)`, après intersection des portées demandées avec celles déclarées dans le manifeste de l'extension et avec les permissions de l'utilisateur.
- Vérifiés par `pkg/auth.PluginJWTVerifier` dans `HTTPAuthMiddleware`, quand la requête vient de l'hôte d'extensions.

### Liste blanche de portées

Seules des capacités orientées lecture et étroitement délimitées sont autorisées : `library:read`, `media:read`, `metadata:read`, `playback:read`, `playback:control:self`, `favorite:read`, `playlist:read`, `notification:read`.

### Portées interdites (rejetées d'emblée à la frappe du jeton)

`admin:*`, `auth:*`, `user:write`, `library:write`, `system:*`.

Tout jeton portant une portée interdite est rejeté aussi bien à la frappe qu'à la vérification.

## Conséquences

- Une extension compromise ne peut pas monter en privilèges vers l'administration, ni modifier les bibliothèques, ni toucher à l'authentification.
- La courte durée de vie limite les fenêtres de rejeu ; `jti` permet une révocation si besoin.
- Les manifestes d'extension doivent déclarer leurs portées d'avance ; l'interface les présente au moment de l'installation.
- Ajouter une portée exige une mise à jour explicite de la liste blanche, et une revue.

## Références

- `internal/pkg/auth/plugin_jwt.go`
- `internal/app/plugin/token_service.go`
- [ADR-0001](0001-hexagonal-architecture.md)

## Amendement (2026-05-23)

L'implémentation s'est écartée de ce texte de février 2026 ; la décision (jeton court, portées filtrées, RS256, durée de vie de 5 minutes) tient toujours, mais les points suivants sont corrigés :

- **La notation des portées se fait avec des points**, non avec des deux-points : `media.read`, `library.write`, `notification.sender`, `api.library`, etc. L'ensemble interdit correspond aux préfixes `admin.`, `auth.` et `system.`, plus les portées exactes `user.write` et `library.write` (`pkg/auth.forbiddenPrefix` et l'ensemble d'exclusions exactes de `plugin_jwt.go`).
- La liste d'autorisation réelle est `pkg/auth.AllowedPluginScopes` (`plugin_jwt.go`), plus large que la liste en lecture seule d'origine de cette décision : elle inclut par exemple `media.write`, `api.*`, `notification.sender`, ainsi que les capacités `public` et `prefs` apparues plus tard.
- Noms des méthodes : la frappe se fait par `TokenService.IssueToken` (et non `Mint`) ; la vérification par `JWTGenerator.ParsePluginToken` (il n'existe pas de `PluginJWTVerifier`).
- L'application n'est plus « on filtre à la frappe puis on fait confiance » : c'est un **refus par défaut au niveau de l'intercepteur**, voir [ADR-0012](0012-plugin-token-deny-by-default.md).
