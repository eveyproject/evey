---
source_sha: 65686e994e8b0eed0c73e145729f00f455caa301b21644be8c6b23c67d7964d3
---
# Pourquoi Evey

Les films, vous les avez déjà. Vous avez numérisé les disques, gardé les
fichiers, rangé les dossiers. Ce qui manque, c'est ce qui les rend agréables à
regarder : un mur d'affiches, l'épisode où vous vous étiez arrêté, la bonne
piste audio, la même bibliothèque sur la télé, le téléphone et l'ordinateur,
et un moyen de la partager avec les gens qui vivent avec vous sans la confier à
une entreprise.

Evey, c'est cette partie-là. Il tourne sur votre machine - un NAS, un mini-PC,
un vieux portable - et sert vos propres fichiers à vos propres écrans. Cinq
engagements guident chacune de ses décisions, et quand deux d'entre eux
entrent en conflit, voici l'ordre dans lequel ils l'emportent.

## 1. Pouvoir être ouvert sur Internet sans crainte

Un serveur multimédia n'est utile que si on peut l'atteindre depuis le canapé,
depuis le train et depuis chez ses parents. Evey est donc fait pour être
joignable, pas caché, et ses réglages par défaut partent du principe que le
réseau est hostile :

- l'accès distant s'ouvre quand vous publiez le serveur à une adresse publique,
  et reste fermé sinon - vous pouvez toujours en décider explicitement ;
- quand le serveur ne peut pas vérifier qui vous êtes (une base de données qui
  ne répond pas, un réglage parental illisible), il refuse plutôt que de
  laisser passer la requête ;
- les extensions n'obtiennent que les permissions déclarées par leur auteur, et
  le noyau les confine ;
- le serveur ne tourne jamais en administrateur du système.

## 2. S'installer en quelques lignes

Un fichier compose, un conteneur, aucun mot de passe à inventer. La base de
données et le moteur de recherche vivent dans le conteneur et génèrent leurs
propres secrets au premier démarrage. Tout ce que vous voudrez changer
ensuite - langues, limites de conversion, carte graphique, planifications - se
trouve dans le panneau d'administration, pas dans un fichier de configuration
à modifier avant de redémarrer.

## 3. Extensible dans le langage que vous écrivez

Evey s'étend avec des extensions : sources de métadonnées, widgets de la page
d'accueil, thèmes. Une extension est un programme que le serveur lance et avec
lequel il dialogue par une connexion locale ; elle peut donc être écrite dans
le langage que vous connaissez déjà - des kits prêts à l'emploi existent pour
sept d'entre eux. Comme les extensions tournent dans un bac à sable au lieu
d'être crues sur parole, installer celle d'un autre est une idée raisonnable.

## 4. Rapide, et honnête sur la question

La façon la plus rapide de lire un fichier, c'est de l'envoyer tel quel. Evey
le fait dès que l'appareil sait le lire, ne convertit que la partie que
l'appareil ne gère pas (souvent le son seul), et décide une fois par lecture au
lieu de se raviser en plein film. Les chiffres derrière ces choix sont mesurés
sur de vrais fichiers et de vraies machines, pas supposés - une optimisation
qui semblait astucieuse n'a été gardée qu'après avoir prouvé qu'elle donnait la
même réponse que l'outil qu'elle remplaçait.

## 5. Pour beaucoup de spectateurs à la fois, sur du matériel modeste

La cible, c'est un foyer et ses amis sur un NAS, pas un centre de données.
Evey mesure donc ce que votre machine sait faire à son premier démarrage,
n'admet que le nombre de conversions simultanées qu'elle peut tenir, met en
pause son travail de fond quand quelqu'un regarde, et se dimensionne selon le
matériel qu'il trouve. Une machine lente lit moins de flux ; elle ne les lit
pas tous mal.

---

Deux choses qu'Evey n'est délibérément pas. Ce n'est pas un catalogue de tous
les types de médias traités à peu près - la lecture est son métier. Et ce n'est
pas un produit qui vous demande de lui faire confiance : ce que fait le serveur,
et ce qu'il ne sait pas encore empêcher, est écrit dans le
[modèle de sécurité](SECURITY.md) plutôt que sous-entendu.

**Ensuite :** [ce qu'Evey vous offre](discover/features.md) ·
[les appareils sur lesquels il fonctionne](discover/apps.md) ·
[l'installer](installation.md)
