---
source_sha: f46331e2b177b0336d9d4169e0aba094781a65d2b53277dba0bfb59f01e14698
---

# ADR-0012 : autorisation refusée par défaut pour les jetons d'extension

- Statut : acceptée
- Date : 2026-05-23

## Contexte

L'ADR-0003 filtrait les portées d'un jeton d'extension au moment de sa frappe,
puis faisait confiance au jeton. Ce modèle repose entièrement sur l'exhaustivité
du filtre : une portée oubliée dans la liste des interdictions, ou un chemin de
frappe qui contournerait le filtre, et le jeton atteint ce qu'il ne devrait pas.

## Décision

Ajouter un chemin d'autorisation dédié, **refusé par défaut**, pour les requêtes
portant un jeton d'extension. Un tel jeton n'atteint une méthode que s'il
survit aux trois épreuves suivantes.

**Plancher d'administration.** Si la méthode est classée comme réservée à
l'administration, elle est refusée d'emblée, quelles que soient les portées
portées par le jeton. Aucun jeton d'extension, avec aucun jeu de portées,
n'atteint jamais un RPC d'administration, système ou destructeur. Ce plancher
n'est ni contournable ni octroyable.

**Liste blanche de méthodes.** La méthode doit figurer dans une table qui la
relie à l'unique portée qu'elle exige. Absente de la liste, elle est refusée. Le
défaut d'une méthode non listée est donc le refus, à l'inverse du défaut de la
couche RBAC : oublier d'y inscrire une méthode échoue du bon côté.

**Vérification de portée.** Le jeton doit réellement porter la portée que la
liste exige.

**Les capacités octroyables** sont étroites et orientées lecture ou données
propres. `public` n'ouvre que la lecture d'éléments dans une projection sûre,
sans chemin de fichier, en réutilisant les gardes d'accès et de contrôle
parental existants. `prefs` n'ouvre que les préférences de l'utilisateur
courant, dans l'espace de l'extension appelante ; l'identité de l'extension et
celle de l'utilisateur viennent toutes deux du jeton, jamais de la requête.

**Ce qui est verrouillé en dur** ne s'octroie jamais : toute portée sous les
préfixes `admin.`, `auth.` et `system.`, plus les portées exactes `user.write`
et `library.write`. Demandée dans un manifeste, l'installation est refusée ;
présente dans un jeton, elle est rejetée à la frappe.

**Le jeton n'entre jamais dans le bac à sable.** L'iframe ne détient aucun
jeton : le parent le frappe et relaie les appels pour le compte de l'extension,
qui lui envoie des messages corrélés. La politique de sécurité du contenu de
l'iframe ne l'autorise à joindre que les hôtes déclarés dans son manifeste : elle
ne peut pas atteindre l'API d'Evey même si elle reconstruisait une requête.

## Conséquences

- **Un jeton d'extension ne peut pas monter en privilèges.** Le filtrage à la
  frappe est conservé en défense supplémentaire, mais l'application n'en dépend
  plus : l'intercepteur refuse par défaut et bloque les méthodes
  d'administration indépendamment des portées.
- **Exposer une nouvelle méthode aux extensions tient en une ligne explicite.**
  L'oublier refuse l'accès, ce qui est le bon sens de l'échec : la polarité est
  volontairement inversée par rapport à la couche RBAC, où oublier une entrée
  d'administration l'exposerait.
- **Le modèle est vérifié par des extensions d'attaque**, pas seulement par la
  revue : elles sondent le blocage de la politique de contenu, le refus d'accès
  direct à l'API, l'absence de fuite du chemin de fichier, l'isolation entre
  extensions et entre utilisateurs, et le refus des méthodes d'administration.
- C'est une frontière de confiance vérifiée pour du code communautaire sur une
  machine personnelle, pas une garantie multi-locataire hostile.

## Références

- `internal/adapters/driving/grpcapi/interceptors/auth.go` - l'application du
  refus par défaut
- `internal/adapters/driving/grpcapi/interceptors/rbac.go` - la liste blanche
  des méthodes et le plancher d'administration
- `internal/adapters/driving/grpcapi/plugin_prefs_handler.go` - les préférences,
  extension et utilisateur tirés du jeton
- `internal/adapters/driving/grpcapi/library_handler.go` - la projection
  publique, sans chemin de fichier
