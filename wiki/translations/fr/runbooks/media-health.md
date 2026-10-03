---
source_sha: 2dcb9083e6c812b05a9ddfff7acdd81f745f1367b15c876fbc137c91dc3b02c1
reviewed: true
---
# Santé des médias

**Admin → Santé des médias** lit chaque fichier MKV/WebM de vos bibliothèques et
donne à chacun une seule réponse. Le but est une page qui se vide : ce qu'Evey
sait réparer, il le répare ; le reste est signalé une fois et peut être marqué
comme vu.

<figure class="evey-shot" markdown="span">
  ![Admin → Santé des médias après une analyse : un fichier à réparer, une remarque](../images/admin-media-health.webp){ loading=lazy }
  <figcaption>Une analyse avec un fichier qu'Evey sait réparer (son index de lecture manque) et une remarque (des octets en trop après la fin du fichier, sans conséquence) (interface en anglais).</figcaption>
</figure>

## Trois verdicts

| Verdict | Ce qu'il signifie | Ce que fait Evey |
| --- | --- | --- |
| **Sain** | Index, conteneur, tailles et calage de l'audio sont corrects. | Rien. |
| **Réparable ici** | Un défaut qu'Evey répare sur ce serveur. | **Tout réparer** s'en charge en arrière-plan. |
| **Information** | Une mesure faite sur le fichier, qu'Evey ne réécrit pas de lui-même. | Rien. Marquez-la comme vue. |

Aucun ne vous confie une corvée. Une mesure sur
laquelle Evey ne peut pas agir reste une remarque, et une mesure peut se tromper :
si le fichier se lit bien, il va bien.

### Réparable ici

- **Sans index de lecture**, ou avec un index inutilisable. Le symptôme : un déplacement
  dans la lecture ou une reprise lents. L'index est reconstruit dans une copie
  temporaire, la copie est entièrement vérifiée (pistes, durée, index), puis elle
  remplace le fichier.
- **Mauvais conteneur** : le `.mkv` est en réalité un fichier MP4 ou MOV. Il est
  renommé en `.mp4` et la bibliothèque suit ; rien n'est réécrit.
- **Zone endommagée** : une région corrompue au milieu du fichier. La réparation
  la saute et resynchronise la suite, et ces octets sont perdus pour de bon. La
  fenêtre de réparation le demande explicitement (**Resynchroniser aussi N
  fichiers endommagés**) ; case décochée, les fichiers endommagés ne sont pas
  touchés. Il en va de même pour un fichier dont la corruption n'apparaît qu'à la
  réécriture (**Corruption trouvée à la réparation**) : l'analyse ne lit que les
  en-têtes et ne pouvait pas la voir.
- **Pistes rangées par blocs** : des minutes de vidéo, puis des minutes de son,
  au lieu d'être côte à côte. Aucun lecteur ne dépasse le premier bloc.
  **Ré-entrelacer les pistes** les réécrit côte à côte sans réencodage et vérifie
  que chaque paquet a été conservé avant de remplacer le fichier.

### Information

- **Téléchargement incomplet** : la fin du fichier n'est jamais arrivée. Il se
  lit jusqu'à la coupure. Aucun outil ne récupère la partie manquante ; seule la
  source le peut.
- **Recherche imprécise** : l'index reconstruit a encore un trou, le plus souvent
  le générique de fin. Un déplacement dans ce trou arrive à son bord ; partout
  ailleurs, la recherche est exacte.
- **Audio à +N s** : une piste audio commence après l'image. C'est ainsi qu'un
  doublage est calé sur un montage, et tous les lecteurs le respectent. Evey ne le
  réécrit jamais.
- **Pistes audio décalées de N s** : les pistes audio du fichier ne concordent
  pas entre elles, donc l'une d'elles est mal placée. Aucune mesure ne peut dire
  laquelle.
- **Son décalé de N s par rapport à l'image** : une piste audio s'écarte de
  l'endroit où les sous-titres, calés sur l'image, placent les dialogues.
- **Piste audio ramenée à 0** : la trace que laisse une réparation antérieure qui
  a ramené une piste de son décalage déclaré. Si le doublage passe maintenant en
  avance, le décalage retiré n'est plus dans le fichier.
- **Audio -N s** : une piste audio se termine avant l'image. La lecture comble le
  manque par du silence.
- **Octets excédentaires** : des octets en trop après la fin du contenu du
  fichier. Il se lit sans problème.
