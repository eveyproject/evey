---
source_sha: f6d6e61c9914bed7f2744ebf8dec6a79fc1769dd750e8b0f2109b4ab14d44526
reviewed: true
---
# Supervision et logs

Où voir ce que fait le serveur, du plus immédiat au plus agrégé. Pour un
symptôme précis, commencez par le [Dépannage](troubleshooting.md).

## Ce qui est lu en ce moment

**Admin → En cours de lecture** liste tous les flux en cours : qui, quoi, sur
quel appareil, et comment, piste par piste (copiée, transcodée, et le backend
qui l'encode). Un administrateur peut **Arrêter** une session ou **Envoyer un
message** au spectateur.

**Admin → Santé du système** montre le processeur, la mémoire, la bande
passante, le stockage, et la part de la capacité de transcodage utilisée.

<figure class="evey-shot" markdown="span">
  ![Admin → Santé du système sur le serveur de démo](../images/admin-system-health.webp){ loading=lazy }
  <figcaption><strong>Admin → Santé du système</strong>, avec le travail de fond en cours juste en dessous.</figcaption>
</figure>

## Logs

**Admin → Logs & Debug** permet de chercher dans les logs du serveur et de les
suivre en direct, filtrés par domaine (lecture, transcodage, scan,
sous-titres...). C'est le premier endroit où regarder quand un utilisateur
signale un problème. Les mêmes logs sont disponibles avec
`docker compose logs evey`.

## Comment les lectures se sont vraiment passées

**Admin → Qualité streaming** s'appuie sur ce que les lecteurs remontent
pendant la lecture (niveau de mémoire tampon, recharges, débit, au format
CMCD) : à quelle fréquence les spectateurs ont attendu, quels titres coupent le
plus, et quels types d'appareils peinent, sur le dernier jour ou la dernière
semaine.

Le serveur s'en sert aussi. Quand une catégorie de sources fait régulièrement
attendre les spectateurs, il abaisse pour un temps la qualité maximale de cette
catégorie, en plus du plafond mesuré, puis lève la limite de lui-même. Les
limites en cours figurent sous **Limites de qualité automatiques**, avec leur
raison et leur échéance ; **Réinitialiser** en supprime une immédiatement.

## Historique et statistiques

- **Admin → Analytique** : lectures du jour et de la semaine, utilisateurs
  actifs, titres les plus regardés des 30 derniers jours.
- **Admin → Activité** : l'historique des lectures et des actions
  d'administration, et les tâches de fond en cours.

La durée de conservation de chaque historique se règle dans **Admin →
Configuration du serveur → Paramètres du serveur** (sessions de lecture,
journal d'audit, métriques du serveur, qualité de streaming). Un changement
s'applique au nettoyage quotidien suivant.

## Ligne de commande

`evey admin doctor` vérifie tout le serveur en un seul rapport ; les autres
commandes de diagnostic sont listées dans la
[Ligne de commande](../runbooks/cli.md).
