---
source_sha: 4ee97dc5ef2cdb11b0aa01a535284eaf511ab308d1429dbb9882b9a5838ba8ca
hide:
- navigation
---

<pre class="evey-wordmark" aria-hidden="true"><span>███████╗</span><span class="v">██╗   ██╗</span><span>███████╗██╗   ██╗</span>
<span>██╔════╝</span><span class="v">██║   ██║</span><span>██╔════╝╚██╗ ██╔╝</span>
<span>█████╗  </span><span class="v">██║   ██║</span><span>█████╗   ╚████╔╝ </span>
<span>██╔══╝  </span><span class="v">╚██╗ ██╔╝</span><span>██╔══╝    ╚██╔╝  </span>
<span>███████╗</span><span class="v"> ╚████╔╝ </span><span>███████╗   ██║   </span>
<span>╚══════╝</span><span class="v">  ╚═══╝  </span><span>╚══════╝   ╚═╝   </span></pre>

# Evey

**Vos films, vos séries, votre musique et vos photos, diffusés depuis votre
propre machine vers tous les écrans de la maison - et au-delà.**

Evey est un serveur multimédia auto-hébergé. Vous lui indiquez les dossiers que
vous avez déjà, il reconnaît ce qu'ils contiennent, les habille d'affiches, de
résumés et de distributions, et les lit sur le web, sur les téléphones Android,
sur Android TV et sur un Chromecast. Rien ne quitte votre domicile si vous ne
le publiez pas, et personne d'autre ne décide de ce que vous pouvez regarder.

[Découvrir Evey](philosophy.md){ .md-button .md-button--primary }
[L'installer](installation.md){ .md-button }
[Essayer la démo](https://demo.eveyproject.org){ .md-button }

Envie de cliquer avant d'installer ? La démo publique sur
[demo.eveyproject.org](https://demo.eveyproject.org) fait tourner le vrai serveur avec des films libres :
connectez-vous avec `demo` / `demo`. Elle est remise à zéro régulièrement, et les
écrans d'administration sont visibles mais surtout en lecture seule.

## Deux portes d'entrée

<div class="grid cards" markdown>

-   __Découvrir__

    ---

    Ce qu'Evey cherche à être, ce qu'il apporte à ceux qui regardent, et les
    appareils sur lesquels il fonctionne. Aucun terminal nécessaire.

    [Pourquoi Evey](philosophy.md) · [Ce qu'Evey vous offre](discover/features.md) ·
    [Sur tous les écrans](discover/apps.md)

-   __Guide technique__

    ---

    Installer le serveur, les dix premières minutes après son démarrage, chaque
    réglage qu'un spectateur ou un administrateur peut modifier, et que faire le
    jour où quelque chose tourne mal.

    [Installation](installation.md) · [Premiers pas](guide/first-steps.md) ·
    [Administration](guide/admin-settings.md)

</div>

## Pourquoi on l'adopte

- **Il lit le fichier que vous avez.** Quand un appareil sait lire un fichier
  tel quel, Evey l'envoie sans y toucher : qualité d'origine, aucun travail
  pour le serveur. Quand il ne sait pas, Evey ne convertit que ce qui manque,
  avec la carte graphique s'il y en a une.
- **Il s'installe en un fichier.** Un conteneur, un fichier compose, aucun mot
  de passe à inventer. La base de données et le moteur de recherche vivent à
  l'intérieur et se configurent seuls.
- **Il est pensé pour un foyer.** Des profils pour chacun, des profils enfants
  protégés par un code PIN, des limites d'âge appliquées par le serveur plutôt
  que par l'application, et un salon pour regarder ensemble depuis des
  domiciles différents.
- **Il peut être ouvert sur Internet sans crainte.** L'accès distant s'ouvre
  quand vous publiez le serveur et reste fermé sinon, la connexion échoue en
  refusant plutôt qu'en laissant passer, et les extensions tournent dans un bac
  à sable avec les seules permissions qu'elles ont déclarées.
- **C'est un logiciel libre.** Sous licence
  [AGPL-3.0-or-later](https://www.gnu.org/licenses/agpl-3.0.html). Le code est
  sur [GitHub](https://github.com/Evey-project/evey), tout comme la liste de ce
  dont le serveur ne sait pas encore vous protéger.

## Accès rapides

| Vous voulez | Lisez |
| --- | --- |
| L'essayer sur votre machine | [Installation](installation.md) |
| Le mettre sur Internet en HTTPS | [Reverse proxy](reverse-proxy.md) |
| Utiliser votre carte graphique pour la conversion | [Transcodage et matériel](transcoding.md) |
| Savoir ce que fait chaque réglage | [Réglages utilisateur](guide/viewer-settings.md) · [Administration](guide/admin-settings.md) |
| Relancer un serveur après un plantage | [Restauration](runbooks/recovery.md) |
| Écrire une extension | [Extensions](plugins.md) |

Les signalements de sécurité s'adressent à `security@eveyproject.org` - voir le
[bac à sable des extensions et modèle de menace](SECURITY.md).