- **Illisible** : l'analyse n'a pas pu lire le fichier (absent, illisible, ou un
  conteneur refusé par l'analyseur). Vérifiez le montage ou la source.

## Vérifier la synchro à l'œil

Pour les trois remarques sur l'audio, **Vérifier la synchro**, sur la ligne du
fichier, fabrique un court extrait sur le serveur : choisissez un moment, une
piste et un décalage à essayer, puis regardez-le, au ralenti si besoin. Comparer
des pistes dit si elles concordent, jamais laquelle suit l'image ; seul le
visionnage tranche.

**Appliquer N s à cette piste** déplace cette piste du décalage choisi. La modification est
enregistrée comme votre jugement, pas comme une mesure, et **Historique** peut
l'annuler.

## Marquer comme vu

**Vu, ne plus signaler** (un fichier) et **Tout marquer comme vu** (un groupe)
retirent une remarque Information de la vue **En attente**. Seules les remarques
peuvent être marquées : un défaut réparable se répare. La marque vaut pour le
fichier tel qu'il est : si sa taille ou sa date change, l'analyse suivante le
diagnostique à nouveau. **Re-signaler** annule la marque.

Les vues au-dessus du tableau (**En attente**, **Réparable**, **Info**,
**Vus**, **Sauvegardes**, **Tous**) et la zone de recherche servent à traiter une
grande bibliothèque : **En attente** est la liste des choses à faire.

## Analyses

Une analyse lit le début de chaque fichier (quelques kilo-octets) et reprend la
réponse précédente pour un fichier qui n'a pas changé. **Usage des disques** règle
le rythme :

- **Discret** : un fichier à la fois, puis une pause aussi longue que la lecture.
  Les disques restent libres environ la moitié du temps, et quelqu'un peut
  continuer à regarder.
- **Rapide** : plusieurs fichiers à la fois, sans pause. Environ trois fois plus
  rapide sur une bibliothèque répartie sur plusieurs disques, mais la lecture
  peut saccader pendant ce temps.

Deux choses font tout rediagnostiquer à une analyse : un fichier qui a changé, et
une nouvelle version d'Evey dont les contrôles ont changé. La première analyse
après une telle mise à jour est donc longue sur une grande bibliothèque ; les
suivantes redeviennent rapides.

## Sauvegardes, historique et annulation

**Conserver les sauvegardes** est activé par défaut : chaque fichier réécrit garde
son original en `.mkvgobak` à côté de lui. **Restaurer** remet un fichier, **Tout
restaurer** la sélection en cours, et **Supprimer les sauvegardes** les efface
pour de bon. Désactivé, seules les dernières copies existent pendant une passe, et
elles sont supprimées à la fin ; chaque réparation est alors vérifiée plus à fond
avant de remplacer le fichier. Quand garder une copie de chaque fichier
remplirait le volume, la page le dit avant la passe.

Le **Journal des réparations** liste chaque modification qu'une réparation a
apportée aux octets d'un fichier, sur toute la bibliothèque ; **Historique** le
montre pour un seul fichier. Certaines modifications s'y annulent sans aucune
copie :

| Modification | Annulation |
| --- | --- |
| Piste audio déplacée | Applique le décalage exactement inverse. |
| Renommé en `.mp4` | Lui rend son nom d'origine. |
| Index reconstruit | Rien à annuler : l'image et le son sont intacts. |
| Zone corrompue sautée | Seule la sauvegarde rend les octets perdus, c'est pourquoi cette réparation en garde toujours une. |

<figure class="evey-shot" markdown="span">
  ![Admin → Santé des médias après Tout réparer, journal des réparations ouvert](../images/admin-media-health-repaired.webp){ loading=lazy }
  <figcaption>Après <strong>Tout réparer</strong> : l'index est reconstruit, le journal le consigne, et le fichier sort de la file ; la remarque reste. Cet index a été inscrit dans le fichier sur place, ce qui ne garde aucune copie : seul un fichier qui doit être réécrit garde une sauvegarde (interface en anglais).</figcaption>
</figure>

Les réparations écrivent dans vos dossiers de médias. Ils doivent être montés en
lecture-écriture dans le conteneur (pas de `:ro` sur le volume) ; après une passe
qui a rencontré un volume en lecture seule, la page le signale. L'analyse
fonctionne dans les deux cas.

## En ligne de commande

Le même moteur est disponible dans la CLI d'exploitation (voir `cli.md`) :

- `evey admin reindex-no-cue --root <dossier> [--apply] [--resync]` reconstruit
  les index manquants, à blanc sauf avec `--apply` ;
- `evey admin validate-streaming --root <dossier>` affiche le rapport de
  préparation au streaming, en lecture seule.
